// Package nrf is QCore's Network Repository Function.
//
// It exposes two SBI service interfaces per 3GPP TS 29.510:
//
//	Nnrf_NFManagement  — NF register / update / heartbeat / deregister
//	Nnrf_NFDiscovery   — NF discovery by type, service name, PLMN
//
// The HTTP server is a thin translation layer over a Store (usually the
// InMemory registry from pkg/sbi/nrf in single-binary mode). Swap the
// Store for a distributed backend without changing any handler.
package nrf

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/qcore-project/qcore/pkg/events"
	"github.com/qcore-project/qcore/pkg/logger"
	"github.com/qcore-project/qcore/pkg/sbi"
	nrfclient "github.com/qcore-project/qcore/pkg/sbi/nrf"
)

// wireNFProfile is the JSON-serialisable form of nrfclient.NFProfile.
// HeartbeatTimer maps to/from seconds (JSON integer) rather than
// time.Duration, which has no standard JSON form.
type wireNFProfile struct {
	NFInstanceID   string               `json:"nfInstanceId"`
	NFType         nrfclient.NFType     `json:"nfType"`
	NFStatus       nrfclient.NFStatus   `json:"nfStatus,omitempty"`
	Services       []nrfclient.NFService `json:"nfServices,omitempty"`
	PLMN           string               `json:"plmn,omitempty"`
	HeartbeatTimer int                  `json:"heartBeatTimer,omitempty"` // seconds
}

func toWire(p nrfclient.NFProfile) wireNFProfile {
	return wireNFProfile{
		NFInstanceID:   p.NFInstanceID,
		NFType:         p.NFType,
		NFStatus:       p.NFStatus,
		Services:       p.Services,
		PLMN:           p.PLMN,
		HeartbeatTimer: int(p.HeartbeatTimer / time.Second),
	}
}

func fromWire(w wireNFProfile) nrfclient.NFProfile {
	return nrfclient.NFProfile{
		NFInstanceID:   w.NFInstanceID,
		NFType:         w.NFType,
		NFStatus:       w.NFStatus,
		Services:       w.Services,
		PLMN:           w.PLMN,
		HeartbeatTimer: time.Duration(w.HeartbeatTimer) * time.Second,
	}
}

// discoveryResponse is the Nnrf_NFDiscovery 200 OK body (TS 29.510 §6.2.6.2.4).
type discoveryResponse struct {
	ValidityPeriod int             `json:"validityPeriod"` // seconds
	NFInstances    []wireNFProfile `json:"nfInstances"`
}

// Server exposes Nnrf_NFManagement and Nnrf_NFDiscovery over SBI HTTP/2.
type Server struct {
	store   nrfclient.Client
	log     logger.Logger
	mux     *http.ServeMux
	emitter events.Emitter
}

// NewServer creates an NRF server backed by the provided store.
// Pass nrfclient.NewInMemory() for single-binary dev mode.
func NewServer(store nrfclient.Client, log logger.Logger) *Server {
	s := &Server{
		store:   store,
		log:     log.WithField("nf", "nrf"),
		mux:     http.NewServeMux(),
		emitter: &events.NoopEmitter{},
	}
	s.registerRoutes()
	return s
}

// SetEmitter attaches a structured event emitter.
func (s *Server) SetEmitter(e events.Emitter) { s.emitter = e }

// Handler returns the http.Handler for pkg/sbi.NewServer.
func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) registerRoutes() {
	// Nnrf_NFManagement — TS 29.510 §6.2
	s.mux.HandleFunc("PUT /nnrf-nfm/v1/nf-instances/{nfInstanceId}", s.putNFInstance)
	s.mux.HandleFunc("PATCH /nnrf-nfm/v1/nf-instances/{nfInstanceId}", s.patchNFInstance)
	s.mux.HandleFunc("DELETE /nnrf-nfm/v1/nf-instances/{nfInstanceId}", s.deleteNFInstance)
	s.mux.HandleFunc("GET /nnrf-nfm/v1/nf-instances/{nfInstanceId}", s.getNFInstance)

	// Nnrf_NFDiscovery — TS 29.510 §6.3
	s.mux.HandleFunc("GET /nnrf-disc/v1/nf-instances", s.discoverNFInstances)
}

