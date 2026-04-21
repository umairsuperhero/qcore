package ausf

import (
	"context"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/qcore-project/qcore/pkg/logger"
	"github.com/qcore-project/qcore/pkg/sbi"
	"github.com/qcore-project/qcore/pkg/subscriber"
	"github.com/qcore-project/qcore/pkg/udm"
)

// fakeAuthGen mirrors the pattern in pkg/udm: runs real 5G-AKA crypto
// over an in-memory subscriber map, no gorm.
type fakeAuthGen struct {
	subs map[string]*subscriber.Subscriber
}

func (f *fakeAuthGen) Generate5GAuthVector(_ context.Context, imsi, snName string) (*subscriber.AuthVector5G, error) {
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

// TestAUSF_EndToEnd spins AUSF + UDM on two loopback h2c servers and
// drives the full 5G-AKA flow: POST ue-authentications → extract XRES*
// from the stored UDM vector (test back door) → PUT
// 5g-aka-confirmation → expect AUTHENTICATION_SUCCESS + KSEAF.
//
// The "extract XRES*" step is the one thing this test does that a real
// deployment doesn't: the UE computes RES* from the challenge, and we
// don't have a UE in-process. So we cheat by capturing the UDM's
// outbound vector and deriving RES* with the same Milenage call the UE
// would make. This is honest — it proves the AUSF store and compare
// logic works for RES* == XRES*, not just that the numbers match by
// accident.
func TestAUSF_EndToEnd(t *testing.T) {
	log := logger.New("error", "text")

	sub := &subscriber.Subscriber{
		IMSI: "001010000000001",
		Ki:   "465b5ce8b199b49faa5f0a2ee238a6bc",
		OPc:  "cd63cb71954a9f4e48a5994e37a02baf",
		AMF:  "8000",
		SQN:  "000000000001",
	}
	subs := map[string]*subscriber.Subscriber{"001010000000001": sub}
	gen := &fakeAuthGen{subs: subs}

	// Stand up UDM
	udmSvc := udm.NewService(udm.NewStoreSource(fakeStoreFromSubs(subs)), log).
		WithAuthSource(udm.NewStoreAuthSource(gen))
	udmPort := pickFreePort(t)
	udmSrv := sbi.NewServer(sbi.ServerConfig{BindAddress: "127.0.0.1", Port: udmPort, NFType: "UDM"}, log, udmSvc.Handler())
	go func() { _ = udmSrv.Serve() }()

	// Stand up AUSF pointed at UDM
	udmClient := udm.NewClient("http://127.0.0.1:"+strconv.Itoa(udmPort), "AUSF", false)
	ausfSvc := NewService(udmClient, log)
	ausfPort := pickFreePort(t)
	ausfSrv := sbi.NewServer(sbi.ServerConfig{BindAddress: "127.0.0.1", Port: ausfPort, NFType: "AUSF"}, log, ausfSvc.Handler())
	go func() { _ = ausfSrv.Serve() }()

	time.Sleep(100 * time.Millisecond)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = ausfSrv.Shutdown(ctx)
		_ = udmSrv.Shutdown(ctx)
	})

	// AMF-style caller hitting AUSF
	amf := sbi.NewClient("http://127.0.0.1:"+strconv.Itoa(ausfPort), "AMF", false)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	const supi = "imsi-001010000000001"
	const snName = "5G:mnc001.mcc001.3gppnetwork.org"

	// Step 1: AMF → AUSF → UDM → Av5gAka back to AMF.
	var authCtx UEAuthenticationCtx
	if err := amf.DoJSON(ctx, "POST", "/nausf-auth/v1/ue-authentications",
		&AuthenticationInfo{SupiOrSuci: supi, ServingNetworkName: snName}, &authCtx); err != nil {
		t.Fatalf("POST ue-authentications: %v", err)
	}
	if authCtx.AuthType != AuthType5GAka {
		t.Fatalf("authType: got %q", authCtx.AuthType)
	}
	if authCtx.Av5gAuthData.HXResStar == "" {
		t.Fatal("HXResStar missing")
	}
	link, ok := authCtx.Links["5g-aka"]
	if !ok || link.Href == "" {
		t.Fatalf("missing _links[\"5g-aka\"]: %+v", authCtx.Links)
	}

	// Step 2: UE would compute RES* from RAND using its own Ki/OPc. We
	// simulate that here — NOT in the AUSF production path — by
	// re-running Milenage with the same inputs and deriving RES* the
	// same way UDM derived XRES*. On a real UE the RES* comes over NAS.
	resStar := computeUESideRESStar(t, sub, authCtx.Av5gAuthData.RAND, snName)

	// Step 3: AMF → AUSF confirmation.
	var confirm ConfirmationDataResponse
	if err := amf.DoJSON(ctx, "PUT", link.Href, &ConfirmationData{ResStar: resStar}, &confirm); err != nil {
		t.Fatalf("PUT confirmation: %v", err)
	}
	if confirm.AuthResult != AuthResultSuccess {
		t.Errorf("authResult: want SUCCESS, got %q", confirm.AuthResult)
	}
	if confirm.Supi != supi {
		t.Errorf("supi: got %q", confirm.Supi)
	}
	if len(confirm.Kseaf) != 64 {
		t.Errorf("KSEAF: want 64 hex chars, got %d (%q)", len(confirm.Kseaf), confirm.Kseaf)
	}

	// Step 4: same ctxId must not be reusable.
	err := amf.DoJSON(ctx, "PUT", link.Href, &ConfirmationData{ResStar: resStar}, nil)
	pd, isPD := err.(*sbi.ProblemDetails)
	if !isPD {
		t.Fatalf("reuse: want ProblemDetails, got %T: %v", err, err)
	}
	if pd.Status != http.StatusNotFound {
		t.Errorf("reuse: want 404, got %d", pd.Status)
	}
}

