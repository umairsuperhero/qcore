// Package dashboard is QCore's backend-for-frontend (BFF). The browser
// only ever talks to this process. It aggregates state from every NF, proxies
// subscriber CRUD to the HSS admin API, proxies the collector's SSE event
// stream untransformed, controls the built-in simulator, and serves the
// embedded React frontend.
//
// See docs/phase-b-golden-path.md for scope and exit criterion.
package dashboard

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/gorilla/mux"
	"github.com/qcore-project/qcore/pkg/ai"
	"github.com/qcore-project/qcore/pkg/config"
	"github.com/qcore-project/qcore/pkg/events"
	"github.com/qcore-project/qcore/pkg/logger"
	"github.com/qcore-project/qcore/pkg/simulator"
)

// Server wires the dashboard HTTP routes against the upstream NFs.
// It takes the full *config.Config because the RAN-config display (B5)
// needs MME/SPGW values, not just dashboard-local fields.
type Server struct {
	cfg    *config.Config
	log    logger.Logger
	router *mux.Router

	hssURL       *url.URL
	mmeURL       *url.URL
	spgwURL      *url.URL
	collectorURL *url.URL

	sim      *SimulatorController
	aiEngine *ai.Engine
}

// New constructs a Server. URL fields are parsed once; bad URLs return an
// error so a misconfiguration is caught at startup, not at first request.
func New(cfg *config.Config, log logger.Logger) (*Server, error) {
	hssURL, err := url.Parse(cfg.Dashboard.HSSURL)
	if err != nil {
		return nil, err
	}
	mmeURL, err := url.Parse(cfg.Dashboard.MMEURL)
	if err != nil {
		return nil, err
	}
	spgwURL, err := url.Parse(cfg.Dashboard.SPGWURL)
	if err != nil {
		return nil, err
	}
	collectorURL, err := url.Parse(cfg.Dashboard.CollectorURL)
	if err != nil {
		return nil, err
	}

	s := &Server{
		cfg:          cfg,
		log:          log.WithField("component", "dashboard"),
		hssURL:       hssURL,
		mmeURL:       mmeURL,
		spgwURL:      spgwURL,
		collectorURL: collectorURL,
		aiEngine:     ai.NewEngine(cfg.AI, log),
	}

	// Build simulator templates from config + the demo subscriber.
	// The demo subscriber's credentials are the 3GPP TS 35.208 Test Set 1
	// vectors that cmd/hss/main.go seeds on first run.
	mmePLMN, err := simulator.PackPLMN(cfg.MME.PLMN)
	if err != nil {
		return nil, err
	}
	amfPLMN, err := simulator.PackPLMN(cfg.AMF.PLMN)
	if err != nil {
		return nil, err
	}
	sim4G := simulator.Options{
		Mode:          "4g",
		MMEAddr:       cfg.Dashboard.MMES1APAddr,
		TransportMode: cfg.MME.SCTPMode,
		PLMN:          mmePLMN,
		TAC:           cfg.MME.TAC,
		IMSI:          "001010000000001",
		Ki:            "465b5ce8b199b49faa5f0a2ee238a6bc",
		OPc:           "cd63cb71954a9f4e48a5994e37a02baf",
	}
	sim5G := simulator.Options{
		Mode:          "5g",
		MMEAddr:       cfg.Dashboard.AMFNGAPAddr,
		TransportMode: cfg.AMF.SCTPMode,
		PLMN:          amfPLMN,
		TAC:           cfg.AMF.TAC,
		IMSI:          "001010000000001",
		Ki:            "465b5ce8b199b49faa5f0a2ee238a6bc",
		OPc:           "cd63cb71954a9f4e48a5994e37a02baf",
	}
	simEmitter := events.New(cfg.Dashboard.CollectorURL, "simulator", log)
	s.sim = NewSimulatorController(sim4G, sim5G, simEmitter, log)

	s.routes()
	return s, nil
}

// Handler returns the HTTP handler for use with http.Server.
func (s *Server) Handler() http.Handler { return s.router }

func (s *Server) routes() {
	r := mux.NewRouter()

	api := r.PathPrefix("/api").Subrouter()

	api.HandleFunc("/health", s.handleHealth).Methods(http.MethodGet)
	api.HandleFunc("/ran-config", s.handleRANConfig).Methods(http.MethodGet)

	// Subscriber CRUD — proxied to HSS admin API. Strip /api so /api/subscribers
	// hits /api/v1/subscribers on the HSS.
	subsProxy := s.reverseProxy(s.hssURL, func(p string) string {
		return strings.Replace(p, "/api/subscribers", "/api/v1/subscribers", 1)
	})
	api.PathPrefix("/subscribers").Handler(subsProxy)

	// Live event stream — proxied untransformed to preserve AI-consumable shape.
	api.HandleFunc("/events/stream", s.handleEventStream).Methods(http.MethodGet)
	api.HandleFunc("/journeys", s.handleJourneyList).Methods(http.MethodGet)
	api.HandleFunc("/journeys/{id}/events", s.handleJourneyEvents).Methods(http.MethodGet)

	// Simulator control.
	api.HandleFunc("/simulator/status", s.handleSimulatorStatus).Methods(http.MethodGet)
	api.HandleFunc("/simulator/start", s.handleSimulatorStart).Methods(http.MethodPost)
	api.HandleFunc("/simulator/stop", s.handleSimulatorStop).Methods(http.MethodPost)
	api.HandleFunc("/simulator/inject/{scenario}", s.handleSimulatorInject).Methods(http.MethodPost)
	api.HandleFunc("/simulator/custom", s.handleSimulatorCustom).Methods(http.MethodPost)

	// Diagnostics
	api.HandleFunc("/diagnostics/journey/{id}", s.handleDiagnoseJourney).Methods(http.MethodGet)

	// Static frontend at /. Catch-all is last so /api/* wins.
	r.PathPrefix("/").Handler(staticHandler())

	s.router = r
}

// reverseProxy builds a single-host reverse proxy that rewrites the request
// path through rewrite() before forwarding. SSE responses pass through
// naturally because httputil.ReverseProxy flushes on each write.
func (s *Server) reverseProxy(upstream *url.URL, rewrite func(string) string) http.Handler {
	rp := httputil.NewSingleHostReverseProxy(upstream)
	originalDirector := rp.Director
	rp.Director = func(r *http.Request) {
		originalDirector(r)
		if rewrite != nil {
			r.URL.Path = rewrite(r.URL.Path)
		}
		r.Host = upstream.Host
	}
	rp.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		s.log.Warnf("proxy to %s failed: %v", upstream, err)
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
	}
	return rp
}
