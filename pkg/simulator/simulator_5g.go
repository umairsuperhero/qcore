package simulator

import (
	"context"
	"fmt"
	"time"

	"github.com/qcore-project/qcore/pkg/events"
	"github.com/qcore-project/qcore/pkg/ident"
	"github.com/qcore-project/qcore/pkg/nas5g"
	"github.com/qcore-project/qcore/pkg/ngap"
	"github.com/qcore-project/qcore/pkg/subscriber"
)

// attach5g is the linear 5G state machine.
func (c *Client) attach5g(ctx context.Context) (string, error) {
	addr := c.opts.MMEAddr
	if c.opts.Scenario == "wrong_mme_address" {
		addr = "127.0.0.1:1"
	}
	if err := c.connect(addr); err != nil {
		return "connect", err
	}
	defer c.assoc.Close()

	if err := c.ngSetup(); err != nil {
		return "ng_setup", err
	}
	if err := c.initialUE5g(); err != nil {
		return "initial_ue", err
	}
	authReq, err := c.readAuthRequest5g(ctx)
	if err != nil {
		return "auth_request", err
	}
	if err := c.sendAuthResponse5g(authReq); err != nil {
		return "auth_response", err
	}
	if err := c.readSecModeCommand5g(ctx); err != nil {
		return "security_mode_command", err
	}
	if err := c.sendSecModeComplete5g(); err != nil {
		return "security_mode_complete", err
	}
	if err := c.readInitialContextSetup5g(ctx); err != nil {
		return "initial_context_setup", err
	}
	if err := c.sendInitialContextSetupResponse5g(); err != nil {
		return "initial_context_setup_response", err
	}
	if err := c.sendRegistrationComplete5g(); err != nil {
		return "registration_complete", err
	}
	return "", nil
}

func (c *Client) ngSetup() error {
	plmn := c.opts.PLMN
	if c.opts.Scenario == "wrong_plmn" {
		plmn = [3]byte{0x99, 0xF9, 0x99}
	}
	req := &ngap.NGSetupRequest{
		GlobalRANNodeID: ngap.GlobalGNBID{PLMN: plmn, GNBID: 0xABCDE, GNBIDBits: 22},
		RANNodeName:     "qcore-builtin-sim-5g",
		SupportedTAList: []ngap.SupportedTA{{TAC: [3]byte{0x00, byte(c.opts.TAC >> 8), byte(c.opts.TAC)}, BroadcastPLMN: []ngap.BroadcastPLMNItem{{PLMN: plmn}}}},
		DefaultPagingDRX: 1,
	}
	bytes, err := ngap.EncodeNGSetupRequest(req)
	if err != nil {
		return fmt.Errorf("encode NG Setup: %w", err)
	}
	if err := c.assoc.Write(bytes, c.streamID); err != nil {
		return fmt.Errorf("send NG Setup: %w", err)
	}
	c.emit(events.SignalingTx, events.SeverityInfo, "ngap", "Sent NG Setup Request", nil)

	pdu, err := c.readNGAPPDU(2 * time.Second)
	if err != nil {
		return fmt.Errorf("read NG Setup response: %w", err)
	}
	if pdu.Type != ngap.PDUSuccessfulOutcome || pdu.ProcedureCode != ngap.ProcNGSetup {
		return fmt.Errorf("expected NG Setup Response, got proc=%d type=%d", pdu.ProcedureCode, pdu.Type)
	}
	c.emit(events.SignalingRx, events.SeverityInfo, "ngap", "Received NG Setup Response", nil)
	return nil
}

func (c *Client) initialUE5g() error {
	imsi := c.opts.IMSI
	if c.opts.Scenario == "unprovisioned_imsi" {
		// Use an IMSI that will not be found in any subscriber store, keeping
		// the same MCC/MNC so the PLMN portion of the SUCI is valid.
		imsi = "999999999999999"
	}

	// Build a real null-scheme SUCI from the IMSI (TS 24.501 §9.11.3.4).
	// Decode the PLMN to find the MNC length, then extract the MSIN.
	_, mnc := ident.DecodePLMN(c.opts.PLMN)
	msinStart := 3 + len(mnc) // MCC(3) + MNC(2 or 3) digits
	msin := ""
	if msinStart < len(imsi) {
		msin = imsi[msinStart:]
	}

	suciBytes := nas5g.EncodeSUCI(nas5g.SUCI{
		PLMN:             c.opts.PLMN,
		RoutingIndicator: [2]byte{0xFF, 0xFF},
		ProtectionScheme: 0x00, // null scheme
		HomeNetworkPKID:  0x00,
		MSIN:             encodeMSINBCD(msin),
	})
	req := &nas5g.RegistrationRequest{
		RegistrationType: 0x01,
		MobileIdentity:   suciBytes,
		UESecurityCapability: []byte{0x80, 0x80, 0x00, 0x00},
	}
	nasBody := nas5g.EncodeRegistrationRequest(req)

	msg := &ngap.InitialUEMessage{
		RANUENGAPID:          uint64(c.enbUES1ID),
		NASPDU:               nasBody,
		UserLocationInfo:     ngap.UserLocationInformationNR{},
		RRCEstablishmentCause: ngap.RRCMoSignalling,
	}
	bytes, err := ngap.EncodeInitialUEMessage(msg)
	if err != nil {
		return fmt.Errorf("encode Initial UE: %w", err)
	}
	if err := c.assoc.Write(bytes, c.streamID); err != nil {
		return fmt.Errorf("send Initial UE: %w", err)
	}
	c.emit(events.SignalingTx, events.SeverityInfo, "nas5g", "Sent NAS Registration Request", nil)
	return nil
}

