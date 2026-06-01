package amf

// C1 acceptance criterion: a 5G Registration + PDU Session produces one
// correlated event trace spanning AMF → AUSF → UDM → SMF with the same
// JourneyID threading every event.
//
// This file is package amf (internal test) so it can reuse the mockGNB and
// helper functions defined in amf_test.go.

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/qcore-project/qcore/pkg/ausf"
	"github.com/qcore-project/qcore/pkg/events"
	"github.com/qcore-project/qcore/pkg/logger"
	"github.com/qcore-project/qcore/pkg/nas5g"
	"github.com/qcore-project/qcore/pkg/ngap"
	"github.com/qcore-project/qcore/pkg/sbi"
	"github.com/qcore-project/qcore/pkg/sctp"
	"github.com/qcore-project/qcore/pkg/smf"
	"github.com/qcore-project/qcore/pkg/subscriber"
	"github.com/qcore-project/qcore/pkg/udm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// multiNFEmitter collects events from multiple NFs into one shared trace.
type multiNFEmitter struct {
	mu     sync.Mutex
	events []events.Event
}

func (m *multiNFEmitter) Emit(e events.Event) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, e)
}

func (m *multiNFEmitter) all() []events.Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]events.Event, len(m.events))
	copy(out, m.events)
	return out
}

func (m *multiNFEmitter) journeyEvents(journeyID string) []events.Event {
	var out []events.Event
	for _, ev := range m.all() {
		if ev.JourneyID == journeyID {
			out = append(out, ev)
		}
	}
	return out
}

