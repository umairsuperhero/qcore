package spgw

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/qcore-project/qcore/pkg/metrics"
)

// CreateSessionRequest is QCore's HTTP-over-JSON analogue of the S11 GTPv2-C
// Create Session Request. Keeping it REST lets operators poke it with curl
// and keeps us free of a heavy GTPv2-C codec for now — trading interop with
// commercial MMEs for developer ergonomics (our MME is the only client in
// Phase 3, and it's also ours).
type CreateSessionRequest struct {
	IMSI string `json:"imsi"`
	APN  string `json:"apn,omitempty"`
	EBI  uint8  `json:"ebi,omitempty"`
	PLMN string `json:"plmn,omitempty"`
}

// CreateSessionResponse carries everything the MME needs to pass to the eNB
// via the S1AP Initial Context Setup Request.
type CreateSessionResponse struct {
	UEIP    string `json:"ue_ip"`
	SGWTEID uint32 `json:"sgw_teid"`
	SGWAddr string `json:"sgw_addr"`
	EBI     uint8  `json:"ebi"`
	APN     string `json:"apn"`
}

// ModifyBearerRequest is sent once the eNB has allocated its own uplink TEID
// (reported in the S1AP Initial Context Setup Response) so the SGW can fill
// in the downlink side of the tunnel.
type ModifyBearerRequest struct {
	IMSI    string `json:"imsi"`
	ENBTEID uint32 `json:"enb_teid"`
	ENBAddr string `json:"enb_addr"`
}

// ModifyBearerResponse acknowledges a Modify Bearer.
type ModifyBearerResponse struct {
	OK bool `json:"ok"`
}

// API exposes the SPGW's control interface over HTTP.
type API struct {
	svc     *Service
	router  *mux.Router
	metrics *metrics.SPGWMetrics
}

// NewAPI wires up HTTP routes against the service.
func NewAPI(svc *Service) *API {
	a := &API{svc: svc, router: mux.NewRouter()}
	a.routes()
	return a
}

// SetMetrics attaches Prometheus instrumentation to the API. Idempotent.
func (a *API) SetMetrics(m *metrics.SPGWMetrics) {
	a.metrics = m
}

// Handler returns the mux.Router (with metrics middleware if configured).
func (a *API) Handler() http.Handler {
	if a.metrics == nil {
		return a.router
	}
	return a.instrument(a.router)
}

// instrument wraps a handler with a counter for spgw_api_requests_total.
func (a *API) instrument(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		path := r.URL.Path
		if route := mux.CurrentRoute(r); route != nil {
			if tmpl, err := route.GetPathTemplate(); err == nil {
				path = tmpl
			}
		}
		a.metrics.APIRequests.WithLabelValues(r.Method, path, strconv.Itoa(rec.status)).Inc()
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (a *API) routes() {
	a.router.HandleFunc("/api/v1/health", a.handleHealth).Methods(http.MethodGet)
	a.router.HandleFunc("/api/v1/sessions", a.handleCreateSession).Methods(http.MethodPost)
	a.router.HandleFunc("/api/v1/sessions", a.handleListSessions).Methods(http.MethodGet)
	a.router.HandleFunc("/api/v1/sessions/{imsi}", a.handleDeleteSession).Methods(http.MethodDelete)
	a.router.HandleFunc("/api/v1/sessions/{imsi}/modify", a.handleModifyBearer).Methods(http.MethodPost)
	a.router.HandleFunc("/api/v1/stats", a.handleStats).Methods(http.MethodGet)
}

func (a *API) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":   "ok",
		"sessions": a.svc.sessions.Count(),
	})
}

func (a *API) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	var req CreateSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("decode: %v", err))
		return
	}
	resp, err := a.svc.CreateSession(&req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (a *API) handleModifyBearer(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	imsi := vars["imsi"]
	var req ModifyBearerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("decode: %v", err))
		return
	}
	req.IMSI = imsi
	resp, err := a.svc.ModifyBearer(&req)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *API) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	imsi := mux.Vars(r)["imsi"]
	if err := a.svc.DeleteSession(imsi); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleListSessions(w http.ResponseWriter, _ *http.Request) {
	snap := a.svc.sessions.Snapshot()
	out := make([]map[string]any, 0, len(snap))
	for _, b := range snap {
		enbAddr := ""
		if b.ENBAddr != nil {
			enbAddr = b.ENBAddr.String()
		}
		out = append(out, map[string]any{
			"imsi":     b.IMSI,
			"ue_ip":    b.UEIP.String(),
			"sgw_teid": b.SGWTEID,
			"enb_teid": b.ENBTEID,
			"enb_addr": enbAddr,
			"ebi":      b.EBI,
			"apn":      b.APN,
			"age_s":    nowSub(b.CreatedAt),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": out})
}

func (a *API) handleStats(w http.ResponseWriter, _ *http.Request) {
	var uplink, downlink, drops uint64
	if a.svc.dp != nil {
		uplink, downlink, drops = a.svc.dp.Stats()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"sessions": a.svc.sessions.Count(),
		"uplink_packets":   uplink,
		"downlink_packets": downlink,
		"drops":            drops,
		"egress":           a.svc.egress.Name(),
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
