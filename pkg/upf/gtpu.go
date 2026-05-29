package upf

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"

	"github.com/qcore-project/qcore/pkg/gtp"
	"github.com/qcore-project/qcore/pkg/logger"
)

// GTPUServer handles the N3 data plane interface with the gNB.
type GTPUServer struct {
	addr   string
	conn   *net.UDPConn
	log    logger.Logger
	store  *SessionStore
	egress Egress
	
	wg sync.WaitGroup
}

// NewGTPUServer creates a GTP-U listener on the given address (typically :2152).
func NewGTPUServer(addr string, store *SessionStore, egress Egress, log logger.Logger) *GTPUServer {
	return &GTPUServer{
		addr:   addr,
		store:  store,
		egress: egress,
		log:    log.WithField("component", "gtpu"),
	}
}

// Start begins listening for incoming GTP-U packets (Uplink).
func (s *GTPUServer) Start(ctx context.Context) error {
	udpAddr, err := net.ResolveUDPAddr("udp", s.addr)
	if err != nil {
		return fmt.Errorf("resolve N3 address: %w", err)
	}
	
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return fmt.Errorf("listen N3: %w", err)
	}
	s.conn = conn
	
	s.log.WithField("addr", s.addr).Info("GTP-U listener ready")
	
	// Start Uplink (gNB -> UPF -> Internet) worker
	s.wg.Add(1)
	go s.uplinkWorker()
	
	// Start Downlink (Internet -> UPF -> gNB) worker
	s.wg.Add(1)
	go s.downlinkWorker()
	
	// Close connection when context is canceled
	go func() {
		<-ctx.Done()
		s.conn.Close()
	}()
	
	return nil
}

// uplinkWorker reads GTP-U packets from N3, decapsulates them, and sends them to the TUN interface.
func (s *GTPUServer) uplinkWorker() {
	defer s.wg.Done()
	
	buf := make([]byte, 65535)
	for {
		n, addr, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			s.log.WithError(err).Warn("GTP-U read error")
			return
		}
		
		hdr, payload, err := gtp.Decode(buf[:n])
		if err != nil {
			s.log.WithError(err).Debug("Failed to decode GTP-U")
			continue
		}

		// Look up the session by TEID
		sess, err := s.store.GetByTEID(hdr.TEID)
		if err != nil {
			s.log.WithFields(map[string]interface{}{
				"teid": hdr.TEID,
				"src":  addr.String(),
			}).Warn("Received uplink GTP-U for unknown TEID")
			continue
		}
		
		// Security: could check if inner IP src matches sess.UEIP here.
		_ = sess // Mark as used for future IP verification
		
		// Send inner IP packet to egress
		if err := s.egress.Send(payload); err != nil {
			s.log.WithError(err).Error("Failed to write to egress")
		}
	}
}

// downlinkWorker reads raw IP packets from the TUN interface, encapsulates them, and sends them to N3.
func (s *GTPUServer) downlinkWorker() {
	defer s.wg.Done()
	
	for {
		pkt, err := s.egress.Recv()
		if err != nil {
			if errors.Is(err, ErrClosed) {
				return
			}
			s.log.WithError(err).Error("Egress read error")
			continue
		}
		
		// Parse destination IP from the raw IPv4 packet (bytes 16-19)
		if len(pkt) < 20 {
			continue
		}
		dstIP := net.IPv4(pkt[16], pkt[17], pkt[18], pkt[19])
		
		// Look up session by UE IP
		sess, err := s.store.GetByUEIP(dstIP)
		if err != nil {
			s.log.WithField("dstIP", dstIP.String()).Debug("No session found for downlink packet")
			continue
		}
		
		if sess.RemoteTEID == 0 || sess.RemoteIP == nil {
			s.log.WithField("dstIP", dstIP.String()).Warn("Session missing remote gNB TEID/IP")
			continue
		}
		
		// Encapsulate into GTP-U (G-PDU / T-PDU) toward the gNB's TEID.
		gtpPkt, err := gtp.EncodeTPDU(sess.RemoteTEID, pkt)
		if err != nil {
			s.log.WithError(err).Error("Failed to encode downlink GTP-U")
			continue
		}

		remoteAddr := &net.UDPAddr{IP: sess.RemoteIP, Port: 2152}
		if _, err := s.conn.WriteToUDP(gtpPkt, remoteAddr); err != nil {
			s.log.WithError(err).Error("Failed to send downlink GTP-U")
		}
	}
}