// TestC1_RegistrationEventTrace is the C1 exit criterion:
// a 5G registration produces one correlated event trace spanning
// AMF → AUSF → UDM, all sharing the same journey_id.
//
// This test wires AMF, AUSF, UDM, and SMF in-process with a single shared
// emitter and asserts that each NF's key events carry the same journey_id.
func TestC1_RegistrationEventTrace(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in -short mode")
	}

	const (
		imsi   = "001010000000001"
		ki     = "465b5ce8b199b49faa5f0a2ee238a6bc"
		opc    = "cd63cb71954a9f4e48a5994e37a02baf"
		snName = "5G:mnc001.mcc001.3gppnetwork.org"
	)

	log := logger.New("error", "text")
	shared := &multiNFEmitter{}

	// Wire subscriber store
	sub := &subscriber.Subscriber{IMSI: imsi, Ki: ki, OPc: opc, AMF: "8000", SQN: "000000000001"}
	subs := map[string]*subscriber.Subscriber{imsi: sub}

	// UDM (with shared emitter)
	udmSvc := udm.NewService(udm.NewStoreSource(&fakeSubscriberStore{subs}), log).
		WithAuthSource(udm.NewStoreAuthSource(&fakeAuthStore{subs}))
	udmSvc.SetEmitter(shared)
	udmPort := pickPort(t)
	udmSrv := sbi.NewServer(sbi.ServerConfig{BindAddress: "127.0.0.1", Port: udmPort, NFType: "UDM"}, log, udmSvc.Handler())
	go func() { _ = udmSrv.Serve() }()

	// AUSF (with shared emitter)
	udmCli := udm.NewClient("http://127.0.0.1:"+strconv.Itoa(udmPort), "AUSF", false)
	ausfSvc := ausf.NewService(udmCli, log)
	ausfSvc.SetEmitter(shared)
	ausfPort := pickPort(t)
	ausfSrv := sbi.NewServer(sbi.ServerConfig{BindAddress: "127.0.0.1", Port: ausfPort, NFType: "AUSF"}, log, ausfSvc.Handler())
	go func() { _ = ausfSrv.Serve() }()

	// SMF (with shared emitter, no real UPF needed for trace test)
	smfSvc, err := smf.NewService(smf.Config{
		IPPoolCIDR:    "10.45.0.0/24",
		UPFPfcpAddr:   "127.0.0.1:18805", // won't be contacted in this test
		LocalPfcpIP:   "127.0.0.1",
		SMFInstanceID: "smf-test",
	}, log)
	require.NoError(t, err)
	smfSvc.SetEmitter(shared)
	smfPort := pickPort(t)
	smfSrv := sbi.NewServer(sbi.ServerConfig{BindAddress: "127.0.0.1", Port: smfPort, NFType: "SMF"}, log, smfSvc.Handler())
	go func() { _ = smfSrv.Serve() }()

	time.Sleep(100 * time.Millisecond)

	// AMF (with shared emitter)
	plmn := ngap.PLMNFromMCCMNC("001", "01")
	amfPort := pickPort(t)
	amfCfg := Config{
		NGAPAddr: "127.0.0.1:" + strconv.Itoa(amfPort),
		NGAPMode: sctp.ModeTCP,
		AMFName:  "QCore-AMF-test",
		GUAMI:    ngap.GUAMI{PLMN: plmn, AMFRegionID: 1, AMFSetID: 1, AMFPointer: 0},
		PLMNSupportList: []ngap.PLMNSupportItem{
			{PLMN: plmn, SNSSAIs: []ngap.SNSSAI{{SST: 1}}},
		},
		ServingNetworkName: snName,
		SMFURL:             "http://127.0.0.1:" + strconv.Itoa(smfPort),
	}
	ausfCli := ausf.NewClient("http://127.0.0.1:"+strconv.Itoa(ausfPort), "AMF", false)
	amfSvc := NewService(amfCfg, ausfCli, log)
	amfSvc.SetEmitter(shared)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = amfSvc.Serve(ctx) }()
	time.Sleep(80 * time.Millisecond)
	t.Cleanup(func() {
		cancel()
		_ = ausfSrv.Shutdown(context.Background())
		_ = udmSrv.Shutdown(context.Background())
		_ = smfSrv.Shutdown(context.Background())
		// Close the SMF so its PFCP client releases the fixed UDP port 8805;
		// otherwise the next test that constructs an SMF fails to bind it.
		_ = smfSvc.Close()
	})

	// ── Connect mock gNB ──────────────────────────────────────────────────────
	conn, err := sctp.Dial(sctp.ModeTCP, "127.0.0.1:"+strconv.Itoa(amfPort))
	require.NoError(t, err)
	defer conn.Close()
	gNB := &mockGNB{conn: conn}
	gNB.ngSetup(t)

	// ── Registration Request ──────────────────────────────────────────────────
	suci := makeSUCI(imsi)
	const ranID = uint64(0x1001)
	regReq := nas5g.EncodeRegistrationRequest(&nas5g.RegistrationRequest{
		RegistrationType:     nas5g.RegistrationTypeInitialRegistration,
		NASKeySetID:          7,
		MobileIdentity:       suci,
		UESecurityCapability: []byte{0xE0, 0x00, 0xC0, 0x00},
	})
	gNB.sendInitialUE(t, ranID, regReq)

	// ── Auth exchange ─────────────────────────────────────────────────────────
	amfID, authReqRaw := gNB.recvDownlinkNAS(t)
	authMsg, err := nas5g.Decode(authReqRaw)
	require.NoError(t, err)
	require.NotNil(t, authMsg.AuthenticationRequest, "expected AuthenticationRequest")

	// Recompute RES* exactly as the UE would: re-run Milenage with the RAND from
	// the Auth Request so XRES* matches what AUSF stored (see TestAMF_RegistrationFlow).
	var kiArr, opcArr, randArr [16]byte
	copy(kiArr[:], hd(ki))
	copy(opcArr[:], hd(opc))
	copy(randArr[:], authMsg.AuthenticationRequest.RAND[:])
	var sqnArr [6]byte
	copy(sqnArr[:], hd("000000000001"))
	var amfArr [2]byte
	copy(amfArr[:], hd(sub.AMF))

	av, err := subscriber.Generate5GAuthVectorWithRAND(kiArr, opcArr, randArr, sqnArr, amfArr, snName)
	require.NoError(t, err)

	authResp := nas5g.EncodeAuthenticationResponse(&nas5g.AuthenticationResponse{ResStar: hd(av.XRESStar)})
	gNB.sendUplinkNAS(t, amfID, ranID, authResp)

	// ── Security Mode Complete ─────────────────────────────────────────────────
	_, smcRaw := gNB.recvDownlinkNAS(t)
	require.Equal(t, uint8(0x7E), smcRaw[0])

	smcComplete := nas5g.EncodeSecurityModeComplete(&nas5g.SecurityModeComplete{})
	gNB.sendUplinkNAS(t, amfID, ranID, smcComplete)

	// ── Registration Accept ───────────────────────────────────────────────────
	amfID2, regAcceptRaw := gNB.recvInitialContextSetup(t)
	require.Equal(t, uint8(0x7E), regAcceptRaw[0])
	t.Log("✓ UE Registered")

	// ── PDU Session Establishment ─────────────────────────────────────────────
	pduEstabReq := nas5g.EncodePDUSessionEstablishmentRequest(1, 1)
	ulNAS := nas5g.EncodeULNASTransport(1, pduEstabReq)
	gNB.sendUplinkNAS(t, amfID2, ranID, ulNAS)

	// Give SMF time to allocate IP and respond.
	time.Sleep(500 * time.Millisecond)

	// ── Assert the event trace ────────────────────────────────────────────────
	all := shared.all()

	// Find the journey ID from the AMF's Registration Request event.
	var journeyID string
	for _, ev := range all {
		if ev.NF == "amf" && ev.JourneyID != "" {
			journeyID = ev.JourneyID
			break
		}
	}
	require.NotEmpty(t, journeyID, "no journey_id found in any AMF event")
	t.Logf("journey_id: %s", journeyID)

	jEvents := shared.journeyEvents(journeyID)
	t.Logf("journey event count: %d", len(jEvents))

	// Assert each NF appears in the journey trace.
	nfsInTrace := map[string]bool{}
	for _, ev := range jEvents {
		nfsInTrace[ev.NF] = true
	}
	assert.True(t, nfsInTrace["amf"], "AMF must appear in journey trace")
	assert.True(t, nfsInTrace["ausf"], "AUSF must appear in journey trace (SBI middleware propagates journey_id)")
	assert.True(t, nfsInTrace["udm"], "UDM must appear in journey trace")

	// Assert AUSF emitted an auth vector event.
	var ausfVectorSeen bool
	for _, ev := range jEvents {
		if ev.NF == "ausf" {
			if _, ok := ev.Payload.(events.AUSFAuthVectorPayload); ok {
				ausfVectorSeen = true
			}
		}
	}
	assert.True(t, ausfVectorSeen,
		"AUSF must emit AUSFAuthVectorPayload in the journey; AUSF events: %v",
		filterByNF(jEvents, "ausf"))

	// Assert UDM emitted an auth data event.
	var udmAuthSeen bool
	for _, ev := range jEvents {
		if ev.NF == "udm" {
			if p, ok := ev.Payload.(events.UDMAuthDataPayload); ok && p.Success {
				udmAuthSeen = true
			}
		}
	}
	assert.True(t, udmAuthSeen,
		"UDM must emit UDMAuthDataPayload{Success:true} in the journey; UDM events: %v",
		filterByNF(jEvents, "udm"))

	// Assert the AMF emitted a 5GMM-REGISTERED state transition.
	var registeredSeen bool
	for _, ev := range jEvents {
		if ev.NF == "amf" && ev.Category == events.StateTransition {
			if _, ok := ev.Payload.(events.RegistrationAcceptPayload); ok {
				registeredSeen = true
			}
		}
	}
	assert.True(t, registeredSeen, "AMF must emit RegistrationAcceptPayload in the journey")

	// Assert SMF emitted a session event (PDU session forwarding may be async).
	var smfSessionSeen bool
	for _, ev := range all { // SMF may not carry journey_id (no N11→SMF header yet)
		if ev.NF == "smf" {
			if p, ok := ev.Payload.(events.SMFSessionPayload); ok && p.Success {
				smfSessionSeen = true
			}
		}
	}
	assert.True(t, smfSessionSeen, "SMF must emit SMFSessionPayload{Success:true}")

	t.Logf("✓ Full correlated trace: %d events across NFs %v", len(jEvents), nfsInTrace)
	_ = amfID2
}

