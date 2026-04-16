package mme

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/qcore-project/qcore/pkg/config"
	"github.com/qcore-project/qcore/pkg/nas"
	"github.com/qcore-project/qcore/pkg/s1ap"
	"github.com/qcore-project/qcore/pkg/sctp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEndToEndAttachOverWire stands up a real MME (TCP transport), a mock HSS
// (httptest), and a mock eNB (sctp.Dial client) and runs the full Attach
// procedure through real S1AP encode/decode and the network. This exercises
// the same code path UERANSIM would use — minus kernel SCTP framing.
func TestEndToEndAttachOverWire(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in -short mode")
	}

	// --- Mock HSS ---
	const (
		testIMSI  = "001010000000001"
		testRAND  = "23553cbe9637a89d218ae64dae47bf35"
		testXRES  = "a54211d5e3ba50bf"
		testAUTN  = "55f328997700016b357843210000000e"
		testKASME = "d27e9d7c3f5abc1e8f0a7b6c2d4e8f9a1b3c5d7e9f0a1b2c3d4e5f6a7b8c9da0"
	)

	hss := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/health" {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintln(w, `{"status":"ok"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]string{
				"rand": testRAND, "xres": testXRES, "autn": testAUTN, "kasme": testKASME,
			},
		})
	}))
	defer hss.Close()

	// --- Real MME, listening on a random local port (TCP fallback transport) ---
	port := pickFreePort(t)
	cfg := &config.MMEConfig{
		Name:        "qcore-mme-it",
		BindAddress: "127.0.0.1",
		S1APPort:    port,
		APIPort:     port + 1,
		SCTPMode:    "tcp",
		PLMN:        "00101",
		HSSURL:      hss.URL,
		TAC:         1,
		MMEGroupID:  1,
		MMECode:     1,
		RelCapacity: 127,
	}
	plmn := [3]byte{0x00, 0xF1, 0x10}
	s6a := NewS6aClient(hss.URL, noopLogger{})
	mme := New(cfg, plmn, noopLogger{}, nil, s6a)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, mme.Start(ctx))
	defer mme.Stop()

	// --- Mock eNB connects and runs the attach flow ---
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	assoc, err := dialWithRetry(addr, 2*time.Second)
	require.NoError(t, err, "mock eNB failed to connect to MME")
	defer assoc.Close()

	const enbUES1APID uint32 = 7
	const streamID uint16 = 0

	// Step 1: S1 SETUP REQUEST
	s1Setup := &s1ap.S1SetupRequest{
		GlobalENBID: s1ap.GlobalENBID{
			PLMN: plmn, ENBID: 0xABCDE, Type: s1ap.MacroENBID,
		},
		ENBName: "mock-enb",
		SupportedTAs: []s1ap.SupportedTA{
			{TAC: 1, PLMNs: [][3]byte{plmn}},
		},
		PagingDRX: 1, // v64
	}
	s1SetupBytes, err := s1ap.EncodeS1SetupRequest(s1Setup)
	require.NoError(t, err)
	require.NoError(t, assoc.Write(s1SetupBytes, streamID))

	// Receive S1 SETUP RESPONSE
	pduBytes, _, err := readWithTimeout(assoc, 2*time.Second)
	require.NoError(t, err)
	pdu, err := s1ap.DecodePDU(pduBytes)
	require.NoError(t, err)
	require.Equal(t, s1ap.PDUSuccessfulOutcome, pdu.Type, "expected S1 SETUP RESPONSE")
	require.Equal(t, s1ap.ProcS1Setup, pdu.ProcedureCode)

	// Step 2: INITIAL UE MESSAGE with NAS ATTACH REQUEST
	encodedIMSI, err := nas.EncodeIMSI(testIMSI)
	require.NoError(t, err)
	nasBody := append([]byte{
		uint8(nas.SecurityHeaderPlainNAS<<4) | uint8(nas.EPSMobilityManagement),
		uint8(nas.MsgTypeAttachRequest),
		0x71,                       // KSI=7 | attach type=1
		uint8(len(encodedIMSI)),    // mobile identity LV length
	}, encodedIMSI...)
	nasBody = append(nasBody, 0x02, 0xE0, 0xE0)                   // UE network capability LV
	nasBody = append(nasBody, 0x00, 0x05, 0xD0, 0x11, 0x27, 0x1D, 0x31) // ESM container LV-E

	initUE := &s1ap.InitialUEMessage{
		ENBUES1APID: enbUES1APID,
		NASPDU:      nasBody,
		TAI:         s1ap.TAI{PLMN: plmn, TAC: 1},
		ECGI:        s1ap.ECGI{PLMN: plmn, CellID: 0x0000001},
		RRCCause:    s1ap.RRCMoSignalling,
	}
	initUEBytes, err := s1ap.EncodeInitialUEMessage(initUE)
	require.NoError(t, err)
	require.NoError(t, assoc.Write(initUEBytes, streamID))

	// Receive DOWNLINK NAS TRANSPORT carrying AUTH REQUEST
	dlNAS, mmeUEID := readDownlinkNAS(t, assoc)
	dlH, _, err := nas.ParseHeader(dlNAS)
	require.NoError(t, err)
	require.Equal(t, nas.MsgTypeAuthenticationRequest, dlH.MessageType)

	// Step 3: send NAS AUTH RESPONSE with the correct RES = XRES from the mock HSS
	xresBytes, err := nas.HexToBytes(testXRES)
	require.NoError(t, err)
	authRespNAS := buildAuthResponseNAS(xresBytes)
	ulMsg := &s1ap.UplinkNASTransport{
		MMEUES1APID: mmeUEID,
		ENBUES1APID: enbUES1APID,
		NASPDU:      authRespNAS,
		TAI:         s1ap.TAI{PLMN: plmn, TAC: 1},
		ECGI:        s1ap.ECGI{PLMN: plmn, CellID: 0x0000001},
	}
	ulBytes, err := s1ap.EncodeUplinkNASTransport(ulMsg)
	require.NoError(t, err)
	require.NoError(t, assoc.Write(ulBytes, streamID))

	// Receive DOWNLINK NAS TRANSPORT carrying SECURITY MODE COMMAND
	dlNAS2, _ := readDownlinkNAS(t, assoc)
	secH, _, err := nas.ParseHeader(dlNAS2)
	require.NoError(t, err)
	assert.Equal(t, nas.MsgTypeSecurityModeCommand, secH.MessageType)
	assert.Equal(t, nas.SecurityHeaderIntegrityProtectedNewCtx, secH.SecurityHeader)

	// Step 4: send NAS SECURITY MODE COMPLETE.
	// MAC must be computed with direction=0 (uplink). The MME initialised ULCount=1
	// (it counted the AUTH RESPONSE), so SECURITY MODE COMPLETE is the second uplink
	// NAS message and must use COUNT=1.
	secModeCompletePlain := []byte{
		uint8(nas.SecurityHeaderPlainNAS<<4) | uint8(nas.EPSMobilityManagement),
		uint8(nas.MsgTypeSecurityModeComplete),
	}
	kasme, _ := nas.HexToBytes(testKASME)
	kNASint, err := nas.DeriveKNASint(kasme, 2)
	require.NoError(t, err)
	wrappedSMC, err := wrapNASUplink(kNASint, 1, nas.SecurityHeaderIntegrityProtectedCiphered, secModeCompletePlain)
	require.NoError(t, err)

	ulMsg.NASPDU = wrappedSMC
	ulBytes, err = s1ap.EncodeUplinkNASTransport(ulMsg)
	require.NoError(t, err)
	require.NoError(t, assoc.Write(ulBytes, streamID))

	// Receive INITIAL CONTEXT SETUP REQUEST (carries embedded ATTACH ACCEPT)
	pduBytes, _, err = readWithTimeout(assoc, 2*time.Second)
	require.NoError(t, err)
	pdu, err = s1ap.DecodePDU(pduBytes)
	require.NoError(t, err)
	assert.Equal(t, s1ap.PDUInitiatingMessage, pdu.Type)
	assert.Equal(t, s1ap.ProcInitialContextSetup, pdu.ProcedureCode)

	// Step 5: send INITIAL CONTEXT SETUP RESPONSE
	icsResp := &s1ap.InitialContextSetupResponse{
		MMEUES1APID: mmeUEID,
		ENBUES1APID: enbUES1APID,
	}
	icsRespBytes, err := s1ap.EncodeInitialContextSetupResponse(icsResp)
	require.NoError(t, err)
	require.NoError(t, assoc.Write(icsRespBytes, streamID))

	// Step 6: send NAS ATTACH COMPLETE (wrapped)
	attachCompletePlain := []byte{
		uint8(nas.SecurityHeaderPlainNAS<<4) | uint8(nas.EPSMobilityManagement),
		uint8(nas.MsgTypeAttachComplete),
		0x00, 0x00, // dummy ESM container
	}
	wrappedAC, err := wrapNASUplink(kNASint, 2, nas.SecurityHeaderIntegrityProtectedCiphered, attachCompletePlain)
	require.NoError(t, err)
	ulMsg.NASPDU = wrappedAC
	ulBytes, err = s1ap.EncodeUplinkNASTransport(ulMsg)
	require.NoError(t, err)
	require.NoError(t, assoc.Write(ulBytes, streamID))

	// Receive EMM INFORMATION (MME's response to ATTACH COMPLETE)
	dlNAS3, _ := readDownlinkNAS(t, assoc)
	infoH, _, err := nas.ParseHeader(dlNAS3)
	require.NoError(t, err)
	assert.Equal(t, nas.MsgTypeEMMInformation, infoH.MessageType)

	// Verify MME state: UE should be EMMRegistered with a PDN address.
	require.Eventually(t, func() bool {
		var ue *UEContext
		mme.ues.Range(func(_, v any) bool {
			ue = v.(*UEContext)
			return false
		})
		if ue == nil {
			return false
		}
		ue.mu.RLock()
		ok := ue.EMMState == EMMRegistered && ue.PDNAddr != ""
		ue.mu.RUnlock()
		return ok
	}, 1*time.Second, 20*time.Millisecond, "UE never reached EMMRegistered with PDN address")
}

