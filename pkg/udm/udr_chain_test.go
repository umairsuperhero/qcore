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
