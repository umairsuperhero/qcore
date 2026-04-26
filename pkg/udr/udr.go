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
	SetSQN(ctx context.Context, imsi, newSQN string) error
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

	// TS 29.505 §5.2.2.3 / 29.503 §6.3.6.2.2.
	// Authentication-subscription sits at a different path shape —
	// no servingPlmnId, since Milenage creds are PLMN-independent.
	s.mux.HandleFunc("GET /nudr-dr/v2/subscription-data/{ueId}/authentication-data/authentication-subscription", s.getAuthSubscription)

	// TS 29.505 §5.2.2.3.4. PATCH accepts an RFC 6902 JSON Patch body.
	// Today we only implement the one op UEAU actually needs — replace
	// /sequenceNumber/sqn — because that's what holds back a UDR-backed
	// UEAU flip. Other ops return 422 so a caller can't silently no-op.
	s.mux.HandleFunc("PATCH /nudr-dr/v2/subscription-data/{ueId}/authentication-data/authentication-subscription", s.patchAuthSubscription)
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

// getAuthSubscription — TS 29.505 §5.2.2.3.3. Returns the Milenage
// credentials (K, OPc, AMF, SQN) for a UE in the shape UDM UEAU needs
// to produce a 5G-AKA vector. See the doc comment on
// common.AuthenticationSubscription for the v0.5 plaintext caveat.
func (s *Service) getAuthSubscription(w http.ResponseWriter, r *http.Request) {
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

	resp := common.AuthenticationSubscription{
		AuthenticationMethod:          "5G_AKA",
		EncPermanentKey:               sub.Ki,
		EncOpcKey:                     sub.OPc,
		AuthenticationManagementField: sub.AMF,
		AlgorithmId:                   "milenage",
		SequenceNumber: &common.SequenceNumber{
			SqnScheme: "GENERAL",
			Sqn:       sub.SQN,
		},
		Supi: "imsi-" + sub.IMSI,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// patchAuthSubscription — TS 29.505 §5.2.2.3.4. Applies an RFC 6902 JSON
// Patch to the UE's auth-subscription. QCore supports exactly one op
// shape today: {"op":"replace","path":"/sequenceNumber/sqn","value":"<hex>"}.
// Other ops/paths return 422. A 204 on success mirrors the spec's
// "No Content" response style for successful PATCH.
func (s *Service) patchAuthSubscription(w http.ResponseWriter, r *http.Request) {
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

	var ops []jsonPatchOp
	if err := json.NewDecoder(r.Body).Decode(&ops); err != nil {
		sbi.WriteProblem(w, &sbi.ProblemDetails{
			Status: http.StatusBadRequest,
			Title:  "Bad Request",
			Detail: "malformed JSON Patch body: " + err.Error(),
			Cause:  "MANDATORY_IE_INCORRECT",
		})
		return
	}

	var newSQN string
	for _, op := range ops {
		if op.Op == "replace" && op.Path == "/sequenceNumber/sqn" {
			v, ok := op.Value.(string)
			if !ok {
				sbi.WriteProblem(w, &sbi.ProblemDetails{
					Status: http.StatusUnprocessableEntity,
					Title:  "Unprocessable Entity",
					Detail: "sqn value must be a string",
					Cause:  "MANDATORY_IE_INCORRECT",
				})
				return
			}
			newSQN = v
			continue
		}
		sbi.WriteProblem(w, &sbi.ProblemDetails{
			Status: http.StatusUnprocessableEntity,
			Title:  "Unprocessable Entity",
			Detail: "unsupported patch op " + op.Op + " on " + op.Path + "; only replace on /sequenceNumber/sqn is supported",
			Cause:  "UNSUPPORTED_RESOURCE",
		})
		return
	}
	if newSQN == "" {
		sbi.WriteProblem(w, &sbi.ProblemDetails{
			Status: http.StatusBadRequest,
			Title:  "Bad Request",
			Detail: "no supported patch op in body",
			Cause:  "MANDATORY_IE_MISSING",
		})
		return
	}

	if err := s.store.SetSQN(r.Context(), imsi, newSQN); err != nil {
		if strings.Contains(err.Error(), "not found") {
			sbi.WriteProblem(w, &sbi.ProblemDetails{
				Status: http.StatusNotFound,
				Title:  "Not Found",
				Detail: err.Error(),
				Cause:  "DATA_NOT_FOUND",
			})
			return
		}
		if strings.Contains(err.Error(), "hex") || strings.Contains(err.Error(), "12 hex") {
			sbi.WriteProblem(w, &sbi.ProblemDetails{
				Status: http.StatusBadRequest,
				Title:  "Bad Request",
				Detail: err.Error(),
				Cause:  "MANDATORY_IE_INCORRECT",
			})
			return
		}
		s.log.WithError(err).WithField("ueId", ueID).Error("udr: set sqn failed")
		sbi.WriteProblem(w, sbi.InternalError("sqn update failed"))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type jsonPatchOp struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value any    `json:"value"`
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