// --- Test helpers ---

func pickFreePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	require.NoError(t, ln.Close())
	return port
}

func dialWithRetry(addr string, timeout time.Duration) (sctp.Association, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		assoc, err := sctp.Dial(sctp.ModeTCP, addr)
		if err == nil {
			return assoc, nil
		}
		lastErr = err
		time.Sleep(20 * time.Millisecond)
	}
	return nil, fmt.Errorf("dial %s after %v: %w", addr, timeout, lastErr)
}

func readWithTimeout(assoc sctp.Association, timeout time.Duration) ([]byte, uint16, error) {
	type result struct {
		data     []byte
		streamID uint16
		err      error
	}
	ch := make(chan result, 1)
	go func() {
		data, sid, err := assoc.Read()
		ch <- result{data, sid, err}
	}()
	select {
	case r := <-ch:
		return r.data, r.streamID, r.err
	case <-time.After(timeout):
		return nil, 0, fmt.Errorf("read timed out after %v", timeout)
	}
}

// readDownlinkNAS reads the next PDU, asserts it's a DOWNLINK NAS TRANSPORT,
// and returns the inner NAS PDU plus the MME-UE-S1AP-ID assigned by the MME.
func readDownlinkNAS(t *testing.T, assoc sctp.Association) (nasPDU []byte, mmeUEID uint32) {
	t.Helper()
	pduBytes, _, err := readWithTimeout(assoc, 2*time.Second)
	require.NoError(t, err)
	pdu, err := s1ap.DecodePDU(pduBytes)
	require.NoError(t, err)
	require.Equal(t, s1ap.ProcDownlinkNASTransport, pdu.ProcedureCode)
	ies, err := s1ap.DecodeProtocolIEContainer(pdu.Value)
	require.NoError(t, err)
	dl, err := s1ap.DecodeDownlinkNASTransport(ies)
	require.NoError(t, err)
	return dl.NASPDU, dl.MMEUES1APID
}

// wrapNASUplink mirrors nas.WrapNASWithIntegrity but uses direction=0 (uplink)
// so the MAC matches what the MME expects to verify on inbound NAS PDUs.
func wrapNASUplink(kNASint []byte, count uint32, secHeaderType nas.SecurityHeaderType, plainNAS []byte) ([]byte, error) {
	sn := uint8(count & 0xFF)
	macInput := append([]byte{sn}, plainNAS...)
	mac, err := nas.NASIntegrityProtect(kNASint, count, 0, 0, macInput) // direction=0 (uplink)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, 6+len(plainNAS))
	out = append(out, uint8(secHeaderType<<4)|uint8(nas.EPSMobilityManagement))
	out = append(out, mac...)
	out = append(out, sn)
	out = append(out, plainNAS...)
	return out, nil
}

// buildAuthResponseNAS builds a plain NAS AUTHENTICATION RESPONSE message
// carrying the given RES.
func buildAuthResponseNAS(res []byte) []byte {
	out := []byte{
		uint8(nas.SecurityHeaderPlainNAS<<4) | uint8(nas.EPSMobilityManagement),
		uint8(nas.MsgTypeAuthenticationResponse),
		uint8(len(res)),
	}
	return append(out, res...)
}
