// Package simulator is the built-in UE/eNB control-plane simulator. It
// speaks real S1AP/NAS against a running QCore MME, plays the role of a
// SIM card (including Milenage to derive RES), and supports a fixed
// set of error-injection scenarios that produce the named failure modes
// from charter §9.1.
//
// This is intentionally a control-plane tool, not an RF emulator
// (charter D10). It exists so a developer can run a full 4G attach with
// zero hardware, watch it live in the dashboard, and reproduce the same
// failure traces that Phase C's diagnostic AI will reason over.
package simulator

import (
	"context"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/qcore-project/qcore/pkg/events"
	"github.com/qcore-project/qcore/pkg/logger"
	"github.com/qcore-project/qcore/pkg/nas"
	"github.com/qcore-project/qcore/pkg/s1ap"
	"github.com/qcore-project/qcore/pkg/sctp"
	"github.com/qcore-project/qcore/pkg/subscriber"
)

// Options drives one attach attempt. Defaults give a successful happy-path
// attach against the seeded demo subscriber.
type Options struct {
	Mode     string  // "4g" or "5g", defaults to "4g"
	MMEAddr  string  // host:port for the MME/AMF SCTP listener
	PLMN     [3]byte // packed PLMN (e.g. {0x00, 0xF1, 0x10} = "00101")
	TAC      uint16
	IMSI     string // 15 digits
	Ki       string // 32 hex chars
	OPc      string // 32 hex chars
	Scenario string // empty for happy path, or "wrong_ki" / "wrong_plmn" / "unprovisioned_imsi" / "wrong_mme_address"
}

// Result is what the controller exposes to the dashboard after Run completes.
type Result struct {
	Success    bool
	JourneyID  string
	FailedStep string
	Err        error
	StartedAt  time.Time
	EndedAt    time.Time
}

// Client is one shot, one attach. Do not reuse across attempts.
type Client struct {
	opts    Options
	emitter events.Emitter
	log     logger.Logger

	assoc      sctp.Association
	enbUES1ID  uint32 // also used as RANUENGAPID in 5g mode
	mmeUES1ID  uint32 // also used as AMFUENGAPID in 5g mode
	journeyID  string
	streamID   uint16
	ulNASCount uint32 // post-security uplink NAS counter
	kNASint    []byte // EIA2 (4g) or NIA2 (5g) integrity key
	kNASenc    []byte // NEA0 ciphering key (5g)
}

// Run executes one attach attempt end-to-end (or stops at the first error).
// Emits high-level progress events tagged with nf="simulator" so the live
// trace view shows the UE-side narration alongside the MME's view.
func Run(ctx context.Context, opts Options, emitter events.Emitter, log logger.Logger) Result {
	if emitter == nil {
		emitter = &events.NoopEmitter{}
	}
	c := &Client{
		opts:      opts,
		emitter:   emitter,
		log:       log.WithField("component", "simulator"),
		enbUES1ID: 7,
		streamID:  0,
		journeyID: events.MintJourneyID(),
	}
	res := Result{
		JourneyID: c.journeyID,
		StartedAt: time.Now().UTC(),
	}

	c.emit(events.SignalingTx, events.SeverityInfo, "sctp",
		fmt.Sprintf("Simulator starting %s attach for IMSI %s", opts.Mode, opts.IMSI), nil)

	var step string
	var err error
	if opts.Mode == "5g" {
		step, err = c.attach5g(ctx)
	} else {
		step, err = c.attach(ctx)
	}
	res.EndedAt = time.Now().UTC()
	if err != nil {
		res.Success = false
		res.FailedStep = step
		res.Err = err
		c.emit(events.ErrorEvent, events.SeverityError, "s1ap",
			fmt.Sprintf("Attach failed at %s: %v", step, err),
			events.ErrorPayload{Code: step, Message: err.Error()})
		return res
	}
	res.Success = true
	c.emit(events.SignalingRx, events.SeverityInfo, "s1ap",
		"Attach complete — UE is registered.", nil)
	return res
}

