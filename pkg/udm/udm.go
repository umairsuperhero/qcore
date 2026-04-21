package udm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/qcore-project/qcore/pkg/logger"
	"github.com/qcore-project/qcore/pkg/sbi"
	"github.com/qcore-project/qcore/pkg/sbi/common"
	"github.com/qcore-project/qcore/pkg/subscriber"
)

// AmDataSource is what UDM needs to serve Nudm_SDM am-data. Two impls
// today: a direct adapter over pkg/subscriber (storeSource) and a
// pkg/udr.Client for UDR-backed mode. The interface is the layering
// seam — swapping modes is a constructor-arg change, not a refactor.
//
// Sources receive the raw SUPI ("imsi-<15 digits>") because the UDR
// client passes it straight through as a ueId; the direct adapter
// strips the prefix internally.
type AmDataSource interface {
	GetAmData(ctx context.Context, supi string) (*common.AccessAndMobilitySubscriptionData, error)
}

// Typed errors a source can return. The HTTP handler maps these to
// RFC 7807 responses so both backends produce the same Problem shape.
var (
	ErrNotFound = errors.New("am-data not found")
	ErrBadSupi  = errors.New("malformed SUPI")
)

// SubscriberStore is the subset of pkg/subscriber.Service the direct
// adapter needs. Kept as a named interface so tests can supply a fake
// without pulling in gorm.
//
// pkg/subscriber.Service satisfies this as-is.
type SubscriberStore interface {
	GetSubscriber(ctx context.Context, imsi string) (*subscriber.Subscriber, error)
}

// NewStoreSource adapts a SubscriberStore to AmDataSource for
// direct-mode UDM (no UDR hop). Applies the same SUPI-parse + AMBR
// default that the handler used to do inline.
func NewStoreSource(store SubscriberStore) AmDataSource {
	return &storeSource{store: store}
}

type storeSource struct{ store SubscriberStore }

func (s *storeSource) GetAmData(ctx context.Context, supi string) (*common.AccessAndMobilitySubscriptionData, error) {
	imsi, err := parseIMSISupi(supi)
	if err != nil {
		return nil, err
	}
	sub, err := s.store.GetSubscriber(ctx, imsi)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, ErrNotFound
		}
		return nil, err
	}
	// Default UE-AMBR for v0.5. Real deployments override per-profile;
	// until we add that field to pkg/subscriber.Subscriber we hand out
	// a generous default rather than surprising the caller with nil.
	resp := &common.AccessAndMobilitySubscriptionData{
		SubscribedUeAmbr: &common.AmbrRm{Uplink: "1 Gbps", Downlink: "1 Gbps"},
	}
	if sub.MSISDN != "" {
		resp.Gpsis = []string{"msisdn-" + sub.MSISDN}
	}
	return resp, nil
}

// Service is the UDM NF. Its Handler() is intended to be handed to
// pkg/sbi.NewServer — all HTTP/2, middleware, and TLS concerns live there.
//
// auth is optional: without it, the /nudm-ueau routes respond 501.
// Attach one with WithAuthSource.
type Service struct {
	source AmDataSource
	auth   AuthSource
	log    logger.Logger
	mux    *http.ServeMux
}

// NewService wires a UDM over the given AmDataSource. For direct mode
// pass NewStoreSource(store); for UDR-backed mode pass a pkg/udr.Client.
func NewService(source AmDataSource, log logger.Logger) *Service {
	s := &Service{
		source: source,
		log:    log.WithField("nf", "udm"),
		mux:    http.NewServeMux(),
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
// for a UE identified by SUPI. Delegates to the configured AmDataSource
// and maps typed errors to RFC 7807 responses.
func (s *Service) getAmData(w http.ResponseWriter, r *http.Request) {
	supi := r.PathValue("supi")
	resp, err := s.source.GetAmData(r.Context(), supi)
	if err != nil {
		switch {
		case errors.Is(err, ErrBadSupi):
			sbi.WriteProblem(w, &sbi.ProblemDetails{
				Status: http.StatusBadRequest,
				Title:  "Bad Request",
				Detail: err.Error(),
				Cause:  "MANDATORY_IE_INCORRECT",
			})
		case errors.Is(err, ErrNotFound):
			sbi.WriteProblem(w, &sbi.ProblemDetails{
				Status: http.StatusNotFound,
				Title:  "Not Found",
				Detail: err.Error(),
				Cause:  "USER_NOT_FOUND",
			})
		default:
			s.log.WithError(err).WithField("supi", supi).Error("udm: get am-data failed")
			sbi.WriteProblem(w, sbi.InternalError("am-data lookup failed"))
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// parseIMSISupi accepts a SUPI in the "imsi-<15 digits>" form and returns
// the bare IMSI. Rejects NAI/GCI/GLI forms — they exist in the spec but
// none of QCore's current access types need them, so faking a lookup
// would be misleading. Wraps ErrBadSupi so the handler maps to 400.
func parseIMSISupi(supi string) (string, error) {
	if supi == "" {
		return "", badSupi("SUPI is empty")
	}
	if !strings.HasPrefix(supi, "imsi-") {
		return "", badSupi("only imsi-<IMSI> SUPIs are supported; got " + supi)
	}
	imsi := strings.TrimPrefix(supi, "imsi-")
	if len(imsi) != 15 {
		return "", badSupi("IMSI portion must be 15 digits, got " + imsi)
	}
	for _, c := range imsi {
		if c < '0' || c > '9' {
			return "", badSupi("IMSI must be all digits, got " + imsi)
		}
	}
	return imsi, nil
}

func badSupi(msg string) error {
	return &supiErr{msg: msg}
}

type supiErr struct{ msg string }

func (e *supiErr) Error() string { return e.msg }
func (e *supiErr) Is(target error) bool {
	return target == ErrBadSupi
}