// TestAUSF_ConfirmationFailure — a wrong RES* must produce FAILURE and
// still burn the ctx (can't retry).
func TestAUSF_ConfirmationFailure(t *testing.T) {
	log := logger.New("error", "text")

	sub := &subscriber.Subscriber{
		IMSI: "001010000000001",
		Ki:   "465b5ce8b199b49faa5f0a2ee238a6bc",
		OPc:  "cd63cb71954a9f4e48a5994e37a02baf",
		AMF:  "8000",
		SQN:  "000000000001",
	}
	subs := map[string]*subscriber.Subscriber{"001010000000001": sub}
	gen := &fakeAuthGen{subs: subs}

	udmSvc := udm.NewService(udm.NewStoreSource(fakeStoreFromSubs(subs)), log).
		WithAuthSource(udm.NewStoreAuthSource(gen))
	udmPort := pickFreePort(t)
	udmSrv := sbi.NewServer(sbi.ServerConfig{BindAddress: "127.0.0.1", Port: udmPort, NFType: "UDM"}, log, udmSvc.Handler())
	go func() { _ = udmSrv.Serve() }()

	udmClient := udm.NewClient("http://127.0.0.1:"+strconv.Itoa(udmPort), "AUSF", false)
	ausfSvc := NewService(udmClient, log)
	ausfPort := pickFreePort(t)
	ausfSrv := sbi.NewServer(sbi.ServerConfig{BindAddress: "127.0.0.1", Port: ausfPort, NFType: "AUSF"}, log, ausfSvc.Handler())
	go func() { _ = ausfSrv.Serve() }()

	time.Sleep(100 * time.Millisecond)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = ausfSrv.Shutdown(ctx)
		_ = udmSrv.Shutdown(ctx)
	})

	amf := sbi.NewClient("http://127.0.0.1:"+strconv.Itoa(ausfPort), "AMF", false)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var authCtx UEAuthenticationCtx
	if err := amf.DoJSON(ctx, "POST", "/nausf-auth/v1/ue-authentications",
		&AuthenticationInfo{SupiOrSuci: "imsi-001010000000001", ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org"}, &authCtx); err != nil {
		t.Fatalf("POST: %v", err)
	}
	link := authCtx.Links["5g-aka"]

	// Wrong RES*.
	var confirm ConfirmationDataResponse
	if err := amf.DoJSON(ctx, "PUT", link.Href,
		&ConfirmationData{ResStar: "00000000000000000000000000000000"}, &confirm); err != nil {
		t.Fatalf("PUT: %v", err)
	}
	if confirm.AuthResult != AuthResultFailure {
		t.Errorf("authResult: want FAILURE, got %q", confirm.AuthResult)
	}
	if confirm.Kseaf != "" {
		t.Errorf("KSEAF must not be returned on failure")
	}
}

