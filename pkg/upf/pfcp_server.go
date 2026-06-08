package upf

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/qcore-project/qcore/pkg/events"
	"github.com/qcore-project/qcore/pkg/logger"
	"github.com/qcore-project/qcore/pkg/pfcp"
)

// PFCPServer handles the N4 control plane interface with the SMF.
type PFCPServer struct {
	addr    string
	conn    *net.UDPConn
	log     logger.Logger
	store   *SessionStore
	emitter events.Emitter

	nodeID net.IP
}

// NewPFCPServer creates a new PFCP listener.
func NewPFCPServer(addr string, store *SessionStore, emitter events.Emitter, log logger.Logger) *PFCPServer {
	if emitter == nil {
		emitter = &events.NoopEmitter{}
	}
	return &PFCPServer{
		addr:    addr,
		store:   store,
		emitter: emitter,
		log:     log.WithField("component", "pfcp"),
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
	case pfcp.MsgTypeSessionModificationRequest:
		s.handleSessionModificationRequest(msg, addr)
	default:
		s.log.WithField("type", msg.Header.MessageType).Warn("Unhandled PFCP message type")
	}
}

func (s *PFCPServer) handleAssociationSetupRequest(req *pfcp.Message, addr *net.UDPAddr) {
	s.log.WithField("remote", addr.String()).Info("Received Association Setup Request")
	s.emitter.Emit(events.Event{
		NF:       "upf",
		Category: events.SignalingRx,
		Severity: events.SeverityInfo,
		Protocol: "pfcp",
		Message:  "N4 Association Setup Request received",
		Payload: events.UPFPFCPAssocPayload{
			RemoteAddr: addr.String(),
			Success:    true,
		},
	})

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
	for _, ie := range req.IEs {
		if ie.Type == pfcp.IETypeUEIPAddress {
			if ip, err := pfcp.ParseUEIPAddress(ie); err == nil {
				sess.UEIP = ip
			}
		}
	}

	// We should generate a local SEID to return to the SMF
	sess.SEID = uint64(sess.LocalTEID) // Simplified SEID generation

	s.store.AddSession(sess)

	s.emitter.Emit(events.Event{
		NF:       "upf",
		Category: events.SignalingRx,
		Severity: events.SeverityInfo,
		Protocol: "pfcp",
		Message:  "N4 Session Establishment Request: TEID allocated",
		Payload: events.UPFPFCPSessionPayload{
			SEID:      sess.SEID,
			LocalTEID: sess.LocalTEID,
			Success:   true,
		},
	})

	nodeID := pfcp.NewNodeID(pfcp.NodeIDTypeIPv4, s.nodeID, "")
	cause := pfcp.NewCause(pfcp.CauseRequestAccepted)
	// We need to return an F-SEID pointing to our local SEID so the SMF knows how to address us
	fseid := pfcp.NewFSEID(sess.SEID, s.nodeID, nil)
	fteid := pfcp.NewFTEID(sess.LocalTEID, s.nodeID)

	resp := pfcp.NewSessionEstablishmentResponse(req.Header.SequenceNumber, sess.SEID, nodeID, cause, fseid, fteid)

	if _, err := s.conn.WriteToUDP(resp.Encode(), addr); err != nil {
		s.log.WithError(err).Error("Failed to send Session Establishment Response")
	}
}

func (s *PFCPServer) handleSessionModificationRequest(req *pfcp.Message, addr *net.UDPAddr) {
	var remoteTEID uint32
	var remoteIP net.IP
	for _, ie := range req.IEs {
		if ie.Type != pfcp.IETypeFTEID {
			continue
		}
		teid, ip, err := pfcp.ParseFTEID(ie)
		if err != nil {
			s.log.WithError(err).Warn("Malformed F-TEID in Session Modification Request")
			continue
		}
		remoteTEID = teid
		remoteIP = ip
	}

	cause := pfcp.NewCause(pfcp.CauseRequestAccepted)
	if remoteTEID == 0 || remoteIP == nil {
		cause = pfcp.NewCause(pfcp.CauseRequestRejected)
	} else if err := s.store.UpdateRemoteTunnel(req.Header.SEID, remoteTEID, remoteIP); err != nil {
		s.log.WithError(err).WithField("seid", req.Header.SEID).Warn("PFCP modify for unknown session")
		cause = pfcp.NewCause(pfcp.CauseRequestRejected)
	} else {
		s.log.WithFields(map[string]interface{}{
			"seid":        req.Header.SEID,
			"remote_teid": remoteTEID,
			"remote_ip":   remoteIP.String(),
		}).Info("Updated UPF remote gNB tunnel")
	}

	resp := pfcp.NewSessionModificationResponse(req.Header.SequenceNumber, req.Header.SEID, cause)
	if _, err := s.conn.WriteToUDP(resp.Encode(), addr); err != nil {
		s.log.WithError(err).Error("Failed to send Session Modification Response")
	}
}
