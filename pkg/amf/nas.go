package amf

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/qcore-project/qcore/pkg/ausf"
	"github.com/qcore-project/qcore/pkg/diag"
	"github.com/qcore-project/qcore/pkg/events"
	"github.com/qcore-project/qcore/pkg/ident"
	"github.com/qcore-project/qcore/pkg/nas5g"
	"github.com/qcore-project/qcore/pkg/ngap"
	"github.com/qcore-project/qcore/pkg/udm"
)

// handleNASPDU is the top-level 5G-NAS dispatcher for one UE message.
// It strips the security header if present, decodes the inner message, and
// routes to the appropriate state handler.
func (s *Service) handleNASPDU(ctx context.Context, ue *UEContext, raw []byte) error {
	ue.mu.Lock()
	defer ue.mu.Unlock()

	if len(raw) < 3 {
		return fmt.Errorf("amf: NAS PDU too short: %d bytes", len(raw))
	}

	// Detect security-protected NAS (EPD=0x7E, SecHdr != 0x00)
	plainRaw := raw
	if raw[0] == 0x7E && raw[1] != 0x00 {
		// Integrity-protected: strip 7-byte security header, inner NAS starts at byte 7
		if len(raw) < 7 {
			return fmt.Errorf("amf: security-protected NAS too short")
		}
		// Verify MAC if we have keys (after SMC)
		if len(ue.KNASint) == 16 {
			inner, err := VerifyNAS5GUplink(ue.KNASint, ue.ULCount, raw)
			if err != nil {
				s.log.WithError(err).Warn("amf: NAS MAC verification failed — accepting anyway (v0.6)")
			} else {
				plainRaw = inner
				ue.ULCount++
			}
		} else {
			plainRaw = raw[7:]
		}
	}

	msg, err := nas5g.Decode(plainRaw)
	if err != nil {
		return fmt.Errorf("amf: decode NAS: %w", err)
	}

	s.log.WithField("amf_ue_ngap_id", ue.AMFUENGAPID).
		WithField("msg", msg.Header.MessageType).
		Info("amf: NAS received")

	switch {
	case msg.RegistrationRequest != nil:
		return s.handleRegistrationRequest(ctx, ue, msg.RegistrationRequest)
	case msg.AuthenticationResponse != nil:
		return s.handleAuthenticationResponse(ctx, ue, msg.AuthenticationResponse)
	case msg.AuthenticationFailure != nil:
		return s.handleAuthenticationFailure(ctx, ue, msg.AuthenticationFailure)
	case msg.SecurityModeComplete != nil:
		return s.handleSecurityModeComplete(ctx, ue, msg.SecurityModeComplete)
	case msg.DeregistrationRequest != nil:
		return s.handleDeregistrationRequestUEOrig(ctx, ue, msg.DeregistrationRequest)
	case msg.Header.MessageType == nas5g.MsgTypeRegistrationComplete:
		return s.handleRegistrationComplete(ctx, ue)
	case msg.Header.MessageType == nas5g.MsgTypeULNASTransport:
		return s.handleULNASTransport(ctx, ue, plainRaw)
	default:
		s.log.WithField("type", msg.Header.MessageType).Warn("amf: unhandled NAS message")
		return nil
	}
}

