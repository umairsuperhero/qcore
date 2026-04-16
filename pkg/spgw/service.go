package spgw

import (
	"fmt"
	"net"
	"time"

	"github.com/qcore-project/qcore/pkg/config"
	"github.com/qcore-project/qcore/pkg/logger"
	"github.com/qcore-project/qcore/pkg/metrics"
)

// Service is the top-level SPGW: IP pool, TEID pool, session store,
// user-plane dataplane, HTTP API. It owns the goroutines for both planes.
type Service struct {
	cfg      *config.SPGWConfig
	log      logger.Logger
	sessions *SessionStore
	ipPool   *IPPool
	teidPool *TEIDPool
	dp       *Dataplane
	egress   Egress
	metrics  *metrics.SPGWMetrics // optional; nil-safe everywhere

	sgwAddr net.IP // address we advertise to the MME as the SGW S1-U endpoint
}

// SetMetrics attaches the Prometheus metrics struct. Call before Start so the
// dataplane picks them up too. Safe to never call (metrics paths are nil-safe).
func (s *Service) SetMetrics(m *metrics.SPGWMetrics) {
	s.metrics = m
	if s.dp != nil {
		s.dp.metrics = m
	}
}

// New constructs the SPGW service but does not start any sockets.
func New(cfg *config.SPGWConfig, log logger.Logger) (*Service, error) {
	log = log.WithField("component", "spgw")

	pool, err := NewIPPool(cfg.UEPool, cfg.Gateway)
	if err != nil {
		return nil, fmt.Errorf("ip pool: %w", err)
	}

	egress, err := buildEgress(cfg, log)
	if err != nil {
		return nil, fmt.Errorf("egress: %w", err)
	}

	sgwIP := net.ParseIP(cfg.SGWU1Addr)
	if sgwIP == nil {
		// Fall back to loopback so single-host dev still works.
		sgwIP = net.ParseIP("127.0.0.1")
	}

	return &Service{
		cfg:      cfg,
		log:      log,
		sessions: NewSessionStore(),
		ipPool:   pool,
		teidPool: NewTEIDPool(100), // start at 101; keep 0..100 reserved
		egress:   egress,
		sgwAddr:  sgwIP,
	}, nil
}

// buildEgress selects the egress adapter based on config. Falls back to
// LogEgress on unknown values rather than refusing to start, so a typo in
// config can't take a node down — operators see a warning instead.
func buildEgress(cfg *config.SPGWConfig, log logger.Logger) (Egress, error) {
	switch cfg.Egress {
	case "", "log":
		return NewLogEgress(log), nil
	case "tun":
		eg, err := NewTUNEgress(log, cfg.TUNName, cfg.TUNMTU)
		if err != nil {
			return nil, err
		}
		return eg, nil
	default:
		log.Warnf("unknown egress %q, falling back to log", cfg.Egress)
		return NewLogEgress(log), nil
	}
}

// Start binds the user-plane socket. The HTTP API is started separately by the
// main binary so it can be composed with the /metrics endpoint.
func (s *Service) Start() error {
	addr := fmt.Sprintf("%s:%d", s.cfg.BindAddress, s.cfg.S1UPort)
	dp, err := NewDataplane(addr, s.sessions, s.egress, s.log)
	if err != nil {
		return fmt.Errorf("dataplane: %w", err)
	}
	dp.metrics = s.metrics
	s.dp = dp
	s.log.Infof("SPGW user-plane listening on %s (egress=%s)", dp.LocalAddr(), s.egress.Name())
	go dp.Run()
	return nil
}

// Stop tears down the dataplane and releases sessions.
func (s *Service) Stop() {
	if s.dp != nil {
		s.dp.Stop()
	}
	if s.egress != nil {
		_ = s.egress.Close()
	}
}

// SGWAddr returns the address advertised to the MME for S1-U.
func (s *Service) SGWAddr() net.IP { return s.sgwAddr }

