package ausf

import (
	"context"
	"errors"
	"net/http"

	"github.com/qcore-project/qcore/pkg/sbi"
)

// Client is the AMF-facing consumer for Nausf_UEAuthentication.
// Mirrors pkg/udm.Client in shape.
type Client struct {
	sbi *sbi.Client
}

// NewClient returns an AUSF client pointed at baseURL.
func NewClient(baseURL, callerNFType string, insecureSkipVerify bool) *Client {
	return &Client{sbi: sbi.NewClient(baseURL, callerNFType, insecureSkipVerify)}
}

// Typed errors returned by Client methods.
var (
	ErrAUSFNotFound = errors.New("ausf: subscriber not found")
	ErrAUSFBadInput = errors.New("ausf: bad request")
)

// CreateUEAuth — POST /nausf-auth/v1/ue-authentications.
// AMF calls this at the start of authentication to get RAND, AUTN, HXRES*.
// Returns the full UEAuthenticationCtx including the Location URL for the
// 5g-aka-confirmation leg.
func (c *Client) CreateUEAuth(ctx context.Context, req *AuthenticationInfo) (*UEAuthenticationCtx, string, error) {
	var out UEAuthenticationCtx
	err := c.sbi.DoJSON(ctx, http.MethodPost, "/nausf-auth/v1/ue-authentications", req, &out)
	if err != nil {
		return nil, "", mapAUSFProblem(err)
	}
	// Location is embedded in the _links["5g-aka"].href field.
	link, ok := out.Links["5g-aka"]
	if !ok || link.Href == "" {
		return nil, "", errors.New("ausf: missing _links[5g-aka] in response")
	}
	return &out, link.Href, nil
}

// ConfirmAuth — PUT {authCtxURL}/5g-aka-confirmation.
// AMF calls this after receiving RES* from the UE. On success, returns KSEAF
// (hex-encoded 256-bit key) and the SUPI.
func (c *Client) ConfirmAuth(ctx context.Context, confirmURL string, resStar string) (*ConfirmationDataResponse, error) {
	body := &ConfirmationData{ResStar: resStar}
	var out ConfirmationDataResponse
	err := c.sbi.DoJSON(ctx, http.MethodPut, confirmURL, body, &out)
	if err != nil {
		return nil, mapAUSFProblem(err)
	}
	return &out, nil
}

func mapAUSFProblem(err error) error {
	var pd *sbi.ProblemDetails
	if errors.As(err, &pd) {
		switch pd.Status {
		case http.StatusNotFound:
			return ErrAUSFNotFound
		case http.StatusBadRequest:
			return ErrAUSFBadInput
		}
	}
	return err
}
