package amf

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/qcore-project/qcore/pkg/diag"
	"github.com/qcore-project/qcore/pkg/events"
	"github.com/qcore-project/qcore/pkg/ident"
	"github.com/qcore-project/qcore/pkg/logger"
	"github.com/qcore-project/qcore/pkg/ngap"
	"github.com/qcore-project/qcore/pkg/sctp"
)

// gNBSession handles all NGAP messages for one gNB connection.
// Each gNB gets its own goroutine; UE contexts are shared with the Service.
type gNBSession struct {
	amf        *Service
	conn       sctp.Association
	log        logger.Logger
	gnbAssocID string // stable ID for event correlation, set at accept time

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

func (s *gNBSession) traceNGAP(direction string, raw []byte) {
	if !s.amf.cfg.TraceNGAPHex {
		return
	}
	fields := map[string]interface{}{
		"direction": direction,
		"bytes":     len(raw),
		"hex":       hex.EncodeToString(raw),
	}
	if pdu, err := ngap.DecodePDU(raw); err == nil {
		fields["type"] = pdu.Type
		fields["procedure"] = pdu.ProcedureCode.String()
		fields["procedure_code"] = pdu.ProcedureCode
	}
	s.log.WithFields(fields).Info("amf: NGAP raw PDU")
}

// dispatch decodes one NGAP PDU and routes it.
func (s *gNBSession) dispatch(ctx context.Context, raw []byte) error {
	pdu, err := ngap.DecodePDU(raw)
	if err != nil {
		return fmt.Errorf("decode PDU: %w", err)
	}
	s.traceNGAP("rx", raw)
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
// It emits one structured event per lifecycle step so the hero screen
// (docs/ui-ux-design.md §2) can show exactly what happened and why.
func (s *gNBSession) handleNGSetupRequest(ies []ngap.ProtocolIE) error {
	// ── decode ────────────────────────────────────────────────────────────────
	req, err := ngap.DecodeNGSetupRequest(ies)
	if err != nil {
		result := diag.DiagnoseNGSetupMalformed(err)
		s.amf.emitter.Emit(events.Event{
			NF:       "amf",
			Category: events.ErrorEvent,
			Severity: events.SeverityError,
			Protocol: "ngap",
			Message:  "NGSetupRequest malformed — could not decode",
			Payload: events.NGSetupPayload{
				GNBAssocID:     s.gnbAssocID,
				Success:        false,
				FailureCause:   result.Cause,
				FailureExplain: result.Explanation,
				FixGNBSide:     result.FixGNBSide,
				FixQCoreSide:   result.FixQCoreSide,
			},
		})
		return fmt.Errorf("NGSetupRequest: %w", err)
	}

	s.globalGNBID = &req.GlobalRANNodeID
	s.gnbName = req.RANNodeName

	// ── decode PLMN with the canonical ident codec (A1) ─────────────────────
	gnbMCC, gnbMNC := ident.DecodePLMN([3]byte(req.GlobalRANNodeID.PLMN))

	// ── collect offered TACs and S-NSSAIs ────────────────────────────────────
	var offeredTACs []string
	var offeredSNSSAIs []events.SNSSAIDesc
	for _, ta := range req.SupportedTAList {
		offeredTACs = append(offeredTACs, strings.ToUpper(hex.EncodeToString(ta.TAC[:])))
		for _, bp := range ta.BroadcastPLMN {
			for _, s := range bp.SNSSAI {
				sd := ""
				if len(s.SD) > 0 {
					sd = hex.EncodeToString(s.SD)
				}
				offeredSNSSAIs = appendSNSSAIIfAbsent(offeredSNSSAIs, events.SNSSAIDesc{SST: s.SST, SD: sd})
			}
		}
	}

	// ── emit "request received" event with all decoded fields ─────────────────
	s.amf.emitter.Emit(events.Event{
		NF:       "amf",
		Category: events.SignalingRx,
		Severity: events.SeverityInfo,
		Protocol: "ngap",
		Message:  "NGSetupRequest received",
		Payload: events.NGSetupReceivedPayload{
			GNBAssocID:     s.gnbAssocID,
			GNBName:        req.RANNodeName,
			GNBID:          uint64(req.GlobalRANNodeID.GNBID),
			GNBMCC:         gnbMCC,
			GNBMNC:         gnbMNC,
			OfferedTACs:    offeredTACs,
			OfferedSNSSAIs: offeredSNSSAIs,
		},
	})

	s.log.WithField("gnb_name", req.RANNodeName).
		WithField("plmn", gnbMCC+"/"+gnbMNC).
		WithField("assoc_id", s.gnbAssocID).
		Info("amf: NGSetup from gNB")

	// ── run deterministic diagnostic rules ───────────────────────────────────
	diagCfg := s.amf.diagConfig()
	result := diag.DiagnoseNGSetup(req, diagCfg)

	if !result.OK {
		// Emit structured failure event with cause + fix for both sides.
		firstTAC := ""
		if len(offeredTACs) > 0 {
			firstTAC = offeredTACs[0]
		}
		s.amf.emitter.Emit(events.Event{
			NF:       "amf",
			Category: events.ErrorEvent,
			Severity: events.SeverityError,
			Protocol: "ngap",
			Message:  fmt.Sprintf("NGSetup rejected: %s", result.Cause),
			Payload: events.NGSetupPayload{
				GNBAssocID:     s.gnbAssocID,
				GNBName:        req.RANNodeName,
				GNBID:          uint64(req.GlobalRANNodeID.GNBID),
				PLMN:           gnbMCC + "/" + gnbMNC,
				TAC:            firstTAC,
				Success:        false,
				FailureCause:   result.Cause,
				FailureExplain: result.Explanation,
				FixGNBSide:     result.FixGNBSide,
				FixQCoreSide:   result.FixQCoreSide,
			},
		})

		s.log.WithField("cause", result.Cause).
			WithField("assoc_id", s.gnbAssocID).
			Warn("amf: NGSetup rejected")

		// Send NGAP NGSetupFailure with cause = misc / unspecified.
		fail, encErr := ngap.EncodeNGSetupFailure(&ngap.NGSetupFailure{
			CauseGroup: ngap.CauseMisc,
			CauseValue: 0, // unspecified
		})
		if encErr == nil {
			_ = s.send(fail)
		}
		return nil // error already communicated to gNB via NGAP; non-fatal for session
	}

	// ── success path ──────────────────────────────────────────────────────────
	firstTAC := ""
	if len(offeredTACs) > 0 {
		firstTAC = offeredTACs[0]
	}
	s.amf.emitter.Emit(events.Event{
		NF:       "amf",
		Category: events.SignalingTx,
		Severity: events.SeverityInfo,
		Protocol: "ngap",
		Message:  "NGSetupResponse sent — gNB accepted",
		Payload: events.NGSetupPayload{
			GNBAssocID: s.gnbAssocID,
			GNBName:    req.RANNodeName,
			GNBID:      uint64(req.GlobalRANNodeID.GNBID),
			PLMN:       gnbMCC + "/" + gnbMNC,
			TAC:        firstTAC,
			Success:    true,
		},
	})

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

// diagConfig converts the AMF's Config into the diag.AMFConfig the rule engine needs.
func (s *Service) diagConfig() diag.AMFConfig {
	cfg := diag.AMFConfig{}
	for _, item := range s.cfg.PLMNSupportList {
		cfg.SupportedPLMNs = append(cfg.SupportedPLMNs, item.PLMN)
		cfg.SupportedSNSSAIs = append(cfg.SupportedSNSSAIs, item.SNSSAIs...)
	}
	// TAC: the AMF config doesn't have a formal TAC list — deduplicate from GUAMI isn't
	// relevant. Leave SupportedTACs empty (= accept any TAC). Callers can populate it
	// by using a test-only DiagConfig or a future AMFConfig.SupportedTACs field.
	return cfg
}

func appendSNSSAIIfAbsent(s []events.SNSSAIDesc, v events.SNSSAIDesc) []events.SNSSAIDesc {
	for _, x := range s {
		if x.SST == v.SST && x.SD == v.SD {
			return s
		}
	}
	return append(s, v)
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
	ue.JourneyID = events.MintJourneyID()
	s.amf.addUE(ue)

	s.log.WithField("amf_ue_ngap_id", amfID).WithField("ran_ue_ngap_id", msg.RANUENGAPID).
		WithField("journey_id", ue.JourneyID).
		Info("amf: new UE attached")

	s.amf.emitter.Emit(events.Event{
		JourneyID: ue.JourneyID,
		NF:        "amf",
		Category:  events.SignalingRx,
		Severity:  events.SeverityInfo,
		Protocol:  "ngap",
		Message:   "InitialUEMessage received",
	})

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
		AMFUENGAPID:                 ue.AMFUENGAPID,
		RANUENGAPID:                 ue.RANUENGAPID,
		UEAggregateMaximumBitRateDL: 1000000000,
		UEAggregateMaximumBitRateUL: 1000000000,
		GUAMI:                       s.amf.cfg.GUAMI,
		AllowedNSSAI:                ue.NSSAI,
		NASPDU:                      nasPDU,
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
	s.traceNGAP("tx", raw)
	return s.conn.Write(raw, 0)
}
