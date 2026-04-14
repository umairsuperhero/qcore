package mme

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/qcore-project/qcore/pkg/config"
	"github.com/qcore-project/qcore/pkg/logger"
	"github.com/qcore-project/qcore/pkg/nas"
	"github.com/qcore-project/qcore/pkg/s1ap"
	"github.com/qcore-project/qcore/pkg/sctp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Test doubles ---

// fakeAssoc implements sctp.Association for testing.
// Writes are captured in a channel for inspection.
type fakeAssoc struct {
	mu      sync.Mutex
	written [][]byte
	remote  net.Addr
}

func newFakeAssoc() *fakeAssoc {
	return &fakeAssoc{remote: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1234}}
}

func (f *fakeAssoc) Read() ([]byte, uint16, error) {
	// Blocks forever; tests drive the handler directly.
	select {}
}

func (f *fakeAssoc) Write(data []byte, streamID uint16) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := make([]byte, len(data))
	copy(cp, data)
	f.written = append(f.written, cp)
	return nil
}

func (f *fakeAssoc) Close() error { return nil }

func (f *fakeAssoc) RemoteAddr() net.Addr { return f.remote }
func (f *fakeAssoc) LocalAddr() net.Addr  { return &net.TCPAddr{IP: net.ParseIP("0.0.0.0"), Port: 36412} }

func (f *fakeAssoc) writtenCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.written)
}

func (f *fakeAssoc) lastWritten() []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.written) == 0 {
		return nil
	}
	return f.written[len(f.written)-1]
}

// noopListener satisfies sctp.Listener.
type noopListener struct{}

func (noopListener) Accept() (sctp.Association, error) { return nil, fmt.Errorf("closed") }
func (noopListener) Close() error                      { return nil }
func (noopListener) Addr() net.Addr                    { return &net.TCPAddr{} }

// noopLogger discards all log output.
type noopLogger struct{}

func (noopLogger) Debug(...interface{})                {}
func (noopLogger) Info(...interface{})                 {}
func (noopLogger) Warn(...interface{})                 {}
func (noopLogger) Error(...interface{})                {}
func (noopLogger) Fatal(...interface{})                {}
func (noopLogger) Debugf(string, ...interface{})       {}
func (noopLogger) Infof(string, ...interface{})        {}
func (noopLogger) Warnf(string, ...interface{})        {}
func (noopLogger) Errorf(string, ...interface{})       {}
func (noopLogger) Fatalf(string, ...interface{})       {}
func (noopLogger) WithField(string, interface{}) logger.Logger        { return noopLogger{} }
func (noopLogger) WithFields(map[string]interface{}) logger.Logger    { return noopLogger{} }
func (noopLogger) WithError(error) logger.Logger                      { return noopLogger{} }
func (noopLogger) Writer() io.Writer                                  { return io.Discard }

// newTestMME creates a minimal MME for unit testing. No SCTP listener is started.
func newTestMME(t *testing.T, hssURL string) (*MME, *fakeAssoc) {
	t.Helper()

	cfg := &config.MMEConfig{
		Name:        "qcore-mme-test",
		BindAddress: "0.0.0.0",
		S1APPort:    36412,
		APIPort:     8081,
		SCTPMode:    "tcp",
		PLMN:        "00101",
		HSSURL:      hssURL,
		TAC:         1,
		MMEGroupID:  1,
		MMECode:     1,
		RelCapacity: 127,
	}
	plmn := [3]byte{0x00, 0xF1, 0x10} // MCC=001, MNC=01
	log := noopLogger{}
	s6a := NewS6aClient(hssURL, log)
	mme := New(cfg, plmn, log, nil, s6a)
	mme.listener = noopListener{} // prevent nil panics

	assoc := newFakeAssoc()
	return mme, assoc
}

