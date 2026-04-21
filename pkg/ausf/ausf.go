package ausf

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/qcore-project/qcore/pkg/logger"
	"github.com/qcore-project/qcore/pkg/sbi"
	"github.com/qcore-project/qcore/pkg/subscriber"
	"github.com/qcore-project/qcore/pkg/udm"
)

// AuthType — TS 29.509 §6.1.6.3.5.
type AuthType string

const (
	AuthType5GAka AuthType = "5G_AKA"
)

// AuthResult — TS 29.509 §6.1.6.3.9.
type AuthResult string

const (
	AuthResultSuccess AuthResult = "AUTHENTICATION_SUCCESS"
	AuthResultFailure AuthResult = "AUTHENTICATION_FAILURE"
	AuthResultOngoing AuthResult = "AUTHENTICATION_ONGOING"
)

// AuthenticationInfo — TS 29.509 §6.1.6.2.2. POST body from AMF to
// /nausf-auth/v1/ue-authentications.
type AuthenticationInfo struct {
	SupiOrSuci            string                     `json:"supiOrSuci"`
	ServingNetworkName    string                     `json:"servingNetworkName"`
	ResynchronizationInfo *udm.ResynchronizationInfo `json:"resynchronizationInfo,omitempty"`
	AusfInstanceID        string                     `json:"ausfInstanceId,omitempty"`
	PEI                   string                     `json:"pei,omitempty"`
}

// UEAuthenticationCtx — TS 29.509 §6.1.6.2.3. 201 Created response body.
// Location header carries the same URL as _links["5g-aka"].href.
type UEAuthenticationCtx struct {
	AuthType           AuthType         `json:"authType"`
	Av5gAuthData       Av5gAka          `json:"5gAuthData"`
	Links              map[string]Link  `json:"_links"`
	ServingNetworkName string           `json:"servingNetworkName,omitempty"`
}

// Av5gAka — TS 29.509 §6.1.6.2.4. The 5G-AKA vector AUSF hands to AMF.
// Note this is AUSF's outbound shape (hxresStar) — distinct from UDM's
// Av5gHeAka (xresStar) which stays inside AUSF.
type Av5gAka struct {
	RAND      string `json:"rand"`
	AUTN      string `json:"autn"`
	HXResStar string `json:"hxresStar"`
}

// Link — TS 29.571 LinksValueSchema.
type Link struct {
	Href string `json:"href"`
}

// ConfirmationData — TS 29.509 §6.1.6.2.5. PUT body on the
// /5g-aka-confirmation leg.
type ConfirmationData struct {
	ResStar string `json:"resStar"`
}

// ConfirmationDataResponse — TS 29.509 §6.1.6.2.6.
type ConfirmationDataResponse struct {
	AuthResult AuthResult `json:"authResult"`
	Supi       string     `json:"supi,omitempty"`
	Kseaf      string     `json:"kseaf,omitempty"`
}

// UDMClient is the subset of pkg/udm.Client the AUSF needs. Kept as an
// interface so tests can supply a fake without standing up a real UDM.
type UDMClient interface {
	GenerateAuthData(ctx context.Context, supi string, req *udm.AuthenticationInfoRequest) (*udm.AuthenticationInfoResult, error)
}

// Service is the AUSF NF. One UDM client per instance (single-UDM dev
// posture; v0.6+ may grow NRF-based UDM discovery per-request).
type Service struct {
	udm    UDMClient
	log    logger.Logger
	mux    *http.ServeMux
	store  *ctxStore
}

// NewService wires an AUSF over the given UDM client.
func NewService(udmClient UDMClient, log logger.Logger) *Service {
	s := &Service{
		udm:   udmClient,
		log:   log.WithField("nf", "ausf"),
		mux:   http.NewServeMux(),
		store: newCtxStore(),
	}
	s.registerRoutes()
	return s
}

// Handler returns the raw mux for pkg/sbi to wrap.
func (s *Service) Handler() http.Handler {
	return s.mux
}

func (s *Service) registerRoutes() {
	s.mux.HandleFunc("POST /nausf-auth/v1/ue-authentications", s.postUEAuth)
	s.mux.HandleFunc("PUT /nausf-auth/v1/ue-authentications/{authCtxId}/5g-aka-confirmation", s.putConfirm)
}

