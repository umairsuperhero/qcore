package mme

import (
	"encoding/json"
	"fmt"
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
	api.HandleFunc("/ues/{imsi}/page", a.pagingTrigger).Methods("POST")
	api.HandleFunc("/enbs", a.listENBs).Methods("GET")
}

// enbInfo is the JSON representation of a connected eNB for the /enbs endpoint.
type enbInfo struct {
	RemoteAddr string   `json:"remote_addr"`
	ENBName    string   `json:"enb_name,omitempty"`
	GlobalID   string   `json:"global_enb_id,omitempty"` // hex
	PLMN       string   `json:"plmn,omitempty"`           // hex
	TACs       []uint16 `json:"tacs,omitempty"`
}

func (a *API) listENBs(w http.ResponseWriter, r *http.Request) {
	var enbs []enbInfo
	a.mme.enbs.Range(func(_, value any) bool {
		enb, ok := value.(*EnbContext)
		if !ok {
			return true
		}
		enb.mu.RLock()
		info := enbInfo{
			RemoteAddr: enb.Assoc.RemoteAddr().String(),
			ENBName:    enb.ENBName,
			GlobalID:   fmt.Sprintf("%x", enb.GlobalENBID.ENBID),
			PLMN:       fmt.Sprintf("%x", enb.GlobalENBID.PLMN),
		}
		for _, ta := range enb.SupportedTAs {
			info.TACs = append(info.TACs, ta.TAC)
		}
		enb.mu.RUnlock()
		enbs = append(enbs, info)
		return true
	})
	if enbs == nil {
		enbs = []enbInfo{}
	}
	respondJSON(w, http.StatusOK, enbs)
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

// pagingTrigger handles POST /api/v1/ues/{imsi}/page.
// It sends an S1AP PAGING message to all eNBs supporting the UE's TAI.
// Used to wake up an ECM-IDLE UE for incoming data or signaling.
func (a *API) pagingTrigger(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	imsi := vars["imsi"]
	if imsi == "" {
		http.Error(w, "missing IMSI", http.StatusBadRequest)
		return
	}

	count, err := a.mme.TriggerPaging(imsi)
	if err != nil {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"imsi":         imsi,
		"enbs_paged":   count,
	})
}

func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