// newTestEnb creates an EnbContext backed by a fake association.
func newTestEnb(assoc *fakeAssoc) *EnbContext {
	return &EnbContext{
		Assoc: assoc,
		GlobalENBID: GlobalENBID{
			PLMN:  [3]byte{0x00, 0xF1, 0x10},
			ENBID: 0x12345,
			Type:  MacroENB,
		},
		ENBName: "test-enb",
		SupportedTAs: []SupportedTA{
			{TAC: 0x0001, PLMNs: [][3]byte{{0x00, 0xF1, 0x10}}},
		},
	}
}

// buildS1SetupRequestIEs builds ProtocolIE slice for S1 SETUP REQUEST.
func buildS1SetupRequestIEs(t *testing.T, plmn [3]byte, enbID uint32, tac uint16) []s1ap.ProtocolIE {
	t.Helper()

	genbID := s1ap.GlobalENBID{PLMN: plmn, ENBID: enbID, Type: s1ap.MacroENBID}
	genbIDBytes, err := s1ap.EncodeGlobalENBID(genbID)
	require.NoError(t, err)

	tas := []s1ap.SupportedTA{{TAC: tac, PLMNs: [][3]byte{plmn}}}
	tasBytes, err := s1ap.EncodeSupportedTAs(tas)
	require.NoError(t, err)

	nameEnc := s1ap.NewPEREncoder()
	require.NoError(t, nameEnc.PutOctetString([]byte("test-enb")))

	drxEnc := s1ap.NewPEREncoder()
	require.NoError(t, drxEnc.PutConstrainedInt(1, 0, 3))

	return []s1ap.ProtocolIE{
		{ID: s1ap.IEID_Global_ENB_ID, Criticality: s1ap.CriticalityReject, Value: genbIDBytes},
		{ID: s1ap.IEID_ENBname, Criticality: s1ap.CriticalityIgnore, Value: nameEnc.Bytes()},
		{ID: s1ap.IEID_SupportedTAs, Criticality: s1ap.CriticalityReject, Value: tasBytes},
		{ID: s1ap.IEID_DefaultPagingDRX, Criticality: s1ap.CriticalityIgnore, Value: drxEnc.Bytes()},
	}
}

// buildInitialUEMessageIEs builds the ProtocolIE slice for INITIAL UE MESSAGE
// carrying an ATTACH REQUEST NAS PDU.
func buildInitialUEMessageIEs(t *testing.T, enbUEID uint32, imsi string, plmn [3]byte, tac uint16) []s1ap.ProtocolIE {
	t.Helper()

	// Build NAS ATTACH REQUEST
	encodedIMSI, err := nas.EncodeIMSI(imsi)
	require.NoError(t, err)

	nasBody := make([]byte, 0, 32)
	nasBody = append(nasBody,
		// NAS header: plain EMM
		uint8(nas.SecurityHeaderPlainNAS<<4)|uint8(nas.EPSMobilityManagement),
		uint8(nas.MsgTypeAttachRequest),
		// KSI=7 (no key), attach type=1 (EPS only)
		0x71,
		// EPS mobile identity (LV)
		uint8(len(encodedIMSI)),
	)
	nasBody = append(nasBody, encodedIMSI...)
	// UE network capability (LV)
	nasBody = append(nasBody, 0x02, 0xE0, 0xE0)
	// ESM message container (LV-E) - minimal
	nasBody = append(nasBody, 0x00, 0x05, 0xD0, 0x11, 0x27, 0x1D, 0x31)

	// Build S1AP IEs
	enbIDEnc := s1ap.NewPEREncoder()
	require.NoError(t, enbIDEnc.PutConstrainedInt(int64(enbUEID), 0, 0x00FFFFFF))

	nasEnc := s1ap.NewPEREncoder()
	require.NoError(t, nasEnc.PutOctetString(nasBody))

	tai := s1ap.TAI{PLMN: plmn, TAC: tac}
	taiBytes, err := s1ap.EncodeTAI(tai)
	require.NoError(t, err)

	ecgi := s1ap.ECGI{PLMN: plmn, CellID: 0x0000001}
	ecgiBytes, err := s1ap.EncodeECGI(ecgi)
	require.NoError(t, err)

	causeEnc := s1ap.NewPEREncoder()
	require.NoError(t, causeEnc.PutConstrainedInt(int64(s1ap.RRCMoSignalling), 0, 5))

	return []s1ap.ProtocolIE{
		{ID: s1ap.IEID_ENB_UE_S1AP_ID, Criticality: s1ap.CriticalityReject, Value: enbIDEnc.Bytes()},
		{ID: s1ap.IEID_NAS_PDU, Criticality: s1ap.CriticalityReject, Value: nasEnc.Bytes()},
		{ID: s1ap.IEID_TAI, Criticality: s1ap.CriticalityIgnore, Value: taiBytes},
		{ID: s1ap.IEID_EUTRAN_CGI, Criticality: s1ap.CriticalityIgnore, Value: ecgiBytes},
		{ID: s1ap.IEID_RRC_Establishment_Cause, Criticality: s1ap.CriticalityIgnore, Value: causeEnc.Bytes()},
	}
}

