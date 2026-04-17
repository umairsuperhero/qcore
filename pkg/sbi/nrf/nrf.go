// Package nrf models the 5G Network Repository Function.
//
// Per 3GPP TS 29.510, the NRF is the service-discovery backbone of the 5G
// control plane: every NF (AMF, SMF, AUSF, UDM, UDR, PCF, ...) registers
// itself on startup, heartbeats while alive, and queries the NRF when it
// needs to find a peer.
//
// Scope of this package:
//	- NFProfile / NFService / NFStatus types (a subset of TS 29.510 §6.1)
//	- Client interface: Register / Deregister / Discover / Heartbeat
//	- Two implementations:
//	    * InMemory — single-process registry, ideal for unit tests and for
//	      the single-binary `qcore dev` posture we want in v0.8.
//	    * HTTP — thin wrapper around pkg/sbi.Client for a real network NRF.
//
// A full NRF server implementation (exposing Nnrf_NFManagement +
// Nnrf_NFDiscovery over SBI) is intentionally *not* here — it belongs in
// pkg/nrf when that NF service lands.
package nrf

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// NFStatus values per TS 29.510 §6.1.6.2.2.
type NFStatus string

const (
	StatusRegistered   NFStatus = "REGISTERED"
	StatusSuspended    NFStatus = "SUSPENDED"
	StatusUndiscoverable NFStatus = "UNDISCOVERABLE"
)

// NFType values most relevant to QCore's v1.0 scope. Not exhaustive; add as
// needed. Values match 3GPP TS 29.510 §6.1.6.3.3.
type NFType string

const (
	NFTypeAMF NFType = "AMF"
	NFTypeSMF NFType = "SMF"
	NFTypeUPF NFType = "UPF"
	NFTypeAUSF NFType = "AUSF"
	NFTypeUDM NFType = "UDM"
	NFTypeUDR NFType = "UDR"
	NFTypePCF NFType = "PCF"
	NFTypeNRF NFType = "NRF"
)

// NFService describes one service-based interface exposed by an NF. An NF may
// expose several (e.g. AMF offers Namf_Communication and Namf_EventExposure).
type NFService struct {
	ServiceName string   `json:"serviceName"` // "nudm-sdm", "namf-comm", ...
	Versions    []string `json:"versions"`    // "v1", "v2"
	Scheme      string   `json:"scheme"`      // "http" (h2c) or "https"
	FQDN        string   `json:"fqdn,omitempty"`
	IPAddr      string   `json:"ipAddr,omitempty"`
	Port        int      `json:"port"`
}

// NFProfile is the bundle an NF sends at registration time.
type NFProfile struct {
	NFInstanceID string      `json:"nfInstanceId"` // UUID, stable across restarts
	NFType       NFType      `json:"nfType"`
	NFStatus     NFStatus    `json:"nfStatus"`
	Services     []NFService `json:"nfServices"`
	PLMN         string      `json:"plmn,omitempty"` // "00101" etc.
	// Heartbeat cadence the NF advertises. NRF expects one heartbeat per
	// interval; misses trigger automatic deregistration.
	HeartbeatTimer time.Duration `json:"-"`

	updatedAt time.Time
}

// DiscoveryQuery is what Client.Discover takes. Mirrors the subset of
// Nnrf_NFDiscovery query params that QCore's v1.0 uses.
type DiscoveryQuery struct {
	TargetNFType  NFType
	RequesterType NFType
	ServiceName   string // optional — narrow to one service
	PLMN          string // optional — required in roaming scenarios (out of v1.0 scope)
}

// Client is the NF-facing interface. Concrete implementations live alongside.
type Client interface {
	Register(ctx context.Context, p *NFProfile) error
	Deregister(ctx context.Context, nfInstanceID string) error
	Heartbeat(ctx context.Context, nfInstanceID string) error
	Discover(ctx context.Context, q DiscoveryQuery) ([]NFProfile, error)
}

// ErrNotFound is returned by Discover when no NF matches the query and by
// Deregister/Heartbeat against unknown instance IDs.
var ErrNotFound = errors.New("nrf: not found")

// --- InMemory implementation ---

// InMemory is a process-local NRF — the registry is a map. Useful for:
//   - Unit tests across NFs
//   - Single-binary dev mode (`qcore dev`) where everything shares a process
type InMemory struct {
	mu       sync.RWMutex
	profiles map[string]NFProfile // keyed by NFInstanceID
}

// NewInMemory returns an in-process NRF client/registry.
func NewInMemory() *InMemory {
	return &InMemory{profiles: make(map[string]NFProfile)}
}

func (m *InMemory) Register(_ context.Context, p *NFProfile) error {
	if p == nil || p.NFInstanceID == "" {
		return fmt.Errorf("nrf: register requires NFInstanceID")
	}
	if p.NFType == "" {
		return fmt.Errorf("nrf: register requires NFType")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if p.NFStatus == "" {
		p.NFStatus = StatusRegistered
	}
	p.updatedAt = time.Now()
	m.profiles[p.NFInstanceID] = *p
	return nil
}

func (m *InMemory) Deregister(_ context.Context, nfInstanceID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.profiles[nfInstanceID]; !ok {
		return ErrNotFound
	}
	delete(m.profiles, nfInstanceID)
	return nil
}

func (m *InMemory) Heartbeat(_ context.Context, nfInstanceID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.profiles[nfInstanceID]
	if !ok {
		return ErrNotFound
	}
	p.updatedAt = time.Now()
	m.profiles[nfInstanceID] = p
	return nil
}

func (m *InMemory) Discover(_ context.Context, q DiscoveryQuery) ([]NFProfile, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []NFProfile
	for _, p := range m.profiles {
		if q.TargetNFType != "" && p.NFType != q.TargetNFType {
			continue
		}
		if q.PLMN != "" && p.PLMN != "" && p.PLMN != q.PLMN {
			continue
		}
		if q.ServiceName != "" && !hasService(p, q.ServiceName) {
			continue
		}
		if p.NFStatus != StatusRegistered {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

func hasService(p NFProfile, name string) bool {
	for _, s := range p.Services {
		if s.ServiceName == name {
			return true
		}
	}
	return false
}
