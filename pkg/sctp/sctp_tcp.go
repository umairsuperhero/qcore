package sctp

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
)

// TCP fallback transport for development.
// Messages are length-prefixed: [4-byte big-endian length][2-byte stream ID][payload].
// This is NOT real SCTP — it's a convenience so you can develop and test the MME
// on Windows/macOS without kernel SCTP support.

const (
	headerSize    = 6 // 4 bytes length + 2 bytes stream ID
	maxMessageLen = 65535
)

// tcpAssociation wraps a TCP connection to implement the Association interface.
type tcpAssociation struct {
	conn net.Conn
}

func dialTCP(addr string) (Association, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("TCP dial %s: %w", addr, err)
	}
	return &tcpAssociation{conn: conn}, nil
}

func (a *tcpAssociation) Read() ([]byte, uint16, error) {
	header := make([]byte, headerSize)
	if _, err := io.ReadFull(a.conn, header); err != nil {
		return nil, 0, fmt.Errorf("reading message header: %w", err)
	}

	msgLen := binary.BigEndian.Uint32(header[0:4])
	streamID := binary.BigEndian.Uint16(header[4:6])

	if msgLen > maxMessageLen {
		return nil, 0, fmt.Errorf("message too large: %d bytes (max %d)", msgLen, maxMessageLen)
	}
	if msgLen == 0 {
		return []byte{}, streamID, nil
	}

	data := make([]byte, msgLen)
	if _, err := io.ReadFull(a.conn, data); err != nil {
		return nil, 0, fmt.Errorf("reading message body (%d bytes): %w", msgLen, err)
	}

	return data, streamID, nil
}

func (a *tcpAssociation) Write(data []byte, streamID uint16) error {
	if len(data) > maxMessageLen {
		return fmt.Errorf("message too large: %d bytes (max %d)", len(data), maxMessageLen)
	}

	header := make([]byte, headerSize)
	binary.BigEndian.PutUint32(header[0:4], uint32(len(data)))
	binary.BigEndian.PutUint16(header[4:6], streamID)

	if _, err := a.conn.Write(header); err != nil {
		return fmt.Errorf("writing header: %w", err)
	}
	if _, err := a.conn.Write(data); err != nil {
		return fmt.Errorf("writing body: %w", err)
	}
	return nil
}

func (a *tcpAssociation) Close() error {
	return a.conn.Close()
}

func (a *tcpAssociation) RemoteAddr() net.Addr {
	return a.conn.RemoteAddr()
}

func (a *tcpAssociation) LocalAddr() net.Addr {
	return a.conn.LocalAddr()
}

// tcpListener wraps a TCP listener to implement the Listener interface.
type tcpListener struct {
	ln net.Listener
}

func listenTCP(addr string) (Listener, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("TCP listen %s: %w", addr, err)
	}
	return &tcpListener{ln: ln}, nil
}

func (l *tcpListener) Accept() (Association, error) {
	conn, err := l.ln.Accept()
	if err != nil {
		return nil, err
	}
	return &tcpAssociation{conn: conn}, nil
}

func (l *tcpListener) Close() error {
	return l.ln.Close()
}

func (l *tcpListener) Addr() net.Addr {
	return l.ln.Addr()
}