// mockHSS starts a test HTTP server that returns a canned auth vector.
// Uses 3GPP TS 35.208 Milenage test set 2 values (hex-encoded).
func mockHSS(t *testing.T) (*httptest.Server, string, string) {
	t.Helper()

	// Hardcoded test auth vector. All counts are verified:
	//   RAND:  32 hex chars = 16 bytes ✓
	//   XRES:  16 hex chars =  8 bytes ✓
	//   AUTN:  32 hex chars = 16 bytes ✓
	//   KASME: 64 hex chars = 32 bytes ✓
	const (
		testRAND  = "23553cbe9637a89d218ae64dae47bf35"
		testXRES  = "a54211d5e3ba50bf"
		testAUTN  = "55f328997700016b357843210000000e"
		testKASME = "d27e9d7c3f5abc1e8f0a7b6c2d4e8f9a1b3c5d7e9f0a1b2c3d4e5f6a7b8c9da0"
	)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/health" {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintln(w, `{"status":"ok"}`)
			return
		}
		resp := map[string]interface{}{
			"data": map[string]string{
				"rand":  testRAND,
				"xres":  testXRES,
				"autn":  testAUTN,
				"kasme": testKASME,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))

	return ts, testXRES, testKASME
}

// --- Tests ---

func TestHandleS1Setup_Success(t *testing.T) {
	mme, assoc := newTestMME(t, "http://localhost:9999")
	enb := &EnbContext{Assoc: assoc}
	plmn := [3]byte{0x00, 0xF1, 0x10}

	ies := buildS1SetupRequestIEs(t, plmn, 0x12345, 0x0001)
	mme.handleS1Setup(context.Background(), enb, ies, 0)

	// Should have sent exactly one message (S1 SETUP RESPONSE)
	assert.Equal(t, 1, assoc.writtenCount())

	// Verify it's a Successful Outcome PDU for S1 Setup
	pdu, err := s1ap.DecodePDU(assoc.lastWritten())
	require.NoError(t, err)
	assert.Equal(t, s1ap.PDUSuccessfulOutcome, pdu.Type)
	assert.Equal(t, s1ap.ProcS1Setup, pdu.ProcedureCode)

	// eNB context should be populated
	enb.mu.RLock()
	defer enb.mu.RUnlock()
	assert.Equal(t, "test-enb", enb.ENBName)
	assert.Equal(t, uint32(0x12345), enb.GlobalENBID.ENBID)
}

