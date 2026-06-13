package smf

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/qcore-project/qcore/pkg/events"
	"github.com/qcore-project/qcore/pkg/logger"
)

// Config holds all SMF tunables.
type Config struct {
	// IPPoolCIDR is the subnet from which UE IPs are allocated.
	IPPoolCIDR string

	// UPFPfcpAddr is the UPF's N4 endpoint (e.g. "127.0.0.1:8805")
	UPFPfcpAddr string

	// LocalPfcpIP is the SMF's local IP to bind for N4
	LocalPfcpIP string

	// SMFInstanceID is the UUID for NRF registration
	SMFInstanceID string
}

// Service is the SMF Network Function.
type Service struct {
	cfg     Config
	log     logger.Logger
	mux     *http.ServeMux
	emitter events.Emitter

	ipam    *IPAM
	pfcpCli *PFCPClient

	mu       sync.Mutex
	sessions map[string]*SessionContext
}

type SessionContext struct {
	SUPI         string
	PDUSessionID int
	UEIP         string
	UPFSEID      uint64
	UPFTEID      uint32
	UPFN3IP      string
}

// NewService creates an SMF service.
func NewService(cfg Config, log logger.Logger) (*Service, error) {
	// 1. Initialize IP Address Manager
	ipam, err := NewIPAM(cfg.IPPoolCIDR)
	if err != nil {
		return nil, err
	}

	// 2. Initialize PFCP Client
	pfcpCli, err := NewPFCPClient(cfg.LocalPfcpIP, cfg.UPFPfcpAddr, log)
	if err != nil {
		return nil, err
	}

	s := &Service{
		cfg:      cfg,
		log:      log.WithField("nf", "smf"),
		mux:      http.NewServeMux(),
		emitter:  &events.NoopEmitter{},
		ipam:     ipam,
		pfcpCli:  pfcpCli,
		sessions: make(map[string]*SessionContext),
	}

	s.registerRoutes()
	return s, nil
}

// SetEmitter attaches a structured event emitter.
func (s *Service) SetEmitter(e events.Emitter) { s.emitter = e }

// Handler returns the raw mux for pkg/sbi to wrap.
func (s *Service) Handler() http.Handler {
	return s.mux
}

func (s *Service) registerRoutes() {
	s.mux.HandleFunc("GET /health", s.handleHealth)
	s.mux.HandleFunc("POST /nsmf-pdusession/v1/sm-contexts", s.postSMContexts)
	s.mux.HandleFunc("POST /nsmf-pdusession/v1/sm-contexts/modify", s.postSMContextModify)
}

func (s *Service) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// Start runs background tasks like UPF association.
func (s *Service) Start(ctx context.Context) error {
	s.log.Info("smf: starting background tasks")

	// Example: Try to associate with UPF in the background
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
				// T3 Placeholder: Send Association Setup Request
				// Once T4 (UPF) is built, this will successfully bootstrap N4.
				s.log.Debug("smf: sending association setup to UPF...")
			}
		}
	}()

	return nil
}

// Close cleans up resources.
func (s *Service) Close() error {
	if s.pfcpCli != nil {
		return s.pfcpCli.Close()
	}
	return nil
}
