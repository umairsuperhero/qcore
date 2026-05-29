//go:build linux

package upf

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

const (
	tunDevice = "/dev/net/tun"
	// IFF_TUN indicates a TUN device (IP packets) vs TAP (Ethernet)
	iffTun = 0x0001
	// IFF_NO_PI tells the kernel not to prepend the 4-byte packet info header
	iffNoPi = 0x1000
)

// ifReq matches struct ifreq from <linux/if.h>
type ifReq struct {
	Name  [16]byte
	Flags uint16
	_     [22]byte
}

// TunEgress sends/receives raw IP packets to the host OS network stack via a TUN interface.
type TunEgress struct {
	f      *os.File
	name   string
	closed bool
}

// NewTunEgress creates a Linux TUN device with the given name (e.g. "qcore-upf").
// Requires CAP_NET_ADMIN.
func NewTunEgress(name string) (*TunEgress, error) {
	if len(name) > 15 {
		return nil, errors.New("tun name too long")
	}

	f, err := os.OpenFile(tunDevice, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", tunDevice, err)
	}

	var req ifReq
	req.Flags = iffTun | iffNoPi
	copy(req.Name[:], name)

	// ioctl TUNSETIFF
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), uintptr(0x400454ca), uintptr(unsafe.Pointer(&req)))
	if errno != 0 {
		_ = f.Close()
		return nil, fmt.Errorf("ioctl TUNSETIFF failed: %v", errno)
	}

	return &TunEgress{
		f:    f,
		name: name,
	}, nil
}

func (t *TunEgress) Send(pkt []byte) error {
	_, err := t.f.Write(pkt)
	return err
}

func (t *TunEgress) Recv() ([]byte, error) {
	buf := make([]byte, 2048)
	n, err := t.f.Read(buf)
	if err != nil {
		if t.closed {
			return nil, ErrClosed
		}
		return nil, err
	}
	return buf[:n], nil
}

func (t *TunEgress) Close() error {
	t.closed = true
	return t.f.Close()
}

func (t *TunEgress) Name() string {
	return t.name
}
