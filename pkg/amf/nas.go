package amf

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/qcore-project/qcore/pkg/ausf"
	"github.com/qcore-project/qcore/pkg/nas5g"
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
	case msg.SecurityModeComplete != nil:
		return s.handleSecurityModeComplete(ctx, ue, msg.SecurityModeComplete)
	case msg.Header.MessageType == nas5g.MsgTypeRegistrationComplete:
		return s.handleRegistrationComplete(ctx, ue)
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
	// For real deployments, SUCI would be sent as-is to AUSF which resolves it via SIDF.
	supiOrSuci := s.suciToString(ue.SUCI)

	// Call AUSF for 5G-AKA authentication vector
	ausfReq := &ausf.AuthenticationInfo{
		SupiOrSuci:         supiOrSuci,
		ServingNetworkName: s.cfg.ServingNetworkName,
	}
	authCtx, confirmURL, err := s.ausfCli.CreateUEAuth(ctx, ausfReq)
	if err != nil {
		s.log.WithError(err).WithField("supi", supiOrSuci).Error("amf: AUSF auth failed")
		return err
	}

	ue.AuthCtxURL = stripBaseURL(confirmURL)
	ue.SUPI = authCtx.Links["5g-aka"].Href // real SUPI comes in ConfirmationDataResponse; store URL for now

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

	// Confirm with AUSF
	confirmURL := ue.AuthCtxURL
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

	kamf, err := DeriveKAMF(kseaf, ue.SUPI, []byte{0x00, 0x00})
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

	// Build and send Security Mode Command (integrity-protected with new context)
	secAlgoByte := (ue.EncAlgID & 0x0F) | ((ue.IntAlgID & 0x0F) << 4)
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
	return ue.gNB.sendInitialContextSetup(ue, regAcceptProtected)
}

// handleRegistrationComplete — UE acknowledged Registration Accept.
func (s *Service) handleRegistrationComplete(ctx context.Context, ue *UEContext) error {
	s.log.WithField("supi", ue.SUPI).Info("amf: Registration Complete — UE fully registered")
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
// "imsi-<MCC><MNC><MSIN>" using the PLMN BCD layout from TS 24.501 §9.11.3.4.
func nullSchemeSUCItoSUPI(mobileID []byte) string {
	if len(mobileID) < 9 {
		return ""
	}
	// Decode PLMN (bytes 1-3): layout mirrors PLMNFromMCCMNC in pkg/ngap.
	// byte1: hi=MCC2, lo=MCC1; byte2: hi=MNC1, lo=MCC3; byte3: hi=MNC3, lo=MNC2
	p := mobileID[1:4]
	mcc := fmt.Sprintf("%d%d%d", p[0]&0x0F, (p[0]>>4)&0x0F, p[1]&0x0F)
	mnc1 := (p[1] >> 4) & 0x0F
	mnc2 := p[2] & 0x0F
	mnc3 := (p[2] >> 4) & 0x0F
	var mnc string
	if mnc3 == 0xF {
		mnc = fmt.Sprintf("%d%d", mnc1, mnc2)
	} else {
		mnc = fmt.Sprintf("%d%d%d", mnc1, mnc2, mnc3)
	}

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

// stripBaseURL extracts the path portion from a full URL (e.g. from Location header).
// AUSF returns a full URL; the client needs just the path for relative requests.
func stripBaseURL(u string) string {
	// Find path after scheme://host
	for i := 0; i < len(u)-1; i++ {
		if u[i] == '/' && u[i+1] == '/' {
			// skip scheme://host
			rest := u[i+2:]
			idx := strings.IndexByte(rest, '/')
			if idx >= 0 {
				return rest[idx:]
			}
			return "/"
		}
	}
	return u // already a path
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

