package spgw

import (
	"fmt"
	"net"
	"sync/atomic"
	"time"

	"github.com/qcore-project/qcore/pkg/gtp"
	"github.com/qcore-project/qcore/pkg/logger"
	"github.com/qcore-project/qcore/pkg/metrics"
)

// Dataplane owns the GTP-U UDP socket. It decapsulates uplink T-PDUs and
// hands the inner IP packets to the egress. For downlink, callers (or the
// egress pump goroutine) call Forward to encapsulate an IP packet and send
// it toward the eNB.
type Dataplane struct {
	conn     *net.UDPConn
	sessions *SessionStore
	egress   Egress
	log      logger.Logger
	metrics  *metrics.SPGWMetrics // optional; nil-safe

	uplinkCount   uint64
	downlinkCount uint64
	dropCount     uint64

	quit chan struct{}
}

// dropCause is a small enum so call sites don't sprinkle string literals.
type dropCause string

const (
	dropDecode    dropCause = "decode"
	dropUnknownTE dropCause = "unknown_teid"
	dropEgress    dropCause = "egress_send"
	dropNoBearer  dropCause = "no_bearer"
	dropNoENB     dropCause = "no_enb"
	dropEncode    dropCause = "encode"
	dropDLWrite   dropCause = "dl_write"
)

func (d *Dataplane) dropMetric(c dropCause) {
	atomic.AddUint64(&d.dropCount, 1)
	if d.metrics != nil {
		d.metrics.Drops.WithLabelValues(string(c)).Inc()
	}
}

// NewDataplane binds a UDP socket on the given address (":2152" by default).
func NewDataplane(addr string, sessions *SessionStore, egress Egress, log logger.Logger) (*Dataplane, error) {
	if addr == "" {
		addr = fmt.Sprintf(":%d", gtp.PortU)
	}
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", addr, err)
	}
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, fmt.Errorf("listen udp %s: %w", addr, err)
	}
	return &Dataplane{
		conn:     conn,
		sessions: sessions,
		egress:   egress,
		log:      log.WithField("component", "spgw-u"),
		quit:     make(chan struct{}),
	}, nil
}

// LocalAddr returns the bound UDP address (useful when addr was ":0").
func (d *Dataplane) LocalAddr() *net.UDPAddr {
	return d.conn.LocalAddr().(*net.UDPAddr)
}

// Run pumps packets until Stop is called. Also spawns a goroutine that
// forwards downlink packets emerging from the egress toward the correct eNB.
func (d *Dataplane) Run() {
	go d.downlinkPump()
	d.uplinkLoop()
}

// Stop closes the UDP socket and terminates the pumps.
func (d *Dataplane) Stop() {
	select {
	case <-d.quit:
		return
	default:
		close(d.quit)
	}
	_ = d.conn.Close()
}

// Stats returns a snapshot of counters.
func (d *Dataplane) Stats() (uplink, downlink, drops uint64) {
	return atomic.LoadUint64(&d.uplinkCount),
		atomic.LoadUint64(&d.downlinkCount),
		atomic.LoadUint64(&d.dropCount)
}

func (d *Dataplane) uplinkLoop() {
	buf := make([]byte, 65535)
	for {
		_ = d.conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		n, src, err := d.conn.ReadFromUDP(buf)
		if err != nil {
			select {
			case <-d.quit:
				return
			default:
			}
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			d.log.Debugf("UDP read error: %v", err)
			continue
		}
		d.handleUplink(src, buf[:n])
	}
}

