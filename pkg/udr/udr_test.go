package udr

import (
	"context"
	"errors"
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
	// Mirror the production pkg/subscriber.Service.SetSQN validation so the
	// handler's "hex"/"12 hex" error-string branch is reachable in tests.
	if len(newSQN) != 12 {
		return fmt.Errorf("SQN must be 12 hex chars, got %d", len(newSQN))
	}
	for _, c := range newSQN {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return fmt.Errorf("SQN must be hex: %q", newSQN)
		}
	}
	s, ok := f.subs[imsi]
	if !ok {
		return fmt.Errorf("subscriber %s not found", imsi)
	}
	s.SQN = newSQN
	return nil
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
		var resp common.AccessAndMobilitySubscriptionData
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

// TestUDR_AuthenticationSubscription covers the auth-sub endpoint: the
// shape the UDM UEAU's UDR-backed AuthSource consumes. Drives the typed
// client (Client.GetAuthenticationSubscription) on the happy path so
// both the wire and the client-side error-mapping are exercised.
func TestUDR_AuthenticationSubscription(t *testing.T) {
	log := logger.New("error", "text")

	store := &fakeStore{subs: map[string]*subscriber.Subscriber{
		"001010000000001": {
			IMSI: "001010000000001",
			Ki:   "465b5ce8b199b49faa5f0a2ee238a6bc",
			OPc:  "cd63cb71954a9f4e48a5994e37a02baf",
			AMF:  "b9b9",
			SQN:  "ff9bb4d0b607",
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

	baseURL := "http://127.0.0.1:" + strconv.Itoa(port)
	client := NewClient(baseURL, "TEST-UDM", false)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	t.Run("happy path via typed client", func(t *testing.T) {
		resp, err := client.GetAuthenticationSubscription(ctx, "imsi-001010000000001")
		if err != nil {
			t.Fatalf("GetAuthenticationSubscription: %v", err)
		}
		if resp.AuthenticationMethod != "5G_AKA" {
			t.Errorf("authenticationMethod: got %q, want 5G_AKA", resp.AuthenticationMethod)
		}
		if resp.EncPermanentKey != "465b5ce8b199b49faa5f0a2ee238a6bc" {
			t.Errorf("encPermanentKey: got %q", resp.EncPermanentKey)
		}
		if resp.EncOpcKey != "cd63cb71954a9f4e48a5994e37a02baf" {
			t.Errorf("encOpcKey: got %q", resp.EncOpcKey)
		}
		if resp.AuthenticationManagementField != "b9b9" {
			t.Errorf("AMF: got %q", resp.AuthenticationManagementField)
		}
		if resp.AlgorithmId != "milenage" {
			t.Errorf("algorithmId: got %q", resp.AlgorithmId)
		}
		if resp.SequenceNumber == nil || resp.SequenceNumber.Sqn != "ff9bb4d0b607" {
			t.Errorf("sqn: got %+v", resp.SequenceNumber)
		}
		if resp.Supi != "imsi-001010000000001" {
			t.Errorf("supi: got %q", resp.Supi)
		}
	})

	t.Run("unknown ueId returns ErrNotFound via typed client", func(t *testing.T) {
		_, err := client.GetAuthenticationSubscription(ctx, "imsi-999999999999999")
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("want ErrNotFound, got %v", err)
		}
	})

	t.Run("malformed ueId returns ErrBadUeID via typed client", func(t *testing.T) {
		_, err := client.GetAuthenticationSubscription(ctx, "nai-foo@bar")
		if !errors.Is(err, ErrBadUeID) {
			t.Errorf("want ErrBadUeID, got %v", err)
		}
	})

	t.Run("PATCH replace /sequenceNumber/sqn persists to store", func(t *testing.T) {
		if err := client.UpdateAuthSubscriptionSQN(ctx, "imsi-001010000000001", "ff9bb4d0b608"); err != nil {
			t.Fatalf("UpdateAuthSubscriptionSQN: %v", err)
		}
		// Roundtrip — GET should return the new SQN.
		got, err := client.GetAuthenticationSubscription(ctx, "imsi-001010000000001")
		if err != nil {
			t.Fatalf("re-GET: %v", err)
		}
		if got.SequenceNumber == nil || got.SequenceNumber.Sqn != "ff9bb4d0b608" {
			t.Errorf("sqn did not persist: %+v", got.SequenceNumber)
		}
	})

	t.Run("PATCH on unknown ueId returns ErrNotFound", func(t *testing.T) {
		err := client.UpdateAuthSubscriptionSQN(ctx, "imsi-999999999999999", "000000000001")
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("want ErrNotFound, got %v", err)
		}
	})

	t.Run("PATCH with unsupported op returns 422", func(t *testing.T) {
		// Drive the raw SBI client so we can shape a non-standard body.
		raw := sbi.NewClient(baseURL, "TEST-UDM", false)
		body := []map[string]any{
			{"op": "remove", "path": "/sequenceNumber/sqn"},
		}
		err := raw.DoJSON(ctx, "PATCH",
			"/nudr-dr/v2/subscription-data/imsi-001010000000001/authentication-data/authentication-subscription",
			body, nil)
		pd, ok := err.(*sbi.ProblemDetails)
		if !ok {
			t.Fatalf("want *ProblemDetails, got %T: %v", err, err)
		}
		if pd.Status != http.StatusUnprocessableEntity {
			t.Errorf("status: want 422, got %d", pd.Status)
		}
	})

	t.Run("PATCH with malformed SQN value returns 400", func(t *testing.T) {
		err := client.UpdateAuthSubscriptionSQN(ctx, "imsi-001010000000001", "zzzz")
		if !errors.Is(err, ErrBadUeID) {
			t.Errorf("want ErrBadUeID (via 400 path), got %v", err)
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
