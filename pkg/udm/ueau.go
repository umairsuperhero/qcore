package udm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/qcore-project/qcore/pkg/events"
	"github.com/qcore-project/qcore/pkg/sbi"
	"github.com/qcore-project/qcore/pkg/subscriber"
)

// Nudm_UEAuthentication — TS 29.503 §5.3. The UE-Authentication service
// that the AUSF calls to get a fresh authentication vector for a SUPI.
// QCore today only supports the 5G-AKA auth type; EAP-AKA' returns 501.
//
// Request/response types below mirror the OpenAPI schemas from TS 29.503
// Annex A — names are in lowerCamelCase to match the wire format.

// AuthType is the authentication method the UDM selected for this SUPI.
// TS 29.503 §6.3.6.3.3.
type AuthType string

const (
	AuthType5GAka      AuthType = "5G_AKA"
	AuthTypeEAPAkaPrim AuthType = "EAP_AKA_PRIME"
)

// AuthenticationInfoRequest — TS 29.503 §6.3.6.2.2.
// servingNetworkName is mandatory and binds the vector to a specific
// serving network per TS 33.501 Annex A.
type AuthenticationInfoRequest struct {
	ServingNetworkName    string                 `json:"servingNetworkName"`
	ResynchronizationInfo *ResynchronizationInfo `json:"resynchronizationInfo,omitempty"`
	AusfInstanceID        string                 `json:"ausfInstanceId,omitempty"`
	SupportedFeatures     string                 `json:"supportedFeatures,omitempty"`
}

// ResynchronizationInfo — TS 29.503 §6.3.6.2.6. Sent by the AUSF when
// the UE reported SQN desync; UDM is expected to re-sync from AUTS and
// re-issue. QCore doesn't implement resync yet — presence of this field
// returns 501.
type ResynchronizationInfo struct {
	RAND string `json:"rand"`
	AUTS string `json:"auts"`
}

// AuthenticationInfoResult — TS 29.503 §6.3.6.2.3.
type AuthenticationInfoResult struct {
	AuthType             AuthType   `json:"authType"`
	AuthenticationVector *Av5gHeAka `json:"authenticationVector,omitempty"`
	Supi                 string     `json:"supi,omitempty"`
	SupportedFeatures    string     `json:"supportedFeatures,omitempty"`
}

// Av5gHeAka — TS 29.503 §6.3.6.2.7. The 5G Home Environment AKA vector.
// avType is literally "5G_HE_AKA".
type Av5gHeAka struct {
	AvType   string `json:"avType"`
	RAND     string `json:"rand"`
	XResStar string `json:"xresStar"`
	AUTN     string `json:"autn"`
	KAUSF    string `json:"kausf"`
}

// AuthSource is the UEAU backend seam — analogous to AmDataSource for
// SDM. Real deployments have UDM fetch auth-subscription from UDR; for
// v0.5 QCore reads directly from pkg/subscriber.
type AuthSource interface {
	GenerateAv(ctx context.Context, supi, servingNetworkName string) (*Av5gHeAka, error)
	ResyncAndGenerateAv(ctx context.Context, supi, snName, randHex, autsHex string) (*Av5gHeAka, error)
}

// AuthGenerator is the slice of pkg/subscriber.Service that the direct
// auth source adapter needs. pkg/subscriber.Service satisfies this as-is.
type AuthGenerator interface {
	Generate5GAuthVector(ctx context.Context, imsi, snName string) (*subscriber.AuthVector5G, error)
	Resync5GAuthVector(ctx context.Context, imsi, snName, randHex, autsHex string) (*subscriber.AuthVector5G, error)
}

// NewStoreAuthSource wraps an AuthGenerator (e.g. pkg/subscriber.Service)
// as an AuthSource. Translates between SUPI wire form and bare IMSI and
// stamps avType="5G_HE_AKA" on the returned vector.
func NewStoreAuthSource(gen AuthGenerator) AuthSource {
	return &storeAuthSource{gen: gen}
}

type storeAuthSource struct{ gen AuthGenerator }