func TestHandleS1Setup_PLMNMismatch(t *testing.T) {
	mme, assoc := newTestMME(t, "http://localhost:9999")
	enb := &EnbContext{Assoc: assoc}
	// eNB advertises a PLMN that doesn't match the MME's PLMN
	wrongPLMN := [3]byte{0x02, 0xF8, 0x39}

	ies := buildS1SetupRequestIEs(t, wrongPLMN, 0x99999, 0x0002)
	mme.handleS1Setup(context.Background(), enb, ies, 0)

	// Should have sent exactly one message (S1 SETUP FAILURE)
	assert.Equal(t, 1, assoc.writtenCount())

	pdu, err := s1ap.DecodePDU(assoc.lastWritten())
	require.NoError(t, err)
	assert.Equal(t, s1ap.PDUUnsuccessfulOutcome, pdu.Type)
	assert.Equal(t, s1ap.ProcS1Setup, pdu.ProcedureCode)
}

func TestHandleInitialUEMessage_SendsAuthRequest(t *testing.T) {
	ts, _, _ := mockHSS(t)
	defer ts.Close()

	mme, assoc := newTestMME(t, ts.URL)
	enb := newTestEnb(assoc)

	const imsi = "001010000000001"
	plmn := [3]byte{0x00, 0xF1, 0x10}
	ies := buildInitialUEMessageIEs(t, 42, imsi, plmn, 0x0001)

	mme.handleInitialUEMessage(context.Background(), enb, ies, 0)

	// Should have sent one message: DOWNLINK NAS TRANSPORT (AUTH REQUEST)
	require.Equal(t, 1, assoc.writtenCount(), "expected AUTH REQUEST to be sent")

	// Verify it's a DownlinkNASTransport PDU
	pdu, err := s1ap.DecodePDU(assoc.lastWritten())
	require.NoError(t, err)
	assert.Equal(t, s1ap.PDUInitiatingMessage, pdu.Type)
	assert.Equal(t, s1ap.ProcDownlinkNASTransport, pdu.ProcedureCode)

	// Verify NAS AUTH REQUEST is inside
	ies2, err := s1ap.DecodeProtocolIEContainer(pdu.Value)
	require.NoError(t, err)
	ulMsg, err := s1ap.DecodeUplinkNASTransport(ies2) // reuse decoder for ID+NAS extraction
	require.NoError(t, err)
	require.NotEmpty(t, ulMsg.NASPDU)

	h, _, err := nas.ParseHeader(ulMsg.NASPDU)
	require.NoError(t, err)
	assert.Equal(t, nas.MsgTypeAuthenticationRequest, h.MessageType)

	// UE context should be stored
	assert.Equal(t, 1, mme.GetUECount())

	// UE context should have IMSI and XRES populated
	var ue *UEContext
	mme.ues.Range(func(_, v any) bool {
		ue = v.(*UEContext)
		return false
	})
	require.NotNil(t, ue)
	assert.Equal(t, imsi, ue.IMSI)
	assert.NotEmpty(t, ue.XRES)
	assert.NotEmpty(t, ue.KASME)
}