// postUEAuth — TS 29.509 §5.2.2.2.2. Creates a UE auth context: fetches
// Av5gHeAka from UDM, splits it into {keep XRES*+KAUSF} and {ship
// RAND+AUTN+HXRES*}, and returns 201 Created with a Location pointing
// at the 5g-aka-confirmation subresource.
func (s *Service) postUEAuth(w http.ResponseWriter, r *http.Request) {
	var req AuthenticationInfo
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sbi.WriteProblem(w, &sbi.ProblemDetails{
			Status: http.StatusBadRequest,
			Title:  "Bad Request",
			Detail: "malformed AuthenticationInfo: " + err.Error(),
			Cause:  "MANDATORY_IE_INCORRECT",
		})
		return
	}
	if req.SupiOrSuci == "" {
		sbi.WriteProblem(w, &sbi.ProblemDetails{
			Status: http.StatusBadRequest,
			Title:  "Bad Request",
			Detail: "supiOrSuci is required",
			Cause:  "MANDATORY_IE_MISSING",
		})
		return
	}
	if req.ServingNetworkName == "" {
		sbi.WriteProblem(w, &sbi.ProblemDetails{
			Status: http.StatusBadRequest,
			Title:  "Bad Request",
			Detail: "servingNetworkName is required",
			Cause:  "MANDATORY_IE_MISSING",
		})
		return
	}

	udmResp, err := s.udm.GenerateAuthData(r.Context(), req.SupiOrSuci, &udm.AuthenticationInfoRequest{
		ServingNetworkName: req.ServingNetworkName,
	})
	if err != nil {
		switch {
		case errors.Is(err, udm.ErrNotFound):
			sbi.WriteProblem(w, &sbi.ProblemDetails{
				Status: http.StatusNotFound,
				Title:  "Not Found",
				Detail: "subscriber unknown to UDM",
				Cause:  "USER_NOT_FOUND",
			})
		case errors.Is(err, udm.ErrBadSupi):
			sbi.WriteProblem(w, &sbi.ProblemDetails{
				Status: http.StatusBadRequest,
				Title:  "Bad Request",
				Detail: "UDM rejected SUPI",
				Cause:  "MANDATORY_IE_INCORRECT",
			})
		default:
			s.log.WithError(err).Error("ausf: UDM call failed")
			sbi.WriteProblem(w, sbi.InternalError("UDM generate-auth-data failed"))
		}
		return
	}
	if udmResp.AuthenticationVector == nil {
		s.log.Error("ausf: UDM response missing authenticationVector")
		sbi.WriteProblem(w, sbi.InternalError("UDM returned empty vector"))
		return
	}

	he := udmResp.AuthenticationVector
	rnd, err := hexDecode16(he.RAND)
	if err != nil {
		s.log.WithError(err).Error("ausf: bad RAND from UDM")
		sbi.WriteProblem(w, sbi.InternalError("bad RAND from UDM"))
		return
	}
	xres, err := hexDecode16(he.XResStar)
	if err != nil {
		s.log.WithError(err).Error("ausf: bad XRES* from UDM")
		sbi.WriteProblem(w, sbi.InternalError("bad XRES* from UDM"))
		return
	}
	kausfBytes, err := hexDecode32(he.KAUSF)
	if err != nil {
		s.log.WithError(err).Error("ausf: bad KAUSF from UDM")
		sbi.WriteProblem(w, sbi.InternalError("bad KAUSF from UDM"))
		return
	}

	hxres := subscriber.DeriveHXRESStar(rnd, xres)

	ctxID, err := newCtxID()
	if err != nil {
		s.log.WithError(err).Error("ausf: ctx id generation failed")
		sbi.WriteProblem(w, sbi.InternalError("internal error"))
		return
	}
	s.store.put(ctxID, &authCtx{
		supi:     udmResp.Supi,
		xresStar: xres,
		kausf:    kausfBytes,
		snName:   req.ServingNetworkName,
	})

	href := "/nausf-auth/v1/ue-authentications/" + ctxID + "/5g-aka-confirmation"
	resp := UEAuthenticationCtx{
		AuthType: AuthType5GAka,
		Av5gAuthData: Av5gAka{
			RAND:      he.RAND,
			AUTN:      he.AUTN,
			HXResStar: hex.EncodeToString(hxres[:]),
		},
		Links:              map[string]Link{"5g-aka": {Href: href}},
		ServingNetworkName: req.ServingNetworkName,
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Location", href)
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(resp)
}

