package upf

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/qcore-project/qcore/pkg/logger"
	"github.com/qcore-project/qcore/pkg/pfcp"
)

// PFCPServer handles the N4 control plane interface with the SMF.
type PFCPServer struct {
	addr   string
	conn   *net.UDPConn
	log    logger.Logger
	store  *SessionStore

	nodeID net.IP
}

// NewPFCPServer creates a new PFCP listener.
func NewPFCPServer(addr string, store *SessionStore, log logger.Logger) *PFCPServer {
	return &PFCPServer{
		addr:   addr,
		store:  store,
		log:    log.WithField("component", "pfcp"),
	}
}

// Start begins listening for incoming PFCP messages.
func (s *PFCPServer) Start(ctx context.Context) error {
	udpAddr, err := net.ResolveUDPAddr("udp", s.addr)
	if err != nil {
		return fmt.Errorf("resolve N4 address: %w", err)
	}
	
	// We use the bind IP as our NodeID
	s.nodeID = udpAddr.IP
	if len(s.nodeID) == 0 || s.nodeID.IsUnspecified() {
		// Fallback for 0.0.0.0
		s.nodeID = net.ParseIP("127.0.0.1").To4()
	} else {
		s.nodeID = s.nodeID.To4()
	}
	
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return fmt.Errorf("listen N4: %w", err)
	}
	s.conn = conn
	
	s.log.WithField("addr", s.addr).Info("PFCP listener ready")
	
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.readLoop()
	}()
	
	go func() {
		<-ctx.Done()
		s.conn.Close()
	}()
	
	return nil
}

func (s *PFCPServer) readLoop() {
	buf := make([]byte, 2048)
	for {
		n, addr, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			s.log.WithError(err).Warn("PFCP read error")
			return
		}
		
		msg, err := pfcp.DecodeMessage(buf[:n])
		if err != nil {
			s.log.WithError(err).Warn("Failed to decode PFCP message")
			continue
		}
		
		s.handleMessage(msg, addr)
	}
}

func (s *PFCPServer) handleMessage(msg *pfcp.Message, addr *net.UDPAddr) {
	switch msg.Header.MessageType {
	case pfcp.MsgTypeAssociationSetupRequest:
		s.handleAssociationSetupRequest(msg, addr)
	case pfcp.MsgTypeSessionEstablishmentRequest:
		s.handleSessionEstablishmentRequest(msg, addr)
	// TODO: Session Modification and Deletion
	default:
		s.log.WithField("type", msg.Header.MessageType).Warn("Unhandled PFCP message type")
	}
}

func (s *PFCPServer) handleAssociationSetupRequest(req *pfcp.Message, addr *net.UDPAddr) {
	s.log.WithField("remote", addr.String()).Info("Received Association Setup Request")
	
	ts := pfcp.NewRecoveryTimeStamp(time.Now())
	nodeID := pfcp.NewNodeID(pfcp.NodeIDTypeIPv4, s.nodeID, "")
	cause := pfcp.NewCause(pfcp.CauseRequestAccepted)
	
	resp := pfcp.NewAssociationSetupResponse(req.Header.SequenceNumber, nodeID, cause, ts)
	
	if _, err := s.conn.WriteToUDP(resp.Encode(), addr); err != nil {
		s.log.WithError(err).Error("Failed to send Association Setup Response")
	}
}

func (s *PFCPServer) handleSessionEstablishmentRequest(req *pfcp.Message, addr *net.UDPAddr) {
	s.log.WithFields(map[string]interface{}{
		"remote": addr.String(),
		"seid":   req.Header.SEID,
	}).Info("Received Session Establishment Request")
	
	// Create a new session in our store
	sess := &Session{}
	
	// In a real UPF, we would iterate through req.IEs to parse:
	// - Create PDRs (Packet Detection Rules) to find UE IP and TEIDs
	// - Create FARs (Forwarding Action Rules) to determine N3 vs N6 routing
	// For this phase, we allocate a new local TEID that the SMF can give to the AMF/gNB.
	
	sess.LocalTEID = s.store.AllocateTEID()
	
	// We should generate a local SEID to return to the SMF
	sess.SEID = uint64(sess.LocalTEID) // Simplified SEID generation
	
	s.store.AddSession(sess)
	
	nodeID := pfcp.NewNodeID(pfcp.NodeIDTypeIPv4, s.nodeID, "")
	cause := pfcp.NewCause(pfcp.CauseRequestAccepted)
	// We need to return an F-SEID pointing to our local SEID so the SMF knows how to address us
	fseid := pfcp.NewFSEID(sess.SEID, s.nodeID, nil)

	resp := pfcp.NewSessionEstablishmentResponse(req.Header.SequenceNumber, sess.SEID, nodeID, cause, fseid)
	
	if _, err := s.conn.WriteToUDP(resp.Encode(), addr); err != nil {
		s.log.WithError(err).Error("Failed to send Session Establishment Response")
	}
}
