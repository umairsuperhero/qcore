package udm

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
	"github.com/qcore-project/qcore/pkg/sbi/common"
	"github.com/qcore-project/qcore/pkg/subscriber"
)

// fakeStore satisfies SubscriberStore with an in-memory map. Keeps the
// UDM test free of gorm/sqlite — the pkg/subscriber package owns DB
// behaviour and has its own tests for it.
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

func (f *fakeStore) SetSQN(_ context.Context, imsi, newSQN string) error {
	s, ok := f.subs[imsi]
	if !ok {
		return fmt.Errorf("subscriber %s not found", imsi)
	}
	s.SQN = newSQN
	return nil
}

// TestUDM_SDM_AmData spins the UDM up behind a real pkg/sbi server in h2c
// mode and exercises the Nudm_SDM /am-data endpoint end-to-end: happy
// path, unknown SUPI (404), malformed SUPI (400). That covers the three
// things every SBI endpoint must get right: shape on success, spec-cause
// on miss, defensive parsing.
func TestUDM_SDM_AmData(t *testing.T) {
	log := logger.New("error", "text")

	store := &fakeStore{subs: map[string]*subscriber.Subscriber{
		"001010000000001": {
			IMSI:   "001010000000001",
			MSISDN: "15551234567",
			Ki:     "465b5ce8b199b49faa5f0a2ee238a6bc",
			OPc:    "cd63cb71954a9f4e48a5994e37a02baf",
		},
	}}

	udm := NewService(NewStoreSource(store), log)

	port := pickFreePort(t)
	srv := sbi.NewServer(sbi.ServerConfig{
		BindAddress: "127.0.0.1",
		Port:        port,
		NFType:      "UDM",
	}, log, udm.Handler())

	go func() { _ = srv.Serve() }()
	// Give the listener a moment to bind. Mirrors pkg/sbi's own test.
	time.Sleep(50 * time.Millisecond)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})

	client := sbi.NewClient("http://127.0.0.1:"+strconv.Itoa(port), "TEST-CALLER", false)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	t.Run("happy path", func(t *testing.T) {
		var resp common.AccessAndMobilitySubscriptionData
		if err := client.DoJSON(ctx, "GET", "/nudm-sdm/v2/imsi-001010000000001/am-data", nil, &resp); err != nil {
			t.Fatalf("DoJSON: %v", err)
		}
		if len(resp.Gpsis) != 1 || resp.Gpsis[0] != "msisdn-15551234567" {
			t.Errorf("expected gpsis=[msisdn-15551234567], got %v", resp.Gpsis)
		}
		if resp.SubscribedUeAmbr == nil || resp.SubscribedUeAmbr.Uplink == "" {
			t.Errorf("expected SubscribedUeAmbr populated, got %+v", resp.SubscribedUeAmbr)
		}
	})

	t.Run("unknown SUPI returns 404 USER_NOT_FOUND", func(t *testing.T) {
		err := client.DoJSON(ctx, "GET", "/nudm-sdm/v2/imsi-999999999999999/am-data", nil, nil)
		pd, ok := err.(*sbi.ProblemDetails)
		if !ok {
			t.Fatalf("expected *sbi.ProblemDetails, got %T: %v", err, err)
		}
		if pd.Status != http.StatusNotFound {
			t.Errorf("status: want 404, got %d", pd.Status)
		}
		if pd.Cause != "USER_NOT_FOUND" {
			t.Errorf("cause: want USER_NOT_FOUND, got %q", pd.Cause)
		}
	})

	t.Run("malformed SUPI returns 400", func(t *testing.T) {
		err := client.DoJSON(ctx, "GET", "/nudm-sdm/v2/nai-foo@bar/am-data", nil, nil)
		pd, ok := err.(*sbi.ProblemDetails)
		if !ok {
			t.Fatalf("expected *sbi.ProblemDetails, got %T: %v", err, err)
		}
		if pd.Status != http.StatusBadRequest {
			t.Errorf("status: want 400, got %d", pd.Status)
		}
	})
}

// pickFreePort grabs an ephemeral port the OS hands out, then closes the
// listener so the server can bind it. Classic TOCTOU but fine on loopback
// in tests — lifted from pkg/sbi/server_test.go.
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
