package mme

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"

	"github.com/qcore-project/qcore/pkg/nas"
	"github.com/qcore-project/qcore/pkg/s1ap"
)

// handleS1APMessage decodes and dispatches an S1AP PDU.
func (m *MME) handleS1APMessage(ctx context.Context, enb *EnbContext, data []byte, streamID uint16) {
	pdu, err := s1ap.DecodePDU(data)
	if err != nil {
		m.log.Errorf("Failed to decode S1AP PDU from %s: %v", enb.Assoc.RemoteAddr(), err)
		return
	}

	m.log.Infof("S1AP %s: %s from %s", pdu.Type, pdu.ProcedureCode, enb.Assoc.RemoteAddr())

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

	case s1ap.ProcUEContextReleaseRequest:
		m.handleUEContextReleaseRequest(ctx, enb, ies, streamID)

	case s1ap.ProcInitialContextSetup:
		if pdu.Type == s1ap.PDUSuccessfulOutcome {
			m.handleInitialContextSetupResponse(ctx, enb, ies, streamID)
		}

	default:
		m.log.Warnf("Unhandled S1AP procedure: %s (%d)", pdu.ProcedureCode, pdu.ProcedureCode)
	}
}

// handleS1Setup processes an S1 Setup Request from an eNodeB.
func (m *MME) handleS1Setup(ctx context.Context, enb *EnbContext, ies []s1ap.ProtocolIE, streamID uint16) {
	_ = ctx

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

	// Build S1 Setup Response
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
		CauseValue: 0, // unspecified
	}
	failBytes, err := s1ap.EncodeS1SetupFailure(fail)
	if err != nil {
		m.log.Errorf("Failed to encode S1SetupFailure: %v", err)
		return
	}
	enb.Assoc.Write(failBytes, streamID)
}

