package mme

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/qcore-project/qcore/pkg/logger"
)

// AuthVectorResponse mirrors the HSS API response for auth vector generation.
type AuthVectorResponse struct {
	RAND  string `json:"rand"`
	XRES  string `json:"xres"`
	AUTN  string `json:"autn"`
	KASME string `json:"kasme"`
}

// S6aClient communicates with the HSS via REST API.
// This simulates the S6a/Diameter interface — same logical operation
// (Authentication-Information-Request/Answer), different transport.
// When Diameter is added later, this interface stays the same.
type S6aClient struct {
	hssURL     string
	httpClient *http.Client
	log        logger.Logger
}

// NewS6aClient creates a new client for the HSS REST API.
func NewS6aClient(hssURL string, log logger.Logger) *S6aClient {
	return &S6aClient{
		hssURL: hssURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		log: log.WithField("component", "s6a-client"),
	}
}

// AuthenticationInformationRequest fetches an authentication vector from the HSS
// for the given IMSI. This is the S6a AIR/AIA equivalent.
func (c *S6aClient) AuthenticationInformationRequest(imsi string) (*AuthVectorResponse, error) {
	url := fmt.Sprintf("%s/api/v1/subscribers/%s/auth-vector", c.hssURL, imsi)

	c.log.Debugf("AIR for IMSI=%s -> %s", imsi, url)

	resp, err := c.httpClient.Post(url, "application/json", nil)
	if err != nil {
		return nil, fmt.Errorf("HSS unreachable at %s: %w (is qcore-hss running?)", c.hssURL, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading HSS response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("IMSI %s not found in HSS — add it with: qcore-hss subscriber add --imsi %s --ki <key> --opc <opc>", imsi, imsi)
		}
		return nil, fmt.Errorf("HSS returned %d: %s", resp.StatusCode, string(body))
	}

	// The HSS wraps the response in {"data": {...}}
	var wrapper struct {
		Data AuthVectorResponse `json:"data"`
	}
	if err := json.Unmarshal(body, &wrapper); err != nil {
		return nil, fmt.Errorf("decoding HSS response: %w", err)
	}

	c.log.Debugf("AIA for IMSI=%s: RAND=%s...", imsi, wrapper.Data.RAND[:8])
	return &wrapper.Data, nil
}

// HealthCheck verifies the HSS is reachable.
func (c *S6aClient) HealthCheck() error {
	url := fmt.Sprintf("%s/api/v1/health", c.hssURL)
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return fmt.Errorf("HSS unreachable at %s: %w", c.hssURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HSS unhealthy (status %d)", resp.StatusCode)
	}
	return nil
}
