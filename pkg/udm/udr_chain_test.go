package udm

import (
	"context"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/qcore-project/qcore/pkg/logger"
	"github.com/qcore-project/qcore/pkg/sbi"
	"github.com/qcore-project/qcore/pkg/sbi/common"
	"github.com/qcore-project/qcore/pkg/subscriber"
	"github.com/qcore-project/qcore/pkg/udr"
)

// TestUDM_over_UDR_chain stands the full UDM→UDR→store chain up over two
// real pkg/sbi servers on loopback and drives it with an SBI client, so
// the AmDataSource seam is exercised end-to-end over HTTP/2 h2c rather
// than through an in-process shim. Three shapes: happy path,
// unknown SUPI (UDR 404/DATA_NOT_FOUND → UDM 404/USER_NOT_FOUND),
// malformed SUPI (UDR 400 → UDM 400/MANDATORY_IE_INCORRECT).
func TestUDM_over_UDR_chain(t *testing.T) {
	log := logger.New("error", "text")

	store := &fakeStore{subs: map[string]*subscriber.Subscriber{
		"001010000000001": {
			IMSI:   "001010000000001",
			MSISDN: "15551234567",
			Ki:     "465b5ce8b199b49faa5f0a2ee238a6bc",
			OPc:    "cd63cb71954a9f4e48a5994e37a02baf",
		},
	}}

	// 1. UDR server — the stateful end of the chain.
	udrSvc := udr.NewService(store, log)
	udrPort := pickFreePort(t)
	udrSrv := sbi.NewServer(sbi.ServerConfig{
		BindAddress: "127.0.0.1",
		Port:        udrPort,
		NFType:      "UDR",
	}, log, udrSvc.Handler())
	go func() { _ = udrSrv.Serve() }()

	// 2. UDM server — uses a UDR client as its AmDataSource.
	udrClient := udr.NewClient("http://127.0.0.1:"+strconv.Itoa(udrPort), "UDM", false)
	udmSvc := NewService(NewUDRSource(udrClient, "00101"), log)
	udmPort := pickFreePort(t)
	udmSrv := sbi.NewServer(sbi.ServerConfig{
		BindAddress: "127.0.0.1",
		Port:        udmPort,
		NFType:      "UDM",
	}, log, udmSvc.Handler())
	go func() { _ = udmSrv.Serve() }()

	time.Sleep(100 * time.Millisecond)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = udmSrv.Shutdown(ctx)
		_ = udrSrv.Shutdown(ctx)
	})

	caller := sbi.NewClient("http://127.0.0.1:"+strconv.Itoa(udmPort), "TEST-AMF", false)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	t.Run("happy path traverses UDM→UDR", func(t *testing.T) {
		var resp common.AccessAndMobilitySubscriptionData
		if err := caller.DoJSON(ctx, "GET", "/nudm-sdm/v2/imsi-001010000000001/am-data", nil, &resp); err != nil {
			t.Fatalf("DoJSON: %v", err)
		}
		if len(resp.Gpsis) != 1 || resp.Gpsis[0] != "msisdn-15551234567" {
			t.Errorf("gpsis: got %v", resp.Gpsis)
		}
		if resp.SubscribedUeAmbr == nil || resp.SubscribedUeAmbr.Uplink == "" {
			t.Errorf("SubscribedUeAmbr: got %+v", resp.SubscribedUeAmbr)
		}
	})

	t.Run("unknown SUPI surfaces as UDM 404 USER_NOT_FOUND", func(t *testing.T) {
		err := caller.DoJSON(ctx, "GET", "/nudm-sdm/v2/imsi-999999999999999/am-data", nil, nil)
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
	})

	t.Run("malformed SUPI surfaces as UDM 400", func(t *testing.T) {
		err := caller.DoJSON(ctx, "GET", "/nudm-sdm/v2/nai-foo@bar/am-data", nil, nil)
		pd, ok := err.(*sbi.ProblemDetails)
		if !ok {
			t.Fatalf("want *sbi.ProblemDetails, got %T: %v", err, err)
		}
		if pd.Status != http.StatusBadRequest {
			t.Errorf("status: want 400, got %d", pd.Status)
		}
	})
}