// handleInitialUEMessage processes an Initial UE Message (carries NAS Attach Request).
// Flow: decode ATTACH REQUEST → get IMSI → call HSS → send AUTH REQUEST.
func (m *MME) handleInitialUEMessage(ctx context.Context, enb *EnbContext, ies []s1ap.ProtocolIE, streamID uint16) {
	_ = ctx

	msg, err := s1ap.DecodeInitialUEMessage(ies)
	if err != nil {
		m.log.Errorf("Failed to decode InitialUEMessage: %v", err)
		return
	}

	m.log.Infof("Initial UE Message: eNB-UE-S1AP-ID=%d, NAS PDU=%d bytes, TAI=%x:%d",
		msg.ENBUES1APID, len(msg.NASPDU), msg.TAI.PLMN, msg.TAI.TAC)

	// --- Parse the NAS PDU ---
	h, bodyOff, err := nas.ParseHeader(msg.NASPDU)
	if err != nil {
		m.log.Errorf("Failed to parse NAS header: %v", err)
		return
	}
	if h.MessageType == nas.MsgTypeServiceRequest {
		m.handleServiceRequest(ctx, enb, msg, streamID)
		return
	}
	if h.MessageType != nas.MsgTypeAttachRequest {
		m.log.Warnf("Unexpected NAS message in Initial UE Message: %s", h.MessageType)
		return
	}

	attachReq, err := nas.DecodeAttachRequest(msg.NASPDU[bodyOff:])
	if err != nil {
		m.log.Errorf("Failed to decode ATTACH REQUEST: %v", err)
		return
	}

	if attachReq.IMSI == "" {
		// GUTI attach: allocate UE context and send IDENTITY REQUEST for IMSI.
		m.log.Infof("ATTACH REQUEST: no IMSI (GUTI/TMSI attach), sending IDENTITY REQUEST")
		mmeUEID := m.allocateUEID()
		ue := &UEContext{
			MMEUES1APID: mmeUEID,
			ENBUES1APID: msg.ENBUES1APID,
			EMMState:    EMMRegisterInitiated,
			ECMState:    ECMConnected,
			ENB:         enb,
			NASStreamID: streamID,
			TAI:         TAI{PLMN: msg.TAI.PLMN, TAC: msg.TAI.TAC},
			ECGI:        ECGI{PLMN: msg.ECGI.PLMN, CellID: msg.ECGI.CellID},
		}
		m.ues.Store(mmeUEID, ue)
		if m.metrics != nil {
			m.metrics.ActiveUEs.WithLabelValues().Inc()
		}
		idReq := nas.EncodeIdentityRequest(1) // request IMSI
		if err := m.sendDownlinkNAS(enb, mmeUEID, msg.ENBUES1APID, idReq, streamID); err != nil {
			m.log.Errorf("Failed to send IDENTITY REQUEST: %v", err)
		}
		return
	}

	m.log.Infof("ATTACH REQUEST: IMSI=%s, attachType=%d", attachReq.IMSI, attachReq.AttachType)

	if m.metrics != nil {
		m.metrics.AttachRequests.WithLabelValues().Inc()
	}

	// --- Allocate UE context ---
	mmeUEID := m.allocateUEID()
	ue := &UEContext{
		MMEUES1APID:         mmeUEID,
		ENBUES1APID:         msg.ENBUES1APID,
		IMSI:                attachReq.IMSI,
		EMMState:            EMMRegisterInitiated,
		ECMState:            ECMConnected,
		ENB:                 enb,
		NASStreamID:         streamID,
		UENetworkCapability: attachReq.UENetworkCapability,
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

	// --- Fetch auth vector from HSS ---
	av, err := m.s6a.AuthenticationInformationRequest(attachReq.IMSI)
	if err != nil {
		m.log.Errorf("HSS auth vector request failed for IMSI=%s: %v", attachReq.IMSI, err)
		if m.metrics != nil {
			m.metrics.AttachFailures.WithLabelValues("hss_error").Inc()
		}
		m.ues.Delete(mmeUEID)
		return
	}

	// Decode hex-encoded auth vector components
	rand16, err := nas.HexToBytes(av.RAND)
	if err != nil || len(rand16) != 16 {
		m.log.Errorf("Invalid RAND from HSS: %v", err)
		return
	}
	xres, err := nas.HexToBytes(av.XRES)
	if err != nil {
		m.log.Errorf("Invalid XRES from HSS: %v", err)
		return
	}
	autn16, err := nas.HexToBytes(av.AUTN)
	if err != nil || len(autn16) != 16 {
		m.log.Errorf("Invalid AUTN from HSS: %v", err)
		return
	}
	kasme, err := nas.HexToBytes(av.KASME)
	if err != nil || len(kasme) != 32 {
		m.log.Errorf("Invalid KASME from HSS: %v", err)
		return
	}

	// Store auth vector in UE context
	ue.mu.Lock()
	ue.RAND = rand16
	ue.XRES = xres
	ue.AUTN = autn16
	ue.KASME = kasme
	ue.mu.Unlock()

	if m.metrics != nil {
		m.metrics.AuthRequests.WithLabelValues("sent").Inc()
	}

	// --- Send NAS Authentication Request ---
	authReq := &nas.AuthenticationRequest{
		NASKeySetIdentifier: 0, // KSI = 0 for first attach
	}
	copy(authReq.RAND[:], rand16)
	copy(authReq.AUTN[:], autn16)

	authReqNAS, err := nas.EncodeAuthenticationRequest(authReq)
	if err != nil {
		m.log.Errorf("Failed to encode AUTH REQUEST: %v", err)
		return
	}

	if err := m.sendDownlinkNAS(enb, mmeUEID, msg.ENBUES1APID, authReqNAS, streamID); err != nil {
		m.log.Errorf("Failed to send AUTH REQUEST to eNB: %v", err)
		return
	}

	m.log.Infof("Sent AUTH REQUEST to UE (IMSI=%s, MME-UE-S1AP-ID=%d)", attachReq.IMSI, mmeUEID)
}

// handleServiceRequest handles a NAS Service Request arriving in an InitialUEMessage.
// Called when an ECM-IDLE UE wakes up (e.g., in response to paging) and re-connects.
// The eNB should provide the UE's S-TMSI in the InitialUEMessage so we can look up
// the existing context and re-establish the S1 connection.
func (m *MME) handleServiceRequest(ctx context.Context, enb *EnbContext, msg *s1ap.InitialUEMessage, streamID uint16) {
	_ = ctx

	m.log.Infof("Service Request from eNB: eNB-UE-S1AP-ID=%d, S-TMSI present=%v",
		msg.ENBUES1APID, msg.STMSIPresent)

	var ue *UEContext

	// Try to find UE by S-TMSI
	if msg.STMSIPresent {
		if v, ok := m.tmsis.Load(msg.MTMSI); ok {
			ue = v.(*UEContext)
		}
	}

	if ue == nil {
		// UE context not found — ask UE to re-attach
		m.log.Warnf("Service Request: UE context not found (TMSI=%x), requesting re-attach", msg.MTMSI)
		// Send SERVICE REJECT: cause=0x09 (implicitly detached)
		serviceReject := []byte{
			uint8(nas.SecurityHeaderPlainNAS<<4) | uint8(nas.EPSMobilityManagement),
			0x4E, // Service Reject
			0x09, // cause: implicitly detached
		}
		// Allocate a temporary MME ID to send the rejection
		tmpUEID := m.allocateUEID()
		_ = m.sendDownlinkNAS(enb, tmpUEID, msg.ENBUES1APID, serviceReject, streamID)
		return
	}

	// Found UE — re-establish S1 context with new eNB-UE-S1AP-ID
	ue.mu.Lock()
	oldENBUEID := ue.ENBUES1APID
	ue.ENBUES1APID = msg.ENBUES1APID
	ue.ENB = enb
	ue.ECMState = ECMConnected
	ue.NASStreamID = streamID
	// Update location if TAI changed
	if msg.TAI.TAC != 0 {
		ue.TAI = TAI{PLMN: msg.TAI.PLMN, TAC: msg.TAI.TAC}
	}
	ue.mu.Unlock()

	m.log.Infof("Service Request: found UE=%d (IMSI=%s), old eNB-UE-ID=%d, new eNB-UE-ID=%d",
		ue.MMEUES1APID, ue.IMSI, oldENBUEID, msg.ENBUES1APID)

	// Re-establish radio bearer via Initial Context Setup (no NAS PDU this time)
	if err := m.sendInitialContextSetup(enb, ue, nil, streamID); err != nil {
		m.log.Errorf("Failed to send INITIAL CONTEXT SETUP for Service Request UE=%d: %v",
			ue.MMEUES1APID, err)
	}
}

// handleUplinkNASTransport dispatches uplink NAS messages during the attach flow.
func (m *MME) handleUplinkNASTransport(ctx context.Context, enb *EnbContext, ies []s1ap.ProtocolIE, streamID uint16) {
	_ = ctx
	_ = enb

	msg, err := s1ap.DecodeUplinkNASTransport(ies)
	if err != nil {
		m.log.Errorf("Failed to decode UplinkNASTransport: %v", err)
		return
	}

	// Look up UE context
	ueVal, ok := m.ues.Load(msg.MMEUES1APID)
	if !ok {
		m.log.Warnf("UE not found for MME-UE-S1AP-ID=%d", msg.MMEUES1APID)
		return
	}
	ue := ueVal.(*UEContext)

	// Parse NAS header (handles both plain and integrity-protected)
	h, bodyOff, err := nas.ParseHeader(msg.NASPDU)
	if err != nil {
		m.log.Errorf("Failed to parse NAS header from UE=%d: %v", msg.MMEUES1APID, err)
		return
	}

	m.log.Infof("Uplink NAS %s from MME-UE-S1AP-ID=%d", h.MessageType, msg.MMEUES1APID)

	// Verify uplink MAC if a security context is established and the message is integrity-protected.
	ue.mu.RLock()
	secCtx := ue.SecurityCtx
	ulCount := uint32(0)
	if secCtx != nil {
		ulCount = secCtx.ULCount
	}
	ue.mu.RUnlock()

	if secCtx != nil && (h.SecurityHeader == nas.SecurityHeaderIntegrityProtectedCiphered ||
		h.SecurityHeader == nas.SecurityHeaderIntegrityProtected) {
		ok, verr := nas.VerifyNASUplinkIntegrity(secCtx.KNASint, ulCount, msg.NASPDU)
		if verr != nil || !ok {
			m.log.Warnf("NAS integrity check failed for UE=%d (count=%d, err=%v)", msg.MMEUES1APID, ulCount, verr)
			// Drop the message — do not process it
			return
		}
		// Advance UL count after successful verification
		ue.mu.Lock()
		ue.SecurityCtx.ULCount++
		ue.mu.Unlock()
	}

	switch h.MessageType {
	case nas.MsgTypeIdentityResponse:
		m.handleIdentityResponse(ue, msg.NASPDU[bodyOff:], streamID)
	case nas.MsgTypeAuthenticationResponse:
		m.handleAuthResponse(ue, msg.NASPDU[bodyOff:], streamID)
	case nas.MsgTypeAuthenticationFailure:
		m.log.Warnf("UE=%d sent AUTH FAILURE — auth rejected", msg.MMEUES1APID)
		if m.metrics != nil {
			m.metrics.AttachFailures.WithLabelValues("auth_failure").Inc()
		}
		m.cleanupUE(ue)
	case nas.MsgTypeSecurityModeComplete:
		m.handleSecurityModeComplete(ue, msg.NASPDU[bodyOff:], streamID)
	case nas.MsgTypeAttachComplete:
		m.handleAttachComplete(ue)
	case nas.MsgTypeDetachRequest:
		m.handleDetachRequest(ue, msg.NASPDU[bodyOff:], streamID)
	default:
		m.log.Warnf("Unhandled uplink NAS message: %s (UE=%d)", h.MessageType, msg.MMEUES1APID)
	}
}

// handleAuthResponse verifies the UE's RES and sends a Security Mode Command.
func (m *MME) handleAuthResponse(ue *UEContext, body []byte, streamID uint16) {
	resp, err := nas.DecodeAuthenticationResponse(body)
	if err != nil {
		m.log.Errorf("Failed to decode AUTH RESPONSE for UE=%d: %v", ue.MMEUES1APID, err)
		return
	}

	ue.mu.RLock()
	xres := ue.XRES
	kasme := ue.KASME
	ueCap := ue.UENetworkCapability
	ue.mu.RUnlock()

	if !nas.VerifyAuthResponse(resp.RES, xres) {
		m.log.Warnf("AUTH FAILURE for UE=%d: RES mismatch", ue.MMEUES1APID)
		if m.metrics != nil {
			m.metrics.AttachFailures.WithLabelValues("res_mismatch").Inc()
		}
		m.ues.Delete(ue.MMEUES1APID)
		return
	}

	m.log.Infof("Auth verified for UE=%d (IMSI=%s)", ue.MMEUES1APID, ue.IMSI)
	if m.metrics != nil {
		m.metrics.AuthRequests.WithLabelValues("success").Inc()
	}

	// Derive NAS keys: EEA0 (null cipher, alg_id=0) and EIA2 (AES-CMAC, alg_id=2)
	kNASenc, err := nas.DeriveKNASenc(kasme, 0) // EEA0
	if err != nil {
		m.log.Errorf("Failed to derive KNASenc: %v", err)
		return
	}
	kNASint, err := nas.DeriveKNASint(kasme, 2) // EIA2
	if err != nil {
		m.log.Errorf("Failed to derive KNASint: %v", err)
		return
	}
	keNB, err := nas.DeriveKeNB(kasme, 0) // UL NAS COUNT = 0 for first attach
	if err != nil {
		m.log.Errorf("Failed to derive KeNB: %v", err)
		return
	}

	ue.mu.Lock()
	ue.SecurityCtx = &SecurityContext{
		KASME:   kasme,
		KNASenc: kNASenc,
		KNASint: kNASint,
		KeNB:    keNB,
		NASAlg: NASAlgorithm{
			Ciphering: 0, // EEA0
			Integrity: 2, // EIA2
		},
		DLCount: 0,
		ULCount: 1, // UE has sent one uplink NAS (AUTH RESPONSE)
	}
	ue.mu.Unlock()

	// Build Security Mode Command (plain NAS; will be wrapped with integrity)
	//   SelectedNASSecAlg: EEA0 (0x00) in high nibble, EIA2 (0x02) in low nibble → 0x02
	secModeCmd := &nas.SecurityModeCommand{
		SelectedNASSecAlg:   0x02, // EEA0 | EIA2
		NASKeySetIdentifier: 0,    // KSI = 0
		ReplayedUESecCap:    ueCap,
	}

	plainSecModeCmd, err := nas.EncodeSecurityModeCommand(secModeCmd)
	if err != nil {
		m.log.Errorf("Failed to encode SECURITY MODE COMMAND: %v", err)
		return
	}

	// Integrity-protect with the new NAS security context (header type 3)
	wrappedCmd, err := nas.WrapNASWithIntegrity(kNASint, 0, nas.SecurityHeaderIntegrityProtectedNewCtx, plainSecModeCmd)
	if err != nil {
		m.log.Errorf("Failed to wrap SECURITY MODE COMMAND: %v", err)
		return
	}

	ue.mu.Lock()
	ue.NASdlCount = 1 // first DL message sent
	ue.mu.Unlock()

	enb := ue.ENB
	if err := m.sendDownlinkNAS(enb, ue.MMEUES1APID, ue.ENBUES1APID, wrappedCmd, streamID); err != nil {
		m.log.Errorf("Failed to send SECURITY MODE COMMAND: %v", err)
		return
	}

	m.log.Infof("Sent SECURITY MODE COMMAND to UE=%d", ue.MMEUES1APID)
}

// handleSecurityModeComplete completes security establishment and sends ATTACH ACCEPT.
func (m *MME) handleSecurityModeComplete(ue *UEContext, body []byte, streamID uint16) {
	_, err := nas.DecodeSecurityModeComplete(body)
	if err != nil {
		m.log.Errorf("Failed to decode SECURITY MODE COMPLETE for UE=%d: %v", ue.MMEUES1APID, err)
		return
	}

	m.log.Infof("Security Mode Complete from UE=%d (IMSI=%s)", ue.MMEUES1APID, ue.IMSI)

	// Allocate PDN address. Prefer the real SPGW via S11 when available; fall
	// back to the internal placeholder allocator so control-plane-only runs
	// (no SPGW) still complete attach.
	var (
		pdnAddr string
		sgwTEID uint32
		sgwAddr string
	)
	if m.s11 != nil && m.s11.Enabled() {
		resp, err := m.s11.CreateSession(&S11CreateSessionRequest{
			IMSI: ue.IMSI,
			APN:  "internet",
			EBI:  5,
			PLMN: m.cfg.PLMN,
		})
		if err != nil {
			m.log.Warnf("S11 CreateSession failed for IMSI=%s: %v (falling back to local IP allocation)", ue.IMSI, err)
		} else {
			pdnAddr = resp.UEIP
			sgwTEID = resp.SGWTEID
			sgwAddr = resp.SGWAddr
			m.log.Infof("S11 Create Session OK: IMSI=%s UE-IP=%s SGW-TEID=0x%x SGW=%s",
				ue.IMSI, pdnAddr, sgwTEID, sgwAddr)
		}
	}
	if pdnAddr == "" {
		pdnAddr = m.allocatePDNAddress()
		// Placeholder transport-layer info so the E-RAB IE encodes correctly.
		sgwTEID = 1
		sgwAddr = "127.0.0.1"
	}

	tmsi := m.allocateTMSI()
	ue.mu.Lock()
	ue.PDNAddr = pdnAddr
	ue.SGWTEID = sgwTEID
	ue.SGWAddr = sgwAddr
	ue.TMSI = tmsi
	ue.EMMState = EMMRegistered
	kNASint := ue.SecurityCtx.KNASint
	dlCount := ue.NASdlCount
	ue.NASdlCount++
	ue.mu.Unlock()

	// Register in TMSI index for Service Request lookup
	m.tmsis.Store(tmsi, ue)

	// Build ATTACH ACCEPT with embedded default bearer activation and GUTI
	pdn := net.ParseIP(pdnAddr)
	if pdn == nil {
		m.log.Errorf("Invalid PDN address allocated: %s", pdnAddr)
		return
	}

	attachAcceptNAS, err := nas.EncodeAttachAcceptFull(nas.AttachAcceptParams{
		PLMN:        m.plmn,
		TAC:         m.cfg.TAC,
		BearerID:    5,
		APN:         "internet",
		PDN:         pdn,
		GUTIPresent: true,
		MMEGroupID:  m.mmeGroupID,
		MMECode:     m.mmeCode,
		TMSI:        tmsi,
	})
	if err != nil {
		m.log.Errorf("Failed to encode ATTACH ACCEPT for UE=%d: %v", ue.MMEUES1APID, err)
		return
	}

	// Integrity-protect with established context (header type 2)
	wrappedAccept, err := nas.WrapNASWithIntegrity(kNASint, dlCount, nas.SecurityHeaderIntegrityProtectedCiphered, attachAcceptNAS)
	if err != nil {
		m.log.Errorf("Failed to wrap ATTACH ACCEPT: %v", err)
		return
	}

	// Send INITIAL CONTEXT SETUP REQUEST (carries the ATTACH ACCEPT as embedded NAS PDU).
	// This is the correct S1AP procedure: the eNB delivers the NAS to the UE via RRC and
	// simultaneously sets up the radio bearer and GTP tunnel.
	enb := ue.ENB
	if err := m.sendInitialContextSetup(enb, ue, wrappedAccept, streamID); err != nil {
		m.log.Errorf("Failed to send INITIAL CONTEXT SETUP to eNB for UE=%d: %v", ue.MMEUES1APID, err)
		return
	}

	m.log.Infof("Sent INITIAL CONTEXT SETUP REQUEST to eNB for UE=%d (IMSI=%s, IP=%s)", ue.MMEUES1APID, ue.IMSI, pdnAddr)
	if m.metrics != nil {
		m.metrics.AttachSuccess.WithLabelValues().Inc()
	}
}

// handleAttachComplete acknowledges the UE's ATTACH COMPLETE and sends EMM INFORMATION.
func (m *MME) handleAttachComplete(ue *UEContext) {
	m.log.Infof("ATTACH COMPLETE from UE=%d (IMSI=%s) — UE is now registered", ue.MMEUES1APID, ue.IMSI)

	// Send EMM INFORMATION (optional) so the UE displays the network name and syncs time.
	emmInfo := nas.EncodeEMMInformation(m.cfg.Name)

	ue.mu.Lock()
	kNASint := []byte(nil)
	dlCount := uint32(0)
	if ue.SecurityCtx != nil {
		kNASint = ue.SecurityCtx.KNASint
		dlCount = ue.NASdlCount
		ue.NASdlCount++
	}
	ue.mu.Unlock()

	var wrapped []byte
	var err error
	if kNASint != nil {
		wrapped, err = nas.WrapNASWithIntegrity(kNASint, dlCount, nas.SecurityHeaderIntegrityProtectedCiphered, emmInfo)
		if err != nil {
			m.log.Warnf("Failed to wrap EMM INFORMATION for UE=%d: %v", ue.MMEUES1APID, err)
			return
		}
	} else {
		wrapped = emmInfo
	}

	enb := ue.ENB
	if err := m.sendDownlinkNAS(enb, ue.MMEUES1APID, ue.ENBUES1APID, wrapped, ue.NASStreamID); err != nil {
		m.log.Warnf("Failed to send EMM INFORMATION to UE=%d: %v", ue.MMEUES1APID, err)
	} else {
		m.log.Infof("Sent EMM INFORMATION to UE=%d (network name: %q)", ue.MMEUES1APID, m.cfg.Name)
	}
}

// handleIdentityResponse processes a NAS Identity Response (sent in response to our Identity Request).
// It extracts the IMSI, updates the UE context, fetches an auth vector, and sends AUTH REQUEST.
func (m *MME) handleIdentityResponse(ue *UEContext, body []byte, streamID uint16) {
	resp, err := nas.DecodeIdentityResponse(body)
	if err != nil {
		m.log.Errorf("Failed to decode IDENTITY RESPONSE for UE=%d: %v", ue.MMEUES1APID, err)
		m.cleanupUE(ue)
		return
	}
	if resp.IdentityType != 1 || resp.IMSI == "" {
		m.log.Warnf("IDENTITY RESPONSE from UE=%d did not contain IMSI (type=%d)", ue.MMEUES1APID, resp.IdentityType)
		m.cleanupUE(ue)
		return
	}

	ue.mu.Lock()
	ue.IMSI = resp.IMSI
	ue.mu.Unlock()

	m.log.Infof("IDENTITY RESPONSE: UE=%d IMSI=%s", ue.MMEUES1APID, resp.IMSI)
	if m.metrics != nil {
		m.metrics.AttachRequests.WithLabelValues().Inc()
	}

	// Fetch auth vector from HSS
	av, err := m.s6a.AuthenticationInformationRequest(resp.IMSI)
	if err != nil {
		m.log.Errorf("HSS auth vector request failed for IMSI=%s: %v", resp.IMSI, err)
		if m.metrics != nil {
			m.metrics.AttachFailures.WithLabelValues("hss_error").Inc()
		}
		m.cleanupUE(ue)
		return
	}

	rand16, err := nas.HexToBytes(av.RAND)
	if err != nil || len(rand16) != 16 {
		m.log.Errorf("Invalid RAND from HSS: %v", err)
		m.cleanupUE(ue)
		return
	}
	xres, err := nas.HexToBytes(av.XRES)
	if err != nil {
		m.log.Errorf("Invalid XRES from HSS: %v", err)
		m.cleanupUE(ue)
		return
	}
	autn16, err := nas.HexToBytes(av.AUTN)
	if err != nil || len(autn16) != 16 {
		m.log.Errorf("Invalid AUTN from HSS: %v", err)
		m.cleanupUE(ue)
		return
	}
	kasme, err := nas.HexToBytes(av.KASME)
	if err != nil || len(kasme) != 32 {
		m.log.Errorf("Invalid KASME from HSS: %v", err)
		m.cleanupUE(ue)
		return
	}

	ue.mu.Lock()
	ue.RAND = rand16
	ue.XRES = xres
	ue.AUTN = autn16
	ue.KASME = kasme
	ue.UENetworkCapability = nil // not available for GUTI attach without ATTACH REQUEST UE cap
	ue.mu.Unlock()

	if m.metrics != nil {
		m.metrics.AuthRequests.WithLabelValues("sent").Inc()
	}

	authReq := &nas.AuthenticationRequest{NASKeySetIdentifier: 0}
	copy(authReq.RAND[:], rand16)
	copy(authReq.AUTN[:], autn16)

	authReqNAS, err := nas.EncodeAuthenticationRequest(authReq)
	if err != nil {
		m.log.Errorf("Failed to encode AUTH REQUEST for UE=%d: %v", ue.MMEUES1APID, err)
		return
	}

	enb := ue.ENB
	if err := m.sendDownlinkNAS(enb, ue.MMEUES1APID, ue.ENBUES1APID, authReqNAS, streamID); err != nil {
		m.log.Errorf("Failed to send AUTH REQUEST to eNB for UE=%d: %v", ue.MMEUES1APID, err)
		return
	}

	m.log.Infof("Sent AUTH REQUEST to UE=%d (IMSI=%s)", ue.MMEUES1APID, resp.IMSI)
}

// handleUEContextReleaseRequest handles an eNB-initiated UE Context Release Request.
// The eNB sends this when the RRC connection is released. We respond with a UE Context
// Release Command and clean up the UE state.
func (m *MME) handleUEContextReleaseRequest(ctx context.Context, enb *EnbContext, ies []s1ap.ProtocolIE, streamID uint16) {
	_ = ctx

	req, err := s1ap.DecodeUEContextReleaseRequest(ies)
	if err != nil {
		m.log.Errorf("Failed to decode UEContextReleaseRequest: %v", err)
		return
	}

	m.log.Infof("UE Context Release Request: MME-UE-S1AP-ID=%d, cause=%d/%d",
		req.MMEUES1APID, req.CauseGroup, req.CauseValue)

	// Send UE Context Release Command back to eNB
	cmd := &s1ap.UEContextReleaseCommand{
		MMEUES1APID: req.MMEUES1APID,
		ENBUES1APID: req.ENBUES1APID,
		CauseGroup:  req.CauseGroup,
		CauseValue:  req.CauseValue,
	}
	encoded, err := s1ap.EncodeUEContextReleaseCommand(cmd)
	if err != nil {
		m.log.Errorf("Failed to encode UEContextReleaseCommand: %v", err)
		return
	}
	if err := enb.Assoc.Write(encoded, streamID); err != nil {
		m.log.Errorf("Failed to send UEContextReleaseCommand: %v", err)
	}

	// Clean up UE context
	ueVal, ok := m.ues.Load(req.MMEUES1APID)
	if ok {
		m.cleanupUE(ueVal.(*UEContext))
	}
}

// handleDetachRequest processes a UE-initiated Detach Request.
// If the UE is not switching off, we send a Detach Accept. Either way, we release
// the UE context by sending an UE Context Release Command to the eNB.
func (m *MME) handleDetachRequest(ue *UEContext, body []byte, streamID uint16) {
	req, err := nas.DecodeDetachRequest(body)
	if err != nil {
		m.log.Errorf("Failed to decode Detach Request for UE=%d: %v", ue.MMEUES1APID, err)
		return
	}

	m.log.Infof("Detach Request from UE=%d (IMSI=%s, type=%d, switchOff=%v)",
		ue.MMEUES1APID, ue.IMSI, req.DetachType, req.SwitchOff)

	// Send Detach Accept unless UE is switching off (TS 24.301 §5.5.2.2.2)
	if !req.SwitchOff {
		detachAccept := nas.EncodeDetachAccept()

		ue.mu.Lock()
		var wrapped []byte
		if ue.SecurityCtx != nil {
			dlCount := ue.NASdlCount
			ue.NASdlCount++
			w, werr := nas.WrapNASWithIntegrity(ue.SecurityCtx.KNASint, dlCount,
				nas.SecurityHeaderIntegrityProtectedCiphered, detachAccept)
			if werr != nil {
				m.log.Warnf("Failed to wrap Detach Accept for UE=%d: %v", ue.MMEUES1APID, werr)
				wrapped = detachAccept
			} else {
				wrapped = w
			}
		} else {
			wrapped = detachAccept
		}
		ue.mu.Unlock()

		enb := ue.ENB
		if err := m.sendDownlinkNAS(enb, ue.MMEUES1APID, ue.ENBUES1APID, wrapped, streamID); err != nil {
			m.log.Warnf("Failed to send Detach Accept to UE=%d: %v", ue.MMEUES1APID, err)
		}
	}

	// Release the S1 context: send UE Context Release Command to eNB, then clean up.
	cmd := &s1ap.UEContextReleaseCommand{
		MMEUES1APID: ue.MMEUES1APID,
		ENBUES1APID: ue.ENBUES1APID,
		CauseGroup:  s1ap.CauseNAS,
		CauseValue:  2, // nas-detach
	}
	encoded, err := s1ap.EncodeUEContextReleaseCommand(cmd)
	if err != nil {
		m.log.Errorf("Failed to encode UEContextReleaseCommand for detach UE=%d: %v", ue.MMEUES1APID, err)
	} else if err := ue.ENB.Assoc.Write(encoded, streamID); err != nil {
		m.log.Warnf("Failed to send UEContextReleaseCommand after detach for UE=%d: %v", ue.MMEUES1APID, err)
	}

	m.cleanupUE(ue)
}

// cleanupUE removes a UE context from the main map, the TMSI index, and updates metrics.
func (m *MME) cleanupUE(ue *UEContext) {
	m.ues.Delete(ue.MMEUES1APID)
	ue.mu.RLock()
	tmsi := ue.TMSI
	imsi := ue.IMSI
	hadSession := ue.PDNAddr != ""
	ue.mu.RUnlock()
	if tmsi != 0 {
		m.tmsis.Delete(tmsi)
	}
	// Best-effort release of the SPGW session. Run async to keep cleanup fast;
	// any failure is logged but not surfaced.
	if hadSession && m.s11 != nil && m.s11.Enabled() && imsi != "" {
		go func(imsi string) {
			if err := m.s11.DeleteSession(imsi); err != nil {
				m.log.Debugf("S11 DeleteSession for IMSI=%s: %v", imsi, err)
			}
		}(imsi)
	}
	if m.metrics != nil {
		m.metrics.ActiveUEs.WithLabelValues().Dec()
	}
	m.log.Infof("Cleaned up UE context for MME-UE-S1AP-ID=%d (IMSI=%s)", ue.MMEUES1APID, ue.IMSI)
}

// sendInitialContextSetup sends an INITIAL CONTEXT SETUP REQUEST to the eNB.
// The NAS ATTACH ACCEPT is embedded in the first E-RAB item so the eNB can deliver
// it to the UE during RRC reconfiguration (radio bearer setup).
// A dummy GTP TEID and loopback SGW address are used since we have no real S-GW yet.
func (m *MME) sendInitialContextSetup(enb *EnbContext, ue *UEContext, attachAcceptNAS []byte, streamID uint16) error {
	ue.mu.RLock()
	secCtx := ue.SecurityCtx
	ue.mu.RUnlock()

	// Build UE security capability bitmaps from what the UE reported in Attach Request.
	// Format (TS 36.413 §9.2.1.35): 16-bit BIT STRING, MSB = EEA0/EIA0, etc.
	// If we don't have the UE capabilities, advertise EEA0+EIA2 (our selected algorithms).
	var encAlgs, intAlgs [2]byte
	ue.mu.RLock()
	ueCap := ue.UENetworkCapability
	ue.mu.RUnlock()
	if len(ueCap) >= 2 {
		// NAS UENetworkCapability byte 0 = EPS encryption support
		// NAS UENetworkCapability byte 1 = EPS integrity support
		// Both are in the same bit ordering as the S1AP 16-bit bitmaps (MSB = alg 0)
		encAlgs[0] = ueCap[0]
		intAlgs[0] = ueCap[1]
	} else {
		// Fallback: advertise EEA0 (null cipher) + EIA2 (AES-CMAC)
		encAlgs[0] = 0x80 // EEA0 bit set
		intAlgs[0] = 0x20 // EIA2 bit set (bit 3 from MSB)
	}

	if secCtx == nil {
		return fmt.Errorf("sendInitialContextSetup: no security context for UE=%d", ue.MMEUES1APID)
	}

	var keNB [32]byte
	copy(keNB[:], secCtx.KeNB)

	// Pull the SGW S1-U endpoint set at attach time (via S11 if SPGW is up,
	// else the local placeholder).
	ue.mu.RLock()
	sgwAddr := ue.SGWAddr
	sgwTEID := ue.SGWTEID
	ue.mu.RUnlock()
	if sgwAddr == "" {
		sgwAddr = "127.0.0.1"
	}
	if sgwTEID == 0 {
		sgwTEID = 1
	}
	var teidBytes [4]byte
	binary.BigEndian.PutUint32(teidBytes[:], sgwTEID)

	req := &s1ap.InitialContextSetupRequest{
		MMEUES1APID:       ue.MMEUES1APID,
		ENBUES1APID:       ue.ENBUES1APID,
		UEAggMaxBitRateDL: 50000000, // 50 Mbps — placeholder; real value from HSS subscription
		UEAggMaxBitRateUL: 25000000, // 25 Mbps
		ERABs: []s1ap.ERABToSetup{
			{
				ERABID:             5, // default bearer EPS bearer ID
				QCI:                9, // internet non-GBR
				ARPLevel:           8, // mid-priority
				TransportLayerAddr: net.ParseIP(sgwAddr),
				GTPTEID:            teidBytes,
				NASPDU:             attachAcceptNAS,
			},
		},
		UESecEncAlgs: encAlgs,
		UESecIntAlgs: intAlgs,
		SecurityKey:  keNB,
	}

	encoded, err := s1ap.EncodeInitialContextSetupRequest(req)
	if err != nil {
		return fmt.Errorf("encoding INITIAL CONTEXT SETUP REQUEST: %w", err)
	}
	if err := enb.Assoc.Write(encoded, streamID); err != nil {
		return fmt.Errorf("writing INITIAL CONTEXT SETUP REQUEST: %w", err)
	}
	return nil
}

// handleInitialContextSetupResponse processes the eNB's confirmation that the
// radio bearer is set up. If the response includes E-RAB setup results with
// the eNB-allocated S1-U TEID, we fire an S11 Modify Bearer toward the SPGW
// so downlink packets can flow.
func (m *MME) handleInitialContextSetupResponse(ctx context.Context, enb *EnbContext, ies []s1ap.ProtocolIE, streamID uint16) {
	_ = ctx
	_ = streamID

	resp, err := s1ap.DecodeInitialContextSetupResponse(ies)
	if err != nil {
		m.log.Errorf("Failed to decode INITIAL CONTEXT SETUP RESPONSE: %v", err)
		return
	}
	m.log.Infof("INITIAL CONTEXT SETUP RESPONSE: MME-UE-S1AP-ID=%d — radio bearer established (%d E-RAB(s))",
		resp.MMEUES1APID, len(resp.ERABs))

	ueVal, ok := m.ues.Load(resp.MMEUES1APID)
	if !ok {
		return
	}
	ue := ueVal.(*UEContext)

	// Learn the eNB S1-U endpoint. Prefer the transport-layer address carried
	// in the E-RABSetupItem; fall back to the eNB's S1AP peer address if the
	// encoder didn't include it.
	var enbAddr string
	var enbTEID uint32
	if len(resp.ERABs) > 0 {
		r := resp.ERABs[0]
		enbTEID = binary.BigEndian.Uint32(r.GTPTEID[:])
		if r.TransportLayerAddr != nil {
			enbAddr = r.TransportLayerAddr.String()
		}
	}
	if enbAddr == "" && enb != nil && enb.Assoc != nil {
		// Strip port: the eNB's S1AP source port is not the GTP-U port.
		if host, _, splitErr := net.SplitHostPort(enb.Assoc.RemoteAddr().String()); splitErr == nil {
			enbAddr = host
		}
	}

	ue.mu.Lock()
	ue.ENBTEID = enbTEID
	ue.ENBAddr = enbAddr
	imsi := ue.IMSI
	ue.mu.Unlock()

	if m.s11 != nil && m.s11.Enabled() && imsi != "" && enbAddr != "" && enbTEID != 0 {
		if err := m.s11.ModifyBearer(imsi, &S11ModifyBearerRequest{
			ENBTEID: enbTEID,
			ENBAddr: enbAddr,
		}); err != nil {
			m.log.Warnf("S11 ModifyBearer failed for IMSI=%s: %v", imsi, err)
			return
		}
		m.log.Infof("S11 ModifyBearer OK: IMSI=%s eNB=%s eNB-TEID=0x%x", imsi, enbAddr, enbTEID)
	}
}

// sendDownlinkNAS wraps a NAS PDU in a DownlinkNASTransport S1AP message and sends it.
func (m *MME) sendDownlinkNAS(enb *EnbContext, mmeUEID, enbUEID uint32, nasPDU []byte, streamID uint16) error {
	s1msg := &s1ap.DownlinkNASTransport{
		MMEUES1APID: mmeUEID,
		ENBUES1APID: enbUEID,
		NASPDU:      nasPDU,
	}
	encoded, err := s1ap.EncodeDownlinkNASTransport(s1msg)
	if err != nil {
		return fmt.Errorf("encoding DownlinkNASTransport: %w", err)
	}
	if err := enb.Assoc.Write(encoded, streamID); err != nil {
		return fmt.Errorf("writing to eNB: %w", err)
	}
	return nil
}
