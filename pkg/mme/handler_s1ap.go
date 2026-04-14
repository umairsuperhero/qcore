package mme

import (
	"context"
	"fmt"

	"github.com/qcore-project/qcore/pkg/s1ap"
)

// handleS1APMessage decodes and dispatches an S1AP PDU.
func (m *MME) handleS1APMessage(ctx context.Context, enb *EnbContext, data []byte, streamID uint16) {
	pdu, err := s1ap.DecodePDU(data)
	if err != nil {
		m.log.Errorf("Failed to decode S1AP PDU from %s: %v", enb.Assoc.RemoteAddr(), err)
		return
	}

	m.log.Infof("S1AP %s: %s (proc=%d) from %s",
		pdu.Type, pdu.ProcedureCode, pdu.ProcedureCode, enb.Assoc.RemoteAddr())

	// Decode the ProtocolIE container from the PDU value
	ies, err := s1ap.DecodeProtocolIEContainer(pdu.Value)
	if err != nil {
		m.log.Errorf("Failed to decode ProtocolIE container: %v", err)
		return
	}

	switch pdu.ProcedureCode {
	case s1ap.ProcS1Setup:
		m.handleS1Setup(ctx, enb, ies, streamID)

	case s1ap.ProcInitialUEMessage:
		m.handleInitialUEMessage(ctx, enb, ies, streamID)

	case s1ap.ProcUplinkNASTransport:
		m.handleUplinkNASTransport(ctx, enb, ies, streamID)

	default:
		m.log.Warnf("Unhandled S1AP procedure: %s (%d)", pdu.ProcedureCode, pdu.ProcedureCode)
	}
}

// handleS1Setup processes an S1 Setup Request from an eNodeB.
func (m *MME) handleS1Setup(ctx context.Context, enb *EnbContext, ies []s1ap.ProtocolIE, streamID uint16) {
	req, err := s1ap.DecodeS1SetupRequest(ies)
	if err != nil {
		m.log.Errorf("Failed to decode S1SetupRequest: %v", err)
		m.sendS1SetupFailure(enb, streamID)
		return
	}

	m.log.Infof("S1 Setup from eNB: name=%q, eNB-ID=0x%x, PLMN=%x, TAs=%d",
		req.ENBName, req.GlobalENBID.ENBID, req.GlobalENBID.PLMN, len(req.SupportedTAs))

	// Validate: at least one TA must match our configured PLMN
	matched := false
	for _, ta := range req.SupportedTAs {
		for _, plmn := range ta.PLMNs {
			if plmn == m.plmn {
				matched = true
				break
			}
		}
		if matched {
			break
		}
	}

	if !matched {
		m.log.Warnf("S1 Setup rejected: no matching PLMN (eNB PLMNs vs our %x)", m.plmn)
		if m.metrics != nil {
			m.metrics.S1SetupRequests.WithLabelValues("rejected").Inc()
		}
		m.sendS1SetupFailure(enb, streamID)
		return
	}

	// Store eNB info
	enb.mu.Lock()
	enb.GlobalENBID = GlobalENBID{
		PLMN:  req.GlobalENBID.PLMN,
		ENBID: req.GlobalENBID.ENBID,
		Type:  ENBIDType(req.GlobalENBID.Type),
	}
	enb.ENBName = req.ENBName
	enb.SupportedTAs = make([]SupportedTA, len(req.SupportedTAs))
	for i, ta := range req.SupportedTAs {
		enb.SupportedTAs[i] = SupportedTA{
			TAC:   ta.TAC,
			PLMNs: ta.PLMNs,
		}
	}
	enb.PagingDRX = PagingDRX(req.PagingDRX)
	enb.mu.Unlock()

	// Send S1 Setup Response
	resp := &s1ap.S1SetupResponse{
		MMEName: m.cfg.Name,
		ServedGUMMEIs: []s1ap.ServedGUMMEI{
			{
				ServedPLMNs:    [][3]byte{m.plmn},
				ServedGroupIDs: []uint16{m.mmeGroupID},
				ServedMMECs:    []uint8{m.mmeCode},
			},
		},
		RelativeCapacity: m.relCapacity,
	}

	respBytes, err := s1ap.EncodeS1SetupResponse(resp)
	if err != nil {
		m.log.Errorf("Failed to encode S1SetupResponse: %v", err)
		return
	}

	if err := enb.Assoc.Write(respBytes, streamID); err != nil {
		m.log.Errorf("Failed to send S1SetupResponse: %v", err)
		return
	}

	if m.metrics != nil {
		m.metrics.S1SetupRequests.WithLabelValues("success").Inc()
	}

	m.log.Infof("S1 Setup complete for %s", enb)
}

