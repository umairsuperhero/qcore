// Package sctp provides an abstraction over SCTP transport for S1AP signaling.
// It defines interfaces that can be implemented by native SCTP (pion/sctp) or
// a TCP fallback for development on platforms without SCTP support.
package sctp

import (
	"fmt"
	"net"
)

// Association represents a single SCTP association (or TCP connection in fallback mode).
type Association interface {
	// Read reads a complete message from the association.
	Read() (data []byte, streamID uint16, err error)
	// Write sends a complete message on the given stream.
	Write(data []byte, streamID uint16) error
	// Close terminates the association.
	Close() error
	// RemoteAddr returns the remote address.
	RemoteAddr() net.Addr
	// LocalAddr returns the local address.
	LocalAddr() net.Addr
}

// Listener accepts incoming SCTP associations (or TCP connections).
type Listener interface {
	// Accept waits for and returns the next association.
	Accept() (Association, error)
	// Close stops listening.
	Close() error
	// Addr returns the listener's network address.
	Addr() net.Addr
}

// Mode selects the transport implementation.
type Mode string

const (
	ModeTCP  Mode = "tcp"  // TCP fallback for development
	ModeSCTP Mode = "sctp" // Native SCTP (requires pion/sctp or kernel support)
)

// Listen creates a new listener on the given address.
// For now, only TCP mode is implemented. Native SCTP will be added in Session 10.
func Listen(mode Mode, addr string) (Listener, error) {
	switch mode {
	case ModeTCP, "":
		return listenTCP(addr)
	default:
		return nil, fmt.Errorf("unsupported SCTP mode %q (available: tcp)", mode)
	}
}

// Dial connects to a remote address. Used by test clients and mock eNodeBs.
func Dial(mode Mode, addr string) (Association, error) {
	switch mode {
	case ModeTCP, "":
		return dialTCP(addr)
	default:
		return nil, fmt.Errorf("unsupported SCTP mode %q (available: tcp)", mode)
	}
}
