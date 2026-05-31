package nrf

import (
	"context"
	"fmt"
	"time"

	"github.com/qcore-project/qcore/pkg/events"
	"github.com/qcore-project/qcore/pkg/logger"
)

const defaultHeartbeatInterval = 30 * time.Second

// LifecycleManager handles an NF's NRF registration lifecycle:
//   - Register on Start
//   - Periodic heartbeat until ctx is cancelled
//   - Deregister on ctx cancel
//
// Emits Phase-A events for observability (registration success/failure,
// each heartbeat, deregistration). All errors are logged; registration failure
// is non-fatal (the NF still starts — static config keeps it routable).
type LifecycleManager struct {
	client  Client
	profile *NFProfile
	emitter events.Emitter
	log     logger.Logger
	nrfURL  string // for event payloads only
}

// NewLifecycleManager returns a manager. If emitter is nil a no-op emitter is used.
func NewLifecycleManager(client Client, profile *NFProfile, nrfURL string, emitter events.Emitter, log logger.Logger) *LifecycleManager {
	if emitter == nil {
		emitter = &events.NoopEmitter{}
	}
	return &LifecycleManager{
		client:  client,
		profile: profile,
		emitter: emitter,
		log:     log.WithField("component", "nrf-lifecycle"),
		nrfURL:  nrfURL,
	}
}

// Start registers with the NRF and maintains the heartbeat until ctx is
// cancelled, then deregisters. Runs synchronously; call in a goroutine.
func (m *LifecycleManager) Start(ctx context.Context) {
	if err := m.client.Register(ctx, m.profile); err != nil {
		m.log.WithError(err).Warnf("NRF registration failed for %s — continuing with static config", m.profile.NFType)
		m.emitter.Emit(events.Event{
			NF:       string(m.profile.NFType),
			Category: events.ErrorEvent,
			Severity: events.SeverityWarn,
			Protocol: "sbi",
			Message:  fmt.Sprintf("NRF registration failed: %v", err),
		})
	} else {
		m.log.Infof("Registered %s with NRF at %s", m.profile.NFType, m.nrfURL)
		m.emitter.Emit(events.Event{
			NF:       string(m.profile.NFType),
			Category: events.StateTransition,
			Severity: events.SeverityInfo,
			Protocol: "sbi",
			Message:  "Registered with NRF",
			Payload: events.NFRegisteredPayload{
				NFType:       string(m.profile.NFType),
				NFInstanceID: m.profile.NFInstanceID,
				NRFURL:       m.nrfURL,
			},
		})
	}

	interval := m.profile.HeartbeatTimer
	if interval <= 0 {
		interval = defaultHeartbeatInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			m.deregister()
			return
		case <-ticker.C:
			if err := m.client.Heartbeat(ctx, m.profile.NFInstanceID); err != nil {
				m.log.WithError(err).Warn("NRF heartbeat failed")
			}
		}
	}
}

func (m *LifecycleManager) deregister() {
	// Use a fresh background context — the parent is already cancelled.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := m.client.Deregister(ctx, m.profile.NFInstanceID); err != nil {
		m.log.WithError(err).Warn("NRF deregistration failed")
	} else {
		m.log.Infof("Deregistered %s from NRF", m.profile.NFType)
	}
}

// DiscoverFirst queries the NRF for the first matching NF and returns its
// base URL. If the NRF is unreachable or returns no results, it falls back to
// staticFallback and emits a warning event. Returns an error only if both the
// NRF and the static fallback are empty.
func DiscoverFirst(
	ctx context.Context,
	nrfCli Client,
	q DiscoveryQuery,
	staticFallback string,
	emitter events.Emitter,
	log logger.Logger,
) (string, error) {
	if emitter == nil {
		emitter = &events.NoopEmitter{}
	}

	profiles, err := nrfCli.Discover(ctx, q)
	if err == nil && len(profiles) > 0 {
		// Pick the first result (NRF already filters by status=REGISTERED).
		p := profiles[0]
		resolvedURL := staticFallback // default if no service URL
		if len(p.Services) > 0 {
			svc := p.Services[0]
			scheme := svc.Scheme
			if scheme == "" {
				scheme = "http"
			}
			host := svc.FQDN
			if host == "" {
				host = svc.IPAddr
			}
			if host != "" && svc.Port > 0 {
				resolvedURL = fmt.Sprintf("%s://%s:%d", scheme, host, svc.Port)
			}
		}
		emitter.Emit(events.Event{
			NF:       string(q.RequesterType),
			Category: events.StateTransition,
			Severity: events.SeverityInfo,
			Protocol: "sbi",
			Message:  fmt.Sprintf("Discovered %s via NRF", q.TargetNFType),
			Payload: events.NFDiscoveredPayload{
				RequesterType: string(q.RequesterType),
				TargetType:    string(q.TargetNFType),
				ServiceName:   q.ServiceName,
				ResolvedURL:   resolvedURL,
			},
		})
		return resolvedURL, nil
	}

	// NRF miss — fall back to static config.
	reason := "no results"
	if err != nil {
		reason = err.Error()
	}
	log.WithField("target", q.TargetNFType).Warnf("NRF discovery failed (%s); using static fallback %s", reason, staticFallback)
	emitter.Emit(events.Event{
		NF:       string(q.RequesterType),
		Category: events.ErrorEvent,
		Severity: events.SeverityWarn,
		Protocol: "sbi",
		Message:  fmt.Sprintf("NRF discovery failed for %s: %s — using static fallback", q.TargetNFType, reason),
		Payload: events.NFDiscoveredPayload{
			RequesterType: string(q.RequesterType),
			TargetType:    string(q.TargetNFType),
			ServiceName:   q.ServiceName,
			ResolvedURL:   staticFallback,
			Fallback:      true,
		},
	})
	if staticFallback == "" {
		return "", fmt.Errorf("nrf: no %s found and no static fallback configured", q.TargetNFType)
	}
	return staticFallback, nil
}
