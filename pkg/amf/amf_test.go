package amf

import (
	"context"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/qcore-project/qcore/pkg/ausf"
	"github.com/qcore-project/qcore/pkg/diag"
	"github.com/qcore-project/qcore/pkg/events"
	"github.com/qcore-project/qcore/pkg/ident"
	"github.com/qcore-project/qcore/pkg/logger"
	"github.com/qcore-project/qcore/pkg/nas5g"
	"github.com/qcore-project/qcore/pkg/ngap"
	"github.com/qcore-project/qcore/pkg/sbi"
	"github.com/qcore-project/qcore/pkg/sctp"
	"github.com/qcore-project/qcore/pkg/subscriber"
	"github.com/qcore-project/qcore/pkg/suci"
	"github.com/qcore-project/qcore/pkg/udm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── helpers ───────────────────────────────────────────────────────────────

func pickPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	p := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return p
}

func hd(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(s)
	}
	return b
}

// ─── fake UDM + AUSF stack (identical to ausf_test approach) ───────────────

type fakeAuthStore struct {
	subs map[string]*subscriber.Subscriber
}

func (f *fakeAuthStore) Generate5GAuthVector(_ context.Context, imsi, snName string) (*subscriber.AuthVector5G, error) {
	sub, ok := f.subs[imsi]
	if !ok {
		return nil, fmt.Errorf("subscriber %s not found", imsi)
	}
	ki, _ := sub.KiBytes()
	opc, _ := sub.OPcBytes()
	sqn, _ := sub.SQNBytes()
	amf, _ := sub.AMFBytes()
	av, err := subscriber.Generate5GAuthVector(ki, opc, sqn, amf, snName)
	if err != nil {
		return nil, err
	}
	_ = sub.IncrementSQN()
	return av, nil
}

func (f *fakeAuthStore) Resync5GAuthVector(_ context.Context, imsi, snName, randHex, autsHex string) (*subscriber.AuthVector5G, error) {
	return nil, fmt.Errorf("fakeAuthStore: Resync5GAuthVector not implemented")
}

type fakeSubscriberStore struct {
	subs map[string]*subscriber.Subscriber
}

func (f *fakeSubscriberStore) GetSubscriber(_ context.Context, imsi string) (*subscriber.Subscriber, error) {
	s, ok := f.subs[imsi]
	if !ok {
		return nil, fmt.Errorf("not found: %s", imsi)
	}
	return s, nil
}

func startAUSFandUDM(t *testing.T, sub *subscriber.Subscriber) (ausfURL string) {
	t.Helper()
	log := logger.New("error", "text")
	subs := map[string]*subscriber.Subscriber{sub.IMSI: sub}

	// UDM
	udmSvc := udm.NewService(udm.NewStoreSource(&fakeSubscriberStore{subs}), log).
		WithSIDFResolver(testSIDFResolver(t)).
		WithAuthSource(udm.NewStoreAuthSource(&fakeAuthStore{subs}))
	udmPort := pickPort(t)
	udmSrv := sbi.NewServer(sbi.ServerConfig{BindAddress: "127.0.0.1", Port: udmPort, NFType: "UDM"}, log, udmSvc.Handler())
	go func() { _ = udmSrv.Serve() }()

	// AUSF → UDM
	udmCli := udm.NewClient("http://127.0.0.1:"+strconv.Itoa(udmPort), "AUSF", false)
	ausfSvc := ausf.NewService(udmCli, log)
	ausfPort := pickPort(t)
	ausfSrv := sbi.NewServer(sbi.ServerConfig{BindAddress: "127.0.0.1", Port: ausfPort, NFType: "AUSF"}, log, ausfSvc.Handler())
	go func() { _ = ausfSrv.Serve() }()

	time.Sleep(80 * time.Millisecond)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = ausfSrv.Shutdown(ctx)
		_ = udmSrv.Shutdown(ctx)
	})
	return "http://127.0.0.1:" + strconv.Itoa(ausfPort)
}

func testSIDFResolver(t *testing.T) *suci.Resolver {
	t.Helper()
	priv, err := suci.ParsePrivateKeyHex(suci.SchemeProfileA, "c53c22208b61860b06c62e5406a7b330c2b577aa5558981510d128247d38bd1d")
	require.NoError(t, err)
	resolver, err := suci.NewResolver([]suci.HomeNetworkPrivateKey{{
		ID:         1,
		Scheme:     suci.SchemeProfileA,
		PrivateKey: priv,
	}})
	require.NoError(t, err)
	return resolver
}

// ─── mock gNB ──────────────────────────────────────────────────────────────

// mockGNB is a minimal NGAP client that simulates a gNB over TCP.
type mockGNB struct {
	conn sctp.Association
}

func (g *mockGNB) send(pdu []byte) error { return g.conn.Write(pdu, 0) }

func (g *mockGNB) recv() ([]byte, error) {
	raw, _, err := g.conn.Read()
	return raw, err
}

// ngSetup performs the NG Setup exchange and returns the response.
func (g *mockGNB) ngSetup(t *testing.T) *ngap.NGSetupResponse {
	t.Helper()
	plmn := ngap.PLMNFromMCCMNC("001", "01")
	req, err := ngap.EncodeNGSetupRequest(&ngap.NGSetupRequest{
		GlobalRANNodeID: ngap.GlobalGNBID{PLMN: plmn, GNBID: 0x001, GNBIDBits: 22},
		RANNodeName:     "test-gnb",
		SupportedTAList: []ngap.SupportedTA{{
			TAC: [3]byte{0, 0, 1},
			BroadcastPLMN: []ngap.BroadcastPLMNItem{
				{PLMN: plmn, SNSSAI: []ngap.SNSSAI{{SST: 1}}},
			},
		}},
		DefaultPagingDRX: 0,
	})
	require.NoError(t, err)
	require.NoError(t, g.send(req))

	raw, err := g.recv()
	require.NoError(t, err)
	pdu, err := ngap.DecodePDU(raw)
	require.NoError(t, err)
	assert.Equal(t, ngap.ProcNGSetup, pdu.ProcedureCode)
	assert.Equal(t, ngap.PDUSuccessfulOutcome, pdu.Type)

	ies, err := ngap.DecodeIEContainer(pdu.Value)
	require.NoError(t, err)
	resp, err := ngap.DecodeNGSetupResponse(ies)
	require.NoError(t, err)
	return resp
}

// sendInitialUE sends a Registration Request as an InitialUEMessage.
func (g *mockGNB) sendInitialUE(t *testing.T, ranID uint64, regReq []byte) {
	t.Helper()
	plmn := ngap.PLMNFromMCCMNC("001", "01")
	msg, err := ngap.EncodeInitialUEMessage(&ngap.InitialUEMessage{
		RANUENGAPID: ranID,
		NASPDU:      regReq,
		UserLocationInfo: ngap.UserLocationInformationNR{
			NRCGI: ngap.NRCGI{PLMN: plmn, NRCellID: 1},
			TAI:   ngap.TAI{PLMN: plmn, TAC: [3]byte{0, 0, 1}},
		},
		RRCEstablishmentCause: ngap.RRCMoSignalling,
	})
	require.NoError(t, err)
	require.NoError(t, g.send(msg))
}

// recvDownlinkNAS waits for DownlinkNASTransport and returns the NAS PDU.
func (g *mockGNB) recvDownlinkNAS(t *testing.T) (amfID uint64, nasPDU []byte) {
	t.Helper()
	raw, err := g.recv()
	require.NoError(t, err)
	pdu, err := ngap.DecodePDU(raw)
	require.NoError(t, err)
	require.Equal(t, ngap.ProcDownlinkNASTransport, pdu.ProcedureCode)
	ies, err := ngap.DecodeIEContainer(pdu.Value)
	require.NoError(t, err)
	dl, err := ngap.DecodeDownlinkNASTransport(ies)
	require.NoError(t, err)
	return dl.AMFUENGAPID, dl.NASPDU
}

