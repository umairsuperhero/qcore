package amf

import (
	"context"
	"fmt"

	"github.com/qcore-project/qcore/pkg/logger"
	"github.com/qcore-project/qcore/pkg/ngap"
	"github.com/qcore-project/qcore/pkg/sctp"
)

// gNBSession handles all NGAP messages for one gNB connection.
// Each gNB gets its own goroutine; UE contexts are shared with the Service.
type gNBSession struct {
	amf  *Service
	conn sctp.Association
	log  logger.Logger

	// gNB identity set during NGSetup
	globalGNBID *ngap.GlobalGNBID
	gnbName     string
}

// run reads NGAP PDUs from the gNB and dispatches them until the connection
// closes or ctx is cancelled.
func (s *gNBSession) run(ctx context.Context) {
	defer s.conn.Close()
	s.log.Info("amf: gNB connected")
	for {
		raw, _, err := s.conn.Read()
		if err != nil {
			select {
			case <-ctx.Done():
			default:
				s.log.WithError(err).Warn("amf: gNB read error")
			}
			return
		}
		if err := s.dispatch(ctx, raw); err != nil {
			s.log.WithError(err).Warn("amf: dispatch error")
		}
	}
}

// dispatch decodes one NGAP PDU and routes it.
func (s *gNBSession) dispatch(ctx context.Context, raw []byte) error {
	pdu, err := ngap.DecodePDU(raw)
	if err != nil {
		return fmt.Errorf("decode PDU: %w", err)
	}
	ies, err := ngap.DecodeIEContainer(pdu.Value)
	if err != nil {
		return fmt.Errorf("decode IEs: %w", err)
	}

	switch {
	case pdu.Type == ngap.PDUInitiatingMessage && pdu.ProcedureCode == ngap.ProcNGSetup:
		return s.handleNGSetupRequest(ies)

	case pdu.Type == ngap.PDUInitiatingMessage && pdu.ProcedureCode == ngap.ProcInitialUEMessage:
		return s.handleInitialUEMessage(ctx, ies)

	case pdu.Type == ngap.PDUInitiatingMessage && pdu.ProcedureCode == ngap.ProcUplinkNASTransport:
		return s.handleUplinkNASTransport(ctx, ies)

	case pdu.Type == ngap.PDUSuccessfulOutcome && pdu.ProcedureCode == ngap.ProcInitialContextSetup:
		return s.handleInitialContextSetupResponse(ies)

	case pdu.Type == ngap.PDUInitiatingMessage && pdu.ProcedureCode == ngap.ProcUEContextRelease:
		return s.handleUEContextReleaseRequest(ies)

	default:
		s.log.WithField("proc", pdu.ProcedureCode).WithField("type", pdu.Type).Debug("amf: unhandled NGAP PDU")
		return nil
	}
}

// handleNGSetupRequest processes a gNB NG Setup Request and replies.
func (s *gNBSession) handleNGSetupRequest(ies []ngap.ProtocolIE) error {
	req, err := ngap.DecodeNGSetupRequest(ies)
	if err != nil {
		return fmt.Errorf("NGSetupRequest: %w", err)
	}
	s.globalGNBID = &req.GlobalRANNodeID
	s.gnbName = req.RANNodeName
	s.log.WithField("gnb_name", req.RANNodeName).Info("amf: NGSetup from gNB")

	resp := &ngap.NGSetupResponse{
		AMFName:          s.amf.cfg.AMFName,
		RelativeCapacity: 255,
		ServedGUAMIList: []ngap.ServedGUAMIItem{
			{GUAMI: s.amf.cfg.GUAMI},
		},
		PLMNSupportList: s.amf.cfg.PLMNSupportList,
	}
	pdu, err := ngap.EncodeNGSetupResponse(resp)
	if err != nil {
		return fmt.Errorf("encode NGSetupResponse: %w", err)
	}
	return s.send(pdu)
}