func (d *Dataplane) handleUplink(src *net.UDPAddr, raw []byte) {
	hdr, payload, err := gtp.Decode(raw)
	if err != nil {
		d.dropMetric(dropDecode)
		d.log.Debugf("invalid GTP-U from %s: %v", src, err)
		return
	}

	switch hdr.MessageType {
	case gtp.MsgEchoRequest:
		d.handleEcho(src, hdr)
		return
	case gtp.MsgTPDU:
		// fall through
	default:
		d.log.Debugf("unhandled GTP-U message type %d from %s", hdr.MessageType, src)
		return
	}

	bearer, ok := d.sessions.GetBySGWTEID(hdr.TEID)
	if !ok {
		d.dropMetric(dropUnknownTE)
		d.log.Warnf("uplink T-PDU for unknown TEID=0x%x from %s (dropping)", hdr.TEID, src)
		return
	}

	atomic.AddUint64(&d.uplinkCount, 1)
	if d.metrics != nil {
		d.metrics.UplinkPackets.WithLabelValues().Inc()
		d.metrics.UplinkBytes.WithLabelValues().Add(float64(len(payload)))
	}

	// Opportunistically learn the eNB's source address if it changed. This
	// matters when the eNB is behind NAT or its IP wasn't known at attach.
	if bearer.ENBAddr == nil || !bearer.ENBAddr.Equal(src.IP) {
		d.log.Debugf("learned eNB U-plane addr %s for IMSI=%s TEID=0x%x", src.IP, bearer.IMSI, hdr.TEID)
	}

	// Make a defensive copy of the IP payload before handing to the egress —
	// the shared read buffer will be reused on the next ReadFromUDP.
	pkt := make([]byte, len(payload))
	copy(pkt, payload)

	if err := d.egress.Send(pkt); err != nil {
		d.log.Warnf("egress send failed: %v", err)
		d.dropMetric(dropEgress)
	}
}

func (d *Dataplane) handleEcho(src *net.UDPAddr, hdr *gtp.Header) {
	if d.metrics != nil {
		d.metrics.EchoRequests.WithLabelValues().Inc()
	}
	// Echo Response: type=2, TEID=0, Recovery IE value 0.
	respHdr := &gtp.Header{
		Flags:       0x02, // S flag
		MessageType: gtp.MsgEchoResponse,
		TEID:        0,
		Sequence:    hdr.Sequence,
	}
	recovery := []byte{14, 0}
	pkt, err := gtp.Encode(respHdr, recovery)
	if err != nil {
		return
	}
	_, _ = d.conn.WriteToUDP(pkt, src)
}

// downlinkPump forwards IP packets emerging from the egress toward the
// appropriate eNB by looking up the bearer bound to the packet's destination IP.
func (d *Dataplane) downlinkPump() {
	for {
		select {
		case <-d.quit:
			return
		default:
		}
		pkt, err := d.egress.Recv()
		if err != nil {
			return // egress closed
		}
		d.Forward(pkt)
	}
}

// Forward encapsulates an IP packet destined for a UE and sends it to the
// appropriate eNB over S1-U. Exported so tests can inject downlink packets
// without going through an egress.
func (d *Dataplane) Forward(ipPkt []byte) {
	_, dst, _ := parseIPv4Headers(ipPkt)
	if dst == nil {
		d.dropMetric(dropDecode)
		return
	}
	bearer, ok := d.sessions.GetByUEIP(dst)
	if !ok {
		d.dropMetric(dropNoBearer)
		d.log.Debugf("no bearer for UE-IP=%s, dropping downlink", dst)
		return
	}
	if bearer.ENBAddr == nil || bearer.ENBTEID == 0 {
		d.dropMetric(dropNoENB)
		d.log.Debugf("bearer for UE-IP=%s has no eNB endpoint yet, dropping", dst)
		return
	}
	pkt, err := gtp.EncodeTPDU(bearer.ENBTEID, ipPkt)
	if err != nil {
		d.dropMetric(dropEncode)
		return
	}
	enbAddr := &net.UDPAddr{IP: bearer.ENBAddr, Port: gtp.PortU}
	if _, err := d.conn.WriteToUDP(pkt, enbAddr); err != nil {
		d.dropMetric(dropDLWrite)
		d.log.Debugf("downlink write failed: %v", err)
		return
	}
	atomic.AddUint64(&d.downlinkCount, 1)
	if d.metrics != nil {
		d.metrics.DownlinkPackets.WithLabelValues().Inc()
		d.metrics.DownlinkBytes.WithLabelValues().Add(float64(len(ipPkt)))
	}
}