// Dataplane exposes the dataplane for tests.
func (s *Service) Dataplane() *Dataplane { return s.dp }

// Egress exposes the egress adapter for tests.
func (s *Service) Egress() Egress { return s.egress }

// Sessions exposes the session store (read-only from callers' perspective).
func (s *Service) Sessions() *SessionStore { return s.sessions }

// CreateSession allocates a UE IP + SGW TEID and stores a new bearer. Used by
// the HTTP S11 API when the MME fires Create Session Request.
func (s *Service) CreateSession(req *CreateSessionRequest) (*CreateSessionResponse, error) {
	if req == nil || req.IMSI == "" {
		return nil, fmt.Errorf("missing IMSI")
	}
	// If a session already exists for this IMSI (e.g. re-attach), delete it first.
	if old, ok := s.sessions.GetByIMSI(req.IMSI); ok {
		s.log.Infof("replacing existing session for IMSI=%s (UE-IP=%s, TEID=0x%x)",
			old.IMSI, old.UEIP, old.SGWTEID)
		s.ipPool.Release(old.UEIP)
		_, _, _ = s.sessions.Delete(req.IMSI)
	}

	ip, err := s.ipPool.Allocate()
	if err != nil {
		return nil, fmt.Errorf("allocating UE IP: %w", err)
	}
	teid := s.teidPool.Next()

	apn := req.APN
	if apn == "" {
		apn = "internet"
	}
	ebi := req.EBI
	if ebi == 0 {
		ebi = 5
	}

	b := &Bearer{
		IMSI:      req.IMSI,
		SGWTEID:   teid,
		SGWAddr:   s.sgwAddr,
		UEIP:      ip,
		EBI:       ebi,
		APN:       apn,
		CreatedAt: time.Now(),
	}
	if err := s.sessions.Add(b); err != nil {
		s.ipPool.Release(ip)
		return nil, fmt.Errorf("add session: %w", err)
	}

	s.log.Infof("created session IMSI=%s UE-IP=%s SGW-TEID=0x%x APN=%s EBI=%d",
		req.IMSI, ip, teid, apn, ebi)

	if s.metrics != nil {
		s.metrics.SessionsCreated.WithLabelValues().Inc()
		s.metrics.ActiveSessions.WithLabelValues().Set(float64(s.sessions.Count()))
	}

	return &CreateSessionResponse{
		UEIP:    ip.String(),
		SGWTEID: teid,
		SGWAddr: s.sgwAddr.String(),
		EBI:     ebi,
		APN:     apn,
	}, nil
}

// ModifyBearer records the eNB's S1-U endpoint so downlink packets can flow.
func (s *Service) ModifyBearer(req *ModifyBearerRequest) (*ModifyBearerResponse, error) {
	if req == nil || req.IMSI == "" {
		return nil, fmt.Errorf("missing IMSI")
	}
	enbIP := net.ParseIP(req.ENBAddr)
	if enbIP == nil {
		return nil, fmt.Errorf("invalid eNB address %q", req.ENBAddr)
	}
	if err := s.sessions.UpdateENB(req.IMSI, req.ENBTEID, enbIP); err != nil {
		return nil, err
	}
	s.log.Infof("modified bearer IMSI=%s eNB=%s TEID=0x%x", req.IMSI, enbIP, req.ENBTEID)
	return &ModifyBearerResponse{OK: true}, nil
}

// DeleteSession tears down a session and recycles its IP.
func (s *Service) DeleteSession(imsi string) error {
	ip, _, err := s.sessions.Delete(imsi)
	if err != nil {
		return err
	}
	if ip != nil {
		s.ipPool.Release(ip)
		s.log.Infof("deleted session IMSI=%s (freed UE-IP=%s)", imsi, ip)
	}
	if s.metrics != nil {
		s.metrics.SessionsDeleted.WithLabelValues().Inc()
		s.metrics.ActiveSessions.WithLabelValues().Set(float64(s.sessions.Count()))
	}
	return nil
}
