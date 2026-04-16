//go:build linux

package spgw

import (
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/qcore-project/qcore/pkg/logger"
	"golang.org/x/sys/unix"
)

// TUNEgress is a Linux-only egress that reads/writes IP packets from a TUN
// device. The operator is expected to have configured the device's address
// (the SPGW gateway IP) and any required SNAT/forwarding rules out of band —
// see docs/PHASE3.md for a sample iptables setup. We deliberately keep this
// implementation packet-mover-only so it stays auditable.
//
// Layout:
//
//	uplink:   SPGW.handleUplink → egress.Send(pkt) → write(tunFD, pkt)
//	downlink: read(tunFD, buf) → egress queue → SPGW.downlinkPump → Forward
type TUNEgress struct {
	log     logger.Logger
	name    string
	mtu     int
	fd      int
	file    *os.File
	queue   chan []byte
	done    chan struct{}
	closed  atomic.Bool
	closeMu sync.Mutex

	uplinkBytes   uint64
	downlinkBytes uint64
	writeErrs     uint64
	readErrs      uint64
}

const (
	// IFF_TUN | IFF_NO_PI: layer-3 packets, no 4-byte protocol info prefix.
	cIFF_TUN   = 0x0001
	cIFF_NO_PI = 0x1000
)

// ifReq matches struct ifreq from <linux/if.h>. We only use the .name and
// .flags fields; pad the rest to ensure a stable 40-byte size.
type ifReq struct {
	Name  [unix.IFNAMSIZ]byte
	Flags uint16
	_     [22]byte
}

// NewTUNEgress opens /dev/net/tun and binds it to the named device with TUN
// (layer-3, IPv4) framing. The device must already exist or this call must be
// running as root so the kernel will create it. Returns the Egress interface
// (the concrete *TUNEgress is also accessible via tunEgressFromInterface in tests).
func NewTUNEgress(log logger.Logger, name string, mtu int) (Egress, error) {
	if name == "" {
		name = "qcore0"
	}
	if mtu <= 0 {
		mtu = 1400
	}
	if len(name) >= unix.IFNAMSIZ {
		return nil, fmt.Errorf("tun device name %q too long (max %d)", name, unix.IFNAMSIZ-1)
	}

	fd, err := unix.Open("/dev/net/tun", unix.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("open /dev/net/tun: %w (need root or CAP_NET_ADMIN)", err)
	}

	var req ifReq
	copy(req.Name[:], name)
	req.Flags = cIFF_TUN | cIFF_NO_PI

	if _, _, errno := unix.Syscall(
		unix.SYS_IOCTL,
		uintptr(fd),
		uintptr(unix.TUNSETIFF),
		uintptr(unsafe.Pointer(&req)),
	); errno != 0 {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("TUNSETIFF on %q: %w", name, errno)
	}

	// Wrap fd in *os.File so reads can be interrupted via Close.
	file := os.NewFile(uintptr(fd), "/dev/net/tun")

	t := &TUNEgress{
		log:   log.WithField("egress", "tun").WithField("dev", name),
		name:  name,
		mtu:   mtu,
		fd:    fd,
		file:  file,
		queue: make(chan []byte, 256),
		done:  make(chan struct{}),
	}

	go t.readLoop()
	t.log.Infof("TUN egress ready: dev=%s mtu=%d", name, mtu)
	return t, nil
}

// Send writes one decapsulated uplink IP packet to the TUN device. The kernel
// then routes it like any other IP packet (subject to whatever SNAT /
// forwarding rules the operator configured).
func (t *TUNEgress) Send(pkt []byte) error {
	if t.closed.Load() {
		return io.ErrClosedPipe
	}
	if len(pkt) > t.mtu+20 {
		// IPv4 header may push us slightly over MTU; tolerate but log.
		t.log.Debugf("uplink packet length %d > MTU %d", len(pkt), t.mtu)
	}
	n, err := t.file.Write(pkt)
	if err != nil {
		atomic.AddUint64(&t.writeErrs, 1)
		return fmt.Errorf("tun write: %w", err)
	}
	atomic.AddUint64(&t.uplinkBytes, uint64(n))
	return nil
}

// Recv blocks until the read goroutine has a downlink packet. Returns
// (nil, io.EOF) after Close.
func (t *TUNEgress) Recv() ([]byte, error) {
	select {
	case pkt, ok := <-t.queue:
		if !ok {
			return nil, io.EOF
		}
		return pkt, nil
	case <-t.done:
		return nil, io.EOF
	}
}

// Close shuts the device and unblocks Recv.
func (t *TUNEgress) Close() error {
	t.closeMu.Lock()
	defer t.closeMu.Unlock()
	if t.closed.Swap(true) {
		return nil
	}
	close(t.done)
	// Closing the *os.File interrupts any blocked Read in the goroutine.
	err := t.file.Close()
	// Drain in case anything is still queued.
	go func() {
		for range t.queue {
		}
	}()
	close(t.queue)
	return err
}

// Name identifies the egress in logs/metrics.
func (t *TUNEgress) Name() string { return "tun:" + t.name }

// Stats returns counters useful for /metrics surfacing later.
func (t *TUNEgress) Stats() (uplinkBytes, downlinkBytes, writeErrs, readErrs uint64) {
	return atomic.LoadUint64(&t.uplinkBytes),
		atomic.LoadUint64(&t.downlinkBytes),
		atomic.LoadUint64(&t.writeErrs),
		atomic.LoadUint64(&t.readErrs)
}

func (t *TUNEgress) readLoop() {
	buf := make([]byte, 65535)
	for {
		n, err := t.file.Read(buf)
		if err != nil {
			if t.closed.Load() {
				return
			}
			atomic.AddUint64(&t.readErrs, 1)
			t.log.Debugf("tun read: %v", err)
			return
		}
		if n == 0 {
			continue
		}
		// Defensive copy; the queue may hold the packet for some time.
		pkt := make([]byte, n)
		copy(pkt, buf[:n])
		atomic.AddUint64(&t.downlinkBytes, uint64(n))

		select {
		case t.queue <- pkt:
		case <-t.done:
			return
		default:
			// Queue full; drop oldest by skipping. Keep things bounded.
			t.log.Debugf("downlink queue full, dropping %d bytes", n)
		}
	}
}

