package nrf

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/qcore-project/qcore/pkg/events"
	"github.com/qcore-project/qcore/pkg/logger"
)

func sampleProfile(id string, nfType NFType, services ...string) *NFProfile {
	var svcs []NFService
	for _, s := range services {
		svcs = append(svcs, NFService{
			ServiceName: s,
			Versions:    []string{"v1"},
			Scheme:      "http",
			Port:        8080,
		})
	}
	return &NFProfile{
		NFInstanceID: id,
		NFType:       nfType,
		Services:     svcs,
		PLMN:         "00101",
	}
}

func TestInMemory_RegisterDiscover(t *testing.T) {
	m := NewInMemory()
	ctx := context.Background()

	udm := sampleProfile("udm-1", NFTypeUDM, "nudm-sdm", "nudm-ueau")
	ausf := sampleProfile("ausf-1", NFTypeAUSF, "nausf-auth")

	if err := m.Register(ctx, udm); err != nil {
		t.Fatalf("register udm: %v", err)
	}
	if err := m.Register(ctx, ausf); err != nil {
		t.Fatalf("register ausf: %v", err)
	}

	// Discover UDM — should return one.
	udms, err := m.Discover(ctx, DiscoveryQuery{TargetNFType: NFTypeUDM, RequesterType: NFTypeAMF})
	if err != nil {
		t.Fatalf("discover udm: %v", err)
	}
	if len(udms) != 1 || udms[0].NFInstanceID != "udm-1" {
		t.Errorf("expected 1 UDM 'udm-1', got %+v", udms)
	}

	// Service-filtered discovery.
	sdms, err := m.Discover(ctx, DiscoveryQuery{
		TargetNFType:  NFTypeUDM,
		RequesterType: NFTypeAMF,
		ServiceName:   "nudm-sdm",
	})
	if err != nil || len(sdms) != 1 {
		t.Errorf("expected 1 UDM with nudm-sdm, got len=%d err=%v", len(sdms), err)
	}

	// Service that nobody offers.
	none, err := m.Discover(ctx, DiscoveryQuery{
		TargetNFType:  NFTypeUDM,
		RequesterType: NFTypeAMF,
		ServiceName:   "nudm-nonexistent",
	})
	if err != nil || len(none) != 0 {
		t.Errorf("expected 0 UDMs with phantom service, got len=%d err=%v", len(none), err)
	}
}

func TestInMemory_DeregisterRemovesFromDiscovery(t *testing.T) {
	m := NewInMemory()
	ctx := context.Background()
	_ = m.Register(ctx, sampleProfile("udm-1", NFTypeUDM, "nudm-sdm"))

	if err := m.Deregister(ctx, "udm-1"); err != nil {
		t.Fatalf("deregister: %v", err)
	}
	udms, _ := m.Discover(ctx, DiscoveryQuery{TargetNFType: NFTypeUDM, RequesterType: NFTypeAMF})
	if len(udms) != 0 {
		t.Errorf("expected empty after deregister, got %+v", udms)
	}
}

