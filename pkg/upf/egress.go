package upf

import (
	"errors"
)

// Egress represents the N6 interface of the UPF.
// Decapsulated uplink packets (from UE) are sent here to the internet.
// Downlink packets (from internet to UE) are received from here.
type Egress interface {
	// Send routes a decapsulated uplink IP packet out.
	Send(pkt []byte) error

	// Recv blocks until a downlink IP packet arrives for a UE.
	Recv() ([]byte, error)

	// Close releases the egress resources.
	Close() error

	// Name returns an identifier for this egress.
	Name() string
}

var (
	ErrClosed = errors.New("upf: egress closed")
)

// DummyEgress provides a safe fallback when TUN is unavailable (e.g. non-Linux or missing caps).
type DummyEgress struct {
	done   chan struct{}
	closed bool
}

func NewDummyEgress() *DummyEgress {
	return &DummyEgress{done: make(chan struct{})}
}

func (d *DummyEgress) Send(pkt []byte) error { return nil }

func (d *DummyEgress) Recv() ([]byte, error) {
	<-d.done
	return nil, ErrClosed
}

func (d *DummyEgress) Close() error {
	if !d.closed {
		d.closed = true
		close(d.done)
	}
	return nil
}

func (d *DummyEgress) Name() string { return "dummy" }