// sendS1SetupFailure sends an S1 Setup Failure response.
func (m *MME) sendS1SetupFailure(enb *EnbContext, streamID uint16) {
	fail := &s1ap.S1SetupFailure{
		CauseGroup: s1ap.CauseMisc,
		CauseValue: 0, // control-processing-overload (placeholder)
	}
	failBytes, err := s1ap.EncodeS1SetupFailure(fail)
	if err != nil {
		m.log.Errorf("Failed to encode S1SetupFailure: %v", err)
		return
	}
	enb.Assoc.Write(failBytes, streamID)
}

// handleInitialUEMessage processes an Initial UE Message (carries NAS Attach Request).
func (m *MME) handleInitialUEMessage(ctx context.Context, enb *EnbContext, ies []s1ap.ProtocolIE, streamID uint16) {
	msg, err := s1ap.DecodeInitialUEMessage(ies)
	if err != nil {
		m.log.Errorf("Failed to decode InitialUEMessage: %v", err)
		return
	}

	m.log.Infof("Initial UE Message: eNB-UE-S1AP-ID=%d, NAS PDU=%d bytes, TAI=%x:%d",
		msg.ENBUES1APID, len(msg.NASPDU), msg.TAI.PLMN, msg.TAI.TAC)

	if m.metrics != nil {
		m.metrics.AttachRequests.WithLabelValues().Inc()
	}

	// Allocate MME-UE-S1AP-ID
	mmeUEID := m.allocateUEID()

	// Create UE context
	ue := &UEContext{
		MMEUES1APID: mmeUEID,
		ENBUES1APID: msg.ENBUES1APID,
		EMMState:    EMMRegisterInitiated,
		ECMState:    ECMConnected,
		ENB:         enb,
		TAI: TAI{
			PLMN: msg.TAI.PLMN,
			TAC:  msg.TAI.TAC,
		},
		ECGI: ECGI{
			PLMN:   msg.ECGI.PLMN,
			CellID: msg.ECGI.CellID,
		},
	}
	m.ues.Store(mmeUEID, ue)

	if m.metrics != nil {
		m.metrics.ActiveUEs.WithLabelValues().Inc()
	}

	// TODO(session-4): decode NAS PDU from msg.NASPDU, extract IMSI,
	// call HSS for auth vector, send NAS Auth Request via DownlinkNASTransport
	m.log.Infof("UE context created: MME-UE-S1AP-ID=%d (NAS handling pending)", mmeUEID)

	_ = ctx
	_ = streamID
	_ = fmt.Sprintf("placeholder") // suppress unused import
}

// handleUplinkNASTransport processes an Uplink NAS Transport message.
func (m *MME) handleUplinkNASTransport(ctx context.Context, enb *EnbContext, ies []s1ap.ProtocolIE, streamID uint16) {
	msg, err := s1ap.DecodeUplinkNASTransport(ies)
	if err != nil {
		m.log.Errorf("Failed to decode UplinkNASTransport: %v", err)
		return
	}

	m.log.Infof("Uplink NAS Transport: MME-UE-S1AP-ID=%d, eNB-UE-S1AP-ID=%d, NAS PDU=%d bytes",
		msg.MMEUES1APID, msg.ENBUES1APID, len(msg.NASPDU))

	// Look up UE context
	ueVal, ok := m.ues.Load(msg.MMEUES1APID)
	if !ok {
		m.log.Warnf("UE not found for MME-UE-S1AP-ID=%d", msg.MMEUES1APID)
		return
	}
	_ = ueVal.(*UEContext)

	// TODO(session-4): decode NAS PDU, dispatch to auth/security/attach handlers
	m.log.Debugf("Uplink NAS dispatching not yet implemented")

	_ = ctx
	_ = streamID
}
