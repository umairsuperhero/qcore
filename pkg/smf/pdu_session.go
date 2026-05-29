package smf

import (
	"encoding/json"
	"net/http"

	"github.com/qcore-project/qcore/pkg/sbi"
)

// SMContextCreateData — TS 29.502 §6.1.6.2.2.
// Sent by AMF to create a new SM context.
type SMContextCreateData struct {
	Supi            string `json:"supi"`
	Pei             string `json:"pei,omitempty"`
	Gpsi            string `json:"gpsi,omitempty"`
	PduSessionID    int    `json:"pduSessionId"`
	Dnn             string `json:"dnn"`
	SNssai          SNssai `json:"snssai,omitempty"`
	ServingNfId     string `json:"servingNfId"`
	Guami           *GUAMI `json:"guami,omitempty"`
	ServingNetwork  *PLMN  `json:"servingNetwork"`
	RequestType     string `json:"requestType,omitempty"`
	N1SmMsg         *RefToBinaryData `json:"n1SmMsg,omitempty"`
}

// SMContextCreatedData — TS 29.502 §6.1.6.2.3.
// Returned by SMF upon successful context creation.
type SMContextCreatedData struct {
	PduSessionID    int              `json:"pduSessionId"`
	SNssai          SNssai           `json:"snssai,omitempty"`
	UpCnxState      string           `json:"upCnxState,omitempty"` // ACTIVATED or DEACTIVATED
	N1SmMsg         *RefToBinaryData `json:"n1SmMsg,omitempty"`
	N2SmInfo        *RefToBinaryData `json:"n2SmInfo,omitempty"`
}

type SNssai struct {
	Sst int    `json:"sst"`
	Sd  string `json:"sd,omitempty"`
}

type GUAMI struct {
	PlmnId PLMN   `json:"plmnId"`
	AmfId  string `json:"amfId"`
}

type PLMN struct {
	Mcc string `json:"mcc"`
	Mnc string `json:"mnc"`
}

type RefToBinaryData struct {
	ContentId string `json:"contentId"`
}

// postSMContexts — TS 29.502 §5.2.2.2.2. Creates a PDU Session.
func (s *Service) postSMContexts(w http.ResponseWriter, r *http.Request) {
	// Parse Multipart or JSON body. For QCore v0.6, we assume standard JSON payload structure
	// while skipping full multipart NAS/NGAP binary attachments for this stub.
	var req SMContextCreateData
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sbi.WriteProblem(w, &sbi.ProblemDetails{
			Status: http.StatusBadRequest,
			Title:  "Bad Request",
			Detail: "malformed SMContextCreateData: " + err.Error(),
			Cause:  "MANDATORY_IE_INCORRECT",
		})
		return
	}
	
	if req.Supi == "" {
		sbi.WriteProblem(w, &sbi.ProblemDetails{
			Status: http.StatusBadRequest,
			Title:  "Bad Request",
			Detail: "supi is required",
			Cause:  "MANDATORY_IE_MISSING",
		})
		return
	}

	// 1. Allocate UE IP Address
	ueIP, err := s.ipam.Allocate()
	if err != nil {
		s.log.WithError(err).Error("smf: failed to allocate UE IP")
		sbi.WriteProblem(w, sbi.InternalError("IP pool exhausted"))
		return
	}
	
	s.log.WithField("ip", ueIP.String()).Info("smf: allocated UE IP")

	// 2. Setup PFCP Session (Deferred to T4 implementation when UPF is real)
	// In T3 we just implement the framework. We would call s.pfcpCli.SendRequest() here
	// to send a PFCP Session Establishment Request to the UPF.
	
	// Create the created response
	resp := SMContextCreatedData{
		PduSessionID: req.PduSessionID,
		UpCnxState:   "ACTIVATED",
	}

	// 3. Return 201 Created
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Location", "/nsmf-pdusession/v1/sm-contexts/"+req.Supi)
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(resp)
}