// TestAUSF_UnknownSUPI — UDM 404 must surface as AUSF 404.
func TestAUSF_UnknownSUPI(t *testing.T) {
	log := logger.New("error", "text")
	subs := map[string]*subscriber.Subscriber{} // empty
	gen := &fakeAuthGen{subs: subs}

	udmSvc := udm.NewService(udm.NewStoreSource(fakeStoreFromSubs(subs)), log).
		WithAuthSource(udm.NewStoreAuthSource(gen))
	udmPort := pickFreePort(t)
	udmSrv := sbi.NewServer(sbi.ServerConfig{BindAddress: "127.0.0.1", Port: udmPort, NFType: "UDM"}, log, udmSvc.Handler())
	go func() { _ = udmSrv.Serve() }()

	udmClient := udm.NewClient("http://127.0.0.1:"+strconv.Itoa(udmPort), "AUSF", false)
	ausfSvc := NewService(udmClient, log)
	ausfPort := pickFreePort(t)
	ausfSrv := sbi.NewServer(sbi.ServerConfig{BindAddress: "127.0.0.1", Port: ausfPort, NFType: "AUSF"}, log, ausfSvc.Handler())
	go func() { _ = ausfSrv.Serve() }()

	time.Sleep(100 * time.Millisecond)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = ausfSrv.Shutdown(ctx)
		_ = udmSrv.Shutdown(ctx)
	})

	amf := sbi.NewClient("http://127.0.0.1:"+strconv.Itoa(ausfPort), "AMF", false)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := amf.DoJSON(ctx, "POST", "/nausf-auth/v1/ue-authentications",
		&AuthenticationInfo{SupiOrSuci: "imsi-999999999999999", ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org"}, nil)
	pd, ok := err.(*sbi.ProblemDetails)
	if !ok {
		t.Fatalf("want *sbi.ProblemDetails, got %T: %v", err, err)
	}
	if pd.Status != http.StatusNotFound {
		t.Errorf("status: want 404, got %d", pd.Status)
	}
	if pd.Cause != "USER_NOT_FOUND" {
		t.Errorf("cause: want USER_NOT_FOUND, got %q", pd.Cause)
	}
}

// computeUESideRESStar simulates what the UE does on the air: given RAND
// and its own Ki/OPc, it runs the same Milenage / Annex A.4 derivations
// that UDM ran and hands RES* to AUSF via AMF. We duplicate that here
// only because there's no UE in-process; the AUSF code never sees this
// path in production.
func computeUESideRESStar(t *testing.T, sub *subscriber.Subscriber, randHex, snName string) string {
	t.Helper()
	ki, err := sub.KiBytes()
	if err != nil {
		t.Fatalf("KiBytes: %v", err)
	}
	opc, err := sub.OPcBytes()
	if err != nil {
		t.Fatalf("OPcBytes: %v", err)
	}
	randBytes, err := hex.DecodeString(randHex)
	if err != nil || len(randBytes) != 16 {
		t.Fatalf("bad RAND hex %q", randHex)
	}
	var randArr [16]byte
	copy(randArr[:], randBytes)

	res, ck, ik, _, err := subscriber.F2345(ki, opc, randArr)
	if err != nil {
		t.Fatalf("F2345: %v", err)
	}
	resStar := subscriber.DeriveRESStar(ck, ik, snName, randArr, res)
	return hex.EncodeToString(resStar[:])
}

// fakeStoreFromSubs returns something that satisfies udm.SubscriberStore
// for the map we already built. Separate from fakeAuthGen because UDM
// wants a GetSubscriber-only interface for SDM.
func fakeStoreFromSubs(subs map[string]*subscriber.Subscriber) udm.SubscriberStore {
	return &fakeStore{subs: subs}
}

type fakeStore struct {
	subs map[string]*subscriber.Subscriber
}

func (f *fakeStore) GetSubscriber(_ context.Context, imsi string) (*subscriber.Subscriber, error) {
	s, ok := f.subs[imsi]
	if !ok {
		return nil, fmt.Errorf("subscriber %s not found", imsi)
	}
	return s, nil
}

// Lifted from pkg/sbi/server_test.go. Keeps each test self-contained on
// its own ephemeral port.
func pickFreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("pick port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port
}

