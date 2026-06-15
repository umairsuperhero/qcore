package smf

import (
	"errors"
	"net"
	"sync"
)

var (
	ErrPoolExhausted = errors.New("smf: IP pool exhausted")
	ErrInvalidPool   = errors.New("smf: invalid IP pool CIDR")
)

// IPAM assigns IPv4 addresses from a configured subnet to UEs.
type IPAM struct {
	mu     sync.Mutex
	ipNet  *net.IPNet
	nextIP net.IP
	inUse  map[string]bool // string representation of net.IP
}

// NewIPAM creates an IPAM for the given CIDR.
func NewIPAM(cidr string) (*IPAM, error) {
	ip, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, ErrInvalidPool
	}

	// Start with the first usable IP
	startIP := nextIP(ip.Mask(ipNet.Mask))

	return &IPAM{
		ipNet:  ipNet,
		nextIP: startIP,
		inUse:  make(map[string]bool),
	}, nil
}

// Allocate returns the next available IP address.
func (m *IPAM) Allocate() (net.IP, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	start := make(net.IP, len(m.nextIP))
	copy(start, m.nextIP)

	for {
		if !m.ipNet.Contains(m.nextIP) {
			// Wrap around to the first usable address of the configured pool.
			// We must mask the pool's network address, not the current nextIP:
			// once nextIP has run past the end it may land on another subnet's
			// boundary (e.g. .8 for a /29), which masks to itself and would
			// hand out addresses outside the pool.
			m.nextIP = nextIP(m.ipNet.IP.Mask(m.ipNet.Mask))
		}

		candidate := make(net.IP, len(m.nextIP))
		copy(candidate, m.nextIP)

		m.nextIP = nextIP(m.nextIP)

		// Skip broadcast address roughly
		if candidate[len(candidate)-1] == 255 {
			continue
		}

		if !m.inUse[candidate.String()] {
			m.inUse[candidate.String()] = true
			return candidate, nil
		}

		if m.nextIP.Equal(start) {
			return nil, ErrPoolExhausted
		}
	}
}

// Release marks an IP as available.
func (m *IPAM) Release(ip net.IP) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.inUse, ip.String())
}

func nextIP(ip net.IP) net.IP {
	next := make(net.IP, len(ip))
	copy(next, ip)
	for j := len(next) - 1; j >= 0; j-- {
		next[j]++
		if next[j] > 0 {
			break
		}
	}
	return next
}