func (c *Client) readAuthRequest5g(_ context.Context) (*nas5g.AuthenticationRequest, error) {
	nasPDU, amfUEID, err := c.readDownlinkNAS5g(3 * time.Second)
	if err != nil {
		return nil, err
	}
	c.mmeUES1ID = uint32(amfUEID)

	hdr, err := nas5g.DecodeHeader(nasPDU)
	if err != nil {
		return nil, fmt.Errorf("parse NAS header: %w", err)
	}
	switch hdr.MessageType {
	case nas5g.MsgTypeAuthenticationReject:
		return nil, fmt.Errorf("AMF sent Authentication Reject (likely unprovisioned subscriber)")
	case nas5g.MsgTypeRegistrationReject:
		// AMF sends this when AUSF lookup fails (e.g. unprovisioned IMSI).
		// Byte 3 carries the 5GMM cause code.
		cause := nas5g.Cause5GMM(0)
		if len(nasPDU) > 3 {
			cause = nas5g.Cause5GMM(nasPDU[3])
		}
		return nil, fmt.Errorf("AMF sent Registration Reject (cause 0x%02X)", uint8(cause))
	case nas5g.MsgTypeAuthenticationRequest:
		// expected — fall through
	default:
		return nil, fmt.Errorf("expected Authentication Request, got message type 0x%02X", uint8(hdr.MessageType))
	}
	const nasHeaderLen = 3 // plain 5GMM header: EPD + SecurityHeaderType + MessageType
	body := nasPDU[nasHeaderLen:]
	if len(body) < 1+16+1+16 {
		return nil, fmt.Errorf("Authentication Request body too short: %d", len(body))
	}
	req := &nas5g.AuthenticationRequest{NASKeySetID: body[0] & 0x0F}
	copy(req.RAND[:], body[1:17])
	autnLen := int(body[17])
	if autnLen != 16 {
		return nil, fmt.Errorf("AUTN length %d, expected 16", autnLen)
	}
	copy(req.AUTN[:], body[18:34])
	c.emit(events.SignalingRx, events.SeverityInfo, "nas5g", "Received NAS Authentication Request", nil)
	return req, nil
}

func (c *Client) sendAuthResponse5g(authReq *nas5g.AuthenticationRequest) error {
	ki, err := hexBytes16(c.opts.Ki)
	if err != nil {
		return fmt.Errorf("parse Ki: %w", err)
	}
	if c.opts.Scenario == "wrong_ki" {
		ki[0] ^= 0xFF
	}
	opc, err := hexBytes16(c.opts.OPc)
	if err != nil {
		return fmt.Errorf("parse OPc: %w", err)
	}

	res, _, _, _, err := subscriber.F2345(ki, opc, authReq.RAND)
	if err != nil {
		return fmt.Errorf("Milenage F2345: %w", err)
	}

	authResp := nas5g.EncodeAuthenticationResponse(&nas5g.AuthenticationResponse{
		ResStar: res[:], // Simulator currently uses res directly, in reality RES* is derived from Kausf
	})

	ulMsg := &ngap.UplinkNASTransport{
		AMFUENGAPID:      uint64(c.mmeUES1ID),
		RANUENGAPID:      uint64(c.enbUES1ID),
		NASPDU:           authResp,
	}
	bytes, err := ngap.EncodeUplinkNASTransport(ulMsg)
	if err != nil {
		return fmt.Errorf("encode Uplink NAS (auth response): %w", err)
	}
	if err := c.assoc.Write(bytes, c.streamID); err != nil {
		return fmt.Errorf("send Auth Response: %w", err)
	}
	c.emit(events.SignalingTx, events.SeverityInfo, "nas5g", "Sent NAS Authentication Response", nil)
	return nil
}

func (c *Client) readSecModeCommand5g(_ context.Context) error {
	nasPDU, _, err := c.readDownlinkNAS5g(3 * time.Second)
	if err != nil {
		return err
	}
	hdr, err := nas5g.DecodeHeader(nasPDU)
	if err != nil {
		return fmt.Errorf("parse NAS header: %w", err)
	}
	if hdr.MessageType != nas5g.MsgTypeSecurityModeCommand {
		return fmt.Errorf("expected Security Mode Command, got message type %d", hdr.MessageType)
	}
	c.emit(events.SignalingRx, events.SeverityInfo, "nas5g", "Received NAS Security Mode Command", nil)
	return nil
}