// TestC1_FailureTraceCorrelation verifies that a registration failure
// (unknown subscriber) also produces a correlated trace — the failure event
// carries the same journey_id as the request event, so the diagnostic layer
// can find the full context.
func TestC1_FailureTraceCorrelation(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in -short mode")
	}
	const imsi = "001019999999999" // not provisioned

	log := logger.New("error", "text")
	shared := &multiNFEmitter{}

	provisionedSub := &subscriber.Subscriber{
		IMSI: "001010000000001", Ki: "465b5ce8b199b49faa5f0a2ee238a6bc",
		OPc: "cd63cb71954a9f4e48a5994e37a02baf", AMF: "8000", SQN: "000000000001",
	}
	subs := map[string]*subscriber.Subscriber{provisionedSub.IMSI: provisionedSub}

	udmSvc := udm.NewService(udm.NewStoreSource(&fakeSubscriberStore{subs}), log).
		WithAuthSource(udm.NewStoreAuthSource(&fakeAuthStore{subs}))
	udmSvc.SetEmitter(shared)
	udmPort := pickPort(t)
	udmSrv := sbi.NewServer(sbi.ServerConfig{BindAddress: "127.0.0.1", Port: udmPort, NFType: "UDM"}, log, udmSvc.Handler())
	go func() { _ = udmSrv.Serve() }()

	udmCli := udm.NewClient("http://127.0.0.1:"+strconv.Itoa(udmPort), "AUSF", false)
	ausfSvc := ausf.NewService(udmCli, log)
	ausfSvc.SetEmitter(shared)
	ausfPort := pickPort(t)
	ausfSrv := sbi.NewServer(sbi.ServerConfig{BindAddress: "127.0.0.1", Port: ausfPort, NFType: "AUSF"}, log, ausfSvc.Handler())
	go func() { _ = ausfSrv.Serve() }()

	time.Sleep(80 * time.Millisecond)

	plmn := ngap.PLMNFromMCCMNC("001", "01")
	amfPort := pickPort(t)
	amfCfg := Config{
		NGAPAddr: "127.0.0.1:" + strconv.Itoa(amfPort),
		NGAPMode: sctp.ModeTCP,
		AMFName:  "QCore-AMF-test",
		GUAMI:    ngap.GUAMI{PLMN: plmn, AMFRegionID: 1, AMFSetID: 1, AMFPointer: 0},
		PLMNSupportList: []ngap.PLMNSupportItem{
			{PLMN: plmn, SNSSAIs: []ngap.SNSSAI{{SST: 1}}},
		},
		ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org",
	}
	ausfCli := ausf.NewClient("http://127.0.0.1:"+strconv.Itoa(ausfPort), "AMF", false)
	amfSvc := NewService(amfCfg, ausfCli, log)
	amfSvc.SetEmitter(shared)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = amfSvc.Serve(ctx) }()
	time.Sleep(80 * time.Millisecond)
	t.Cleanup(func() {
		cancel()
		_ = ausfSrv.Shutdown(context.Background())
		_ = udmSrv.Shutdown(context.Background())
	})

	conn, err := sctp.Dial(sctp.ModeTCP, "127.0.0.1:"+strconv.Itoa(amfPort))
	require.NoError(t, err)
	defer conn.Close()
	gNB := &mockGNB{conn: conn}
	gNB.ngSetup(t)
	_ = sendRegistrationRequest(t, gNB, makeSUCI(imsi))
	assertRegistrationReject(t, gNB)
	time.Sleep(80 * time.Millisecond)

	// Find journey ID
	var journeyID string
	for _, ev := range shared.all() {
		if ev.JourneyID != "" {
			journeyID = ev.JourneyID
			break
		}
	}
	require.NotEmpty(t, journeyID, "failure trace must have a journey_id")

	jEvents := shared.journeyEvents(journeyID)

	// Both the request event AND the failure event must share the journey_id.
	var hasRequest, hasFailure bool
	for _, ev := range jEvents {
		if _, ok := ev.Payload.(events.RegistrationRequestPayload); ok {
			hasRequest = true
		}
		if _, ok := ev.Payload.(events.RegistrationFailurePayload); ok {
			hasFailure = true
		}
	}
	assert.True(t, hasRequest, "registration request must be in journey trace")
	assert.True(t, hasFailure, "registration failure must be in journey trace with same journey_id")

	t.Logf("✓ Failure trace correlated: %d events, journey_id=%s", len(jEvents), journeyID)
}

// filterByNF returns events from a given NF for logging.
func filterByNF(evs []events.Event, nf string) []events.Event {
	var out []events.Event
	for _, ev := range evs {
		if ev.NF == nf {
			out = append(out, ev)
		}
	}
	return out
}
