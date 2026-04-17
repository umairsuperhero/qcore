package nrf

import (
	"context"
	"errors"
	"testing"
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
