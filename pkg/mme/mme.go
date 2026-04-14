// Package mme implements the Mobility Management Entity (MME) for QCore's LTE EPC.
// The MME handles S1AP signaling from eNodeBs, NAS messaging with UEs, and
// communicates with the HSS for authentication.
package mme

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/qcore-project/qcore/pkg/config"
	"github.com/qcore-project/qcore/pkg/logger"
	"github.com/qcore-project/qcore/pkg/metrics"
	"github.com/qcore-project/qcore/pkg/sctp"
)

// MME is the core Mobility Management Entity service.
type MME struct {
	cfg     *config.MMEConfig
	log     logger.Logger
	metrics *metrics.MMEMetrics
	s6a     *S6aClient

	// PLMN identity
	plmn [3]byte

	// GUMMEI components
	mmeGroupID  uint16
	mmeCode     uint8
	relCapacity uint8

	// State
	listener sctp.Listener
	enbs     sync.Map // map[string]*EnbContext (key: remote addr)
	ues      sync.Map // map[uint32]*UEContext  (key: MME-UE-S1AP-ID)
	nextUEID uint32   // atomic counter for MME-UE-S1AP-ID allocation
}

// New creates a new MME instance.
func New(cfg *config.MMEConfig, plmn [3]byte, log logger.Logger, m *metrics.MMEMetrics, s6a *S6aClient) *MME {
	return &MME{
		cfg:         cfg,
		log:         log.WithField("component", "mme"),
		metrics:     m,
		s6a:         s6a,
		plmn:        plmn,
		mmeGroupID:  cfg.MMEGroupID,
		mmeCode:     cfg.MMECode,
		relCapacity: cfg.RelCapacity,
	}
}

// Start begins accepting S1AP connections from eNodeBs.
func (m *MME) Start(ctx context.Context) error {
	addr := fmt.Sprintf("%s:%d", m.cfg.BindAddress, m.cfg.S1APPort)
	mode := sctp.Mode(m.cfg.SCTPMode)

	ln, err := sctp.Listen(mode, addr)
	if err != nil {
		return fmt.Errorf("listening on %s (mode=%s): %w", addr, mode, err)
	}
	m.listener = ln

	m.log.Infof("MME S1AP listening on %s (mode=%s)", addr, mode)

	go m.acceptLoop(ctx)
	return nil
}

// Stop shuts down the MME and closes all associations.
func (m *MME) Stop() {
	if m.listener != nil {
		m.listener.Close()
	}

	// Close all eNB associations
	m.enbs.Range(func(key, value any) bool {
		if enb, ok := value.(*EnbContext); ok {
			enb.Assoc.Close()
		}
		m.enbs.Delete(key)
		return true
	})

	m.log.Info("MME stopped")
}

// acceptLoop accepts new eNodeB connections.
func (m *MME) acceptLoop(ctx context.Context) {
	for {
		assoc, err := m.listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				m.log.Errorf("Accept error: %v", err)
				continue
			}
		}

		remote := assoc.RemoteAddr().String()
		m.log.Infof("New eNodeB connection from %s", remote)

		enb := &EnbContext{
			Assoc: assoc,
		}
		m.enbs.Store(remote, enb)

		if m.metrics != nil {
			m.metrics.ConnectedENBs.WithLabelValues().Inc()
		}

		go m.handleAssociation(ctx, enb)
	}
}

// handleAssociation reads messages from a single eNodeB association.
func (m *MME) handleAssociation(ctx context.Context, enb *EnbContext) {
	defer func() {
		remote := enb.Assoc.RemoteAddr().String()
		m.enbs.Delete(remote)
		enb.Assoc.Close()
		if m.metrics != nil {
			m.metrics.ConnectedENBs.WithLabelValues().Dec()
		}
		m.log.Infof("eNodeB disconnected: %s", remote)
	}()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		data, streamID, err := enb.Assoc.Read()
		if err != nil {
			m.log.Debugf("Read error from %s: %v", enb.Assoc.RemoteAddr(), err)
			return
		}

		m.log.Debugf("Received %d bytes on stream %d from %s",
			len(data), streamID, enb.Assoc.RemoteAddr())

		// S1AP message dispatch will be wired in Session 3
		m.handleS1APMessage(ctx, enb, data, streamID)
	}
}

// allocateUEID returns a new unique MME-UE-S1AP-ID.
func (m *MME) allocateUEID() uint32 {
	return atomic.AddUint32(&m.nextUEID, 1)
}

// GetENBCount returns the number of connected eNodeBs.
func (m *MME) GetENBCount() int {
	count := 0
	m.enbs.Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
}

// GetUECount returns the number of tracked UEs.
func (m *MME) GetUECount() int {
	count := 0
	m.ues.Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
}