// attach is the linear state machine. Each helper returns its name on error
// so the dashboard can show "where it stopped." Errors are returned, not
// panicked, because most "failures" are expected (error-injection scenarios).
func (c *Client) attach(ctx context.Context) (string, error) {
	addr := c.opts.MMEAddr
	if c.opts.Scenario == "wrong_mme_address" {
		// Deliberately point at a port nothing listens on.
		addr = "127.0.0.1:1"
	}
	if err := c.connect(addr); err != nil {
		return "connect", err
	}
	defer c.assoc.Close()

	if err := c.s1Setup(); err != nil {
		return "s1_setup", err
	}
	if err := c.initialUE(); err != nil {
		return "initial_ue", err
	}
	authReq, err := c.readAuthRequest(ctx)
	if err != nil {
		return "auth_request", err
	}
	if err := c.sendAuthResponse(authReq); err != nil {
		return "auth_response", err
	}
	if err := c.readSecModeCommand(ctx); err != nil {
		return "security_mode_command", err
	}
	if err := c.sendSecModeComplete(authReq); err != nil {
		return "security_mode_complete", err
	}
	if err := c.readInitialContextSetup(ctx); err != nil {
		return "initial_context_setup", err
	}
	if err := c.sendInitialContextSetupResponse(); err != nil {
		return "initial_context_setup_response", err
	}
	if err := c.sendAttachComplete(); err != nil {
		return "attach_complete", err
	}
	if err := c.readEMMInformation(ctx); err != nil {
		return "emm_information", err
	}
	return "", nil
}

// --- Steps ---

