package udr

import (
	"context"
	"errors"
	"net/http"

	"github.com/qcore-project/qcore/pkg/sbi"
	"github.com/qcore-project/qcore/pkg/sbi/common"
)

// Client is the Nudr_DataRepository consumer side — paired with Service.
// It wraps pkg/sbi.Client with one method per UDR endpoint and lifts the
// spec's Problem shapes into typed sentinel errors so callers don't parse
// HTTP status codes themselves.
type Client struct {
	sbi *sbi.Client
}

// Typed errors from the UDR wire. Callers use errors.Is to branch.
var (
	// ErrNotFound — UDR returned 404 / DATA_NOT_FOUND for the resource.
	ErrNotFound = errors.New("udr: data not found")
	// ErrBadUeID — UDR rejected the ueId as malformed (400).
	ErrBadUeID = errors.New("udr: bad ueId")
)

// NewClient returns a UDR client pointed at baseURL (e.g. "http://udr:8080").
// callerNFType is stamped on outbound requests as X-Qcore-NFType for access
// logs on the UDR side. insecureSkipVerify is dev-only for h2c/self-signed
// TLS setups.
func NewClient(baseURL, callerNFType string, insecureSkipVerify bool) *Client {
	return &Client{sbi: sbi.NewClient(baseURL, callerNFType, insecureSkipVerify)}
}

// GetAuthenticationSubscription — TS 29.505 §5.2.2.3.3. Fetches Milenage
// credentials (K, OPc, AMF, SQN) for ueId in AuthenticationSubscription
// shape, so a UDM UEAU backend can run 5G-AKA without touching the raw
// subscriber store.
//
// Path has no servingPlmnId segment (creds are PLMN-independent).
func (c *Client) GetAuthenticationSubscription(ctx context.Context, ueID string) (*common.AuthenticationSubscription, error) {
	path := "/nudr-dr/v2/subscription-data/" + ueID + "/authentication-data/authentication-subscription"
	var resp common.AuthenticationSubscription
	if err := c.sbi.DoJSON(ctx, "GET", path, nil, &resp); err != nil {
		var pd *sbi.ProblemDetails
		if errors.As(err, &pd) {
			switch pd.Status {
			case http.StatusNotFound:
				return nil, ErrNotFound
			case http.StatusBadRequest:
				return nil, ErrBadUeID
			}
		}
		return nil, err
	}
	return &resp, nil
}

// UpdateAuthSubscriptionSQN — TS 29.505 §5.2.2.3.4. Persists an advanced
// SQN for ueId by sending an RFC 6902 JSON Patch with a single replace
// op on /sequenceNumber/sqn. Used by a UDR-backed UDM UEAU after it has
// generated an auth vector and incremented the counter locally.
func (c *Client) UpdateAuthSubscriptionSQN(ctx context.Context, ueID, newSQN string) error {
	path := "/nudr-dr/v2/subscription-data/" + ueID + "/authentication-data/authentication-subscription"
	body := []map[string]any{
		{"op": "replace", "path": "/sequenceNumber/sqn", "value": newSQN},
	}
	if err := c.sbi.DoJSON(ctx, "PATCH", path, body, nil); err != nil {
		var pd *sbi.ProblemDetails
		if errors.As(err, &pd) {
			switch pd.Status {
			case http.StatusNotFound:
				return ErrNotFound
			case http.StatusBadRequest:
				return ErrBadUeID
			}
		}
		return err
	}
	return nil
}

// GetAmData — TS 29.504 §5.2.2.2.3. Fetches AccessAndMobilitySubscriptionData
// for ueId ("imsi-<15 digits>") under servingPlmnID.
//
// servingPlmnID is required by the UDR URL shape even though QCore's UDR
// ignores it today — passing an empty string would yield a malformed URL,
// so callers must supply a placeholder like "00101" until PLMN routing
// is wired end-to-end.
func (c *Client) GetAmData(ctx context.Context, ueID, servingPlmnID string) (*common.AccessAndMobilitySubscriptionData, error) {
	path := "/nudr-dr/v2/subscription-data/" + ueID + "/" + servingPlmnID + "/provisioned-data/am-data"
	var resp common.AccessAndMobilitySubscriptionData
	if err := c.sbi.DoJSON(ctx, "GET", path, nil, &resp); err != nil {
		var pd *sbi.ProblemDetails
		if errors.As(err, &pd) {
			switch pd.Status {
			case http.StatusNotFound:
				return nil, ErrNotFound
			case http.StatusBadRequest:
				return nil, ErrBadUeID
			}
		}
		return nil, err
	}
	return &resp, nil
}