// handleInitialUEMessage processes the first NAS message from a UE.
func (s *gNBSession) handleInitialUEMessage(ctx context.Context, ies []ngap.ProtocolIE) error {
	msg, err := ngap.DecodeInitialUEMessage(ies)
	if err != nil {
		return fmt.Errorf("InitialUEMessage: %w", err)
	}

	amfID := s.amf.allocUEID()
	ue := newUEContext(amfID, msg.RANUENGAPID, s)
	ue.UserLocation = msg.UserLocationInfo
	s.amf.addUE(ue)

	s.log.WithField("amf_ue_ngap_id", amfID).WithField("ran_ue_ngap_id", msg.RANUENGAPID).
		Info("amf: new UE attached")

	return s.amf.handleNASPDU(ctx, ue, msg.NASPDU)
}

// handleUplinkNASTransport routes subsequent UE NAS messages.
func (s *gNBSession) handleUplinkNASTransport(ctx context.Context, ies []ngap.ProtocolIE) error {
	msg, err := ngap.DecodeUplinkNASTransport(ies)
	if err != nil {
		return fmt.Errorf("UplinkNASTransport: %w", err)
	}
	ue, ok := s.amf.getUE(msg.AMFUENGAPID)
	if !ok {
		s.log.WithField("amf_ue_ngap_id", msg.AMFUENGAPID).Warn("amf: unknown UE in UL-NAS")
		return nil
	}
	return s.amf.handleNASPDU(ctx, ue, msg.NASPDU)
}

// handleInitialContextSetupResponse — gNB confirmed the UE context was created.
func (s *gNBSession) handleInitialContextSetupResponse(ies []ngap.ProtocolIE) error {
	resp, err := ngap.DecodeInitialContextSetupResponse(ies)
	if err != nil {
		return fmt.Errorf("InitialContextSetupResponse: %w", err)
	}
	ue, ok := s.amf.getUE(resp.AMFUENGAPID)
	if !ok {
		return nil
	}
	s.log.WithField("amf_ue_ngap_id", ue.AMFUENGAPID).Info("amf: InitialContextSetup confirmed by gNB")
	return nil
}

// handleUEContextReleaseRequest — gNB requests UE context teardown.
func (s *gNBSession) handleUEContextReleaseRequest(ies []ngap.ProtocolIE) error {
	// For now: acknowledge with UEContextReleaseCommand then Complete.
	// A real AMF would look up the UE and update state.
	s.log.Debug("amf: UEContextReleaseRequest received")
	return nil
}

// sendDownlinkNAS wraps a NAS PDU in DownlinkNASTransport and sends it.
func (s *gNBSession) sendDownlinkNAS(ue *UEContext, nasPDU []byte) error {
	pdu, err := ngap.EncodeDownlinkNASTransport(&ngap.DownlinkNASTransport{
		AMFUENGAPID: ue.AMFUENGAPID,
		RANUENGAPID: ue.RANUENGAPID,
		NASPDU:      nasPDU,
	})
	if err != nil {
		return err
	}
	return s.send(pdu)
}

// sendInitialContextSetup sends InitialContextSetupRequest to the gNB.
func (s *gNBSession) sendInitialContextSetup(ue *UEContext, nasPDU []byte) error {
	req := &ngap.InitialContextSetupRequest{
		AMFUENGAPID:  ue.AMFUENGAPID,
		RANUENGAPID:  ue.RANUENGAPID,
		GUAMI:        s.amf.cfg.GUAMI,
		AllowedNSSAI: ue.NSSAI,
		NASPDU:       nasPDU,
	}
	// Security capabilities
	if len(ue.UESecCaps) >= 4 {
		req.UESecurityCapabilities.NREncAlgs = [2]byte{ue.UESecCaps[0], ue.UESecCaps[1]}
		req.UESecurityCapabilities.NRIntAlgs = [2]byte{ue.UESecCaps[2], ue.UESecCaps[3]}
	}
	if len(ue.UESecCaps) >= 8 {
		req.UESecurityCapabilities.EUTRAEncAlgs = [2]byte{ue.UESecCaps[4], ue.UESecCaps[5]}
		req.UESecurityCapabilities.EUTRAIntAlgs = [2]byte{ue.UESecCaps[6], ue.UESecCaps[7]}
	}
	// KgNB → SecurityKey
	if len(ue.KgNB) >= 32 {
		copy(req.SecurityKey[:], ue.KgNB[:32])
	}

	pdu, err := ngap.EncodeInitialContextSetupRequest(req)
	if err != nil {
		return err
	}
	return s.send(pdu)
}

func (s *gNBSession) send(raw []byte) error {
	return s.conn.Write(raw, 0)
}