// putNFInstance handles PUT /nnrf-nfm/v1/nf-instances/{nfInstanceId}.
// Creates or fully replaces an NF profile (register + update, TS 29.510 §6.2.6.2.2).
func (s *Server) putNFInstance(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("nfInstanceId")
	if id == "" {
		sbi.WriteProblem(w, &sbi.ProblemDetails{Status: http.StatusBadRequest, Title: "Bad Request", Detail: "missing nfInstanceId"})
		return
	}
	var wire wireNFProfile
	if err := json.NewDecoder(r.Body).Decode(&wire); err != nil {
		sbi.WriteProblem(w, &sbi.ProblemDetails{Status: http.StatusBadRequest, Title: "Bad Request", Detail: "malformed NFProfile: " + err.Error()})
		return
	}
	wire.NFInstanceID = id // enforce path ID
	p := fromWire(wire)
	if err := s.store.Register(r.Context(), &p); err != nil {
		s.log.WithError(err).Error("nrf: register failed")
		sbi.WriteProblem(w, sbi.InternalError("register failed"))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(toWire(p))
}

// patchNFInstance handles PATCH /nnrf-nfm/v1/nf-instances/{nfInstanceId}.
// Used for heartbeat (TS 29.510 §6.2.6.2.3). We accept any PATCH body as a
// heartbeat — a real NRF validates JSON Patch ops; we just update the timestamp.
func (s *Server) patchNFInstance(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("nfInstanceId")
	if err := s.store.Heartbeat(r.Context(), id); err != nil {
		if errors.Is(err, nrfclient.ErrNotFound) {
			sbi.WriteProblem(w, &sbi.ProblemDetails{Status: http.StatusNotFound, Title: "Not Found", Detail: "NF instance not registered"})
			return
		}
		sbi.WriteProblem(w, sbi.InternalError("heartbeat failed"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// deleteNFInstance handles DELETE /nnrf-nfm/v1/nf-instances/{nfInstanceId}.
func (s *Server) deleteNFInstance(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("nfInstanceId")
	if err := s.store.Deregister(r.Context(), id); err != nil {
		if errors.Is(err, nrfclient.ErrNotFound) {
			sbi.WriteProblem(w, &sbi.ProblemDetails{Status: http.StatusNotFound, Title: "Not Found", Detail: "NF instance not registered"})
			return
		}
		sbi.WriteProblem(w, sbi.InternalError("deregister failed"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// getNFInstance handles GET /nnrf-nfm/v1/nf-instances/{nfInstanceId}.
func (s *Server) getNFInstance(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("nfInstanceId")
	profiles, err := s.store.Discover(r.Context(), nrfclient.DiscoveryQuery{})
	if err != nil {
		sbi.WriteProblem(w, sbi.InternalError("store error"))
		return
	}
	for _, p := range profiles {
		if p.NFInstanceID == id {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(toWire(p))
			return
		}
	}
	sbi.WriteProblem(w, &sbi.ProblemDetails{Status: http.StatusNotFound, Title: "Not Found", Detail: "NF instance not found"})
}

// discoverNFInstances handles GET /nnrf-disc/v1/nf-instances.
// Supported query params: target-nf-type, requester-nf-type, service-names.
func (s *Server) discoverNFInstances(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	query := nrfclient.DiscoveryQuery{
		TargetNFType:  nrfclient.NFType(q.Get("target-nf-type")),
		RequesterType: nrfclient.NFType(q.Get("requester-nf-type")),
		ServiceName:   q.Get("service-names"),
		PLMN:          q.Get("target-plmn-list"),
	}
	profiles, err := s.store.Discover(r.Context(), query)
	if err != nil {
		sbi.WriteProblem(w, sbi.InternalError("discovery failed"))
		return
	}
	wires := make([]wireNFProfile, len(profiles))
	for i, p := range profiles {
		wires[i] = toWire(p)
	}
	resp := discoveryResponse{
		ValidityPeriod: 3600,
		NFInstances:    wires,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
