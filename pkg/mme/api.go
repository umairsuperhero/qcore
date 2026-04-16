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
	api.HandleFunc("/ues", a.listUEs).Methods("GET")
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

// ueInfo is the JSON representation of a registered UE for the /ues endpoint.
type ueInfo struct {
	MMEUES1APID uint32 `json:"mme_ue_s1ap_id"`
	ENBUES1APID uint32 `json:"enb_ue_s1ap_id"`
	IMSI        string `json:"imsi,omitempty"`
	EMMState    string `json:"emm_state"`
	ECMState    string `json:"ecm_state"`
	PDNAddr     string `json:"pdn_addr,omitempty"`
}

func (a *API) listUEs(w http.ResponseWriter, r *http.Request) {
	var ues []ueInfo
	a.mme.ues.Range(func(_, value any) bool {
		ue, ok := value.(*UEContext)
		if !ok {
			return true
		}
		ue.mu.RLock()
		info := ueInfo{
			MMEUES1APID: ue.MMEUES1APID,
			ENBUES1APID: ue.ENBUES1APID,
			IMSI:        ue.IMSI,
			EMMState:    ue.EMMState.String(),
			ECMState:    ue.ECMState.String(),
			PDNAddr:     ue.PDNAddr,
		}
		ue.mu.RUnlock()
		ues = append(ues, info)
		return true
	})
	if ues == nil {
		ues = []ueInfo{} // return [] not null
	}
	respondJSON(w, http.StatusOK, ues)
}

func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
