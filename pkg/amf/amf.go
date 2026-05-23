// Package amf is QCore's 5G Access and Mobility Management Function.
//
// Per 3GPP TS 23.501 §6.2.1, the AMF is the single point of contact for UEs
// over the N2 (NGAP) interface and is responsible for:
//
//   - N2 / NGAP signaling with gNBs (TS 38.413)
//   - NAS 5GMM message handling (TS 24.501)
//   - UE authentication via AUSF (Nausf_UEAuthentication, TS 29.509)
//   - Subscription data retrieval via UDM (Nudm_SDM, TS 29.503)
//   - Serving-AMF registration via UDM (Nudm_UECM, TS 29.503)
//   - NF registration / heartbeat with NRF (Nnrf_NFManagement, TS 29.510)
//
// Milestone for v0.6: a UERANSIM 5G UE completes REGISTRATION.
// Flow: NGSetup → InitialUEMessage(RegistrationReq) → Auth → SMC → RegistrationAccept.
package amf

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/qcore-project/qcore/pkg/ausf"
	"github.com/qcore-project/qcore/pkg/logger"
	"github.com/qcore-project/qcore/pkg/ngap"
	"github.com/qcore-project/qcore/pkg/sctp"
)

// Config holds all AMF tunables. Zero values are dev-friendly defaults.
type Config struct {
	// NGAP listener address ("0.0.0.0:38412" for production, "127.0.0.1:38412" for dev)
	NGAPAddr string

	// NGAP transport mode — "tcp" for dev/test, "sctp" for UERANSIM
	NGAPMode sctp.Mode

	// GUAMI identifies this AMF instance
	GUAMI ngap.GUAMI

	// PLMNs this AMF serves
	PLMNSupportList []ngap.PLMNSupportItem

	// AMFName is included in NGSetupResponse
	AMFName string

	// ServingNetworkName is the SN-Name sent to AUSF for 5G-AKA binding.
	// Format: "5G:mnc<MNC>.mcc<MCC>.3gppnetwork.org"
	ServingNetworkName string

	// AMFInstanceID is the UUID used for NRF registration
	AMFInstanceID string
}

// Service is the AMF. Call Serve() to block and accept gNB connections.
type Service struct {
	cfg     Config
	log     logger.Logger
	ausfCli *ausf.Client

	// nextAMFUENGAPID atomically assigns IDs to UE contexts.
	nextID atomic.Uint64

	mu  sync.RWMutex
	ues map[uint64]*UEContext // keyed by AMF-UE-NGAP-ID
}

// NewService creates an AMF service.
// ausfCli is required; udmCli is optional (UECM anchor after registration).
func NewService(cfg Config, ausfCli *ausf.Client, log logger.Logger) *Service {
	return &Service{
		cfg:     cfg,
		log:     log.WithField("nf", "amf"),
		ausfCli: ausfCli,
		ues:     make(map[uint64]*UEContext),
	}
}

// Serve listens for gNB NGAP connections and handles them in goroutines.
// Blocks until ctx is cancelled.
func (s *Service) Serve(ctx context.Context) error {
	addr := s.cfg.NGAPAddr
	if addr == "" {
		addr = "0.0.0.0:38412"
	}
	mode := s.cfg.NGAPMode
	if mode == "" {
		mode = sctp.ModeTCP
	}

	ln, err := sctp.Listen(mode, addr)
	if err != nil {
		return fmt.Errorf("amf: listen %s: %w", addr, err)
	}
	s.log.WithField("addr", ln.Addr()).Info("AMF NGAP listener ready")
	defer ln.Close()

	go func() {
		<-ctx.Done()
		ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				s.log.WithError(err).Warn("amf: accept failed")
				continue
			}
		}
		sess := &gNBSession{
			amf:  s,
			conn: conn,
			log:  s.log.WithField("gnb", conn.RemoteAddr()),
		}
		go sess.run(ctx)
	}
}

// allocUEID returns a fresh AMF-UE-NGAP-ID.
func (s *Service) allocUEID() uint64 {
	return s.nextID.Add(1)
}

func (s *Service) addUE(ue *UEContext) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ues[ue.AMFUENGAPID] = ue
}

func (s *Service) getUE(amfID uint64) (*UEContext, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ue, ok := s.ues[amfID]
	return ue, ok
}

func (s *Service) removeUE(amfID uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.ues, amfID)
}

