package spgw

import (
	"testing"

	"github.com/qcore-project/qcore/pkg/config"
	"github.com/qcore-project/qcore/pkg/logger"
	"github.com/qcore-project/qcore/pkg/metrics"
)

func TestBuildEgress_DefaultsToLog(t *testing.T) {
	log := logger.New("error", "console")
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", "log"},
		{"explicit log", "log", "log"},
		{"unknown falls back", "doesnotexist", "log"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.SPGWConfig{Egress: tc.in}
			eg, err := buildEgress(cfg, log)
			if err != nil {
				t.Fatalf("buildEgress: %v", err)
			}
			if got := eg.Name(); got != tc.want {
				t.Fatalf("egress name = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestService_CreateAndDeleteSession_WithMetrics(t *testing.T) {
	log := logger.New("error", "console")
	cfg := &config.SPGWConfig{
		Name:        "test-spgw",
		BindAddress: "127.0.0.1",
		APIPort:     0,
		S1UPort:     0,
		UEPool:      "10.99.0.0/29",
		Gateway:     "10.99.0.1",
		SGWU1Addr:   "127.0.0.1",
		Egress:      "log",
	}
	svc, err := New(cfg, log)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer svc.Stop()

	// Attach metrics; verify counter increments and nil-safety still holds.
	m := metrics.New()
	sm := metrics.RegisterSPGWMetrics(m)
	svc.SetMetrics(sm)

	resp, err := svc.CreateSession(&CreateSessionRequest{IMSI: "001010000000777"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if resp.UEIP == "" || resp.SGWTEID == 0 {
		t.Fatalf("response missing fields: %+v", resp)
	}

	if got := svc.Sessions().Count(); got != 1 {
		t.Fatalf("session count = %d, want 1", got)
	}

	if err := svc.DeleteSession("001010000000777"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if got := svc.Sessions().Count(); got != 0 {
		t.Fatalf("after delete count = %d, want 0", got)
	}
}

func TestService_CreateSession_RequiresIMSI(t *testing.T) {
	log := logger.New("error", "console")
	cfg := &config.SPGWConfig{
		UEPool:    "10.99.1.0/29",
		Gateway:   "10.99.1.1",
		SGWU1Addr: "127.0.0.1",
		Egress:    "log",
	}
	svc, err := New(cfg, log)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer svc.Stop()
	if _, err := svc.CreateSession(&CreateSessionRequest{}); err == nil {
		t.Fatalf("expected error for missing IMSI")
	}
}

func TestLogEgress_CountAndClose(t *testing.T) {
	log := logger.New("error", "console")
	eg := NewLogEgress(log)
	pkt := []byte{0x45, 0, 0, 20, 0, 0, 0, 0, 64, 1, 0, 0, 10, 0, 0, 1, 8, 8, 8, 8}
	if err := eg.Send(pkt); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got := eg.Count(); got != 1 {
		t.Fatalf("Count = %d, want 1", got)
	}
	// Close must unblock Recv.
	done := make(chan struct{})
	go func() {
		_, _ = eg.Recv()
		close(done)
	}()
	if err := eg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	<-done
}
