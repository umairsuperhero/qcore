//go:build linux

package sctp

import (
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	SOL_SCTP    = 132
	SCTP_EVENTS = 11

	// SCTP_SNDINFO / SCTP_RCVINFO / SCTP_SNDRCVINFO typically come in control data
	SCTP_SNDRCV = 8 // cmsg_type for sctp_sndrcvinfo
)

// sctpSndRcvInfo is the linux C struct sctp_sndrcvinfo (roughly 32 bytes on 64-bit)
type sctpSndRcvInfo struct {
	Stream uint16
	Ssn    uint16
	Flags  uint16
	Ppid   uint32
	Context uint32
	Timetolive uint32
	Tsn uint32
	Cumtsn uint32
	AssocID int32
}

type sctpAssociation struct {
	fd       int
	peerAddr syscall.Sockaddr
	peerStr  string
	in       chan sctpMessage
	closed   bool
	mu       sync.Mutex
	parent   *sctpListener
}

type sctpMessage struct {
	data     []byte
	streamID uint16
	err      error
}

func (a *sctpAssociation) Read() ([]byte, uint16, error) {
	msg, ok := <-a.in
	if !ok {
		return nil, 0, fmt.Errorf("sctp association closed")
	}
	return msg.data, msg.streamID, msg.err
}

func (a *sctpAssociation) Write(data []byte, streamID uint16) error {
	// Build sctp_sndrcvinfo for control msg
	info := sctpSndRcvInfo{
		Stream: streamID,
		Ppid:   1006632960, // htonl(60) for NGAP
	}

	// cmsg header: Length (size_t -> 8 bytes on 64-bit), Level (int32), Type (int32)
	// We use unix.CmsgSpace to format it
	cmsgs := unix.CmsgSpace(32)
	control := make([]byte, cmsgs)

	// Cmsg data building
	h := (*unix.Cmsghdr)(unsafe.Pointer(&control[0]))
	h.Level = SOL_SCTP
	h.Type = SCTP_SNDRCV
	h.SetLen(unix.CmsgLen(32))
	
	infoBytes := (*[32]byte)(unsafe.Pointer(&info))[:]
	copy(control[unix.CmsgLen(0):], infoBytes)

	return syscall.Sendmsg(a.fd, data, control, a.peerAddr, 0)
}

func (a *sctpAssociation) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return nil
	}
	a.closed = true
	close(a.in)
	if a.parent != nil {
		a.parent.mu.Lock()
		delete(a.parent.assocs, a.peerStr)
		a.parent.mu.Unlock()
	}
	return nil
}

func (a *sctpAssociation) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0} // Mocked for interface compat
}

func (a *sctpAssociation) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0} // Mocked
}

type sctpListener struct {
	fd     int
	addr   string
	assocs map[string]*sctpAssociation
	accept chan Association
	mu     sync.Mutex
	closed bool
}

func parseAddr(addr string) (syscall.Sockaddr, error) {
	parts := strings.Split(addr, ":")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid address format")
	}
	ip := net.ParseIP(parts[0])
	port, _ := strconv.Atoi(parts[1])
	
	sa := &syscall.SockaddrInet4{Port: port}
	if ip != nil {
		ip = ip.To4()
		if ip != nil {
			copy(sa.Addr[:], ip)
		}
	}
	return sa, nil
}

func sockaddrStr(sa syscall.Sockaddr) string {
	if s4, ok := sa.(*syscall.SockaddrInet4); ok {
		return fmt.Sprintf("%d.%d.%d.%d:%d", s4.Addr[0], s4.Addr[1], s4.Addr[2], s4.Addr[3], s4.Port)
	}
	return "unknown"
}

func listenSCTP(addr string) (Listener, error) {
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_SEQPACKET, syscall.IPPROTO_SCTP)
	if err != nil {
		return nil, fmt.Errorf("sctp socket: %w", err)
	}

	sa, err := parseAddr(addr)
	if err != nil {
		syscall.Close(fd)
		return nil, err
	}

	if err := syscall.Bind(fd, sa); err != nil {
		syscall.Close(fd)
		return nil, fmt.Errorf("sctp bind: %w", err)
	}

	// Enable SCTP_EVENTS to get sctp_sndrcvinfo in recvmsg
	// struct sctp_event_subscribe { 10 uint8 fields }
	events := []byte{1, 1, 1, 1, 1, 1, 1, 1, 1, 1}
	if err := syscall.SetsockoptString(fd, SOL_SCTP, SCTP_EVENTS, string(events)); err != nil {
		syscall.Close(fd)
		return nil, fmt.Errorf("sctp setsockopt SCTP_EVENTS: %w", err)
	}

	if err := syscall.Listen(fd, 5); err != nil {
		syscall.Close(fd)
		return nil, fmt.Errorf("sctp listen: %w", err)
	}

	l := &sctpListener{
		fd:     fd,
		addr:   addr,
		assocs: make(map[string]*sctpAssociation),
		accept: make(chan Association, 10),
	}

	go l.recvLoop()
	return l, nil
}