func TestInMemory_DeregisterUnknownReturnsNotFound(t *testing.T) {
	m := NewInMemory()
	err := m.Deregister(context.Background(), "never-registered")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestInMemory_HeartbeatUnknownReturnsNotFound(t *testing.T) {
	m := NewInMemory()
	err := m.Heartbeat(context.Background(), "ghost")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestInMemory_RegisterValidation(t *testing.T) {
	m := NewInMemory()
	ctx := context.Background()

	if err := m.Register(ctx, nil); err == nil {
		t.Error("expected error on nil profile")
	}
	if err := m.Register(ctx, &NFProfile{NFType: NFTypeUDM}); err == nil {
		t.Error("expected error on missing instance id")
	}
	if err := m.Register(ctx, &NFProfile{NFInstanceID: "x"}); err == nil {
		t.Error("expected error on missing nf type")
	}
}

// TestLifecycleManager_RegistersAndDeregisters verifies that Start registers
// the NF with the InMemory NRF and that cancelling the ctx triggers
// deregistration, satisfying the A3 acceptance criterion.
func TestLifecycleManager_RegistersAndDeregisters(t *testing.T) {
	m := NewInMemory()
	log := logger.New("error", "text")
	profile := sampleProfile("amf-1", NFTypeAMF)
	profile.HeartbeatTimer = 500 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())

	lcm := NewLifecycleManager(m, profile, "http://nrf:8083", &events.NoopEmitter{}, log)
	done := make(chan struct{})
	go func() {
		lcm.Start(ctx)
		close(done)
	}()

	// Give Start time to register.
	time.Sleep(80 * time.Millisecond)

	amfs, err := m.Discover(context.Background(), DiscoveryQuery{TargetNFType: NFTypeAMF, RequesterType: NFTypeSMF})
	if err != nil || len(amfs) != 1 {
		t.Fatalf("expected AMF registered; got len=%d err=%v", len(amfs), err)
	}

	// Cancel → deregister.
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("LifecycleManager.Start did not return after ctx cancel")
	}

	amfs, _ = m.Discover(context.Background(), DiscoveryQuery{TargetNFType: NFTypeAMF, RequesterType: NFTypeSMF})
	if len(amfs) != 0 {
		t.Errorf("expected AMF deregistered after ctx cancel; got %+v", amfs)
	}
}

// TestDiscoverFirst_NRFHit verifies the happy path: NRF returns a profile and
// DiscoverFirst returns its URL.
func TestDiscoverFirst_NRFHit(t *testing.T) {
	m := NewInMemory()
	p := sampleProfile("ausf-1", NFTypeAUSF, "nausf-auth")
	p.Services[0].IPAddr = "127.0.0.1"
	p.Services[0].Port = 8006
	_ = m.Register(context.Background(), p)

	log := logger.New("error", "text")
	url, err := DiscoverFirst(context.Background(), m,
		DiscoveryQuery{TargetNFType: NFTypeAUSF, RequesterType: NFTypeAMF},
		"http://fallback:9999", &events.NoopEmitter{}, log)
	if err != nil {
		t.Fatalf("DiscoverFirst: %v", err)
	}
	if url != "http://127.0.0.1:8006" {
		t.Errorf("expected NRF-resolved URL, got %q", url)
	}
}

// TestDiscoverFirst_FallsBackOnMiss verifies that when the NRF has no match,
// DiscoverFirst returns the static fallback URL (not an error).
func TestDiscoverFirst_FallsBackOnMiss(t *testing.T) {
	m := NewInMemory() // empty registry
	log := logger.New("error", "text")
	url, err := DiscoverFirst(context.Background(), m,
		DiscoveryQuery{TargetNFType: NFTypeAUSF, RequesterType: NFTypeAMF},
		"http://static-ausf:8006", &events.NoopEmitter{}, log)
	if err != nil {
		t.Fatalf("expected fallback, got error: %v", err)
	}
	if url != "http://static-ausf:8006" {
		t.Errorf("expected static fallback URL, got %q", url)
	}
}

// TestDiscoverFirst_ErrorOnEmptyFallback verifies that when the NRF misses
// and no static fallback is configured, DiscoverFirst returns an error.
func TestDiscoverFirst_ErrorOnEmptyFallback(t *testing.T) {
	m := NewInMemory()
	log := logger.New("error", "text")
	_, err := DiscoverFirst(context.Background(), m,
		DiscoveryQuery{TargetNFType: NFTypeAUSF, RequesterType: NFTypeAMF},
		"", &events.NoopEmitter{}, log)
	if err == nil {
		t.Fatal("expected error when NRF misses and fallback is empty")
	}
}

func TestInMemory_DiscoverFiltersSuspended(t *testing.T) {
	m := NewInMemory()
	ctx := context.Background()
	p := sampleProfile("udm-1", NFTypeUDM, "nudm-sdm")
	p.NFStatus = StatusSuspended
	if err := m.Register(ctx, p); err != nil {
		t.Fatalf("register: %v", err)
	}
	udms, err := m.Discover(ctx, DiscoveryQuery{TargetNFType: NFTypeUDM, RequesterType: NFTypeAMF})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(udms) != 0 {
		t.Errorf("suspended NF should be excluded from discovery; got %+v", udms)
	}
}
