package udm

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/qcore-project/qcore/pkg/logger"
	"github.com/qcore-project/qcore/pkg/sbi"
	"github.com/qcore-project/qcore/pkg/subscriber"
)

// SubscriberStore is the subset of pkg/subscriber.Service that UDM needs.
// Defined as a local interface so tests can supply a fake without dragging
// in gorm, and so pkg/udm stays blind to the storage layer — the whole
// point of UDM is that it's the data face, not the database.
//
// pkg/subscriber.Service satisfies this interface as-is.
type SubscriberStore interface {
	GetSubscriber(ctx context.Context, imsi string) (*subscriber.Subscriber, error)
}

// Service is the UDM NF. Its Handler() is intended to be handed to
// pkg/sbi.NewServer — all HTTP/2, middleware, and TLS concerns live there.
type Service struct {
	store SubscriberStore
	log   logger.Logger
	mux   *http.ServeMux
}

// NewService wires a UDM over the given store. The returned *Service has
// its routes registered; mount it with srv := sbi.NewServer(cfg, log, udm.Handler()).
func NewService(store SubscriberStore, log logger.Logger) *Service {
	s := &Service{
		store: store,
		log:   log.WithField("nf", "udm"),
		mux:   http.NewServeMux(),
	}
	s.registerRoutes()
	return s
}

// Handler returns the raw mux so pkg/sbi (or a test harness) can wrap it
// with its own middleware chain.
func (s *Service) Handler() http.Handler {
	return s.mux
}

func (s *Service) registerRoutes() {
	// Go 1.22+ method-prefixed patterns. {supi} is a path wildcard.
	s.mux.HandleFunc("GET /nudm-sdm/v2/{supi}/am-data", s.getAmData)
}

// getAmData — TS 29.503 §5.2.2.2.2. Returns AccessAndMobilitySubscriptionData
// for a UE identified by SUPI.
//
// 3GPP SUPI format per TS 23.003 §2.2A: "imsi-<15 digits>" for 3GPP access,
// or "nai-..." for non-3GPP. We only handle the IMSI form today; NAI form
// returns 501 so the caller knows it's not yet implemented rather than
// failing silently.
func (s *Service) getAmData(w http.ResponseWriter, r *http.Request) {
	supi := r.PathValue("supi")
	imsi, err := parseIMSISupi(supi)
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
		// subscriber.Service returns "subscriber <imsi> not found" as a
		// bare error. Map that string to 404/USER_NOT_FOUND (TS 29.503
		// §6.1.7.3). Everything else is a 500.
		if strings.Contains(err.Error(), "not found") {
			sbi.WriteProblem(w, &sbi.ProblemDetails{
				Status: http.StatusNotFound,
				Title:  "Not Found",
				Detail: err.Error(),
				Cause:  "USER_NOT_FOUND",
			})
			return
		}
		s.log.WithError(err).WithField("supi", supi).Error("udm: get subscriber failed")
		sbi.WriteProblem(w, sbi.InternalError("subscriber lookup failed"))
		return
	}

	resp := AccessAndMobilitySubscriptionData{
		// Default UE-AMBR for v0.5. Real deployments override per-profile;
		// until we add that field to pkg/subscriber.Subscriber we hand out
		// a generous default rather than surprising the caller with nil.
		SubscribedUeAmbr: &AmbrRm{
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

// parseIMSISupi accepts a SUPI in the "imsi-<15 digits>" form and returns
// the bare IMSI. Rejects NAI/GCI/GLI forms with an explanatory error —
// they exist in the spec but none of QCore's current access types need
// them, so faking a lookup would be misleading.
func parseIMSISupi(supi string) (string, error) {
	if supi == "" {
		return "", errBadSupi("SUPI is empty")
	}
	if !strings.HasPrefix(supi, "imsi-") {
		return "", errBadSupi("only imsi-<IMSI> SUPIs are supported; got " + supi)
	}
	imsi := strings.TrimPrefix(supi, "imsi-")
	if len(imsi) != 15 {
		return "", errBadSupi("IMSI portion must be 15 digits, got " + imsi)
	}
	for _, c := range imsi {
		if c < '0' || c > '9' {
			return "", errBadSupi("IMSI must be all digits, got " + imsi)
		}
	}
	return imsi, nil
}

type supiError string

func (e supiError) Error() string { return string(e) }

func errBadSupi(msg string) error { return supiError(msg) }
