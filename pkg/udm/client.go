package udm

import (
	"context"
	"errors"
	"net/http"

	"github.com/qcore-project/qcore/pkg/sbi"
	"github.com/qcore-project/qcore/pkg/sbi/common"
)

// Client is the consumer counterpart to Service — used by other NFs (most
// notably AUSF) to call this UDM over Nudm_SBI. Mirrors pkg/udr.Client in
// shape: one method per endpoint, typed errors from HTTP status codes.
type Client struct {
	sbi *sbi.Client
}

// NewClient returns a UDM client pointed at baseURL. callerNFType is
// stamped on outbound requests as X-Qcore-NFType for access-log
// attribution on the UDM side.
func NewClient(baseURL, callerNFType string, insecureSkipVerify bool) *Client {
	return &Client{sbi: sbi.NewClient(baseURL, callerNFType, insecureSkipVerify)}
}

// GetAmData — Nudm_SDM §5.2.2.2.2. Fetches AccessAndMobilitySubscriptionData
// for a SUPI in "imsi-<15 digits>" form. Returns ErrNotFound on 404 and
// ErrBadSupi on 400 (reusing the server-side sentinels; semantics match).
func (c *Client) GetAmData(ctx context.Context, supi string) (*common.AccessAndMobilitySubscriptionData, error) {
	path := "/nudm-sdm/v2/" + supi + "/am-data"
	var resp common.AccessAndMobilitySubscriptionData
	if err := c.sbi.DoJSON(ctx, "GET", path, nil, &resp); err != nil {
		return nil, mapProblem(err)
	}
	return &resp, nil
}

// GenerateAuthData — Nudm_UEAU §5.3.2.2.2. AUSF's entry point: hand over
// a SUPI and serving-network name, get back an AuthenticationInfoResult
// with Av5gHeAka. Returns ErrNotFound on 404 (unknown SUPI) and
// ErrBadSupi on 400 (malformed SUPI or missing servingNetworkName).
func (c *Client) GenerateAuthData(ctx context.Context, supi string, req *AuthenticationInfoRequest) (*AuthenticationInfoResult, error) {
	path := "/nudm-ueau/v1/" + supi + "/security-information/generate-auth-data"
	var resp AuthenticationInfoResult
	if err := c.sbi.DoJSON(ctx, "POST", path, req, &resp); err != nil {
		return nil, mapProblem(err)
	}
	return &resp, nil
}

func mapProblem(err error) error {
	var pd *sbi.ProblemDetails
	if errors.As(err, &pd) {
		switch pd.Status {
		case http.StatusNotFound:
			return ErrNotFound
		case http.StatusBadRequest:
			return ErrBadSupi
		}
	}
	return err
}