func (c *Client) sendSecModeComplete5g() error {
	plain := nas5g.EncodeSecurityModeComplete(&nas5g.SecurityModeComplete{})
	// For simulator, we won't fully implement 5G NAS security wrapping just yet.
	// But AMF might enforce it.
	c.ulNASCount = 1
	wrapped := plain // Unprotected for now or use 5G wrap

	ulMsg := &ngap.UplinkNASTransport{
		AMFUENGAPID:      uint64(c.mmeUES1ID),
		RANUENGAPID:      uint64(c.enbUES1ID),
		NASPDU:           wrapped,
	}
	bytes, err := ngap.EncodeUplinkNASTransport(ulMsg)
	if err != nil {
		return err
	}
	if err := c.assoc.Write(bytes, c.streamID); err != nil {
		return err
	}
	c.emit(events.SignalingTx, events.SeverityInfo, "nas5g", "Sent NAS Security Mode Complete", nil)
	return nil
}

func (c *Client) readInitialContextSetup5g(_ context.Context) error {
	pdu, err := c.readNGAPPDU(3 * time.Second)
	if err != nil {
		return err
	}
	if pdu.Type != ngap.PDUInitiatingMessage || pdu.ProcedureCode != ngap.ProcInitialContextSetup {
		return fmt.Errorf("expected Initial Context Setup Request, got proc=%d type=%d", pdu.ProcedureCode, pdu.Type)
	}
	c.emit(events.SignalingRx, events.SeverityInfo, "ngap", "Received Initial Context Setup Request", nil)
	return nil
}

func (c *Client) sendInitialContextSetupResponse5g() error {
	resp := &ngap.InitialContextSetupResponse{
		AMFUENGAPID: uint64(c.mmeUES1ID),
		RANUENGAPID: uint64(c.enbUES1ID),
	}
	bytes, err := ngap.EncodeInitialContextSetupResponse(resp)
	if err != nil {
		return fmt.Errorf("encode Initial Context Setup Response: %w", err)
	}
	if err := c.assoc.Write(bytes, c.streamID); err != nil {
		return fmt.Errorf("send Initial Context Setup Response: %w", err)
	}
	c.emit(events.SignalingTx, events.SeverityInfo, "ngap", "Sent Initial Context Setup Response", nil)
	return nil
}

func (c *Client) sendRegistrationComplete5g() error {
	plain := nas5g.EncodeRegistrationComplete()
	c.ulNASCount = 2
	wrapped := plain

	ulMsg := &ngap.UplinkNASTransport{
		AMFUENGAPID: uint64(c.mmeUES1ID),
		RANUENGAPID: uint64(c.enbUES1ID),
		NASPDU:      wrapped,
	}
	bytes, err := ngap.EncodeUplinkNASTransport(ulMsg)
	if err != nil {
		return err
	}
	if err := c.assoc.Write(bytes, c.streamID); err != nil {
		return err
	}
	c.emit(events.SignalingTx, events.SeverityInfo, "nas5g", "Sent NAS Registration Complete", nil)
	return nil
}

// --- Wire helpers ---

func (c *Client) readNGAPPDU(timeout time.Duration) (*ngap.PDU, error) {
	bytes, err := c.readBytes(timeout)
	if err != nil {
		return nil, err
	}
	pdu, err := ngap.DecodePDU(bytes)
	if err != nil {
		return nil, fmt.Errorf("decode PDU: %w", err)
	}
	return pdu, nil
}

func (c *Client) readDownlinkNAS5g(timeout time.Duration) ([]byte, uint64, error) {
	pdu, err := c.readNGAPPDU(timeout)
	if err != nil {
		return nil, 0, err
	}
	if pdu.ProcedureCode != ngap.ProcDownlinkNASTransport {
		return nil, 0, fmt.Errorf("expected Downlink NAS Transport, got proc=%d", pdu.ProcedureCode)
	}
	ies, err := ngap.DecodeIEContainer(pdu.Value)
	if err != nil {
		return nil, 0, fmt.Errorf("decode IEs: %w", err)
	}
	dl, err := ngap.DecodeDownlinkNASTransport(ies)
	if err != nil {
		return nil, 0, fmt.Errorf("decode Downlink NAS: %w", err)
	}
	return dl.NASPDU, dl.AMFUENGAPID, nil
}

// encodeMSINBCD converts a digit string to packed BCD with low nibble first.
// Each byte holds two consecutive digits: lo nibble = earlier, hi nibble = later.
// An odd-length string gets a 0xF filler in the last high nibble.
// Example: "0000000001" → [0x00, 0x00, 0x00, 0x00, 0x10]
func encodeMSINBCD(msin string) []byte {
	n := (len(msin) + 1) / 2
	b := make([]byte, n)
	for i, ch := range msin {
		d := uint8(ch - '0')
		if i%2 == 0 {
			b[i/2] = d // low nibble of byte (first of the pair)
		} else {
			b[i/2] |= d << 4 // high nibble of byte (second of the pair)
		}
	}
	if len(msin)%2 == 1 {
		b[n-1] |= 0xF0 // pad final high nibble
	}
	return b
}
