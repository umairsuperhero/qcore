package sbi

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/qcore-project/qcore/pkg/logger"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

// ServerConfig is the minimal surface needed to spin up an SBI server.
// Keep this flat — NFs copy the fields from their own config structs.
type ServerConfig struct {
	// BindAddress + Port are where the server listens. Use "0.0.0.0" to
	// accept public traffic, "127.0.0.1" for loopback-only.
	BindAddress string
	Port        int

	// TLSCert/TLSKey are PEM-encoded file paths. Leave empty for h2c
	// (plaintext HTTP/2) — dev only, never production.
	TLSCert string
	TLSKey  string

	// ReadTimeout/WriteTimeout are per-request. 3GPP doesn't mandate a
	// specific value; we default to 30s which covers every SBI call in
	// the 5G control plane — nothing should block longer.
	ReadTimeout  time.Duration
	WriteTimeout time.Duration

	// NFType is the registered type with NRF ("AMF", "SMF", "UDM", ...).
	// Informational on the Server itself; used by the NRF client.
	NFType string
}

// Server wraps an http.Server with h2/h2c and structured logging.
type Server struct {
	cfg    ServerConfig
	log    logger.Logger
	mux    http.Handler
	server *http.Server

	// Middlewares installed in front of mux. Defaults include Recover,
	// RequestID, AccessLog. Append to extend.
	Middlewares []Middleware
}

// NewServer creates an SBI server. Call server.Serve() to block and serve,
// server.Shutdown(ctx) to stop gracefully.
//
// The caller is expected to register handlers on mux *before* Serve — once
// serving starts, routing is effectively frozen.
func NewServer(cfg ServerConfig, log logger.Logger, mux http.Handler) *Server {
	if cfg.ReadTimeout == 0 {
		cfg.ReadTimeout = 30 * time.Second
	}
	if cfg.WriteTimeout == 0 {
		cfg.WriteTimeout = 30 * time.Second
	}
	s := &Server{
		cfg: cfg,
		log: log.WithField("sbi", cfg.NFType),
		mux: mux,
		Middlewares: []Middleware{
			Recover(log),
			RequestID(),
			AccessLog(log),
		},
	}
	return s
}

// Serve blocks until the listener errors. In h2c mode it listens plaintext
// and lets the h2c handler negotiate HTTP/2 cleartext with PRI * HTTP/2.0
// upgrades. In TLS mode it listens h2 over TLS only (we intentionally do not
// fall back to HTTP/1.1 to keep the wire behaviour 5G-spec-pure).
func (s *Server) Serve() error {
	handler := ChainMiddleware(s.mux, s.Middlewares...)
	addr := net.JoinHostPort(s.cfg.BindAddress, fmt.Sprintf("%d", s.cfg.Port))

	if s.cfg.TLSCert == "" || s.cfg.TLSKey == "" {
		s.log.Warnf("SBI starting in H2C (plaintext HTTP/2) mode on %s — dev only, do not expose to untrusted networks", addr)
		h2s := &http2.Server{}
		s.server = &http.Server{
			Addr:         addr,
			Handler:      h2c.NewHandler(handler, h2s),
			ReadTimeout:  s.cfg.ReadTimeout,
			WriteTimeout: s.cfg.WriteTimeout,
		}
		return s.server.ListenAndServe()
	}

	s.log.Infof("SBI starting with TLS on %s (H2)", addr)
	s.server = &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  s.cfg.ReadTimeout,
		WriteTimeout: s.cfg.WriteTimeout,
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			NextProtos: []string{"h2"}, // no h1 fallback
		},
	}
	return s.server.ListenAndServeTLS(s.cfg.TLSCert, s.cfg.TLSKey)
}

// Shutdown performs a graceful shutdown on the underlying http.Server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.server == nil {
		return nil
	}
	return s.server.Shutdown(ctx)
}
