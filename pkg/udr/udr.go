package udr

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/qcore-project/qcore/pkg/logger"
	"github.com/qcore-project/qcore/pkg/sbi"
	"github.com/qcore-project/qcore/pkg/sbi/common"
	"github.com/qcore-project/qcore/pkg/subscriber"
)

// SubscriberStore is the slice of pkg/subscriber.Service that UDR needs.
// Structurally identical to pkg/udm.SubscriberStore today — kept local
// rather than imported to keep the two NFs decoupled (they'll diverge
// once UDR owns its own storage tables).
type SubscriberStore interface {
	GetSubscriber(ctx context.Context, imsi string) (*subscriber.Subscriber, error)
}

// Service is the UDR NF. Hand Handler() to pkg/sbi.NewServer.
type Service struct {
	store SubscriberStore
	log   logger.Logger
	mux   *http.ServeMux
}

// NewService wires a UDR over the given store.
func NewService(store SubscriberStore, log logger.Logger) *Service {
	s := &Service{
		store: store,
		log:   log.WithField("nf", "udr"),
		mux:   http.NewServeMux(),
	}
	s.registerRoutes()
	return s
}

// Handler returns the raw mux for pkg/sbi to wrap.
func (s *Service) Handler() http.Handler {
	return s.mux
}

func (s *Service) registerRoutes() {
	// TS 29.504 §5.2.2.2. {servingPlmnId} is part of the path but we
	// don't use it yet — PLMN-scoped subscription data is a v1.0+
	// concern; today one subscriber = one profile across all PLMNs.
	s.mux.HandleFunc("GET /nudr-dr/v2/subscription-data/{ueId}/{servingPlmnId}/provisioned-data/am-data", s.getAmData)
}

// getAmData — TS 29.504 §5.2.2.2.3. Returns the AM subscription data
// for a UE. ueId in UDR paths is typically in the "imsi-<digits>" form
// (same as SUPI), though TS 29.504 §5.6.3.2 also allows "msisdn-..." and
// GCI/GLI forms. QCore supports the IMSI form for now.
func (s *Service) getAmData(w http.ResponseWriter, r *http.Request) {
	ueID := r.PathValue("ueId")
	imsi, err := parseIMSIUeID(ueID)
	if err != nil {
		sbi.WriteProblem(w, &sbi.ProblemDetails{
			Status: http.StatusBadRequest,
			Title:  "Bad Request",
			Detail: err.Error(),
			Cause:  "MANDATORY_IE_INCORRECT",
		})
		return
	}

	sub, err := s.store.GetSubscriber(r.Context(), imsi)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			sbi.WriteProblem(w, &sbi.ProblemDetails{
				Status: http.StatusNotFound,
				Title:  "Not Found",
				Detail: err.Error(),
				Cause:  "DATA_NOT_FOUND",
			})
			return
		}
		s.log.WithError(err).WithField("ueId", ueID).Error("udr: get subscriber failed")
		sbi.WriteProblem(w, sbi.InternalError("subscriber lookup failed"))
		return
	}

	resp := common.AccessAndMobilitySubscriptionData{
		SubscribedUeAmbr: &common.AmbrRm{
			Uplink:   "1 Gbps",
			Downlink: "1 Gbps",
		},
	}
	if sub.MSISDN != "" {
		resp.Gpsis = []string{"msisdn-" + sub.MSISDN}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// parseIMSIUeID accepts a UDR ueId in "imsi-<15 digits>" form.
func parseIMSIUeID(ueID string) (string, error) {
	if ueID == "" {
		return "", ueIDError("ueId is empty")
	}
	if !strings.HasPrefix(ueID, "imsi-") {
		return "", ueIDError("only imsi-<IMSI> ueIds are supported; got " + ueID)
	}
	imsi := strings.TrimPrefix(ueID, "imsi-")
	if len(imsi) != 15 {
		return "", ueIDError("IMSI portion must be 15 digits, got " + imsi)
	}
	for _, c := range imsi {
		if c < '0' || c > '9' {
			return "", ueIDError("IMSI must be all digits, got " + imsi)
		}
	}
	return imsi, nil
}

type ueIDErr string

func (e ueIDErr) Error() string { return string(e) }

func ueIDError(msg string) error { return ueIDErr(msg) }