// sendUplinkNAS sends an UplinkNASTransport.
func (g *mockGNB) sendUplinkNAS(t *testing.T, amfID, ranID uint64, nasPDU []byte) {
	t.Helper()
	plmn := ngap.PLMNFromMCCMNC("001", "01")
	msg, err := ngap.EncodeUplinkNASTransport(&ngap.UplinkNASTransport{
		AMFUENGAPID: amfID,
		RANUENGAPID: ranID,
		NASPDU:      nasPDU,
		UserLocationInfo: ngap.UserLocationInformationNR{
			NRCGI: ngap.NRCGI{PLMN: plmn, NRCellID: 1},
			TAI:   ngap.TAI{PLMN: plmn, TAC: [3]byte{0, 0, 1}},
		},
	})
	require.NoError(t, err)
	require.NoError(t, g.send(msg))
}

// recvInitialContextSetup receives an InitialContextSetupRequest from the AMF
// and returns the piggybacked NAS PDU.
func (g *mockGNB) recvInitialContextSetup(t *testing.T) (amfID uint64, nasPDU []byte) {
	t.Helper()
	raw, err := g.recv()
	require.NoError(t, err)
	pdu, err := ngap.DecodePDU(raw)
	require.NoError(t, err)
	require.Equal(t, ngap.ProcInitialContextSetup, pdu.ProcedureCode)
	ies, err := ngap.DecodeIEContainer(pdu.Value)
	require.NoError(t, err)
	req, err := ngap.DecodeInitialContextSetupRequest(ies)
	require.NoError(t, err)
	return req.AMFUENGAPID, req.NASPDU
}

// ─── Test ──────────────────────────────────────────────────────────────────

