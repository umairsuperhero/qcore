package mme

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/qcore-project/qcore/pkg/logger"
	"github.com/qcore-project/qcore/pkg/metrics"
)

// API provides a REST interface for MME status and debugging.
type API struct {
	mme     *MME
	log     logger.Logger
	metrics *metrics.MMEMetrics
	router  *mux.Router
}

// NewAPI creates the MME debug/status REST API.
func NewAPI(mme *MME, log logger.Logger, m *metrics.MMEMetrics) *API {
	a := &API{
		mme:     mme,
		log:     log.WithField("component", "mme-api"),
		metrics: m,
		router:  mux.NewRouter(),
	}
	a.registerRoutes()
	return a
}

// Router returns the HTTP router.
func (a *API) Router() *mux.Router {
	return a.router
}

func (a *API) registerRoutes() {
	api := a.router.PathPrefix("/api/v1").Subrouter()
	api.HandleFunc("/health", a.health).Methods("GET")
	api.HandleFunc("/status", a.status).Methods("GET")
}

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"service": "qcore-mme",
	})
}

func (a *API) status(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{
		"connected_enbs": a.mme.GetENBCount(),
		"active_ues":     a.mme.GetUECount(),
	})
}

func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