func (c *Client) connect(addr string) error {
	deadline := time.Now().Add(3 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		assoc, err := sctp.Dial(sctp.ModeTCP, addr)
		if err == nil {
			c.assoc = assoc
			c.emit(events.StateTransition, events.SeverityInfo, "sctp",
				"Connected to MME at "+addr, nil)
			return nil
		}
		lastErr = err
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("dial %s: %w", addr, lastErr)
}

func (c *Client) s1Setup() error {
	plmn := c.opts.PLMN
	if c.opts.Scenario == "wrong_plmn" {
		// Send a PLMN the MME isn't configured for. MME compares its own
		// PLMN against the eNB's TAI; a mismatch produces S1SetupFailure.
		plmn = [3]byte{0x99, 0xF9, 0x99}
	}
	req := &s1ap.S1SetupRequest{
		GlobalENBID:  s1ap.GlobalENBID{PLMN: plmn, ENBID: 0xABCDE, Type: s1ap.MacroENBID},
		ENBName:      "qcore-builtin-sim",
		SupportedTAs: []s1ap.SupportedTA{{TAC: c.opts.TAC, PLMNs: [][3]byte{plmn}}},
		PagingDRX:    1,
	}
	bytes, err := s1ap.EncodeS1SetupRequest(req)
	if err != nil {
		return fmt.Errorf("encode S1 Setup: %w", err)
	}
	if err := c.assoc.Write(bytes, c.streamID); err != nil {
		return fmt.Errorf("send S1 Setup: %w", err)
	}
	c.emit(events.SignalingTx, events.SeverityInfo, "s1ap", "Sent S1 Setup Request",
		events.S1SetupPayload{ENBName: "qcore-builtin-sim", ENBID: 0xABCDE})

	pdu, err := c.readPDU(2 * time.Second)
	if err != nil {
		return fmt.Errorf("read S1 Setup response: %w", err)
	}
	if pdu.Type != s1ap.PDUSuccessfulOutcome || pdu.ProcedureCode != s1ap.ProcS1Setup {
		return fmt.Errorf("expected S1 Setup Response, got proc=%d type=%d", pdu.ProcedureCode, pdu.Type)
	}
	c.emit(events.SignalingRx, events.SeverityInfo, "s1ap", "Received S1 Setup Response", nil)
	return nil
}

func (c *Client) initialUE() error {
	imsi := c.opts.IMSI
	if c.opts.Scenario == "unprovisioned_imsi" {
		imsi = "999999999999999"
	}
	encodedIMSI, err := nas.EncodeIMSI(imsi)
	if err != nil {
		return fmt.Errorf("encode IMSI: %w", err)
	}
	nasBody := append([]byte{
		uint8(nas.SecurityHeaderPlainNAS<<4) | uint8(nas.EPSMobilityManagement),
		uint8(nas.MsgTypeAttachRequest),
		0x71,                    // KSI=7 | attach type=1 (EPS attach)
		uint8(len(encodedIMSI)), // mobile identity LV length
	}, encodedIMSI...)
	nasBody = append(nasBody, 0x02, 0xE0, 0xE0)                          // UE network capability LV
	nasBody = append(nasBody, 0x00, 0x05, 0xD0, 0x11, 0x27, 0x1D, 0x31) // ESM container LV-E (PDN Connectivity)

	msg := &s1ap.InitialUEMessage{
		ENBUES1APID: c.enbUES1ID,
		NASPDU:      nasBody,
		TAI:         s1ap.TAI{PLMN: c.opts.PLMN, TAC: c.opts.TAC},
		ECGI:        s1ap.ECGI{PLMN: c.opts.PLMN, CellID: 0x0000001},
		RRCCause:    s1ap.RRCMoSignalling,
	}
	bytes, err := s1ap.EncodeInitialUEMessage(msg)
	if err != nil {
		return fmt.Errorf("encode Initial UE: %w", err)
	}
	if err := c.assoc.Write(bytes, c.streamID); err != nil {
		return fmt.Errorf("send Initial UE: %w", err)
	}
	c.emit(events.SignalingTx, events.SeverityInfo, "nas4g", "Sent NAS Attach Request",
		events.AttachRequestPayload{IMSI: imsi, AttachType: 1, TAC: c.opts.TAC})
	return nil
}

func (c *Client) readAuthRequest(_ context.Context) (*nas.AuthenticationRequest, error) {
	nasPDU, mmeUEID, err := c.readDownlinkNAS(3 * time.Second)
	if err != nil {
		return nil, err
	}
	c.mmeUES1ID = mmeUEID

	hdr, hdrLen, err := nas.ParseHeader(nasPDU)
	if err != nil {
		return nil, fmt.Errorf("parse NAS header: %w", err)
	}
	// MME may send an Authentication Reject (e.g. unprovisioned IMSI ->
	// HSS returned not-found). Surface that as a labeled failure.
	if hdr.MessageType == nas.MsgTypeAuthenticationReject {
		return nil, fmt.Errorf("MME sent Authentication Reject (likely unprovisioned subscriber)")
	}
	if hdr.MessageType != nas.MsgTypeAuthenticationRequest {
		return nil, fmt.Errorf("expected Authentication Request, got message type %d", hdr.MessageType)
	}
	// Body layout per TS 24.301 §8.2.7: KSI (1) + RAND (16) + AUTN-LV (1+16).
	body := nasPDU[hdrLen:]
	if len(body) < 1+16+1+16 {
		return nil, fmt.Errorf("Authentication Request body too short: %d", len(body))
	}
	req := &nas.AuthenticationRequest{NASKeySetIdentifier: body[0] & 0x0F}
	copy(req.RAND[:], body[1:17])
	autnLen := int(body[17])
	if autnLen != 16 {
		return nil, fmt.Errorf("AUTN length %d, expected 16", autnLen)
	}
	copy(req.AUTN[:], body[18:34])
	c.emit(events.SignalingRx, events.SeverityInfo, "nas4g", "Received NAS Authentication Request",
		events.AuthRequestPayload{IMSI: c.opts.IMSI, RANDHex: hex.EncodeToString(req.RAND[:8])})
	return req, nil
}

func (c *Client) sendAuthResponse(authReq *nas.AuthenticationRequest) error {
	ki, err := hexBytes16(c.opts.Ki)
	if err != nil {
		return fmt.Errorf("parse Ki: %w", err)
	}
	if c.opts.Scenario == "wrong_ki" {
		// Flip a byte so the derived RES doesn't match XRES at the MME.
		ki[0] ^= 0xFF
	}
	opc, err := hexBytes16(c.opts.OPc)
	if err != nil {
		return fmt.Errorf("parse OPc: %w", err)
	}

	res, ck, ik, ak, err := subscriber.F2345(ki, opc, authReq.RAND)
	if err != nil {
		return fmt.Errorf("Milenage F2345: %w", err)
	}

	// AUTN layout: SQN⊕AK (6) || AMF (2) || MAC (8). Recover SQN.
	var sqn [6]byte
	for i := 0; i < 6; i++ {
		sqn[i] = authReq.AUTN[i] ^ ak[i]
	}
	// kASME = KDF(CK||IK, FC=0x10, SN, SQN⊕AK).
	kASME := subscriber.DeriveKASME(ck, ik, c.opts.PLMN, sqn, ak)
	kNASint, err := nas.DeriveKNASint(kASME[:], 2) // alg=2 (128-EIA2)
	if err != nil {
		return fmt.Errorf("derive kNASint: %w", err)
	}
	c.kNASint = kNASint

	// Send NAS Auth Response (plain — pre-security).
	authResp := []byte{
		uint8(nas.SecurityHeaderPlainNAS<<4) | uint8(nas.EPSMobilityManagement),
		uint8(nas.MsgTypeAuthenticationResponse),
		uint8(len(res)),
	}
	authResp = append(authResp, res[:]...)

	ulMsg := &s1ap.UplinkNASTransport{
		MMEUES1APID: c.mmeUES1ID,
		ENBUES1APID: c.enbUES1ID,
		NASPDU:      authResp,
		TAI:         s1ap.TAI{PLMN: c.opts.PLMN, TAC: c.opts.TAC},
		ECGI:        s1ap.ECGI{PLMN: c.opts.PLMN, CellID: 0x0000001},
	}
	bytes, err := s1ap.EncodeUplinkNASTransport(ulMsg)
	if err != nil {
		return fmt.Errorf("encode Uplink NAS (auth response): %w", err)
	}
	if err := c.assoc.Write(bytes, c.streamID); err != nil {
		return fmt.Errorf("send Auth Response: %w", err)
	}
	c.emit(events.SignalingTx, events.SeverityInfo, "nas4g",
		"Sent NAS Authentication Response (RES "+hex.EncodeToString(res[:])+")", nil)
	return nil
}

func (c *Client) readSecModeCommand(_ context.Context) error {
	nasPDU, _, err := c.readDownlinkNAS(3 * time.Second)
	if err != nil {
		return err
	}
	hdr, _, err := nas.ParseHeader(nasPDU)
	if err != nil {
		return fmt.Errorf("parse NAS header: %w", err)
	}
	if hdr.MessageType == nas.MsgTypeAuthenticationReject {
		return fmt.Errorf("MME sent Authentication Reject (RES did not match XRES — wrong Ki?)")
	}
	if hdr.MessageType == nas.MsgTypeAuthenticationFailure {
		return fmt.Errorf("MME sent Authentication Failure")
	}
	if hdr.MessageType != nas.MsgTypeSecurityModeCommand {
		return fmt.Errorf("expected Security Mode Command, got message type %d", hdr.MessageType)
	}
	c.emit(events.SignalingRx, events.SeverityInfo, "nas4g", "Received NAS Security Mode Command", nil)
	return nil
}

func (c *Client) sendSecModeComplete(_ *nas.AuthenticationRequest) error {
	plain := []byte{
		uint8(nas.SecurityHeaderPlainNAS<<4) | uint8(nas.EPSMobilityManagement),
		uint8(nas.MsgTypeSecurityModeComplete),
	}
	// MME's ULCount is at 1 here (it counted the prior plain Auth Response).
	c.ulNASCount = 1
	wrapped, err := wrapUplink(c.kNASint, c.ulNASCount, plain)
	if err != nil {
		return fmt.Errorf("integrity-wrap Security Mode Complete: %w", err)
	}
	if err := c.sendUplinkNAS(wrapped); err != nil {
		return err
	}
	c.emit(events.SignalingTx, events.SeverityInfo, "nas4g", "Sent NAS Security Mode Complete", nil)
	return nil
}

func (c *Client) readInitialContextSetup(_ context.Context) error {
	pdu, err := c.readPDU(3 * time.Second)
	if err != nil {
		return err
	}
	if pdu.Type != s1ap.PDUInitiatingMessage || pdu.ProcedureCode != s1ap.ProcInitialContextSetup {
		return fmt.Errorf("expected Initial Context Setup Request, got proc=%d type=%d", pdu.ProcedureCode, pdu.Type)
	}
	c.emit(events.SignalingRx, events.SeverityInfo, "s1ap", "Received Initial Context Setup Request", nil)
	return nil
}

func (c *Client) sendInitialContextSetupResponse() error {
	resp := &s1ap.InitialContextSetupResponse{
		MMEUES1APID: c.mmeUES1ID,
		ENBUES1APID: c.enbUES1ID,
	}
	bytes, err := s1ap.EncodeInitialContextSetupResponse(resp)
	if err != nil {
		return fmt.Errorf("encode Initial Context Setup Response: %w", err)
	}
	if err := c.assoc.Write(bytes, c.streamID); err != nil {
		return fmt.Errorf("send Initial Context Setup Response: %w", err)
	}
	c.emit(events.SignalingTx, events.SeverityInfo, "s1ap", "Sent Initial Context Setup Response", nil)
	return nil
}

func (c *Client) sendAttachComplete() error {
	plain := []byte{
		uint8(nas.SecurityHeaderPlainNAS<<4) | uint8(nas.EPSMobilityManagement),
		uint8(nas.MsgTypeAttachComplete),
		0x00, 0x00, // empty ESM container
	}
	c.ulNASCount = 2
	wrapped, err := wrapUplink(c.kNASint, c.ulNASCount, plain)
	if err != nil {
		return fmt.Errorf("integrity-wrap Attach Complete: %w", err)
	}
	if err := c.sendUplinkNAS(wrapped); err != nil {
		return err
	}
	c.emit(events.SignalingTx, events.SeverityInfo, "nas4g", "Sent NAS Attach Complete", nil)
	return nil
}

func (c *Client) readEMMInformation(_ context.Context) error {
	nasPDU, _, err := c.readDownlinkNAS(3 * time.Second)
	if err != nil {
		return err
	}
	hdr, _, err := nas.ParseHeader(nasPDU)
	if err != nil {
		return fmt.Errorf("parse NAS header: %w", err)
	}
	if hdr.MessageType != nas.MsgTypeEMMInformation {
		// Not strictly fatal — some MME implementations skip EMM Information.
		// But QCore sends it, so missing means something went wrong.
		return fmt.Errorf("expected EMM Information, got message type %d", hdr.MessageType)
	}
	c.emit(events.SignalingRx, events.SeverityInfo, "nas4g", "Received NAS EMM Information", nil)
	return nil
}

// --- Wire helpers ---

func (c *Client) readPDU(timeout time.Duration) (*s1ap.PDU, error) {
	bytes, err := c.readBytes(timeout)
	if err != nil {
		return nil, err
	}
	pdu, err := s1ap.DecodePDU(bytes)
	if err != nil {
		return nil, fmt.Errorf("decode PDU: %w", err)
	}
	return pdu, nil
}

func (c *Client) readBytes(timeout time.Duration) ([]byte, error) {
	type r struct {
		b   []byte
		err error
	}
	ch := make(chan r, 1)
	go func() {
		b, _, err := c.assoc.Read()
		ch <- r{b, err}
	}()
	select {
	case res := <-ch:
		return res.b, res.err
	case <-time.After(timeout):
		return nil, fmt.Errorf("read timed out after %v", timeout)
	}
}

func (c *Client) readDownlinkNAS(timeout time.Duration) ([]byte, uint32, error) {
	pdu, err := c.readPDU(timeout)
	if err != nil {
		return nil, 0, err
	}
	if pdu.ProcedureCode != s1ap.ProcDownlinkNASTransport {
		return nil, 0, fmt.Errorf("expected Downlink NAS Transport, got proc=%d", pdu.ProcedureCode)
	}
	ies, err := s1ap.DecodeProtocolIEContainer(pdu.Value)
	if err != nil {
		return nil, 0, fmt.Errorf("decode IEs: %w", err)
	}
	dl, err := s1ap.DecodeDownlinkNASTransport(ies)
	if err != nil {
		return nil, 0, fmt.Errorf("decode Downlink NAS: %w", err)
	}
	return dl.NASPDU, dl.MMEUES1APID, nil
}

func (c *Client) sendUplinkNAS(nasPDU []byte) error {
	msg := &s1ap.UplinkNASTransport{
		MMEUES1APID: c.mmeUES1ID,
		ENBUES1APID: c.enbUES1ID,
		NASPDU:      nasPDU,
		TAI:         s1ap.TAI{PLMN: c.opts.PLMN, TAC: c.opts.TAC},
		ECGI:        s1ap.ECGI{PLMN: c.opts.PLMN, CellID: 0x0000001},
	}
	bytes, err := s1ap.EncodeUplinkNASTransport(msg)
	if err != nil {
		return fmt.Errorf("encode Uplink NAS: %w", err)
	}
	return c.assoc.Write(bytes, c.streamID)
}

// wrapUplink integrity-protects an uplink NAS message with direction=0,
// bearer=0, alg=128-EIA2 (per MME's defaults). UL count is the post-security
// uplink counter starting at 1 (Security Mode Complete is the first).
func wrapUplink(kNASint []byte, count uint32, plain []byte) ([]byte, error) {
	sn := uint8(count & 0xFF)
	macInput := append([]byte{sn}, plain...)
	mac, err := nas.NASIntegrityProtect(kNASint, count, 0, 0, macInput)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, 6+len(plain))
	out = append(out, uint8(nas.SecurityHeaderIntegrityProtectedCiphered<<4)|uint8(nas.EPSMobilityManagement))
	out = append(out, mac...)
	out = append(out, sn)
	out = append(out, plain...)
	return out, nil
}

