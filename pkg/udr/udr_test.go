package udr

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/qcore-project/qcore/pkg/logger"
	"github.com/qcore-project/qcore/pkg/sbi"
	"github.com/qcore-project/qcore/pkg/subscriber"
)

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

// TestUDR_DataRepository_AmData covers the single endpoint shipped in
// this cut. Happy path + unknown ueId (404/DATA_NOT_FOUND) + malformed
// ueId (400). Same three shapes as pkg/udm's test — UDR gets the same
// guarantees from the same pkg/sbi plumbing.
func TestUDR_DataRepository_AmData(t *testing.T) {
	log := logger.New("error", "text")

	store := &fakeStore{subs: map[string]*subscriber.Subscriber{
		"001010000000001": {
			IMSI:   "001010000000001",
			MSISDN: "15551234567",
			Ki:     "465b5ce8b199b49faa5f0a2ee238a6bc",
			OPc:    "cd63cb71954a9f4e48a5994e37a02baf",
		},
	}}

	udr := NewService(store, log)

	port := pickFreePort(t)
	srv := sbi.NewServer(sbi.ServerConfig{
		BindAddress: "127.0.0.1",
		Port:        port,
		NFType:      "UDR",
	}, log, udr.Handler())

	go func() { _ = srv.Serve() }()
	time.Sleep(50 * time.Millisecond)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})

	client := sbi.NewClient("http://127.0.0.1:"+strconv.Itoa(port), "TEST-UDM", false)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	const path = "/nudr-dr/v2/subscription-data/imsi-001010000000001/00101/provisioned-data/am-data"

	t.Run("happy path", func(t *testing.T) {
		var resp AccessAndMobilitySubscriptionData
		if err := client.DoJSON(ctx, "GET", path, nil, &resp); err != nil {
			t.Fatalf("DoJSON: %v", err)
		}
		if len(resp.Gpsis) != 1 || resp.Gpsis[0] != "msisdn-15551234567" {
			t.Errorf("expected gpsis=[msisdn-15551234567], got %v", resp.Gpsis)
		}
		if resp.SubscribedUeAmbr == nil || resp.SubscribedUeAmbr.Uplink == "" {
			t.Errorf("expected SubscribedUeAmbr populated, got %+v", resp.SubscribedUeAmbr)
		}
	})

	t.Run("unknown ueId returns 404 DATA_NOT_FOUND", func(t *testing.T) {
		err := client.DoJSON(ctx, "GET", "/nudr-dr/v2/subscription-data/imsi-999999999999999/00101/provisioned-data/am-data", nil, nil)
		pd, ok := err.(*sbi.ProblemDetails)
		if !ok {
			t.Fatalf("expected *sbi.ProblemDetails, got %T: %v", err, err)
		}
		if pd.Status != http.StatusNotFound {
			t.Errorf("status: want 404, got %d", pd.Status)
		}
		if pd.Cause != "DATA_NOT_FOUND" {
			t.Errorf("cause: want DATA_NOT_FOUND, got %q", pd.Cause)
		}
	})

	t.Run("malformed ueId returns 400", func(t *testing.T) {
		err := client.DoJSON(ctx, "GET", "/nudr-dr/v2/subscription-data/nai-foo@bar/00101/provisioned-data/am-data", nil, nil)
		pd, ok := err.(*sbi.ProblemDetails)
		if !ok {
			t.Fatalf("expected *sbi.ProblemDetails, got %T: %v", err, err)
		}
		if pd.Status != http.StatusBadRequest {
			t.Errorf("status: want 400, got %d", pd.Status)
		}
	})
}

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
