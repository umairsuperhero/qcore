package spgw

import (
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
)

// IPPool hands out IPv4 addresses from a configured CIDR. The network address,
// the gateway (configurable), and the broadcast address are reserved.
type IPPool struct {
	mu       sync.Mutex
	network  *net.IPNet
	gateway  net.IP
	netStart uint32 // first host offset (inclusive)
	netEnd   uint32 // last host offset (inclusive)
	next     uint32 // next candidate offset
	inUse    map[uint32]struct{}
	free     []uint32 // LIFO of recycled offsets
}

// NewIPPool builds a pool from a CIDR string (e.g. "10.45.0.0/24") and a
// gateway address that is excluded from allocation.
func NewIPPool(cidr, gateway string) (*IPPool, error) {
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("parsing CIDR %q: %w", cidr, err)
	}
	if ipnet.IP.To4() == nil {
		return nil, fmt.Errorf("IPv6 pools not yet supported: %s", cidr)
	}
	gw := net.ParseIP(gateway)
	if gw == nil || gw.To4() == nil {
		return nil, fmt.Errorf("invalid gateway %q", gateway)
	}
	if !ipnet.Contains(gw) {
		return nil, fmt.Errorf("gateway %s not in network %s", gw, ipnet)
	}

	base := ipToUint32(ipnet.IP.To4())
	ones, bits := ipnet.Mask.Size()
	hostBits := bits - ones
	total := uint32(1) << hostBits
	if total < 4 {
		return nil, fmt.Errorf("CIDR %s has too few hosts", cidr)
	}
	broadcast := base + total - 1
	gwOff := ipToUint32(gw.To4())

	p := &IPPool{
		network:  ipnet,
		gateway:  gw,
		netStart: base + 1,       // skip network address
		netEnd:   broadcast - 1,  // skip broadcast
		next:     base + 1,
		inUse:    make(map[uint32]struct{}),
	}
	// Skip the gateway by marking it as in-use.
	p.inUse[gwOff] = struct{}{}
	return p, nil
}

// Allocate returns the next free IPv4 address or an error if the pool is exhausted.
func (p *IPPool) Allocate() (net.IP, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.free) > 0 {
		off := p.free[len(p.free)-1]
		p.free = p.free[:len(p.free)-1]
		p.inUse[off] = struct{}{}
		return uint32ToIP(off), nil
	}

	for i := uint32(0); i <= p.netEnd-p.netStart; i++ {
		candidate := p.next
		if candidate > p.netEnd {
			candidate = p.netStart
		}
		p.next = candidate + 1
		if _, taken := p.inUse[candidate]; taken {
			continue
		}
		p.inUse[candidate] = struct{}{}
		return uint32ToIP(candidate), nil
	}
	return nil, fmt.Errorf("IP pool exhausted")
}

// Release returns an IP to the pool.
func (p *IPPool) Release(ip net.IP) {
	if ip == nil {
		return
	}
	v4 := ip.To4()
	if v4 == nil {
		return
	}
	off := ipToUint32(v4)
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.inUse[off]; !ok {
		return
	}
	delete(p.inUse, off)
	p.free = append(p.free, off)
}

// Gateway returns the configured gateway IP.
func (p *IPPool) Gateway() net.IP { return p.gateway }

// Network returns the pool's network.
func (p *IPPool) Network() *net.IPNet { return p.network }

func ipToUint32(ip net.IP) uint32 {
	return binary.BigEndian.Uint32(ip)
}

func uint32ToIP(v uint32) net.IP {
	out := make(net.IP, 4)
	binary.BigEndian.PutUint32(out, v)
	return out
}

// TEIDPool is a simple monotonic allocator for GTP-U TEIDs. TEID 0 is
// reserved (per TS 29.281 §5.1) for the path management (Echo) procedure.
type TEIDPool struct {
	next uint32 // atomic
}

// NewTEIDPool creates a new TEID allocator starting at `start` (typically 1
// so the first allocation is non-zero).
func NewTEIDPool(start uint32) *TEIDPool {
	return &TEIDPool{next: start}
}

// Next returns a fresh TEID. Wraps around to 1 if it ever hits 0.
func (t *TEIDPool) Next() uint32 {
	v := atomic.AddUint32(&t.next, 1)
	if v == 0 {
		v = atomic.AddUint32(&t.next, 1)
	}
	return v
}
