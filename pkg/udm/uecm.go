package udm

import (
	"encoding/json"
	"net/http"

	"github.com/qcore-project/qcore/pkg/sbi"
)

// --- Nudm_UECM — UE Context Management (TS 29.503 §6.3) -------------------
//
// When a UE registers, the AMF calls PUT to register itself as the serving
// AMF for the SUPI. UDM records this so other NFs (e.g. SMF, PCF) can ask
// "which AMF is currently serving IMSI X?". DeregistrationData events (e.g.
// roaming) would call DELETE on this resource.
//
// Scope for v0.6: in-memory store, no persistence. The AMF calls this once
// after Registration Accept to anchor itself.

// AmfRegistration holds the serving AMF binding per TS 29.503 §6.3.4.2.3.
type AmfRegistration struct {
	AMFInstanceID     string     `json:"amfInstanceId"`
	SupportedFeatures string     `json:"supportedFeatures,omitempty"`
	PEI               string     `json:"pei,omitempty"` // IMEISV or IMEI
	GUAMI             *GUAMIWire `json:"guami,omitempty"`
	// Rat-Type, access-type etc. are optional in v0.6 scope; add as needed.
}

// GUAMIWire is the SBI JSON form of GUAMI (TS 29.571 §5.4.4).
type GUAMIWire struct {
	PLMNId PLMNIdWire `json:"plmnId"`
	AmfId  string     `json:"amfId"` // hex: regionID(2) + setID(3) + pointer(2)
}

// PLMNIdWire — TS 29.571 §5.4.2
type PLMNIdWire struct {
	MCC string `json:"mcc"`
	MNC string `json:"mnc"`
}

// WithUECM enables the Nudm_UECM routes on the UDM service.
// Call after NewService if you want AMF registration anchoring.
func (s *Service) WithUECM() *Service {
	s.uecm = newUECMStore()
	s.mux.HandleFunc("PUT /nudm-uecm/v1/{ueId}/registrations/amf-3gpp-access", s.putAmfRegistration)
	s.mux.HandleFunc("GET /nudm-uecm/v1/{ueId}/registrations/amf-3gpp-access", s.getAmfRegistration)
	s.mux.HandleFunc("DELETE /nudm-uecm/v1/{ueId}/registrations/amf-3gpp-access", s.deleteAmfRegistration)
	return s
}

// putAmfRegistration — TS 29.503 §6.3.5.2.2.
// AMF calls this after completing UE registration to anchor serving AMF binding.
func (s *Service) putAmfRegistration(w http.ResponseWriter, r *http.Request) {
	ueID := r.PathValue("ueId")
	if ueID == "" {
		sbi.WriteProblem(w, sbi.BadRequest("missing ueId"))
		return
	}
	var reg AmfRegistration
	if err := json.NewDecoder(r.Body).Decode(&reg); err != nil {
		sbi.WriteProblem(w, sbi.BadRequest("malformed AmfRegistration: "+err.Error()))
		return
	}
	if reg.AMFInstanceID == "" {
		sbi.WriteProblem(w, sbi.BadRequest("amfInstanceId is required"))
		return
	}
	s.uecm.put(ueID, reg)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(reg)
}

// getAmfRegistration — TS 29.503 §6.3.5.2.3.
func (s *Service) getAmfRegistration(w http.ResponseWriter, r *http.Request) {
	ueID := r.PathValue("ueId")
	reg, ok := s.uecm.get(ueID)
	if !ok {
		sbi.WriteProblem(w, sbi.NotFound("no AMF registration for "+ueID))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(reg)
}

// deleteAmfRegistration — TS 29.503 §6.3.5.2.4.
func (s *Service) deleteAmfRegistration(w http.ResponseWriter, r *http.Request) {
	ueID := r.PathValue("ueId")
	s.uecm.delete(ueID)
	w.WriteHeader(http.StatusNoContent)
}