func (s *storeAuthSource) GenerateAv(ctx context.Context, supi, snName string) (*Av5gHeAka, error) {
	imsi, err := parseIMSISupi(supi)
	if err != nil {
		return nil, err
	}
	av, err := s.gen.Generate5GAuthVector(ctx, imsi, snName)
	if err != nil {
		// Mirror the string-matching convention pkg/subscriber uses —
		// same rationale as in storeSource.GetAmData.
		if strings.Contains(err.Error(), "not found") {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &Av5gHeAka{
		AvType:   "5G_HE_AKA",
		RAND:     av.RAND,
		XResStar: av.XRESStar,
		AUTN:     av.AUTN,
		KAUSF:    av.KAUSF,
	}, nil
}

func (s *storeAuthSource) ResyncAndGenerateAv(ctx context.Context, supi, snName, randHex, autsHex string) (*Av5gHeAka, error) {
	imsi, err := parseIMSISupi(supi)
	if err != nil {
		return nil, err
	}
	av, err := s.gen.Resync5GAuthVector(ctx, imsi, snName, randHex, autsHex)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &Av5gHeAka{
		AvType:   "5G_HE_AKA",
		RAND:     av.RAND,
		XResStar: av.XRESStar,
		AUTN:     av.AUTN,
		KAUSF:    av.KAUSF,
	}, nil
}

// WithAuthSource attaches an AuthSource to an existing Service and
// registers the Nudm_UEAU routes. Returns the same Service so calls can
// be chained: NewService(am, log).WithAuthSource(auth). Without this
// call, UEAU endpoints respond 501 so the server can't silently skip
// authentication in production.
func (s *Service) WithAuthSource(src AuthSource) *Service {
	s.auth = src
	s.mux.HandleFunc("POST /nudm-ueau/v1/{supi}/security-information/generate-auth-data", s.generateAuthData)
	return s
}

// generateAuthData — TS 29.503 §5.3.2.2.2.
func (s *Service) generateAuthData(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		sbi.WriteProblem(w, &sbi.ProblemDetails{
			Status: http.StatusNotImplemented,
			Title:  "Not Implemented",
			Detail: "UEAU not configured on this UDM",
		})
		return
	}

	supi := r.PathValue("supi")

	var req AuthenticationInfoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sbi.WriteProblem(w, &sbi.ProblemDetails{
			Status: http.StatusBadRequest,
			Title:  "Bad Request",
			Detail: "malformed AuthenticationInfoRequest: " + err.Error(),
			Cause:  "MANDATORY_IE_INCORRECT",
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
	var av *Av5gHeAka
	var err error
	journeyID := events.JourneyIDFromContext(r.Context())

	if req.ResynchronizationInfo != nil {
		av, err = s.auth.ResyncAndGenerateAv(r.Context(), supi, req.ServingNetworkName, req.ResynchronizationInfo.RAND, req.ResynchronizationInfo.AUTS)
	} else {
		av, err = s.auth.GenerateAv(r.Context(), supi, req.ServingNetworkName)
	}

	if err != nil {
		s.emitter.Emit(events.Event{
			JourneyID: journeyID,
			NF:        "udm",
			Category:  events.ErrorEvent,
			Severity:  events.SeverityError,
			Protocol:  "sbi",
			Message:   "Authentication vector generation failed",
			Payload: events.UDMAuthDataPayload{
				SupiOrSuci: supi,
				Success:    false,
				Reason:     err.Error(),
			},
		})
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
		case strings.Contains(err.Error(), "MAC-S verification failed"):
			sbi.WriteProblem(w, &sbi.ProblemDetails{
				Status: http.StatusForbidden,
				Title:  "Forbidden",
				Detail: err.Error(),
				Cause:  "MAC_S_FAILURE",
			})
		default:
			s.log.WithError(err).WithField("supi", supi).Error("udm: generate auth vector failed")
			sbi.WriteProblem(w, sbi.InternalError("auth vector generation failed"))
		}
		return
	}

	s.emitter.Emit(events.Event{
		JourneyID: journeyID,
		NF:        "udm",
		Category:  events.SignalingTx,
		Severity:  events.SeverityInfo,
		Protocol:  "sbi",
		Message:   "Authentication vector generated",
		Payload: events.UDMAuthDataPayload{
			SupiOrSuci: supi,
			SUPI:       supi,
			Success:    true,
		},
	})

	resp := AuthenticationInfoResult{
		AuthType:             AuthType5GAka,
		AuthenticationVector: av,
		Supi:                 supi,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
