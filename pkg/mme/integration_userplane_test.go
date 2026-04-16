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
	"github.com/qcore-project/qcore/pkg/gtp"
	"github.com/qcore-project/qcore/pkg/nas"
	"github.com/qcore-project/qcore/pkg/s1ap"
	"github.com/qcore-project/qcore/pkg/spgw"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEndToEndUserPlane extends TestEndToEndAttachOverWire by plumbing in a
// real SPGW: MME talks to SPGW via HTTP S11, the SPGW allocates the UE IP and
// an SGW S1-U TEID, and after attach the mock eNB sends a GTP-U T-PDU to the
// SPGW and we assert the inner IP packet reaches the egress.
//
// This is the Phase 3 "ping at the protocol level works" milestone. Real TUN
// egress (actually ping-through to the internet) is a follow-on — gated by
// OS + privileges.
func TestEndToEndUserPlane(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in -short mode")
	}

	// --- SPGW ---
	s1uPort := pickFreeUDPPort(t)
	spgwCfg := &config.SPGWConfig{
		Name:        "qcore-spgw-it",
		BindAddress: "127.0.0.1",
		APIPort:     0, // httptest owns the API socket
		S1UPort:     s1uPort,
		UEPool:      "10.45.0.0/24",
		Gateway:     "10.45.0.1",
		SGWU1Addr:   "127.0.0.1",
		Egress:      "log",
	}
	spgwSvc, err := spgw.New(spgwCfg, noopLogger{})
	require.NoError(t, err)
	require.NoError(t, spgwSvc.Start())
	defer spgwSvc.Stop()

	spgwHTTP := httptest.NewServer(spgw.NewAPI(spgwSvc).Handler())
	defer spgwHTTP.Close()

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
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]string{
				"rand": testRAND, "xres": testXRES, "autn": testAUTN, "kasme": testKASME,
			},
		})
	}))
	defer hss.Close()

	// --- MME ---
	port := pickFreePort(t)
	cfg := &config.MMEConfig{
		Name:        "qcore-mme-it",
		BindAddress: "127.0.0.1",
		S1APPort:    port,
		APIPort:     port + 1,
		SCTPMode:    "tcp",
		PLMN:        "00101",
		HSSURL:      hss.URL,
		SPGWURL:     spgwHTTP.URL,
		TAC:         1,
		MMEGroupID:  1,
		MMECode:     1,
		RelCapacity: 127,
	}
	plmn := [3]byte{0x00, 0xF1, 0x10}
	s6a := NewS6aClient(hss.URL, noopLogger{})
	s11 := NewS11Client(spgwHTTP.URL, noopLogger{})
	require.NoError(t, s11.HealthCheck(), "SPGW should be reachable")

	m := New(cfg, plmn, noopLogger{}, nil, s6a, s11)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, m.Start(ctx))
	defer m.Stop()

	// --- Mock eNB ---
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	assoc, err := dialWithRetry(addr, 2*time.Second)
	require.NoError(t, err)
	defer assoc.Close()

	const enbUES1APID uint32 = 7
	const streamID uint16 = 0

	// S1 SETUP
	s1Setup := &s1ap.S1SetupRequest{
		GlobalENBID:  s1ap.GlobalENBID{PLMN: plmn, ENBID: 0xABCDE, Type: s1ap.MacroENBID},
		ENBName:      "mock-enb-u",
		SupportedTAs: []s1ap.SupportedTA{{TAC: 1, PLMNs: [][3]byte{plmn}}},
		PagingDRX:    1,
	}
	s1SetupBytes, err := s1ap.EncodeS1SetupRequest(s1Setup)
	require.NoError(t, err)
	require.NoError(t, assoc.Write(s1SetupBytes, streamID))
	_, _, err = readWithTimeout(assoc, 2*time.Second)
	require.NoError(t, err)

	// INITIAL UE MESSAGE (Attach Request)
	encodedIMSI, err := nas.EncodeIMSI(testIMSI)
	require.NoError(t, err)
	nasBody := append([]byte{
		uint8(nas.SecurityHeaderPlainNAS<<4) | uint8(nas.EPSMobilityManagement),
		uint8(nas.MsgTypeAttachRequest),
		0x71,
		uint8(len(encodedIMSI)),
	}, encodedIMSI...)
	nasBody = append(nasBody, 0x02, 0xE0, 0xE0)
	nasBody = append(nasBody, 0x00, 0x05, 0xD0, 0x11, 0x27, 0x1D, 0x31)

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

	dlNAS, mmeUEID := readDownlinkNAS(t, assoc)
	dlH, _, err := nas.ParseHeader(dlNAS)
	require.NoError(t, err)
	require.Equal(t, nas.MsgTypeAuthenticationRequest, dlH.MessageType)

	// AUTH RESPONSE
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

	// SECURITY MODE COMMAND
	_, _ = readDownlinkNAS(t, assoc)

	// SECURITY MODE COMPLETE
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

	// INITIAL CONTEXT SETUP REQUEST
	pduBytes, _, err := readWithTimeout(assoc, 2*time.Second)
	require.NoError(t, err)
	pdu, err := s1ap.DecodePDU(pduBytes)
	require.NoError(t, err)
	require.Equal(t, s1ap.ProcInitialContextSetup, pdu.ProcedureCode)

	// At this point SPGW should have one session with a UE-IP allocated.
	require.Eventually(t, func() bool { return spgwSvc.Sessions().Count() == 1 },
		2*time.Second, 20*time.Millisecond, "SPGW never saw Create Session")

	// --- Mock eNB S1-U socket: we receive downlink GTP-U on this socket ---
	enbUDP, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	require.NoError(t, err)
	defer enbUDP.Close()
	enbUDPPort := enbUDP.LocalAddr().(*net.UDPAddr).Port

	// INITIAL CONTEXT SETUP RESPONSE — include an E-RABSetupResult so the MME
	// learns our eNB S1-U TEID and address and fires S11 Modify Bearer.
	const enbTEID uint32 = 0x11223344
	icsResp := &s1ap.InitialContextSetupResponse{
		MMEUES1APID: mmeUEID,
		ENBUES1APID: enbUES1APID,
		ERABs: []s1ap.ERABSetupResult{{
			ERABID:             5,
			TransportLayerAddr: net.ParseIP("127.0.0.1"),
			GTPTEID:            [4]byte{0x11, 0x22, 0x33, 0x44},
		}},
	}
	icsRespBytes, err := s1ap.EncodeInitialContextSetupResponse(icsResp)
	require.NoError(t, err)
	require.NoError(t, assoc.Write(icsRespBytes, streamID))

	// Wait for MME to push the eNB TEID through to the SPGW.
	require.Eventually(t, func() bool {
		snap := spgwSvc.Sessions().Snapshot()
		return len(snap) == 1 && snap[0].ENBTEID == enbTEID
	}, 2*time.Second, 20*time.Millisecond, "SPGW never saw Modify Bearer with eNB TEID")

	// Now construct and send an uplink GTP-U T-PDU.
	snap := spgwSvc.Sessions().Snapshot()
	require.Len(t, snap, 1)
	bearer := snap[0]
	ueIP := bearer.UEIP
	sgwTEID := bearer.SGWTEID

	innerPkt := makeICMPEchoIPv4(ueIP, net.ParseIP("8.8.8.8"), 1234, 1)
	gtpPkt, err := gtp.EncodeTPDU(sgwTEID, innerPkt)
	require.NoError(t, err)

	spgwS1UAddr := spgwSvc.Dataplane().LocalAddr()
	_, err = enbUDP.WriteTo(gtpPkt, spgwS1UAddr)
	require.NoError(t, err)

	// Assert the egress saw the uplink packet.
	logEgress, ok := spgwSvc.Egress().(*spgw.LogEgress)
	require.True(t, ok, "expected LogEgress for this test")
	require.Eventually(t, func() bool { return logEgress.Count() >= 1 },
		2*time.Second, 20*time.Millisecond, "egress never saw the uplink IP packet")

	// Sanity: ATTACH COMPLETE still flows cleanly so the UE ends up Registered.
	attachCompletePlain := []byte{
		uint8(nas.SecurityHeaderPlainNAS<<4) | uint8(nas.EPSMobilityManagement),
		uint8(nas.MsgTypeAttachComplete),
		0x00, 0x00,
	}
	wrappedAC, err := wrapNASUplink(kNASint, 2, nas.SecurityHeaderIntegrityProtectedCiphered, attachCompletePlain)
	require.NoError(t, err)
	ulMsg.NASPDU = wrappedAC
	ulBytes, err = s1ap.EncodeUplinkNASTransport(ulMsg)
	require.NoError(t, err)
	require.NoError(t, assoc.Write(ulBytes, streamID))
	_, _ = readDownlinkNAS(t, assoc) // EMM INFORMATION

	assert.Equal(t, 1, spgwSvc.Sessions().Count(), "bearer still present after attach")
	_ = enbUDPPort // reserved for a follow-on downlink assertion (different S1-U port needed)

	t.Logf("Phase 3 user-plane test OK: UE-IP=%s SGW-TEID=0x%x eNB-TEID=0x%x uplink_packets=%d",
		ueIP, sgwTEID, enbTEID, logEgress.Count())
}