// handleRegistrationRequest processes a NAS Registration Request.
// Extracts the mobile identity, calls AUSF for authentication, sends Auth Request.
func (s *Service) handleRegistrationRequest(ctx context.Context, ue *UEContext, req *nas5g.RegistrationRequest) error {
	ue.SUCI = req.MobileIdentity
	ue.UESecCaps = req.UESecurityCapability

	// Build a SUPI or SUCI string for AUSF.
	// For null-scheme SUCI (protection scheme=0), the SUPI is recoverable as "imsi-<MSIN>".
	// For concealed SUCI, pass the full SUCI IE to AUSF/UDM; the SIDF lives in UDM.
	supiOrSuci := s.suciToString(ue.SUCI)
	if supiOrSuci == "" && ue.SUPI != "" {
		// UERANSIM re-registers with the 5G-GUTI QCore assigned during the
		// same live UE context. Until QCore has a global GUTI/TMSI index, use
		// the known SUPI from this context rather than sending an empty AUSF key.
		supiOrSuci = ue.SUPI
	}

	s.emitter.Emit(events.Event{
		JourneyID: ue.JourneyID,
		NF:        "amf",
		Category:  events.SignalingRx,
		Severity:  events.SeverityInfo,
		Protocol:  "nas5g",
		Message:   "Registration Request received",
		Payload: events.RegistrationRequestPayload{
			SUCI:    supiOrSuci,
			RegType: int(req.RegistrationType),
		},
	})

	// Call AUSF for 5G-AKA authentication vector
	ausfReq := &ausf.AuthenticationInfo{
		SupiOrSuci:         supiOrSuci,
		ServingNetworkName: s.cfg.ServingNetworkName,
	}
	ctx = events.WithJourneyID(ctx, ue.JourneyID)
	authCtx, confirmURL, err := s.ausfCli.CreateUEAuth(ctx, ausfReq)
	if err != nil {
		// Distinguish "subscriber not found" from other AUSF errors.
		// UDM returns 404 → AUSF returns 404 → SBI client wraps as "404 Not Found"
		errText := strings.ToLower(err.Error())
		cause := diag.CauseUnknownSubscriber
		if !strings.Contains(errText, "404") && !strings.Contains(errText, "not found") {
			cause = "ausf_error"
		}
		s.log.WithError(err).WithField("supi", supiOrSuci).Error("amf: AUSF auth failed")
		s.emitter.Emit(events.Event{
			JourneyID: ue.JourneyID,
			NF:        "amf",
			Category:  events.ErrorEvent,
			Severity:  events.SeverityError,
			Protocol:  "nas5g",
			Message:   "Registration Reject: AUSF auth failed",
			Payload: events.RegistrationFailurePayload{
				SUCI:   supiOrSuci,
				Cause:  cause,
				Detail: err.Error(),
			},
		})
		// Send a NAS Registration Reject so the UE/simulator learns the outcome
		// rather than timing out waiting for an Authentication Request.
		reject := nas5g.EncodeRegistrationReject(nas5g.Cause5GMM5GSServicesNotAllowed)
		_ = ue.gNB.sendDownlinkNAS(ue, reject)
		return err
	}

	if strings.HasPrefix(supiOrSuci, "imsi-") {
		// Keep SUPI as subscriber identity. The AUSF confirmation endpoint is
		// tracked separately in AuthCtxURL; mixing the two breaks AUTS/SQN
		// resync because it happens before RES* confirmation can return SUPI.
		ue.SUPI = supiOrSuci
	}

	// Decode RAND and AUTN from AUSF response
	randBytes, err := hex.DecodeString(authCtx.Av5gAuthData.RAND)
	if err != nil || len(randBytes) != 16 {
		return fmt.Errorf("amf: invalid RAND from AUSF")
	}
	autnBytes, err := hex.DecodeString(authCtx.Av5gAuthData.AUTN)
	if err != nil || len(autnBytes) != 16 {
		return fmt.Errorf("amf: invalid AUTN from AUSF")
	}
	copy(ue.RAND[:], randBytes)
	copy(ue.AUTN[:], autnBytes)
	ue.AuthCtxURL = confirmURL

	s.emitter.Emit(events.Event{
		JourneyID: ue.JourneyID,
		NF:        "amf",
		Category:  events.SignalingTx,
		Severity:  events.SeverityInfo,
		Protocol:  "nas5g",
		Message:   "Authentication Request sent",
		Payload: events.AuthRequestPayload5G{
			SUPI:    ue.SUPI,
			RANDHex: hex.EncodeToString(ue.RAND[:8]),
		},
	})

	// Send NAS Authentication Request to UE
	authReq := nas5g.EncodeAuthenticationRequest(&nas5g.AuthenticationRequest{
		NASKeySetID: 1,
		ABBA:        []byte{0x00, 0x00},
		RAND:        ue.RAND,
		AUTN:        ue.AUTN,
	})
	ue.State = StateAuthPending
	return ue.gNB.sendDownlinkNAS(ue, authReq)
}

