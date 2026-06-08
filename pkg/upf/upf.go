package upf

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"

	"github.com/qcore-project/qcore/pkg/events"
	"github.com/qcore-project/qcore/pkg/logger"
)

// Config holds all UPF tunables.
type Config struct {
	// PFCPBindAddr is the local N4 listener address (e.g. "0.0.0.0:8805")
	PFCPBindAddr string

	// GTPUBindAddr is the local N3 listener address (e.g. "0.0.0.0:2152")
	GTPUBindAddr string

	// TunDeviceName is the name of the N6 TUN interface (e.g. "qcore-upf")
	TunDeviceName string

	// TunIPv4CIDR is assigned to the TUN interface for UE subnet routing.
	TunIPv4CIDR string

	// EnableNAT installs an iptables MASQUERADE rule for UE egress traffic.
	EnableNAT bool
}

// Service is the User Plane Function.
type Service struct {
	cfg     Config
	log     logger.Logger
	emitter events.Emitter

	store   *SessionStore
	egress  Egress
	pfcpSrv *PFCPServer
	gtpuSrv *GTPUServer
}

// NewService creates a new UPF service.
func NewService(cfg Config, log logger.Logger) (*Service, error) {
	s := &Service{
		cfg:     cfg,
		log:     log.WithField("nf", "upf"),
		emitter: &events.NoopEmitter{},
		store:   NewSessionStore(),
	}

	// Initialize Egress (N6 TUN interface)
	var err error
	if runtime.GOOS == "linux" {
		s.egress, err = NewTunEgress(cfg.TunDeviceName)
		if err != nil {
			s.log.WithError(err).Warn("Failed to create Linux TUN egress. Falling back to DummyEgress.")
			s.egress = NewDummyEgress()
		} else {
			s.log.WithField("dev", cfg.TunDeviceName).Info("Created TUN egress")
			s.configureLinuxEgress()
		}
	} else {
		s.log.Warn("Non-Linux OS detected. Using DummyEgress (no external routing).")
		s.egress = NewDummyEgress()
	}

	// Initialize PFCP (N4) and GTP-U (N3)
	s.pfcpSrv = NewPFCPServer(cfg.PFCPBindAddr, s.store, s.emitter, log)
	s.gtpuSrv = NewGTPUServer(cfg.GTPUBindAddr, s.store, s.egress, log)

	return s, nil
}

func (s *Service) configureLinuxEgress() {
	cidr := s.cfg.TunIPv4CIDR
	if cidr == "" {
		cidr = "10.45.0.1/16"
	}
	run := func(args ...string) {
		if err := exec.Command(args[0], args[1:]...).Run(); err != nil {
			s.log.WithError(err).WithField("cmd", args).Warn("UPF egress setup command failed")
		}
	}
	run("ip", "addr", "add", cidr, "dev", s.cfg.TunDeviceName)
	run("ip", "link", "set", "dev", s.cfg.TunDeviceName, "up")
	run("sysctl", "-w", "net.ipv4.ip_forward=1")
	if s.cfg.EnableNAT {
		ueCIDR := "10.45.0.0/16"
		if cidr == "10.45.0.1/16" {
			ueCIDR = "10.45.0.0/16"
		}
		if err := exec.Command("iptables", "-t", "nat", "-C", "POSTROUTING", "-s", ueCIDR, "!", "-d", ueCIDR, "-j", "MASQUERADE").Run(); err != nil {
			run("iptables", "-t", "nat", "-A", "POSTROUTING", "-s", ueCIDR, "!", "-d", ueCIDR, "-j", "MASQUERADE")
		}
	}
}

// SetEmitter attaches a structured event emitter.
func (s *Service) SetEmitter(e events.Emitter) { s.emitter = e }

// Start boots up the UPF listeners. Blocks until ctx is cancelled.
func (s *Service) Start(ctx context.Context) error {
	s.log.Info("Starting UPF service")

	if err := s.gtpuSrv.Start(ctx); err != nil {
		return fmt.Errorf("start GTP-U server: %w", err)
	}

	if err := s.pfcpSrv.Start(ctx); err != nil {
		return fmt.Errorf("start PFCP server: %w", err)
	}

	<-ctx.Done()
	return ctx.Err()
}

// Close shuts down the UPF and releases resources.
func (s *Service) Close() error {
	s.log.Info("Shutting down UPF service")
	if s.egress != nil {
		return s.egress.Close()
	}
	return nil
}