func TestFullAttachFlow(t *testing.T) {
	ts, xresHex, _ := mockHSS(t)
	defer ts.Close()

	mme, assoc := newTestMME(t, ts.URL)
	enb := newTestEnb(assoc)

	const imsi = "001010000000002"
	plmn := [3]byte{0x00, 0xF1, 0x10}

	// Step 1: Initial UE Message (ATTACH REQUEST)
	ies := buildInitialUEMessageIEs(t, 1, imsi, plmn, 0x0001)
	mme.handleInitialUEMessage(context.Background(), enb, ies, 0)
	require.Equal(t, 1, assoc.writtenCount(), "AUTH REQUEST not sent")

	// Get the allocated MME-UE-S1AP-ID
	var ue *UEContext
	mme.ues.Range(func(_, v any) bool {
		ue = v.(*UEContext)
		return false
	})
	require.NotNil(t, ue)

	// Step 2: AUTH RESPONSE (simulate UE sending correct RES)
	xresBytes, err := nas.HexToBytes(xresHex)
	require.NoError(t, err)

	authRespBody := append([]byte{uint8(len(xresBytes))}, xresBytes...)
	mme.handleAuthResponse(ue, authRespBody, 0)

	// Should now have sent SECURITY MODE COMMAND
	require.Equal(t, 2, assoc.writtenCount(), "SECURITY MODE COMMAND not sent")

	// Verify SEC MODE CMD is integrity-protected
	pdu, err := s1ap.DecodePDU(assoc.lastWritten())
	require.NoError(t, err)
	assert.Equal(t, s1ap.ProcDownlinkNASTransport, pdu.ProcedureCode)

	pduIEs, err := s1ap.DecodeProtocolIEContainer(pdu.Value)
	require.NoError(t, err)
	dlMsg, err := s1ap.DecodeUplinkNASTransport(pduIEs)
	require.NoError(t, err)

	secH, _, err := nas.ParseHeader(dlMsg.NASPDU)
	require.NoError(t, err)
	assert.Equal(t, nas.MsgTypeSecurityModeCommand, secH.MessageType)
	// Outer security header should indicate integrity protection
	assert.Equal(t, nas.SecurityHeaderIntegrityProtectedNewCtx, secH.SecurityHeader)

	// UE security context should be established
	ue.mu.RLock()
	hasSecCtx := ue.SecurityCtx != nil
	ue.mu.RUnlock()
	assert.True(t, hasSecCtx, "security context should be established after AUTH RESPONSE")

	// Step 3: SECURITY MODE COMPLETE → triggers ATTACH ACCEPT
	mme.handleSecurityModeComplete(ue, nil, 0)

	// Should now have sent ATTACH ACCEPT
	require.Equal(t, 3, assoc.writtenCount(), "ATTACH ACCEPT not sent")

	pdu3, err := s1ap.DecodePDU(assoc.lastWritten())
	require.NoError(t, err)
	assert.Equal(t, s1ap.ProcDownlinkNASTransport, pdu3.ProcedureCode)

	pduIEs3, err := s1ap.DecodeProtocolIEContainer(pdu3.Value)
	require.NoError(t, err)
	dlMsg3, err := s1ap.DecodeUplinkNASTransport(pduIEs3)
	require.NoError(t, err)

	acceptH, _, err := nas.ParseHeader(dlMsg3.NASPDU)
	require.NoError(t, err)
	assert.Equal(t, nas.MsgTypeAttachAccept, acceptH.MessageType)

	// UE should be marked registered with a PDN address
	ue.mu.RLock()
	emmState := ue.EMMState
	pdnAddr := ue.PDNAddr
	ue.mu.RUnlock()
	assert.Equal(t, EMMRegistered, emmState)
	assert.NotEmpty(t, pdnAddr)
	assert.NotEqual(t, "0.0.0.0", pdnAddr)

	// Step 4: ATTACH COMPLETE
	mme.handleAttachComplete(ue)
	// No new message sent, just state acknowledgment
	assert.Equal(t, 3, assoc.writtenCount())
}

func TestAllocatePDNAddress(t *testing.T) {
	mme, _ := newTestMME(t, "http://localhost:9999")

	addrs := make(map[string]bool)
	for i := 0; i < 10; i++ {
		addr := mme.allocatePDNAddress()
		ip := net.ParseIP(addr)
		assert.NotNil(t, ip, "allocated address should be valid IP: %s", addr)
		assert.False(t, addrs[addr], "duplicate address allocated: %s", addr)
		addrs[addr] = true
	}
}

func TestGetUEENBCounts(t *testing.T) {
	mme, _ := newTestMME(t, "http://localhost:9999")

	assert.Equal(t, 0, mme.GetUECount())
	assert.Equal(t, 0, mme.GetENBCount())

	mme.ues.Store(uint32(1), &UEContext{})
	mme.ues.Store(uint32(2), &UEContext{})
	assert.Equal(t, 2, mme.GetUECount())
}