// handleAuthenticationFailure handles an Authentication Failure message from
// the UE — sent when the UE's Milenage check of the network's AUTN fails.
// The two common causes are MAC failure (wrong Ki/OPc on network side) and
// SQN failure (sequence number out of sync).
func (s *Service) handleAuthenticationFailure(ctx context.Context, ue *UEContext, fail *nas5g.AuthenticationFailure) error {
	causeName := "unknown"
	cause5GMM := fail.Cause
	switch cause5GMM {
	case nas5g.Cause5GMMMACFailure:
		causeName = "mac_failure"
	case nas5g.Cause5GMMSynchFailure:
		causeName = "sqn_failure"
	}

	s.log.WithField("supi", ue.SUPI).
		WithField("cause", causeName).
		Warn("amf: Authentication Failure from UE")

	s.emitter.Emit(events.Event{
		JourneyID: ue.JourneyID,
		NF:        "amf",
		Category:  events.ErrorEvent,
		Severity:  events.SeverityError,
		Protocol:  "nas5g",
		Message:   fmt.Sprintf("Authentication Failure from UE: %s", causeName),
		Payload: events.AuthenticationFailurePayload{
			SUPI:      ue.SUPI,
			Cause5GMM: uint8(cause5GMM),
			CauseName: causeName,
		},
	})

	if cause5GMM == nas5g.Cause5GMMSynchFailure && len(fail.AUTS) == 14 {
		if ue.ResyncAttempted {
			s.log.WithField("supi", ue.SUPI).Warn("amf: SQN resync already attempted, rejecting")
		} else {
			ue.ResyncAttempted = true
			s.log.WithField("supi", ue.SUPI).Info("amf: attempting SQN resynchronization")

			supiOrSuci := s.suciToString(ue.SUCI)
			if supiOrSuci == "" && ue.SUPI != "" {
				supiOrSuci = ue.SUPI
			}
			ausfReq := &ausf.AuthenticationInfo{
				SupiOrSuci:         supiOrSuci,
				ServingNetworkName: s.cfg.ServingNetworkName,
				ResynchronizationInfo: &udm.ResynchronizationInfo{
					RAND: hex.EncodeToString(ue.RAND[:]),
					AUTS: hex.EncodeToString(fail.AUTS),
				},
			}

			ctx = events.WithJourneyID(ctx, ue.JourneyID)
			authCtx, confirmURL, err := s.ausfCli.CreateUEAuth(ctx, ausfReq)
			if err != nil {
				s.log.WithError(err).Error("amf: AUSF resync auth failed")
			} else {
				if strings.HasPrefix(supiOrSuci, "imsi-") {
					ue.SUPI = supiOrSuci
				}

				randBytes, _ := hex.DecodeString(authCtx.Av5gAuthData.RAND)
				autnBytes, _ := hex.DecodeString(authCtx.Av5gAuthData.AUTN)
				copy(ue.RAND[:], randBytes)
				copy(ue.AUTN[:], autnBytes)
				ue.AuthCtxURL = confirmURL

				s.emitter.Emit(events.Event{
					JourneyID: ue.JourneyID,
					NF:        "amf",
					Category:  events.SignalingTx,
					Severity:  events.SeverityInfo,
					Protocol:  "nas5g",
					Message:   "Authentication Request sent (Resync)",
					Payload: events.AuthRequestPayload5G{
						SUPI:    ue.SUPI,
						RANDHex: hex.EncodeToString(ue.RAND[:8]),
					},
				})

				authReq := nas5g.EncodeAuthenticationRequest(&nas5g.AuthenticationRequest{
					NASKeySetID: 1,
					ABBA:        []byte{0x00, 0x00},
					RAND:        ue.RAND,
					AUTN:        ue.AUTN,
				})
				ue.State = StateAuthPending
				return ue.gNB.sendDownlinkNAS(ue, authReq)
			}
		}
	}

	// Map to a RegistrationFailurePayload so DiagnoseRegistration can match.
	diagCause := diag.CauseAuthMACFailure
	if causeName == "sqn_failure" {
		diagCause = diag.CauseAuthSQNFailure
	}
	s.emitter.Emit(events.Event{
		JourneyID: ue.JourneyID,
		NF:        "amf",
		Category:  events.ErrorEvent,
		Severity:  events.SeverityError,
		Protocol:  "nas5g",
		Message:   "Registration failed",
		Payload: events.RegistrationFailurePayload{
			SUPI:  ue.SUPI,
			Cause: diagCause,
		},
	})

	// Send Registration Reject.
	reject := nas5g.EncodeRegistrationReject(nas5g.Cause5GMM5GSServicesNotAllowed)
	_ = ue.gNB.sendDownlinkNAS(ue, reject)
	ue.State = StateIdle
	return nil
}

