package nrf

import (
	"context"
	"fmt"

	"github.com/qcore-project/qcore/pkg/sbi"
)

// HTTPClient is the network-facing NRF client — wraps pkg/sbi.Client to talk
// to an NRF service (QCore's own pkg/nrf, planned for v0.6, or any
// spec-compliant NRF).
//
// Paths follow 3GPP TS 29.510 §5:
//   - Nnrf_NFManagement: PUT/DELETE/PATCH on /nnrf-nfm/v1/nf-instances/{id}
//   - Nnrf_NFDiscovery:  GET on /nnrf-disc/v1/nf-instances
type HTTPClient struct {
	mgmt *sbi.Client
	disc *sbi.Client
}

// NewHTTPClient returns an NRF client that talks to the given base URL
// (e.g. "http://nrf.example:8000"). The same base URL serves both
// Nnrf_NFManagement and Nnrf_NFDiscovery by convention — the API-version
// prefix disambiguates.
func NewHTTPClient(baseURL, callerNFType string, insecureSkipVerify bool) *HTTPClient {
	return &HTTPClient{
		mgmt: sbi.NewClient(baseURL, callerNFType, insecureSkipVerify),
		disc: sbi.NewClient(baseURL, callerNFType, insecureSkipVerify),
	}
}

func (c *HTTPClient) Register(ctx context.Context, p *NFProfile) error {
	if p == nil || p.NFInstanceID == "" {
		return fmt.Errorf("nrf: register requires NFInstanceID")
	}
	path := fmt.Sprintf("/nnrf-nfm/v1/nf-instances/%s", p.NFInstanceID)
	return c.mgmt.DoJSON(ctx, "PUT", path, p, nil)
}

func (c *HTTPClient) Deregister(ctx context.Context, nfInstanceID string) error {
	path := fmt.Sprintf("/nnrf-nfm/v1/nf-instances/%s", nfInstanceID)
	return c.mgmt.DoJSON(ctx, "DELETE", path, nil, nil)
}

func (c *HTTPClient) Heartbeat(ctx context.Context, nfInstanceID string) error {
	// Per TS 29.510 §5.2.2.3 heartbeats are PATCH of the nfStatus.
	path := fmt.Sprintf("/nnrf-nfm/v1/nf-instances/%s", nfInstanceID)
	body := []map[string]any{
		{"op": "replace", "path": "/nfStatus", "value": StatusRegistered},
	}
	return c.mgmt.DoJSON(ctx, "PATCH", path, body, nil)
}

// discoverResponse matches the minimal SearchResult schema — TS 29.510 §6.2.
type discoverResponse struct {
	ValidityPeriod int         `json:"validityPeriod"`
	NFInstances    []NFProfile `json:"nfInstances"`
}

func (c *HTTPClient) Discover(ctx context.Context, q DiscoveryQuery) ([]NFProfile, error) {
	if q.TargetNFType == "" || q.RequesterType == "" {
		return nil, fmt.Errorf("nrf: Discover requires TargetNFType and RequesterType")
	}
	path := fmt.Sprintf("/nnrf-disc/v1/nf-instances?target-nf-type=%s&requester-nf-type=%s",
		q.TargetNFType, q.RequesterType)
	if q.ServiceName != "" {
		path += "&service-names=" + q.ServiceName
	}
	if q.PLMN != "" {
		path += "&target-plmn-list=" + q.PLMN
	}
	var resp discoverResponse
	if err := c.disc.DoJSON(ctx, "GET", path, nil, &resp); err != nil {
		return nil, err
	}
	return resp.NFInstances, nil
}