func hexBytes16(s string) ([16]byte, error) {
	var out [16]byte
	b, err := hex.DecodeString(s)
	if err != nil {
		return out, err
	}
	if len(b) != 16 {
		return out, fmt.Errorf("expected 16 bytes, got %d", len(b))
	}
	copy(out[:], b)
	return out, nil
}

// emit sends one event tagged as nf="simulator".
func (c *Client) emit(cat events.Category, sev events.Severity, proto, msg string, payload any) {
	c.emitter.Emit(events.Event{
		JourneyID: c.journeyID,
		NF:        "simulator",
		Category:  cat,
		Severity:  sev,
		Protocol:  proto,
		Message:   msg,
		Payload:   payload,
	})
}

// PackPLMN packs a 5- or 6-digit "MCC+MNC" string into the 3-byte
// PLMN-Identity wire format. Useful for tests and CLIs.
func PackPLMN(s string) ([3]byte, error) {
	var out [3]byte
	if len(s) != 5 && len(s) != 6 {
		return out, fmt.Errorf("PLMN must be 5 or 6 digits, got %q", s)
	}
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return out, fmt.Errorf("PLMN has non-digit %q", s)
		}
	}
	mcc := s[:3]
	mnc := s[3:]
	out[0] = (mcc[1]-'0')<<4 | (mcc[0] - '0')
	if len(mnc) == 2 {
		out[1] = 0xF0 | (mcc[2] - '0')
		out[2] = (mnc[1]-'0')<<4 | (mnc[0] - '0')
	} else {
		out[1] = (mnc[2]-'0')<<4 | (mcc[2] - '0')
		out[2] = (mnc[1]-'0')<<4 | (mnc[0] - '0')
	}
	return out, nil
}

// Guard against accidental concurrent runs from the controller.
var runMu sync.Mutex

// RunExclusive serializes Run so two simulator invocations can't trample on
// each other. The dashboard controller calls this rather than Run directly.
func RunExclusive(ctx context.Context, opts Options, emitter events.Emitter, log logger.Logger) Result {
	runMu.Lock()
	defer runMu.Unlock()
	return Run(ctx, opts, emitter, log)
}
