//go:build !linux

package upf

import (
	"errors"
)

// TunEgress is a stub for non-Linux platforms (e.g. macOS).
type TunEgress struct {
	name string
	closed bool
}

// NewTunEgress returns an error on non-Linux platforms.
func NewTunEgress(name string) (*TunEgress, error) {
	return nil, errors.New("TUN egress is only supported on Linux")
}

func (t *TunEgress) Send(pkt []byte) error { return errors.New("not implemented") }

func (t *TunEgress) Recv() ([]byte, error) { return nil, errors.New("not implemented") }

func (t *TunEgress) Close() error { return nil }

func (t *TunEgress) Name() string { return t.name }