// handleAuthenticationResponse receives RES* from UE and confirms with AUSF.
func (s *Service) handleAuthenticationResponse(ctx context.Context, ue *UEContext, resp *nas5g.AuthenticationResponse) error {
	if ue.State != StateAuthPending {
		s.log.WithField("state", ue.State).Warn("amf: unexpected AuthResponse in state")
		return nil
	}
	if len(resp.ResStar) == 0 {
		return fmt.Errorf("amf: empty RES* in AuthResponse")
	}
	resStarHex := hex.EncodeToString(resp.ResStar)

	s.emitter.Emit(events.Event{
		JourneyID: ue.JourneyID,
		NF:        "amf",
		Category:  events.SignalingRx,
		Severity:  events.SeverityInfo,
		Protocol:  "nas5g",
		Message:   "Authentication Response received",
		Payload: events.AuthResponsePayload{
			IMSI:    ue.SUPI,
			Success: true,
		},
	})

	// Confirm with AUSF
	confirmURL := ue.AuthCtxURL
	ctx = events.WithJourneyID(ctx, ue.JourneyID)
	confResp, err := s.ausfCli.ConfirmAuth(ctx, confirmURL, resStarHex)
	if err != nil {
		s.log.WithError(err).Error("amf: AUSF confirmation failed")
		return err
	}
	if confResp.AuthResult != ausf.AuthResultSuccess {
		return fmt.Errorf("amf: AUSF auth result: %s", confResp.AuthResult)
	}

	// Decode KSEAF → KAMF
	kseaf, err := hex.DecodeString(confResp.Kseaf)
	if err != nil || len(kseaf) != 32 {
		return fmt.Errorf("amf: invalid KSEAF from AUSF")
	}

	supi := confResp.Supi
	if supi != "" {
		ue.SUPI = supi
	}

	// K_AMF P0 is the bare SUPI value (TS 33.501 Annex A.7). The "imsi-" prefix
	// is an SBI/JSON representation, NOT part of the SUPI — UERANSIM/free5GC derive
	// K_AMF from the bare IMSI digits. Passing the prefixed form yields a different
	// K_AMF → K_NASint and makes the Security Mode Command MAC fail to verify at the
	// UE ("Security Mode Command integrity check failed"), so strip it here. ue.SUPI
	// keeps the imsi- form for SBI/N11/telemetry.
	kdfSUPI := strings.TrimPrefix(ue.SUPI, "imsi-")
	kamf, err := DeriveKAMF(kseaf, kdfSUPI, []byte{0x00, 0x00})
	if err != nil {
		return err
	}
	ue.KAMF = kamf

	kNASint, err := DeriveKNASint5G(kamf, ue.IntAlgID)
	if err != nil {
		return err
	}
	kNASenc, err := DeriveKNASenc5G(kamf, ue.EncAlgID)
	if err != nil {
		return err
	}
	kgNB, err := DeriveKgNB(kamf, ue.ULCount)
	if err != nil {
		return err
	}
	ue.KNASint = kNASint
	ue.KNASenc = kNASenc
	ue.KgNB = kgNB

	s.log.WithField("supi", ue.SUPI).Info("amf: authentication succeeded, sending SMC")

	s.emitter.Emit(events.Event{
		JourneyID: ue.JourneyID,
		NF:        "amf",
		Category:  events.SignalingTx,
		Severity:  events.SeverityInfo,
		Protocol:  "nas5g",
		Message:   "Security Mode Command sent",
		Payload: events.SecurityModeCommandPayload5G{
			SUPI:      ue.SUPI,
			CipherAlg: ue.EncAlgID,
			IntegAlg:  ue.IntAlgID,
		},
	})

	// Build and send Security Mode Command (integrity-protected with new context)
	secAlgoByte := ((ue.EncAlgID & 0x0F) << 4) | (ue.IntAlgID & 0x0F)
	smcPlain := nas5g.EncodeSecurityModeCommand(&nas5g.SecurityModeCommand{
		NASSecAlgos:       secAlgoByte,
		NASKeySetID:       1,
		ReplayedUESecCaps: ue.UESecCaps,
		IMEISV_Requested:  true,
	})

	// Wrap SMC with integrity protection using the new security context (SecHdr=0x03)
	smcProtected, err := WrapNAS5G(ue.KNASint, ue.DLCount, SecHdrIntegrityProtectedNewCtx, smcPlain)
	if err != nil {
		return err
	}
	ue.DLCount++
	ue.State = StateSecModePending
	return ue.gNB.sendDownlinkNAS(ue, smcProtected)
}