// TestUDM_UEAU_over_UDR_chain is the UEAU twin of TestUDM_over_UDR_chain:
// UDM's AuthSource is a UDR client, UDR reads creds from its SubscriberStore,
// everything goes over h2c on loopback. The important property is that two
// consecutive AMF→UDM UEAU calls must produce different AUTN values, proving
// that the UDR-backed UEAU is both (a) reading fresh SQN on each call and
// (b) PATCHing SQN back to UDR so the next call sees the advance.
func TestUDM_UEAU_over_UDR_chain(t *testing.T) {
	log := logger.New("error", "text")

	store := &fakeStore{subs: map[string]*subscriber.Subscriber{
		"001010000000001": {
			IMSI: "001010000000001",
			Ki:   "465b5ce8b199b49faa5f0a2ee238a6bc",
			OPc:  "cd63cb71954a9f4e48a5994e37a02baf",
			AMF:  "b9b9",
			SQN:  "000000000001",
		},
	}}

	udrSvc := udr.NewService(store, log)
	udrPort := pickFreePort(t)
	udrSrv := sbi.NewServer(sbi.ServerConfig{
		BindAddress: "127.0.0.1", Port: udrPort, NFType: "UDR",
	}, log, udrSvc.Handler())
	go func() { _ = udrSrv.Serve() }()

	udrClient := udr.NewClient("http://127.0.0.1:"+strconv.Itoa(udrPort), "UDM", false)
	// UDM uses UDR for BOTH SDM (via NewUDRSource) and UEAU (via NewUDRAuthSource).
	udmSvc := NewService(NewUDRSource(udrClient, "00101"), log).
		WithAuthSource(NewUDRAuthSource(udrClient))
	udmPort := pickFreePort(t)
	udmSrv := sbi.NewServer(sbi.ServerConfig{
		BindAddress: "127.0.0.1", Port: udmPort, NFType: "UDM",
	}, log, udmSvc.Handler())
	go func() { _ = udmSrv.Serve() }()

	time.Sleep(100 * time.Millisecond)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = udmSrv.Shutdown(ctx)
		_ = udrSrv.Shutdown(ctx)
	})

	caller := sbi.NewClient("http://127.0.0.1:"+strconv.Itoa(udmPort), "TEST-AUSF", false)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req := &AuthenticationInfoRequest{ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org"}

	var first, second AuthenticationInfoResult
	if err := caller.DoJSON(ctx, "POST", "/nudm-ueau/v1/imsi-001010000000001/security-information/generate-auth-data", req, &first); err != nil {
		t.Fatalf("first UEAU: %v", err)
	}
	if first.AuthType != AuthType5GAka || first.AuthenticationVector == nil {
		t.Fatalf("first vector missing: %+v", first)
	}
	if len(first.AuthenticationVector.AUTN) != 32 {
		t.Errorf("first AUTN hex: want 32 chars, got %d (%q)", len(first.AuthenticationVector.AUTN), first.AuthenticationVector.AUTN)
	}

	if err := caller.DoJSON(ctx, "POST", "/nudm-ueau/v1/imsi-001010000000001/security-information/generate-auth-data", req, &second); err != nil {
		t.Fatalf("second UEAU: %v", err)
	}

	// Distinct AUTN is the observable proof that SQN advanced in UDR
	// between calls. (RAND also changes, which changes AUTN too — but if
	// SQN writeback were broken the fakeStore would still have SQN=1
	// both times, and SQN⊕AK bytes inside AUTN would be the same modulo
	// RAND, still yielding different AUTN — so we additionally check
	// the store directly.)
	if first.AuthenticationVector.AUTN == second.AuthenticationVector.AUTN {
		t.Errorf("AUTN repeated across calls: %q", first.AuthenticationVector.AUTN)
	}
	if store.subs["001010000000001"].SQN != "000000000003" {
		t.Errorf("SQN in store: want 000000000003 after two advances, got %s", store.subs["001010000000001"].SQN)
	}
}