func (l *sctpListener) recvLoop() {
	buf := make([]byte, 65535)
	oob := make([]byte, 1024)

	for {
		n, oobn, _, from, err := syscall.Recvmsg(l.fd, buf, oob, 0)
		if err != nil {
			if l.closed {
				return
			}
			continue
		}

		if n >= 2 {
			msgType := binary.LittleEndian.Uint16(buf[:2])
			if msgType >= 0x8000 {
				continue // skip SCTP notification
			}
		}

		peerStr := sockaddrStr(from)
		l.mu.Lock()
		assoc, ok := l.assocs[peerStr]
		if !ok {
			assoc = &sctpAssociation{
				fd:       l.fd,
				peerAddr: from,
				peerStr:  peerStr,
				in:       make(chan sctpMessage, 100),
				parent:   l,
			}
			l.assocs[peerStr] = assoc
			l.accept <- assoc
		}
		l.mu.Unlock()

		// Parse OOB to get stream ID
		var streamID uint16
		if oobn > 0 {
			cmsgs, _ := syscall.ParseSocketControlMessage(oob[:oobn])
			for _, m := range cmsgs {
				if m.Header.Level == SOL_SCTP && m.Header.Type == SCTP_SNDRCV {
					if len(m.Data) >= 2 {
						streamID = binary.LittleEndian.Uint16(m.Data[0:2])
					}
				}
			}
		}

		dataCopy := make([]byte, n)
		copy(dataCopy, buf[:n])
		assoc.in <- sctpMessage{data: dataCopy, streamID: streamID}
	}
}

func (l *sctpListener) Accept() (Association, error) {
	assoc, ok := <-l.accept
	if !ok {
		return nil, fmt.Errorf("listener closed")
	}
	return assoc, nil
}

func (l *sctpListener) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}
	l.closed = true
	close(l.accept)
	return syscall.Close(l.fd)
}

func (l *sctpListener) Addr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("0.0.0.0"), Port: 0}
}

func dialSCTP(addr string) (Association, error) {
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_SEQPACKET, syscall.IPPROTO_SCTP)
	if err != nil {
		return nil, fmt.Errorf("sctp socket: %w", err)
	}

	sa, err := parseAddr(addr)
	if err != nil {
		syscall.Close(fd)
		return nil, err
	}

	events := []byte{1, 1, 1, 1, 1, 1, 1, 1, 1, 1}
	_ = syscall.SetsockoptString(fd, SOL_SCTP, SCTP_EVENTS, string(events))

	// For SOCK_SEQPACKET, we don't strictly connect, but we can call connect() to set default destination
	if err := syscall.Connect(fd, sa); err != nil {
		syscall.Close(fd)
		return nil, fmt.Errorf("sctp connect: %w", err)
	}

	a := &sctpAssociation{
		fd:       fd,
		peerAddr: sa,
		peerStr:  addr,
		in:       make(chan sctpMessage, 100),
	}

	go func() {
		buf := make([]byte, 65535)
		oob := make([]byte, 1024)
		for {
			n, oobn, _, _, err := syscall.Recvmsg(a.fd, buf, oob, 0)
			if err != nil {
				if a.closed {
					return
				}
				continue
			}

			if n >= 2 {
				msgType := binary.LittleEndian.Uint16(buf[:2])
				if msgType >= 0x8000 {
					continue // skip SCTP notification
				}
			}

			var streamID uint16
			if oobn > 0 {
				cmsgs, _ := syscall.ParseSocketControlMessage(oob[:oobn])
				for _, m := range cmsgs {
					if m.Header.Level == SOL_SCTP && m.Header.Type == SCTP_SNDRCV {
						if len(m.Data) >= 2 {
							streamID = binary.LittleEndian.Uint16(m.Data[0:2])
						}
					}
				}
			}

			dataCopy := make([]byte, n)
			copy(dataCopy, buf[:n])
			a.in <- sctpMessage{data: dataCopy, streamID: streamID}
		}
	}()

	return a, nil
}