// handleSecurityModeComplete sends Registration Accept piggybacked on InitialContextSetupRequest.
func (s *Service) handleSecurityModeComplete(ctx context.Context, ue *UEContext, smc *nas5g.SecurityModeComplete) error {
	if ue.State != StateSecModePending {
		s.log.WithField("state", ue.State).Warn("amf: unexpected SMC-Complete in state")
		return nil
	}

	s.log.WithField("supi", ue.SUPI).Info("amf: SMC Complete — sending Registration Accept")

	// Derive a simple 5G-GUTI for the UE
	g := s.cfg.GUAMI
	guti := &nas5g.GUTI5G{
		PLMN:        [3]byte(g.PLMN),
		AMFRegionID: g.AMFRegionID,
		AMFSetID:    g.AMFSetID,
		AMFPointer:  g.AMFPointer,
		TMSI5G:      tmsiFromID(ue.AMFUENGAPID),
	}

	// Build Registration Accept
	regAccept := nas5g.EncodeRegistrationAccept(&nas5g.RegistrationAccept{
		RegistrationResult: 0x01, // 3GPP access
		AssignedGUTI:       guti,
		AllowedNSSAI:       s.allowedNSSAI(ue),
	})

	// Wrap Registration Accept with integrity protection
	regAcceptProtected, err := WrapNAS5G(ue.KNASint, ue.DLCount, SecHdrIntegrityProtected, regAccept)
	if err != nil {
		return err
	}
	ue.DLCount++

	// Send InitialContextSetupRequest to gNB with Registration Accept piggybacked
	ue.State = StateRegistered

	s.emitter.Emit(events.Event{
		JourneyID: ue.JourneyID,
		NF:        "amf",
		Category:  events.StateTransition,
		Severity:  events.SeverityInfo,
		Protocol:  "nas5g",
		Message:   "5GMM-REGISTERED",
		Payload: events.RegistrationAcceptPayload{
			SUPI: ue.SUPI,
		},
	})

	return ue.gNB.sendInitialContextSetup(ue, regAcceptProtected)
}

// handleRegistrationComplete — UE acknowledged Registration Accept.
func (s *Service) handleRegistrationComplete(ctx context.Context, ue *UEContext) error {
	s.log.WithField("supi", ue.SUPI).Info("amf: Registration Complete — UE fully registered")
	return nil
}

// handleDeregistrationRequestUEOrig accepts a UE-originated deregistration.
// This is intentionally small: it supports the UERANSIM CLI-driven re-auth
// trigger used by the SQN-resync interop gate without broad deregistration
// cleanup semantics.
func (s *Service) handleDeregistrationRequestUEOrig(ctx context.Context, ue *UEContext, req *nas5g.DeregistrationRequestUEOrig) error {
	s.log.WithField("supi", ue.SUPI).Info("amf: UE-originated Deregistration Request received")

	accept := nas5g.EncodeDeregistrationAcceptUEOrig()
	if len(ue.KNASint) == 16 {
		protected, err := WrapNAS5G(ue.KNASint, ue.DLCount, SecHdrIntegrityProtected, accept)
		if err != nil {
			return err
		}
		ue.DLCount++
		accept = protected
	}
	ue.State = StateIdle
	if err := ue.gNB.sendDownlinkNAS(ue, accept); err != nil {
		return err
	}
	// UERANSIM deletes its NAS security context before immediately starting
	// the follow-up registration. Reset counters after the protected
	// Deregistration Accept so the next Security Mode Command starts the new
	// context at NAS count 0.
	ue.DLCount = 0
	ue.ULCount = 0
	return nil
}