// pickFreeUDPPort binds :0, reads the port, and closes the socket. Not race-free,
// but good enough for tests — we rebind immediately.
func pickFreeUDPPort(t *testing.T) int {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	require.NoError(t, err)
	port := conn.LocalAddr().(*net.UDPAddr).Port
	require.NoError(t, conn.Close())
	return port
}

// makeICMPEchoIPv4 hand-builds a minimal IPv4 + ICMP echo-request packet.
// Not intended to be RFC-perfect — the SPGW treats it as opaque bytes.
func makeICMPEchoIPv4(src, dst net.IP, id, seq uint16) []byte {
	src = src.To4()
	dst = dst.To4()
	totalLen := 28 // 20 IP header + 8 ICMP header
	pkt := make([]byte, totalLen)
	pkt[0] = 0x45 // version 4 + IHL 5
	// TotalLen
	pkt[2] = byte(totalLen >> 8)
	pkt[3] = byte(totalLen & 0xff)
	// TTL + Proto (ICMP=1)
	pkt[8] = 64
	pkt[9] = 1
	copy(pkt[12:16], src)
	copy(pkt[16:20], dst)
	// ICMP echo request: type=8, code=0
	pkt[20] = 8
	pkt[21] = 0
	// Identifier + sequence
	pkt[24] = byte(id >> 8)
	pkt[25] = byte(id & 0xff)
	pkt[26] = byte(seq >> 8)
	pkt[27] = byte(seq & 0xff)
	return pkt
}