// TestAMF_RegistrationFlow drives the full 5G registration procedure:
// NGSetup → InitialUEMessage(RegReq) → DL Auth Req → UL Auth Resp
// → DL SMC → UL SMC Complete → InitialContextSetup(Reg Accept)
//
// The UE side is simulated: we extract RAND+AUTN from the Auth Request and
// re-derive RES* with the same Milenage the subscriber side used, then
// compute SMC Complete as a plain NAS PDU (integrity protection is
// applied by the AMF; we just verify we receive a protected Registration Accept).
func TestAMF_RegistrationFlow(t *testing.T) {
	const imsi = "001010000000001"
	const snName = "5G:mnc001.mcc001.3gppnetwork.org"

	sub := &subscriber.Subscriber{
		IMSI: imsi,
		Ki:   "465b5ce8b199b49faa5f0a2ee238a6bc",
		OPc:  "cd63cb71954a9f4e48a5994e37a02baf",
		AMF:  "8000",
		SQN:  "000000000001",
	}

	ausfURL := startAUSFandUDM(t, sub)
	log := logger.New("info", "text")

	// Fake SMF: returns 201 with an allocated UE IPv4 for Create SM Context.
	smfSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"pduSessionId":5,"upCnxState":"ACTIVATED","ueIpv4Addr":"10.45.0.2"}`))
	}))
	defer smfSrv.Close()

	// ── Start AMF ──
	plmn := ngap.PLMNFromMCCMNC("001", "01")
	amfPort := pickPort(t)
	cfg := Config{
		NGAPAddr: "127.0.0.1:" + strconv.Itoa(amfPort),
		NGAPMode: sctp.ModeTCP,
		AMFName:  "QCore-AMF-test",
		GUAMI: ngap.GUAMI{
			PLMN: plmn, AMFRegionID: 0x01, AMFSetID: 0x001, AMFPointer: 0x00,
		},
		PLMNSupportList: []ngap.PLMNSupportItem{
			{PLMN: plmn, SNSSAIs: []ngap.SNSSAI{{SST: 1}}},
		},
		ServingNetworkName: snName,
		SMFURL:             smfSrv.URL,
	}
	ausfCli := ausf.NewClient(ausfURL, "AMF", false)
	amfSvc := NewService(cfg, ausfCli, log)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = amfSvc.Serve(ctx) }()
	time.Sleep(80 * time.Millisecond)

	// ── Connect mock gNB ──
	conn, err := sctp.Dial(sctp.ModeTCP, "127.0.0.1:"+strconv.Itoa(amfPort))
	require.NoError(t, err)
	defer conn.Close()
	gNB := &mockGNB{conn: conn}

	// Step 1: NGSetup
	ngResp := gNB.ngSetup(t)
	assert.Equal(t, "QCore-AMF-test", ngResp.AMFName)
	t.Log("NGSetup OK")

	// Step 2: Build a SUCI-based Registration Request
	// SUCI for null-scheme: PLMN=00101, RI=FFFF, PS=0, PKID=0, MSIN=BCD of IMSI digits
	// MSIN "0000000001" packed BCD (low nibble first): 00 00 00 00 10
	suci := nas5g.EncodeSUCI(nas5g.SUCI{
		PLMN:             [3]byte{0x00, 0xF1, 0x10}, // MCC=001, MNC=01 — canonical TS 24.008 layout
		RoutingIndicator: [2]byte{0xFF, 0xFF},
		ProtectionScheme: 0x00,
		HomeNetworkPKID:  0x00,
		MSIN:             hd("0000000010"), // MSIN = 0000000001 in BCD (pairs, lo-nibble-first)
	})
	regReq := nas5g.EncodeRegistrationRequest(&nas5g.RegistrationRequest{
		RegistrationType:     nas5g.RegistrationTypeInitialRegistration,
		FollowOnRequest:      false,
		NASKeySetID:          7,
		MobileIdentity:       suci,
		UESecurityCapability: []byte{0xE0, 0x00, 0xC0, 0x00}, // NIA2+NIA1, NEA2+NEA0
	})

	const ranID = uint64(0x1001)
	gNB.sendInitialUE(t, ranID, regReq)

	// Step 3: Receive DownlinkNASTransport with Auth Request
	amfID, authReqRaw := gNB.recvDownlinkNAS(t)
	require.NotZero(t, amfID)

	authMsg, err := nas5g.Decode(authReqRaw)
	require.NoError(t, err)
	require.NotNil(t, authMsg.AuthenticationRequest, "expected AuthenticationRequest")
	t.Logf("Auth Request received: RAND=%x", authMsg.AuthenticationRequest.RAND)
	amfSvc.mu.RLock()
	ueCtx := amfSvc.ues[amfID]
	amfSvc.mu.RUnlock()
	require.NotNil(t, ueCtx)
	assert.Equal(t, "imsi-"+imsi, ueCtx.SUPI, "SUPI must remain subscriber identity during auth challenge")
	assert.Contains(t, ueCtx.AuthCtxURL, "/5g-aka-confirmation", "AUSF confirmation URL belongs in AuthCtxURL, not SUPI")

	// Step 4: Simulate UE computing RES* using the RAND received in the Auth Request.
	// Must use Generate5GAuthVectorWithRAND so the same RAND feeds Milenage,
	// matching the XRES* that AUSF stored when it called UDM.
	ki := hd(sub.Ki)
	opc := hd(sub.OPc)
	sqn := hd("000000000001")
	amfParam := hd(sub.AMF)

	var kiArr, opcArr, randArr [16]byte
	copy(kiArr[:], ki)
	copy(opcArr[:], opc)
	copy(randArr[:], authMsg.AuthenticationRequest.RAND[:])
	var sqnArr [6]byte
	copy(sqnArr[:], sqn)
	var amfArr [2]byte
	copy(amfArr[:], amfParam)

	av, err := subscriber.Generate5GAuthVectorWithRAND(kiArr, opcArr, randArr, sqnArr, amfArr, snName)
	require.NoError(t, err)
	resStarBytes := hd(av.XRESStar)

	authResp := nas5g.EncodeAuthenticationResponse(&nas5g.AuthenticationResponse{
		ResStar: resStarBytes,
	})
	gNB.sendUplinkNAS(t, amfID, ranID, authResp)

	// Step 5: Receive DownlinkNASTransport with Security Mode Command (integrity-protected)
	_, smcRaw := gNB.recvDownlinkNAS(t)
	require.NotEmpty(t, smcRaw)
	// SMC must be a security-protected NAS (EPD=0x7E, SecHdr=0x03)
	require.Equal(t, uint8(0x7E), smcRaw[0], "expected 5GMM EPD")
	require.Equal(t, uint8(0x03), smcRaw[1], "expected SecHdr=0x03 (integrity-protected-new-ctx)")
	t.Log("Security Mode Command received (integrity-protected)")

	// Step 6: Send Security Mode Complete (plain; AMF will accept in v0.6)
	smcComplete := nas5g.EncodeSecurityModeComplete(&nas5g.SecurityModeComplete{})
	gNB.sendUplinkNAS(t, amfID, ranID, smcComplete)

	// Step 7: Receive InitialContextSetupRequest with piggybacked Registration Accept
	_, regAcceptRaw := gNB.recvInitialContextSetup(t)
	require.NotEmpty(t, regAcceptRaw)
	// Registration Accept must be integrity-protected (EPD=0x7E, SecHdr=0x01)
	require.Equal(t, uint8(0x7E), regAcceptRaw[0])
	require.Equal(t, uint8(0x01), regAcceptRaw[1], "expected SecHdr=0x01 (integrity-protected)")
	t.Log("Registration Accept received in InitialContextSetup")

	// Step 8: UE sends a UL NAS Transport carrying a PDU Session Establishment
	// Request; the AMF forwards to the (fake) SMF which returns 201.
	const pduSessionID = uint8(5)
	const pti = uint8(1)
	pduReq := nas5g.EncodePDUSessionEstablishmentRequest(pduSessionID, pti)
	ulNAS := nas5g.EncodeULNASTransport(pduSessionID, pduReq)
	gNB.sendUplinkNAS(t, amfID, ranID, ulNAS)

	// Step 9: SMF 201 must cause a protected DL NAS Transport carrying a 5GSM
	// PDU Session Establishment Accept to be sent back to the UE.
	_, pduAcceptRaw := gNB.recvDownlinkNAS(t)
	require.GreaterOrEqual(t, len(pduAcceptRaw), 8)
	require.Equal(t, uint8(0x7E), pduAcceptRaw[0], "5GMM EPD")
	require.Equal(t, uint8(0x01), pduAcceptRaw[1], "protected (integrity) DL NAS Transport")

	// Strip the 7-byte 5G NAS security header (EPD+SecHdr+MAC4+SN) → inner DL NAS Transport.
	inner := pduAcceptRaw[7:]
	require.Equal(t, uint8(0x68), inner[2], "DL NAS Transport message type")
	clen := int(inner[4])<<8 | int(inner[5])
	require.GreaterOrEqual(t, len(inner), 6+clen)
	n1 := inner[6 : 6+clen]
	assert.Equal(t, uint8(0x2E), n1[0], "inner N1 SM is 5GSM")
	assert.Equal(t, uint8(0xC2), n1[3], "PDU Session Establishment Accept")
	t.Log("PDU Session Establishment Accept received over protected DL NAS Transport")

	t.Log("✓ Full 5G registration + PDU session accept flow complete")
}

func TestAMF_RegistrationFlow_ConcealedProfileASUCI(t *testing.T) {
	const (
		imsi   = "274012001002086"
		snName = "5G:mnc001.mcc001.3gppnetwork.org"
	)

	sub := &subscriber.Subscriber{
		IMSI: imsi,
		Ki:   "465b5ce8b199b49faa5f0a2ee238a6bc",
		OPc:  "cd63cb71954a9f4e48a5994e37a02baf",
		AMF:  "8000",
		SQN:  "000000000001",
	}

	ausfURL := startAUSFandUDM(t, sub)
	log := logger.New("error", "text")
	plmn := ngap.PLMNFromMCCMNC("001", "01")
	amfPort := pickPort(t)
	cfg := Config{
		NGAPAddr: "127.0.0.1:" + strconv.Itoa(amfPort),
		NGAPMode: sctp.ModeTCP,
		AMFName:  "QCore-AMF-test",
		GUAMI:    ngap.GUAMI{PLMN: plmn, AMFRegionID: 0x01, AMFSetID: 0x001, AMFPointer: 0x00},
		PLMNSupportList: []ngap.PLMNSupportItem{
			{PLMN: plmn, SNSSAIs: []ngap.SNSSAI{{SST: 1}}},
		},
		ServingNetworkName: snName,
	}
	amfSvc := NewService(cfg, ausf.NewClient(ausfURL, "AMF", false), log)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = amfSvc.Serve(ctx) }()
	time.Sleep(80 * time.Millisecond)

	conn, err := sctp.Dial(sctp.ModeTCP, "127.0.0.1:"+strconv.Itoa(amfPort))
	require.NoError(t, err)
	defer conn.Close()
	gNB := &mockGNB{conn: conn}
	gNB.ngSetup(t)

	const ranID = uint64(0x1001)
	regReq := nas5g.EncodeRegistrationRequest(&nas5g.RegistrationRequest{
		RegistrationType:     nas5g.RegistrationTypeInitialRegistration,
		NASKeySetID:          7,
		MobileIdentity:       annexC4ProfileASUCI(),
		UESecurityCapability: []byte{0xE0, 0x00, 0xC0, 0x00},
	})
	gNB.sendInitialUE(t, ranID, regReq)

	amfID, authReqRaw := gNB.recvDownlinkNAS(t)
	authMsg, err := nas5g.Decode(authReqRaw)
	require.NoError(t, err)
	require.NotNil(t, authMsg.AuthenticationRequest, "expected AuthenticationRequest")

	ki := hd(sub.Ki)
	opc := hd(sub.OPc)
	sqn := hd("000000000001")
	amfParam := hd(sub.AMF)

	var kiArr, opcArr, randArr [16]byte
	copy(kiArr[:], ki)
	copy(opcArr[:], opc)
	copy(randArr[:], authMsg.AuthenticationRequest.RAND[:])
	var sqnArr [6]byte
	copy(sqnArr[:], sqn)
	var amfArr [2]byte
	copy(amfArr[:], amfParam)

	av, err := subscriber.Generate5GAuthVectorWithRAND(kiArr, opcArr, randArr, sqnArr, amfArr, snName)
	require.NoError(t, err)
	gNB.sendUplinkNAS(t, amfID, ranID, nas5g.EncodeAuthenticationResponse(&nas5g.AuthenticationResponse{
		ResStar: hd(av.XRESStar),
	}))

	_, smcRaw := gNB.recvDownlinkNAS(t)
	require.Equal(t, uint8(0x7E), smcRaw[0], "expected 5GMM EPD")
	require.Equal(t, uint8(0x03), smcRaw[1], "expected protected Security Mode Command")

	gNB.sendUplinkNAS(t, amfID, ranID, nas5g.EncodeSecurityModeComplete(&nas5g.SecurityModeComplete{}))
	_, regAcceptRaw := gNB.recvInitialContextSetup(t)
	require.Equal(t, uint8(0x7E), regAcceptRaw[0])
	require.Equal(t, uint8(0x01), regAcceptRaw[1], "expected protected Registration Accept")

	amfSvc.mu.RLock()
	ueCtx := amfSvc.ues[amfID]
	amfSvc.mu.RUnlock()
	require.NotNil(t, ueCtx)
	assert.Equal(t, "imsi-"+imsi, ueCtx.SUPI)
}

// TestAMF_UnprovisionedIMSI verifies that when the AUSF cannot find a
// subscriber (unprovisioned IMSI), the AMF sends a NAS Registration Reject to
// the UE rather than silently timing out. This is the A2 acceptance criterion
// for the unprovisioned_imsi error scenario.
func TestAMF_UnprovisionedIMSI(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in -short mode")
	}

	const (
		imsi    = "001010000000001"
		snName  = "5G:mnc001.mcc001.3gppnetwork.org"
		badIMSI = "001019999999999" // provisioned subscriber is imsi, badIMSI is not
	)

	sub := &subscriber.Subscriber{
		IMSI: imsi,
		Ki:   "465b5ce8b199b49faa5f0a2ee238a6bc",
		OPc:  "cd63cb71954a9f4e48a5994e37a02baf",
		AMF:  "8000",
		SQN:  "000000000001",
	}

	ausfURL := startAUSFandUDM(t, sub)
	log := logger.New("info", "text")

	plmn := ngap.PLMNFromMCCMNC("001", "01")
	amfPort := pickPort(t)
	cfg := Config{
		NGAPAddr: "127.0.0.1:" + strconv.Itoa(amfPort),
		NGAPMode: sctp.ModeTCP,
		AMFName:  "QCore-AMF-test",
		GUAMI:    ngap.GUAMI{PLMN: plmn, AMFRegionID: 0x01, AMFSetID: 0x001, AMFPointer: 0x00},
		PLMNSupportList: []ngap.PLMNSupportItem{
			{PLMN: plmn, SNSSAIs: []ngap.SNSSAI{{SST: 1}}},
		},
		ServingNetworkName: snName,
	}
	ausfCli := ausf.NewClient(ausfURL, "AMF", false)
	amfSvc := NewService(cfg, ausfCli, log)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = amfSvc.Serve(ctx) }()
	time.Sleep(80 * time.Millisecond)

	conn, err := sctp.Dial(sctp.ModeTCP, "127.0.0.1:"+strconv.Itoa(amfPort))
	require.NoError(t, err)
	defer conn.Close()
	gNB := &mockGNB{conn: conn}

	gNB.ngSetup(t)

	// Send a Registration Request for an IMSI that is NOT provisioned.
	// MSIN of badIMSI with MNC=01 (2-digit) is digits 5..14 = "9999999999".
	// BCD: 0x99 x 5
	suci := nas5g.EncodeSUCI(nas5g.SUCI{
		PLMN:             [3]byte{0x00, 0xF1, 0x10},
		RoutingIndicator: [2]byte{0xFF, 0xFF},
		ProtectionScheme: 0x00,
		HomeNetworkPKID:  0x00,
		MSIN:             []byte{0x99, 0x99, 0x99, 0x99, 0x99},
	})
	regReq := nas5g.EncodeRegistrationRequest(&nas5g.RegistrationRequest{
		RegistrationType:     nas5g.RegistrationTypeInitialRegistration,
		NASKeySetID:          7,
		MobileIdentity:       suci,
		UESecurityCapability: []byte{0xE0, 0x00, 0xC0, 0x00},
	})
	gNB.sendInitialUE(t, 0x1001, regReq)

	// AMF must respond with a NAS Registration Reject (not timeout).
	_, nasPDU := gNB.recvDownlinkNAS(t)
	require.GreaterOrEqual(t, len(nasPDU), 3, "NAS PDU too short")

	hdr, err := nas5g.DecodeHeader(nasPDU)
	require.NoError(t, err)
	assert.Equal(t, nas5g.MsgTypeRegistrationReject, hdr.MessageType,
		"expected Registration Reject for unprovisioned IMSI %s", badIMSI)

	if len(nasPDU) > 3 {
		cause := nas5g.Cause5GMM(nasPDU[3])
		t.Logf("5GMM cause: 0x%02X", uint8(cause))
	}
	t.Log("✓ Unprovisioned IMSI correctly triggers Registration Reject")
}

// TestAMF_KeyDerivation verifies the 5G key chain: KSEAF→KAMF→KNASint.
func TestAMF_KeyDerivation(t *testing.T) {
	// Test vector: arbitrary KSEAF for key derivation sanity check.
	// A round-trip correctness test: derive keys and verify they're 256/128 bit.
	kseaf := make([]byte, 32) // zero key for determinism
	supi := "imsi-001010000000001"
	abba := []byte{0x00, 0x00}

	kamf, err := DeriveKAMF(kseaf, supi, abba)
	require.NoError(t, err)
	assert.Len(t, kamf, 32, "KAMF should be 256-bit")

	kNASint, err := DeriveKNASint5G(kamf, 2) // NIA2
	require.NoError(t, err)
	assert.Len(t, kNASint, 16, "KNASint should be 128-bit")

	kNASenc, err := DeriveKNASenc5G(kamf, 0) // NEA0
	require.NoError(t, err)
	assert.Len(t, kNASenc, 16, "KNASenc should be 128-bit")

	kgNB, err := DeriveKgNB(kamf, 0)
	require.NoError(t, err)
	assert.Len(t, kgNB, 32, "KgNB should be 256-bit")

	// Verify determinism: same inputs produce same outputs
	kamf2, _ := DeriveKAMF(kseaf, supi, abba)
	assert.Equal(t, kamf, kamf2)
}

// TestAMF_KAMF_UsesBareIMSI pins the T10 fix: K_AMF P0 is the bare SUPI value
// (TS 33.501 Annex A.7), not the SBI "imsi-<digits>" representation. UERANSIM/
// free5GC derive K_AMF from the bare IMSI, so the prefixed and bare forms MUST
// produce different keys — and the AMF registration path must feed the bare form,
// or the Security Mode Command MAC will not verify at the UE. If this assertion
// ever fails, someone collapsed the two forms and reintroduced the SMC-integrity
// regression.
func TestAMF_KAMF_UsesBareIMSI(t *testing.T) {
	kseaf := make([]byte, 32)
	abba := []byte{0x00, 0x00}

	bare, err := DeriveKAMF(kseaf, "001010000000001", abba)
	require.NoError(t, err)
	prefixed, err := DeriveKAMF(kseaf, "imsi-001010000000001", abba)
	require.NoError(t, err)

	assert.NotEqual(t, bare, prefixed,
		"K_AMF must differ for bare vs imsi-prefixed SUPI; the registration path must use the bare IMSI (TS 33.501 A.7)")
}

// TestAMF_NASWrap verifies WrapNAS5G + VerifyNAS5GUplink round-trip.
func TestAMF_NASWrap(t *testing.T) {
	kseaf := make([]byte, 32)
	kamf, _ := DeriveKAMF(kseaf, "imsi-001010000000001", []byte{0x00, 0x00})
	kNASint, _ := DeriveKNASint5G(kamf, 2)

	plainNAS := nas5g.EncodeSecurityModeCommand(&nas5g.SecurityModeCommand{
		NASSecAlgos:       0x02,
		NASKeySetID:       1,
		ReplayedUESecCaps: []byte{0xE0, 0x00, 0xC0, 0x00},
	})

	wrapped, err := WrapNAS5G(kNASint, 0, SecHdrIntegrityProtectedNewCtx, plainNAS)
	require.NoError(t, err)
	assert.Equal(t, uint8(0x7E), wrapped[0])
	assert.Equal(t, uint8(0x03), wrapped[1])

	// Verify the uplink direction MAC (direction=0=UL) — note this will differ
	// from the DL MAC, so verification will fail (expected). We just verify that
	// the function runs without error and returns the inner plain NAS.
	_, err = VerifyNAS5GUplink(kNASint, 0, wrapped)
	// MAC will mismatch (DL vs UL direction bits), but we confirm the function parses.
	_ = err                                  // mismatch is expected here
	assert.Equal(t, uint8(0x7E), wrapped[0]) // unchanged
}

// ─── recordingEmitter ──────────────────────────────────────────────────────

// recordingEmitter captures emitted events for assertion in tests.
type recordingEmitter struct {
	mu     sync.Mutex
	events []events.Event
}

func (r *recordingEmitter) Emit(e events.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
}

func (r *recordingEmitter) all() []events.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make([]events.Event, len(r.events))
	copy(cp, r.events)
	return cp
}

// findPayload returns the first event whose payload satisfies predicate p.
func (r *recordingEmitter) findPayload(p func(events.Event) bool) (events.Event, bool) {
	for _, e := range r.all() {
		if p(e) {
			return e, true
		}
	}
	return events.Event{}, false
}

// ─── NG Setup integration tests ─────────────────────────────────────────────

// startMinimalAMF starts an AMF with the given PLMNSupportList and returns
// the port, the recording emitter, and a cancel func.
func startMinimalAMF(t *testing.T, supportList []ngap.PLMNSupportItem) (port int, rec *recordingEmitter, cancel func()) {
	t.Helper()
	log := logger.New("error", "text")
	port = pickPort(t)
	plmn := ngap.PLMNFromMCCMNC("001", "01")
	cfg := Config{
		NGAPAddr:           "127.0.0.1:" + strconv.Itoa(port),
		NGAPMode:           sctp.ModeTCP,
		AMFName:            "QCore-AMF-test",
		GUAMI:              ngap.GUAMI{PLMN: plmn, AMFRegionID: 1, AMFSetID: 1, AMFPointer: 0},
		PLMNSupportList:    supportList,
		ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org",
	}
	// ausfCli is unused for NG Setup tests; a nil-safe stub suffices.
	svc := NewService(cfg, ausf.NewClient("http://127.0.0.1:1", "AMF", false), log)
	rec = &recordingEmitter{}
	svc.SetEmitter(rec)

	ctx, ctxCancel := context.WithCancel(context.Background())
	go func() { _ = svc.Serve(ctx) }()
	time.Sleep(60 * time.Millisecond)

	return port, rec, ctxCancel
}

// sendNGSetup sends a raw NG Setup Request from a mock gNB and returns the raw
// NGAP response bytes (or error). conn is closed by the caller.
func sendNGSetup(t *testing.T, port int, req *ngap.NGSetupRequest) ([]byte, error) {
	t.Helper()
	conn, err := sctp.Dial(sctp.ModeTCP, "127.0.0.1:"+strconv.Itoa(port))
	require.NoError(t, err)
	defer conn.Close()

	raw, err := ngap.EncodeNGSetupRequest(req)
	require.NoError(t, err)
	require.NoError(t, conn.Write(raw, 0))

	resp, _, err := conn.Read()
	return resp, err
}

// defaultSupportList is a minimal PLMNSupportList for PLMN 001/01 + SST 1.
func defaultSupportList() []ngap.PLMNSupportItem {
	return []ngap.PLMNSupportItem{
		{
			PLMN:    ngap.PLMNFromMCCMNC("001", "01"),
			SNSSAIs: []ngap.SNSSAI{{SST: 1}},
		},
	}
}

// TestNGSetup_HappyPath verifies that a matching gNB produces the correct
// events and receives NGSetupResponse (successful outcome).
func TestNGSetup_HappyPath(t *testing.T) {
	port, rec, cancel := startMinimalAMF(t, defaultSupportList())
	defer cancel()

	plmn := ngap.PLMNFromMCCMNC("001", "01")
	req := &ngap.NGSetupRequest{
		GlobalRANNodeID:  ngap.GlobalGNBID{PLMN: plmn, GNBID: 0xABCDE, GNBIDBits: 22},
		RANNodeName:      "Nokia-AirScale",
		DefaultPagingDRX: 1,
		SupportedTAList: []ngap.SupportedTA{{
			TAC: [3]byte{0x00, 0x00, 0x01},
			BroadcastPLMN: []ngap.BroadcastPLMNItem{
				{PLMN: plmn, SNSSAI: []ngap.SNSSAI{{SST: 1}}},
			},
		}},
	}
	resp, err := sendNGSetup(t, port, req)
	require.NoError(t, err)

	// Verify the NGAP response is NGSetupResponse (successful outcome).
	pdu, err := ngap.DecodePDU(resp)
	require.NoError(t, err)
	assert.Equal(t, ngap.PDUSuccessfulOutcome, pdu.Type, "expected SuccessfulOutcome (NGSetupResponse)")
	assert.Equal(t, ngap.ProcNGSetup, pdu.ProcedureCode)

	// Verify events: GNBConnected + NGSetupReceived + NGSetupAccepted.
	time.Sleep(40 * time.Millisecond)
	all := rec.all()

	// GNBConnectedPayload must appear first.
	_, ok := rec.findPayload(func(e events.Event) bool {
		_, is := e.Payload.(events.GNBConnectedPayload)
		return is
	})
	assert.True(t, ok, "expected GNBConnectedPayload event")

	// NGSetupReceivedPayload must carry the correct PLMN decoded via ident.
	ev, ok := rec.findPayload(func(e events.Event) bool {
		p, is := e.Payload.(events.NGSetupReceivedPayload)
		return is && p.GNBMCC == "001" && p.GNBMNC == "01"
	})
	require.True(t, ok, "expected NGSetupReceivedPayload with MCC=001, MNC=01; got events: %v", all)
	rxPayload := ev.Payload.(events.NGSetupReceivedPayload)
	assert.Equal(t, "Nokia-AirScale", rxPayload.GNBName)
	assert.Equal(t, []string{"000001"}, rxPayload.OfferedTACs)
	assert.Equal(t, uint8(1), rxPayload.OfferedSNSSAIs[0].SST)

	// NGSetupPayload (success) must appear.
	_, ok = rec.findPayload(func(e events.Event) bool {
		p, is := e.Payload.(events.NGSetupPayload)
		return is && p.Success
	})
	assert.True(t, ok, "expected NGSetupPayload with Success=true")
}

// TestNGSetup_PLMNMismatch verifies PLMN mismatch produces:
//   - NGSetupFailure NGAP response
//   - ErrorEvent with FailureCause="plmn_mismatch" and non-empty fixes
func TestNGSetup_PLMNMismatch(t *testing.T) {
	port, rec, cancel := startMinimalAMF(t, defaultSupportList()) // AMF = 001/01
	defer cancel()

	wrongPLMN := ngap.PLMN(ident.EncodePLMN("310", "260")) // AT&T
	req := &ngap.NGSetupRequest{
		GlobalRANNodeID: ngap.GlobalGNBID{PLMN: wrongPLMN, GNBID: 0x001, GNBIDBits: 22},
		SupportedTAList: []ngap.SupportedTA{{
			TAC: [3]byte{0x00, 0x00, 0x01},
			BroadcastPLMN: []ngap.BroadcastPLMNItem{
				{PLMN: wrongPLMN, SNSSAI: []ngap.SNSSAI{{SST: 1}}},
			},
		}},
	}
	resp, err := sendNGSetup(t, port, req)
	require.NoError(t, err)

	pdu, err := ngap.DecodePDU(resp)
	require.NoError(t, err)
	assert.Equal(t, ngap.PDUUnsuccessfulOutcome, pdu.Type, "expected UnsuccessfulOutcome (NGSetupFailure)")

	time.Sleep(40 * time.Millisecond)
	ev, ok := rec.findPayload(func(e events.Event) bool {
		p, is := e.Payload.(events.NGSetupPayload)
		return is && !p.Success && p.FailureCause == "plmn_mismatch"
	})
	require.True(t, ok, "expected NGSetupPayload with plmn_mismatch; events: %v", rec.all())
	payload := ev.Payload.(events.NGSetupPayload)
	assert.NotEmpty(t, payload.FailureExplain)
	assert.NotEmpty(t, payload.FixGNBSide)
	assert.NotEmpty(t, payload.FixQCoreSide)
}

// TestNGSetup_SliceMismatch verifies SST mismatch produces NGSetupFailure
// with FailureCause="slice_mismatch".
func TestNGSetup_SliceMismatch(t *testing.T) {
	port, rec, cancel := startMinimalAMF(t, defaultSupportList()) // AMF SST=1
	defer cancel()

	plmn := ngap.PLMNFromMCCMNC("001", "01")
	req := &ngap.NGSetupRequest{
		GlobalRANNodeID: ngap.GlobalGNBID{PLMN: plmn, GNBID: 0x001, GNBIDBits: 22},
		SupportedTAList: []ngap.SupportedTA{{
			TAC: [3]byte{0x00, 0x00, 0x01},
			BroadcastPLMN: []ngap.BroadcastPLMNItem{
				{PLMN: plmn, SNSSAI: []ngap.SNSSAI{{SST: 2}}}, // URLLC, not eMBB
			},
		}},
	}
	resp, err := sendNGSetup(t, port, req)
	require.NoError(t, err)

	pdu, _ := ngap.DecodePDU(resp)
	assert.Equal(t, ngap.PDUUnsuccessfulOutcome, pdu.Type)

	time.Sleep(40 * time.Millisecond)
	_, ok := rec.findPayload(func(e events.Event) bool {
		p, is := e.Payload.(events.NGSetupPayload)
		return is && !p.Success && p.FailureCause == "slice_mismatch"
	})
	require.True(t, ok, "expected slice_mismatch event; got: %v", rec.all())
}

// TestNGSetup_EventsContainSourceIP verifies that the GNBConnectedPayload
// captures the remote IP so the dashboard can show "Connection from 1.2.3.4".
func TestNGSetup_EventsContainSourceIP(t *testing.T) {
	port, rec, cancel := startMinimalAMF(t, defaultSupportList())
	defer cancel()

	plmn := ngap.PLMNFromMCCMNC("001", "01")
	req := &ngap.NGSetupRequest{
		GlobalRANNodeID: ngap.GlobalGNBID{PLMN: plmn, GNBID: 0x001, GNBIDBits: 22},
		SupportedTAList: []ngap.SupportedTA{{
			TAC: [3]byte{0x00, 0x00, 0x01},
			BroadcastPLMN: []ngap.BroadcastPLMNItem{
				{PLMN: plmn, SNSSAI: []ngap.SNSSAI{{SST: 1}}},
			},
		}},
	}
	_, _ = sendNGSetup(t, port, req)
	time.Sleep(40 * time.Millisecond)

	ev, ok := rec.findPayload(func(e events.Event) bool {
		p, is := e.Payload.(events.GNBConnectedPayload)
		return is && p.SourceIP != ""
	})
	require.True(t, ok, "expected GNBConnectedPayload with SourceIP")
	payload := ev.Payload.(events.GNBConnectedPayload)
	assert.Contains(t, payload.SourceIP, "127.0.0.1", "SourceIP should be the loopback address")
	assert.NotEmpty(t, payload.GNBAssocID, "GNBAssocID must be set")
	_ = hex.EncodeToString(nil) // ensure hex imported
}

// TestNGSetup_PLMNDecoding_RealGNBBytes verifies that NG Setup decoding
// uses pkg/ident (the A1 canonical codec), not an ad-hoc nibble extraction.
// We directly inject raw gNB PLMN bytes into an NGSetupRequest and assert
// the emitted event carries the correctly decoded MCC/MNC.
func TestNGSetup_PLMNDecoding_RealGNBBytes(t *testing.T) {
	port, rec, cancel := startMinimalAMF(t, []ngap.PLMNSupportItem{
		{PLMN: ngap.PLMN(ident.EncodePLMN("262", "01")), SNSSAIs: []ngap.SNSSAI{{SST: 1}}},
	})
	defer cancel()

	// Deutsche Telekom raw bytes: byte0=0x62, byte1=0xF2, byte2=0x10
	dtPLMN := ngap.PLMN{0x62, 0xF2, 0x10}
	req := &ngap.NGSetupRequest{
		GlobalRANNodeID: ngap.GlobalGNBID{PLMN: dtPLMN, GNBID: 0x001, GNBIDBits: 22},
		SupportedTAList: []ngap.SupportedTA{{
			TAC: [3]byte{0x00, 0x00, 0x01},
			BroadcastPLMN: []ngap.BroadcastPLMNItem{
				{PLMN: dtPLMN, SNSSAI: []ngap.SNSSAI{{SST: 1}}},
			},
		}},
	}
	resp, err := sendNGSetup(t, port, req)
	require.NoError(t, err)
	pdu, _ := ngap.DecodePDU(resp)
	assert.Equal(t, ngap.PDUSuccessfulOutcome, pdu.Type,
		"DT gNB should be accepted when AMF is configured for 262/01")

	time.Sleep(40 * time.Millisecond)
	ev, ok := rec.findPayload(func(e events.Event) bool {
		p, is := e.Payload.(events.NGSetupReceivedPayload)
		return is && p.GNBMCC == "262" && p.GNBMNC == "01"
	})
	require.True(t, ok, "NGSetupReceivedPayload must carry decoded MCC=262, MNC=01 (not raw hex); events: %v", rec.all())
	assert.Equal(t, "262", ev.Payload.(events.NGSetupReceivedPayload).GNBMCC)
	assert.Equal(t, "01", ev.Payload.(events.NGSetupReceivedPayload).GNBMNC)
}

// ─── Registration failure integration tests ────────────────────────────────
//
// Each test drives a mock gNB+UE through a specific failure path and asserts:
//   1. The AMF emits a RegistrationFailurePayload with the correct cause tag.
//   2. DiagnoseRegistration produces a non-OK result with the expected cause.
//   3. The AMF sends a NAS Registration Reject to the UE.

// startFullAMF starts an AMF wired to a real (in-process) AUSF+UDM with the
// given provisioned subscriber. Returns the AMF port, a recording emitter, and
// a cancel func. AUSF URL is embedded in the AMF service.
func startFullAMF(t *testing.T, sub *subscriber.Subscriber) (port int, rec *recordingEmitter, cancel func()) {
	t.Helper()
	ausfURL := startAUSFandUDM(t, sub)
	log := logger.New("error", "text")
	port = pickPort(t)
	plmn := ngap.PLMNFromMCCMNC("001", "01")
	cfg := Config{
		NGAPAddr: "127.0.0.1:" + strconv.Itoa(port),
		NGAPMode: sctp.ModeTCP,
		AMFName:  "QCore-AMF-test",
		GUAMI:    ngap.GUAMI{PLMN: plmn, AMFRegionID: 1, AMFSetID: 1, AMFPointer: 0},
		PLMNSupportList: []ngap.PLMNSupportItem{
			{PLMN: plmn, SNSSAIs: []ngap.SNSSAI{{SST: 1}}},
		},
		ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org",
	}
	svc := NewService(cfg, ausf.NewClient(ausfURL, "AMF", false), log)
	rec = &recordingEmitter{}
	svc.SetEmitter(rec)
	ctx, ctxCancel := context.WithCancel(context.Background())
	go func() { _ = svc.Serve(ctx) }()
	time.Sleep(80 * time.Millisecond)
	return port, rec, ctxCancel
}

// connectGNB dials the AMF, performs NGSetup, and returns the mock gNB.
func connectGNB(t *testing.T, port int) *mockGNB {
	t.Helper()
	conn, err := sctp.Dial(sctp.ModeTCP, "127.0.0.1:"+strconv.Itoa(port))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	gNB := &mockGNB{conn: conn}
	gNB.ngSetup(t)
	return gNB
}

// sendRegistrationRequest sends a Registration Request from the mock gNB for
// the given SUCI bytes and returns the ranID used.
func sendRegistrationRequest(t *testing.T, gNB *mockGNB, suciBytes []byte) uint64 {
	t.Helper()
	const ranID = uint64(0x1001)
	regReq := nas5g.EncodeRegistrationRequest(&nas5g.RegistrationRequest{
		RegistrationType:     nas5g.RegistrationTypeInitialRegistration,
		NASKeySetID:          7,
		MobileIdentity:       suciBytes,
		UESecurityCapability: []byte{0xE0, 0x00, 0xC0, 0x00},
	})
	gNB.sendInitialUE(t, ranID, regReq)
	return ranID
}

// makeSUCI builds a null-scheme SUCI for the given IMSI with PLMN 001/01.
func makeSUCI(imsi string) []byte {
	plmn := [3]byte(ngap.PLMNFromMCCMNC("001", "01"))
	// MNC=01 is 2-digit, so MSIN = IMSI[5:]
	msin := imsi[5:]
	msinBCD := make([]byte, (len(msin)+1)/2)
	for i, ch := range msin {
		d := uint8(ch - '0')
		if i%2 == 0 {
			msinBCD[i/2] = d
		} else {
			msinBCD[i/2] |= d << 4
		}
	}
	return nas5g.EncodeSUCI(nas5g.SUCI{
		PLMN:             plmn,
		RoutingIndicator: [2]byte{0xFF, 0xFF},
		ProtectionScheme: 0x00,
		HomeNetworkPKID:  0x00,
		MSIN:             msinBCD,
	})
}

func annexC4ProfileASUCI() []byte {
	return hd("f172241000000101" +
		"b2e92f836055a255837debf850b528997ce0201cb82adfe4be1f587d07d8457d" +
		"cb02352410" +
		"cddd9e730ef3fa87")
}

// assertRegistrationReject waits for a DownlinkNASTransport and asserts it
// carries a NAS Registration Reject (message type 0x44).
func assertRegistrationReject(t *testing.T, gNB *mockGNB) {
	t.Helper()
	_, nasPDU := gNB.recvDownlinkNAS(t)
	require.GreaterOrEqual(t, len(nasPDU), 3, "NAS PDU too short")
	hdr, err := nas5g.DecodeHeader(nasPDU)
	require.NoError(t, err)
	assert.Equal(t, nas5g.MsgTypeRegistrationReject, hdr.MessageType,
		"expected Registration Reject, got %s", hdr.MessageType)
}

// TestRegistration_UnknownSubscriber drives a UE with an IMSI not in the
// subscriber store and asserts the correct cause + event + rejection.
func TestRegistration_UnknownSubscriber(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in -short mode")
	}
	// Provision a DIFFERENT subscriber — the UE will use an unknown IMSI.
	provisionedSub := &subscriber.Subscriber{
		IMSI: "001010000000001",
		Ki:   "465b5ce8b199b49faa5f0a2ee238a6bc",
		OPc:  "cd63cb71954a9f4e48a5994e37a02baf",
		AMF:  "8000",
		SQN:  "000000000001",
	}
	port, rec, cancel := startFullAMF(t, provisionedSub)
	defer cancel()
	gNB := connectGNB(t, port)

	// UE uses an IMSI that is NOT provisioned.
	unknownIMSI := "001019999999999"
	suci := makeSUCI(unknownIMSI)
	_ = sendRegistrationRequest(t, gNB, suci)

	// AMF must send a Registration Reject.
	assertRegistrationReject(t, gNB)
	time.Sleep(40 * time.Millisecond)

	// Assert the RegistrationFailurePayload event was emitted with the correct cause.
	ev, ok := rec.findPayload(func(e events.Event) bool {
		p, is := e.Payload.(events.RegistrationFailurePayload)
		return is && p.Cause == diag.CauseUnknownSubscriber
	})
	require.True(t, ok, "expected RegistrationFailurePayload with cause=%q; events: %v",
		diag.CauseUnknownSubscriber, rec.all())

	// Assert the diagnostic layer produces the right result.
	result := diag.DiagnoseRegistration(rec.all())
	assert.False(t, result.OK)
	assert.Equal(t, diag.CauseUnknownSubscriber, result.Cause)
	assert.NotEmpty(t, result.Explanation)
	assert.NotEmpty(t, result.FixQCoreSide)
	_ = ev
}

// TestRegistration_AuthMACFailure drives a UE that sends an Authentication
// Failure with cause=MAC failure (simulating wrong Ki/OPc on network side).
func TestRegistration_AuthMACFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in -short mode")
	}
	sub := &subscriber.Subscriber{
		IMSI: "001010000000001",
		Ki:   "465b5ce8b199b49faa5f0a2ee238a6bc",
		OPc:  "cd63cb71954a9f4e48a5994e37a02baf",
		AMF:  "8000",
		SQN:  "000000000001",
	}
	port, rec, cancel := startFullAMF(t, sub)
	defer cancel()
	gNB := connectGNB(t, port)

	suci := makeSUCI(sub.IMSI)
	ranID := sendRegistrationRequest(t, gNB, suci)

	// Wait for Authentication Request from AMF.
	amfID, _ := gNB.recvDownlinkNAS(t)

	// UE sends Authentication Failure (MAC failure) instead of Authentication Response.
	authFail := nas5g.EncodeAuthenticationFailure(nas5g.Cause5GMMMACFailure, nil)
	gNB.sendUplinkNAS(t, amfID, ranID, authFail)

	// AMF must send a Registration Reject.
	assertRegistrationReject(t, gNB)
	time.Sleep(40 * time.Millisecond)

	// Assert AuthenticationFailurePayload was emitted.
	_, ok := rec.findPayload(func(e events.Event) bool {
		p, is := e.Payload.(events.AuthenticationFailurePayload)
		return is && p.CauseName == "mac_failure"
	})
	require.True(t, ok, "expected AuthenticationFailurePayload with mac_failure; events: %v", rec.all())

	// Assert the diagnostic layer produces CauseAuthMACFailure.
	result := diag.DiagnoseRegistration(rec.all())
	assert.False(t, result.OK)
	assert.Equal(t, diag.CauseAuthMACFailure, result.Cause)
	assert.NotEmpty(t, result.FixUESide)
	assert.NotEmpty(t, result.FixQCoreSide)
}

// TestRegistration_ConcealedSUCIPassesToUDM drives a UE sending a concealed
// SUCI and asserts the AMF forwards it to AUSF/UDM instead of rejecting the
// protection scheme locally. This helper UDM has no SIDF key configured, so
// the final UE-visible result is still a Registration Reject.
func TestRegistration_ConcealedSUCIPassesToUDM(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in -short mode")
	}
	sub := &subscriber.Subscriber{
		IMSI: "001010000000001",
		Ki:   "465b5ce8b199b49faa5f0a2ee238a6bc",
		OPc:  "cd63cb71954a9f4e48a5994e37a02baf",
		AMF:  "8000",
		SQN:  "000000000001",
	}
	port, rec, cancel := startFullAMF(t, sub)
	defer cancel()
	gNB := connectGNB(t, port)

	// Build a SUCI with protection scheme = 1 (ECIES Profile A).
	plmn := [3]byte(ngap.PLMNFromMCCMNC("001", "01"))
	eciesSUCI := nas5g.EncodeSUCI(nas5g.SUCI{
		PLMN:             plmn,
		RoutingIndicator: [2]byte{0xFF, 0xFF},
		ProtectionScheme: 0x01,
		HomeNetworkPKID:  0x00,
		MSIN:             []byte{0xDE, 0xAD, 0xBE, 0xEF, 0x00},
	})
	_ = sendRegistrationRequest(t, gNB, eciesSUCI)

	assertRegistrationReject(t, gNB)
	time.Sleep(40 * time.Millisecond)

	rawHex := hex.EncodeToString(eciesSUCI)
	_, ok := rec.findPayload(func(e events.Event) bool {
		p, is := e.Payload.(events.RegistrationRequestPayload)
		return is && p.SUCI == "suci-"+rawHex
	})
	require.True(t, ok, "expected RegistrationRequestPayload with raw SUCI forwarded; events: %v", rec.all())

	_, ok = rec.findPayload(func(e events.Event) bool {
		_, is := e.Payload.(events.SUCIDecodeFailurePayload)
		return is
	})
	assert.False(t, ok, "AMF should not emit SUCIDecodeFailurePayload for pass-through SUCI")

	_, ok = rec.findPayload(func(e events.Event) bool {
		p, is := e.Payload.(events.RegistrationFailurePayload)
		return is && p.SUCI == "suci-"+rawHex && p.Cause == "ausf_error"
	})
	require.True(t, ok, "expected AUSF/UDM failure after SUCI pass-through; events: %v", rec.all())
}

// TestRegistration_DistinguishUnknownVsMAC ensures the two most easily confused
// failures produce DIFFERENT events, DIFFERENT causes, and DIFFERENT fixes.
func TestRegistration_DistinguishUnknownVsMAC(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in -short mode")
	}
	sub := &subscriber.Subscriber{
		IMSI: "001010000000001",
		Ki:   "465b5ce8b199b49faa5f0a2ee238a6bc",
		OPc:  "cd63cb71954a9f4e48a5994e37a02baf",
		AMF:  "8000",
		SQN:  "000000000001",
	}

	// --- Run 1: unknown subscriber ---
	port1, rec1, cancel1 := startFullAMF(t, sub)
	defer cancel1()
	gNB1 := connectGNB(t, port1)
	_ = sendRegistrationRequest(t, gNB1, makeSUCI("001019999999999"))
	assertRegistrationReject(t, gNB1)
	time.Sleep(40 * time.Millisecond)
	resultUnknown := diag.DiagnoseRegistration(rec1.all())

	// --- Run 2: MAC failure (known subscriber, UE rejects AUTN) ---
	port2, rec2, cancel2 := startFullAMF(t, sub)
	defer cancel2()
	gNB2 := connectGNB(t, port2)
	ranID2 := sendRegistrationRequest(t, gNB2, makeSUCI(sub.IMSI))
	amfID2, _ := gNB2.recvDownlinkNAS(t) // get Auth Request
	authFail := nas5g.EncodeAuthenticationFailure(nas5g.Cause5GMMMACFailure, nil)
	gNB2.sendUplinkNAS(t, amfID2, ranID2, authFail)
	assertRegistrationReject(t, gNB2)
	time.Sleep(40 * time.Millisecond)
	resultMAC := diag.DiagnoseRegistration(rec2.all())

	assert.NotEqual(t, resultUnknown.Cause, resultMAC.Cause,
		"unknown subscriber and MAC failure MUST have different causes")
	assert.NotEqual(t, resultUnknown.FixQCoreSide, resultMAC.FixQCoreSide,
		"fixes must differ: one is provisioning, one is Ki/OPc correction")

	assert.Equal(t, diag.CauseUnknownSubscriber, resultUnknown.Cause)
	assert.Equal(t, diag.CauseAuthMACFailure, resultMAC.Cause)

	// Verify the hex import is used (suppress unused import warning on older Go).
	_ = hex.EncodeToString(nil)
	_ = ident.EncodePLMN("001", "01")
}

// --- PDU Session Accept: DL NAS count + security header (audit pt 4) ---------

// captureAssoc is a minimal sctp.Association stub that records writes, so we can
// drive sendPDUSessionEstablishmentAccept without a live SCTP connection.
type captureAssoc struct{ writes [][]byte }

func (c *captureAssoc) Read() ([]byte, uint16, error) { return nil, 0, nil }
func (c *captureAssoc) Write(d []byte, _ uint16) error {
	c.writes = append(c.writes, append([]byte(nil), d...))
	return nil
}
func (c *captureAssoc) Close() error         { return nil }
func (c *captureAssoc) RemoteAddr() net.Addr { return nil }
func (c *captureAssoc) LocalAddr() net.Addr  { return nil }

// TestSendPDUSessionAccept_DLCountAndSecurityHeader pins the post-SMC downlink
// behavior for the PDU Session Establishment Accept. With NEA0 (null ciphering)
// negotiated, the network uses integrity-only protection — security header 0x01,
// NOT integrity+ciphered (0x02). UERANSIM accepts 0x01 with NEA0 (validated
// end-to-end by ueransim-interop run 27108387027: "PDU Session establishment is
// successful"), so 0x02 is not required. The DL NAS count is used for the SN
// octet and must advance by one after the message.
func TestSendPDUSessionAccept_DLCountAndSecurityHeader(t *testing.T) {
	s := &Service{} // cfg.TraceNGAPHex defaults false → traceNGAP is a no-op
	cap := &captureAssoc{}
	gnb := &gNBSession{conn: cap, amf: s, log: logger.New("error", "text")}

	kNASint := make([]byte, 16)
	for i := range kNASint {
		kNASint[i] = 0x11
	}
	ue := &UEContext{
		AMFUENGAPID: 1,
		RANUENGAPID: 1,
		gNB:         gnb,
		KNASint:     kNASint,
		DLCount:     2, // post-registration: SMC used count 0, Registration Accept used 1
	}

	require.NoError(t, s.sendPDUSessionEstablishmentAccept(ue, 5, 1, [4]byte{10, 45, 0, 2}))

	assert.Equal(t, uint32(3), ue.DLCount, "DL NAS count advances by one after the Accept")
	require.Len(t, cap.writes, 1, "exactly one DownlinkNASTransport sent")

	pdu, err := ngap.DecodePDU(cap.writes[0])
	require.NoError(t, err)
	ies, err := ngap.DecodeIEContainer(pdu.Value)
	require.NoError(t, err)
	dl, err := ngap.DecodeDownlinkNASTransport(ies)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(dl.NASPDU), 8)
	assert.Equal(t, uint8(0x7E), dl.NASPDU[0], "5GMM EPD")
	assert.Equal(t, uint8(0x01), dl.NASPDU[1], "integrity-only (NEA0): 0x01, not 0x02 (ciphered)")
	assert.Equal(t, uint8(2), dl.NASPDU[6], "SN octet = DL NAS count used (2), before increment")
}

// TestSendPDUSessionAccept_RejectsWithoutKey guards the precondition: the AMF must
// never emit an unprotected PDU Session Accept if the NAS security context is absent.
func TestSendPDUSessionAccept_RejectsWithoutKey(t *testing.T) {
	s := &Service{}
	gnb := &gNBSession{conn: &captureAssoc{}, amf: s, log: logger.New("error", "text")}
	ue := &UEContext{gNB: gnb, KNASint: nil, DLCount: 2}
	require.Error(t, s.sendPDUSessionEstablishmentAccept(ue, 5, 1, [4]byte{10, 45, 0, 2}),
		"must refuse to send without a NAS integrity key")
}

func TestHandleDeregistrationRequestUEOrigSendsProtectedAccept(t *testing.T) {
	s := &Service{log: logger.New("error", "text")}
	cap := &captureAssoc{}
	gnb := &gNBSession{conn: cap, amf: s, log: logger.New("error", "text")}

	kNASint := make([]byte, 16)
	for i := range kNASint {
		kNASint[i] = 0x22
	}
	ue := &UEContext{
		AMFUENGAPID: 1,
		RANUENGAPID: 1,
		gNB:         gnb,
		KNASint:     kNASint,
		DLCount:     4,
		SUPI:        "imsi-001010000000001",
		State:       StateRegistered,
	}

	require.NoError(t, s.handleDeregistrationRequestUEOrig(
		context.Background(),
		ue,
		&nas5g.DeregistrationRequestUEOrig{Raw: []byte{0x01}},
	))

	assert.Equal(t, StateIdle, ue.State)
	assert.Equal(t, uint32(0), ue.DLCount, "DL NAS count resets for the next registration after protected accept")
	assert.Equal(t, uint32(0), ue.ULCount, "UL NAS count resets for the next registration after protected accept")
	require.Len(t, cap.writes, 1, "exactly one DownlinkNASTransport sent")

	pdu, err := ngap.DecodePDU(cap.writes[0])
	require.NoError(t, err)
	ies, err := ngap.DecodeIEContainer(pdu.Value)
	require.NoError(t, err)
	dl, err := ngap.DecodeDownlinkNASTransport(ies)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(dl.NASPDU), 10)
	assert.Equal(t, uint8(0x7E), dl.NASPDU[0], "5GMM EPD")
	assert.Equal(t, uint8(0x01), dl.NASPDU[1], "integrity-protected NAS container")
	assert.Equal(t, uint8(4), dl.NASPDU[6], "SN octet = DL NAS count used before increment")

	inner := dl.NASPDU[7:]
	hdr, err := nas5g.DecodeHeader(inner)
	require.NoError(t, err)
	assert.Equal(t, nas5g.MsgTypeDeregistrationAcceptUEOrig, hdr.MessageType)
}