// handleULNASTransport processes a UL NAS Transport carrying a PDU Session
// Establishment Request. It decodes the N1 SM container and forwards it to the
// SMF via Nsmf_PDUSession_CreateSMContext (TS 29.502 §5.2.2).
func (s *Service) handleULNASTransport(ctx context.Context, ue *UEContext, raw []byte) error {
	const nasHdrLen = 3
	if len(raw) <= nasHdrLen {
		return fmt.Errorf("amf: UL NAS Transport too short")
	}
	ul, err := nas5g.DecodeULNASTransport(raw[nasHdrLen:])
	if err != nil {
		return fmt.Errorf("amf: decode UL NAS Transport: %w", err)
	}

	s.log.WithField("supi", ue.SUPI).
		WithField("pdu_session_id", ul.PDUSessionID).
		Info("amf: UL NAS Transport — forwarding PDU session to SMF")

	smfURL := s.cfg.SMFURL
	if smfURL == "" {
		s.log.Warn("amf: no SMF URL configured, dropping PDU session request")
		return nil
	}

	// Build Nsmf_PDUSession CreateSMContext request.
	createReq := struct {
		Supi         string `json:"supi"`
		PduSessionID int    `json:"pduSessionId"`
		Dnn          string `json:"dnn"`
		ServingNfId  string `json:"servingNfId"`
	}{
		Supi:         ue.SUPI,
		PduSessionID: int(ul.PDUSessionID),
		Dnn:          "internet",
		ServingNfId:  s.cfg.AMFInstanceID,
	}

	s.emitter.Emit(events.Event{
		JourneyID: ue.JourneyID,
		NF:        "amf",
		Category:  events.SignalingTx,
		Severity:  events.SeverityInfo,
		Protocol:  "sbi",
		Message:   "Forwarding PDU Session Establishment to SMF",
		Payload: events.PDUSessionEstablishmentPayload{
			SUPI:         ue.SUPI,
			PDUSessionID: ul.PDUSessionID,
		},
	})

	body, err := json.Marshal(createReq)
	if err != nil {
		return fmt.Errorf("amf: marshal SMF request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		smfURL+"/nsmf-pdusession/v1/sm-contexts",
		bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("amf: create SMF request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.log.WithError(err).Error("amf: SMF PDU session creation failed")
		return nil // non-fatal for registration; UE can retry
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusCreated {
		s.log.WithField("supi", ue.SUPI).Info("amf: SMF created PDU session")
		s.emitter.Emit(events.Event{
			JourneyID: ue.JourneyID,
			NF:        "amf",
			Category:  events.SignalingRx,
			Severity:  events.SeverityInfo,
			Protocol:  "sbi",
			Message:   "PDU Session created successfully",
			Payload: events.PDUSessionResultPayload{
				SUPI:         ue.SUPI,
				PDUSessionID: ul.PDUSessionID,
				DNN:          "internet",
				Success:      true,
			},
		})

		// Relay a PDU Session Establishment Accept to the UE over a protected
		// DL NAS Transport. The SMF returns the allocated UE IPv4 in the 201;
		// the PTI is echoed from the UE's Establishment Request (5GSM octet 3).
		var created struct {
			UeIpv4Addr   string `json:"ueIpv4Addr"`
			SmContextRef string `json:"smContextRef"`
			UpfN3Ip      string `json:"upfN3Ip"`
			UpfN3Teid    uint32 `json:"upfN3Teid"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&created)
		var ueIPv4 [4]byte
		if ip := net.ParseIP(created.UeIpv4Addr); ip != nil {
			if v4 := ip.To4(); v4 != nil {
				copy(ueIPv4[:], v4)
			}
		}
		var pti uint8
		if len(ul.PayloadContainer) >= 3 {
			pti = ul.PayloadContainer[2]
		}
		protected, err := s.buildProtectedPDUSessionEstablishmentAccept(ue, ul.PDUSessionID, pti, ueIPv4)
		if err != nil {
			s.log.WithError(err).Warn("amf: failed to build PDU Session Establishment Accept")
		} else if created.UpfN3Ip != "" && created.UpfN3Teid != 0 {
			ue.PDUSessionID = ul.PDUSessionID
			ue.SMContextRef = created.SmContextRef
			ue.SMFURL = smfURL
			if err := ue.gNB.sendPDUSessionResourceSetup(ue, protected, ul.PDUSessionID, created.UpfN3Ip, created.UpfN3Teid); err != nil {
				s.log.WithError(err).Warn("amf: failed to send PDU Session Resource Setup Request")
			} else {
				s.log.WithField("supi", ue.SUPI).
					WithField("pdu_session_id", ul.PDUSessionID).
					WithField("upf_n3_ip", created.UpfN3Ip).
					WithField("upf_n3_teid", created.UpfN3Teid).
					Info("amf: sent PDU Session Resource Setup Request")
			}
		} else if err := ue.gNB.sendDownlinkNAS(ue, protected); err != nil {
			s.log.WithError(err).Warn("amf: failed to send PDU Session Establishment Accept")
		} else {
			s.log.WithField("supi", ue.SUPI).
				WithField("pdu_session_id", ul.PDUSessionID).
				Info("amf: sent PDU Session Establishment Accept")
			s.emitter.Emit(events.Event{
				JourneyID: ue.JourneyID,
				NF:        "amf",
				Category:  events.SignalingTx,
				Severity:  events.SeverityInfo,
				Protocol:  "nas5g",
				Message:   "PDU Session Establishment Accept sent",
				Payload: events.PDUSessionResultPayload{
					SUPI:         ue.SUPI,
					PDUSessionID: ul.PDUSessionID,
					DNN:          "internet",
					Success:      true,
				},
			})
		}
	} else {
		// Classify the SMF failure cause for the diagnostic layer.
		smfCause := diag.CauseSMFReject
		detail := fmt.Sprintf("SMF returned HTTP %d", resp.StatusCode)
		if resp.StatusCode == http.StatusUnprocessableEntity || resp.StatusCode == http.StatusInternalServerError {
			// 422 / 500 from SMF often means IP pool exhausted or internal error;
			// a more specific cause would require parsing the ProblemDetails body.
			// Keep it as smf_reject for now — the diag fix text covers both cases.
		}
		s.log.WithField("status", resp.StatusCode).Warn("amf: SMF returned non-201 for PDU session")
		s.emitter.Emit(events.Event{
			JourneyID: ue.JourneyID,
			NF:        "amf",
			Category:  events.ErrorEvent,
			Severity:  events.SeverityError,
			Protocol:  "sbi",
			Message:   "PDU Session creation failed",
			Payload: events.PDUSessionResultPayload{
				SUPI:         ue.SUPI,
				PDUSessionID: ul.PDUSessionID,
				DNN:          "internet",
				Success:      false,
				Cause:        smfCause,
				Detail:       detail,
			},
		})
	}
	return nil
}

// sendPDUSessionEstablishmentAccept builds a 5GSM PDU Session Establishment
// Accept (TS 24.501 §8.3.2), wraps it in a DL NAS Transport (§8.2.11), integrity-
// protects it with the UE's established NAS security context and the next DL NAS
// count, and sends it to the gNB over the existing DownlinkNASTransport path.
func (s *Service) sendPDUSessionEstablishmentAccept(ue *UEContext, pduSessionID, pti uint8, ueIPv4 [4]byte) error {
	protected, err := s.buildProtectedPDUSessionEstablishmentAccept(ue, pduSessionID, pti, ueIPv4)
	if err != nil {
		return err
	}
	return ue.gNB.sendDownlinkNAS(ue, protected)
}

func (s *Service) buildProtectedPDUSessionEstablishmentAccept(ue *UEContext, pduSessionID, pti uint8, ueIPv4 [4]byte) ([]byte, error) {
	if len(ue.KNASint) != 16 {
		return nil, fmt.Errorf("amf: no NAS integrity key; cannot protect PDU Session Accept")
	}
	accept := nas5g.EncodePDUSessionEstablishmentAccept(pduSessionID, pti, ueIPv4)
	dlNAS := nas5g.EncodeDLNASTransport(pduSessionID, accept)
	protected, err := WrapNAS5G(ue.KNASint, ue.DLCount, SecHdrIntegrityProtected, dlNAS)
	if err != nil {
		return nil, fmt.Errorf("amf: protect DL NAS Transport: %w", err)
	}
	ue.DLCount++
	return protected, nil
}

func (s *Service) updateSMContextWithGNBTunnel(ctx context.Context, ue *UEContext, sess ngap.PDUSessionResourceSetupResponseItem) error {
	ue.mu.Lock()
	smfURL := ue.SMFURL
	ue.mu.Unlock()
	if smfURL == "" {
		smfURL = s.cfg.SMFURL
	}
	if smfURL == "" {
		return fmt.Errorf("amf: no SMF URL configured")
	}
	modReq := struct {
		Supi         string `json:"supi"`
		PduSessionID int    `json:"pduSessionId"`
		GnbN3Ip      string `json:"gnbN3Ip"`
		GnbN3Teid    uint32 `json:"gnbN3Teid"`
	}{
		Supi:         ue.SUPI,
		PduSessionID: int(sess.PDUSessionID),
		GnbN3Ip:      sess.GNBIP.String(),
		GnbN3Teid:    sess.GNBTEID,
	}
	body, err := json.Marshal(modReq)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, smfURL+"/nsmf-pdusession/v1/sm-contexts/modify", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("SMF modify returned HTTP %d", resp.StatusCode)
	}
	s.log.WithField("supi", ue.SUPI).
		WithField("pdu_session_id", sess.PDUSessionID).
		WithField("gnb_n3_ip", sess.GNBIP.String()).
		WithField("gnb_n3_teid", sess.GNBTEID).
		Info("amf: SMF updated with gNB N3 tunnel")
	return nil
}

// --- helpers ----------------------------------------------------------------

// suciToString converts the raw mobile identity bytes to a SUPI or SUCI string
// suitable for passing to AUSF. For null-scheme SUCI (protection scheme=0),
// decodes to "imsi-<MCC><MNC><MSIN>" so the AUSF→UDM chain can resolve it.
func (s *Service) suciToString(mobileID []byte) string {
	if len(mobileID) < 1 {
		return ""
	}
	idType := mobileID[0] & 0x07
	switch idType {
	case 0x01: // SUCI
		if len(mobileID) >= 8 && mobileID[6]&0x0F == 0x00 {
			if supi := nullSchemeSUCItoSUPI(mobileID); supi != "" {
				return supi
			}
		}
		return "suci-" + hex.EncodeToString(mobileID)
	case 0x02: // 5G-GUTI (re-registration)
		return "" // TODO: GUTI-based re-registration
	default:
		return "suci-" + hex.EncodeToString(mobileID)
	}
}

// nullSchemeSUCItoSUPI decodes a null-scheme SUCI mobile identity IE to
// "imsi-<MCC><MNC><MSIN>" using the canonical PLMN BCD layout from
// TS 24.008 §10.5.1.13 via pkg/ident.
func nullSchemeSUCItoSUPI(mobileID []byte) string {
	if len(mobileID) < 9 {
		return ""
	}
	// Decode PLMN (bytes 1-3) using the canonical TS 24.008 layout:
	//   byte 0 of PLMN: MCC2|MCC1; byte 1: MNC3|MCC3; byte 2: MNC2|MNC1
	mcc, mnc := ident.DecodePLMN([3]byte(mobileID[1:4]))

	// Decode MSIN (bytes 8+): packed BCD, lo nibble = earlier digit.
	var msin strings.Builder
	for _, b := range mobileID[8:] {
		msin.WriteByte('0' + (b & 0x0F))
		if hi := (b >> 4) & 0x0F; hi != 0xF {
			msin.WriteByte('0' + hi)
		}
	}

	return "imsi-" + mcc + mnc + msin.String()
}

func tmsiFromID(id uint64) [4]byte {
	return [4]byte{uint8(id >> 24), uint8(id >> 16), uint8(id >> 8), uint8(id)}
}

func (s *Service) allowedNSSAI(ue *UEContext) []nas5g.NSSAIEntry {
	if len(ue.NSSAI) > 0 {
		entries := make([]nas5g.NSSAIEntry, len(ue.NSSAI))
		for i, n := range ue.NSSAI {
			entries[i] = nas5g.NSSAIEntry{
				SST:       n.SST,
				SDPresent: len(n.SD) >= 3,
			}
			if len(n.SD) >= 3 {
				copy(entries[i].SD[:], n.SD[:3])
			}
		}
		return entries
	}
	// Default: SST=1 (eMBB) if no NSSAI was negotiated
	return []nas5g.NSSAIEntry{{SST: 0x01}}
}
