package spgw

import (
	"net"
	"sync/atomic"

	"github.com/qcore-project/qcore/pkg/logger"
)

// Egress is the "north-side" of the SPGW: where decapsulated uplink IP
// packets go, and where downlink IP packets come from. In Phase 3 we ship
// one implementation — LogEgress — which just records packet metadata.
// A TUN-based implementation is planned for Phase 3b.
type Egress interface {
	// Send hands a decapsulated uplink IP packet to the egress. Implementations
	// must not retain the slice after return.
	Send(pkt []byte) error

	// Recv blocks until the egress has a downlink IP packet to inject. Returns
	// (nil, io.EOF) when the egress is closed. The returned slice is owned by
	// the caller.
	Recv() ([]byte, error)

	// Close releases any resources.
	Close() error

	// Name returns a short identifier for metrics / logs.
	Name() string
}

// LogEgress is a no-op egress that just logs packet metadata. It drops all
// uplink and never emits downlink. Safe default for CI and Windows dev.
type LogEgress struct {
	log     logger.Logger
	count   uint64 // atomic
	done    chan struct{}
	closed  bool
}

// NewLogEgress creates a log-only egress.
func NewLogEgress(log logger.Logger) *LogEgress {
	return &LogEgress{
		log:  log.WithField("egress", "log"),
		done: make(chan struct{}),
	}
}

// Send logs packet metadata (src/dst/proto) without forwarding.
func (l *LogEgress) Send(pkt []byte) error {
	n := atomic.AddUint64(&l.count, 1)
	src, dst, proto := parseIPv4Headers(pkt)
	l.log.Infof("egress[log] #%d: %d bytes, %s → %s (proto=%d)", n, len(pkt), src, dst, proto)
	return nil
}

// Recv blocks until Close is called.
func (l *LogEgress) Recv() ([]byte, error) {
	<-l.done
	return nil, errClosed
}

// Close unblocks any pending Recv.
func (l *LogEgress) Close() error {
	if !l.closed {
		l.closed = true
		close(l.done)
	}
	return nil
}

// Name returns "log".
func (l *LogEgress) Name() string { return "log" }

// Count returns the number of uplink packets the egress has seen (for tests).
func (l *LogEgress) Count() uint64 { return atomic.LoadUint64(&l.count) }

// parseIPv4Headers extracts src/dst/proto from an IPv4 packet. Returns zero
// values if the packet isn't recognisable IPv4.
func parseIPv4Headers(pkt []byte) (src, dst net.IP, proto uint8) {
	if len(pkt) < 20 {
		return nil, nil, 0
	}
	if v := pkt[0] >> 4; v != 4 {
		return nil, nil, 0
	}
	proto = pkt[9]
	src = net.IPv4(pkt[12], pkt[13], pkt[14], pkt[15])
	dst = net.IPv4(pkt[16], pkt[17], pkt[18], pkt[19])
	return
}

// errClosed is returned by Recv after Close.
var errClosed = errClosedEgress{}

type errClosedEgress struct{}

func (errClosedEgress) Error() string { return "egress closed" }