// putConfirm — TS 29.509 §5.2.2.2.3. Compares RES* from the UE (via AMF)
// against the stored XRES*. On match, derives KSEAF and returns
// AUTHENTICATION_SUCCESS; on mismatch, FAILURE. Context is consumed on
// terminal result — a second attempt with the same ctxId 404s.
func (s *Service) putConfirm(w http.ResponseWriter, r *http.Request) {
	ctxID := r.PathValue("authCtxId")
	ctx := s.store.get(ctxID)
	if ctx == nil {
		sbi.WriteProblem(w, &sbi.ProblemDetails{
			Status: http.StatusNotFound,
			Title:  "Not Found",
			Detail: "unknown authCtxId",
			Cause:  "CONTEXT_NOT_FOUND",
		})
		return
	}

	var body ConfirmationData
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		sbi.WriteProblem(w, &sbi.ProblemDetails{
			Status: http.StatusBadRequest,
			Title:  "Bad Request",
			Detail: "malformed ConfirmationData: " + err.Error(),
			Cause:  "MANDATORY_IE_INCORRECT",
		})
		return
	}

	resStar, err := hexDecode16(body.ResStar)
	if err != nil {
		sbi.WriteProblem(w, &sbi.ProblemDetails{
			Status: http.StatusBadRequest,
			Title:  "Bad Request",
			Detail: "resStar must be 32 hex chars: " + err.Error(),
			Cause:  "MANDATORY_IE_INCORRECT",
		})
		return
	}

	// Constant-time compare. RES* == XRES* means the UE and home network
	// agreed on the Milenage output — per TS 33.501 §6.1.3.2 this is the
	// auth success condition.
	s.store.del(ctxID)
	w.Header().Set("Content-Type", "application/json")
	if subtle.ConstantTimeCompare(resStar[:], ctx.xresStar[:]) != 1 {
		_ = json.NewEncoder(w).Encode(ConfirmationDataResponse{
			AuthResult: AuthResultFailure,
			Supi:       ctx.supi,
		})
		return
	}

	kseaf := subscriber.DeriveKSEAF(ctx.kausf, ctx.snName)
	_ = json.NewEncoder(w).Encode(ConfirmationDataResponse{
		AuthResult: AuthResultSuccess,
		Supi:       ctx.supi,
		Kseaf:      hex.EncodeToString(kseaf[:]),
	})
}

// ctxStore is an in-memory authCtxId → auth-context map. Fine for the
// single-instance dev posture; swap for Redis/etcd in v1.0 so any AUSF
// replica can complete any 5G-AKA transaction.
type ctxStore struct {
	mu   sync.Mutex
	ctxs map[string]*authCtx
}

type authCtx struct {
	supi     string
	xresStar [16]byte
	kausf    [32]byte
	snName   string
}

func newCtxStore() *ctxStore                     { return &ctxStore{ctxs: make(map[string]*authCtx)} }
func (s *ctxStore) put(id string, c *authCtx)    { s.mu.Lock(); s.ctxs[id] = c; s.mu.Unlock() }
func (s *ctxStore) del(id string)                { s.mu.Lock(); delete(s.ctxs, id); s.mu.Unlock() }
func (s *ctxStore) get(id string) *authCtx {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ctxs[id]
}

// newCtxID returns a URL-safe 128-bit random identifier. 128 bits of
// entropy is overkill for dev but keeps this boring if AUSF ever runs
// with many AMFs banging on it.
func newCtxID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("rand.Read: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

func hexDecode16(s string) ([16]byte, error) {
	var out [16]byte
	b, err := hex.DecodeString(s)
	if err != nil {
		return out, err
	}
	if len(b) != 16 {
		return out, fmt.Errorf("expected 16 bytes, got %d", len(b))
	}
	copy(out[:], b)
	return out, nil
}

func hexDecode32(s string) ([32]byte, error) {
	var out [32]byte
	b, err := hex.DecodeString(s)
	if err != nil {
		return out, err
	}
	if len(b) != 32 {
		return out, fmt.Errorf("expected 32 bytes, got %d", len(b))
	}
	copy(out[:], b)
	return out, nil
}
