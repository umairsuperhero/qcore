package mme

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/qcore-project/qcore/pkg/logger"
)

// S11CreateSessionRequest mirrors spgw.CreateSessionRequest. We duplicate the
// wire types here rather than importing pkg/spgw to avoid a cycle and keep
// the MME buildable without a live SPGW dependency.
type S11CreateSessionRequest struct {
	IMSI string `json:"imsi"`
	APN  string `json:"apn,omitempty"`
	EBI  uint8  `json:"ebi,omitempty"`
	PLMN string `json:"plmn,omitempty"`
}

// S11CreateSessionResponse is what SPGW returns.
type S11CreateSessionResponse struct {
	UEIP    string `json:"ue_ip"`
	SGWTEID uint32 `json:"sgw_teid"`
	SGWAddr string `json:"sgw_addr"`
	EBI     uint8  `json:"ebi"`
	APN     string `json:"apn"`
}

// S11ModifyBearerRequest carries the eNB's uplink TEID back to the SPGW.
type S11ModifyBearerRequest struct {
	ENBTEID uint32 `json:"enb_teid"`
	ENBAddr string `json:"enb_addr"`
}

// S11Client calls the SPGW HTTP API. This is QCore's HTTP-over-JSON take on
// the S11 interface — see pkg/spgw/api.go for the trade-off rationale.
type S11Client struct {
	baseURL    string
	httpClient *http.Client
	log        logger.Logger
}

// NewS11Client creates an S11 HTTP client pointed at the given SPGW base URL.
// If baseURL is empty the returned client's Enabled() method returns false so
// the MME can fall back to its internal fake-IP allocator.
func NewS11Client(baseURL string, log logger.Logger) *S11Client {
	return &S11Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 5 * time.Second},
		log:        log.WithField("component", "s11-client"),
	}
}

// Enabled reports whether an SPGW URL is configured.
func (c *S11Client) Enabled() bool { return c.baseURL != "" }

// HealthCheck pings the SPGW /api/v1/health endpoint.
func (c *S11Client) HealthCheck() error {
	if !c.Enabled() {
		return fmt.Errorf("no SPGW URL configured")
	}
	resp, err := c.httpClient.Get(c.baseURL + "/api/v1/health")
	if err != nil {
		return fmt.Errorf("SPGW unreachable at %s: %w", c.baseURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("SPGW unhealthy (status %d)", resp.StatusCode)
	}
	return nil
}

// CreateSession invokes POST /api/v1/sessions on the SPGW.
func (c *S11Client) CreateSession(req *S11CreateSessionRequest) (*S11CreateSessionResponse, error) {
	if !c.Enabled() {
		return nil, fmt.Errorf("S11 client disabled (no SPGW URL configured)")
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}
	httpResp, err := c.httpClient.Post(c.baseURL+"/api/v1/sessions", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("POST /sessions: %w", err)
	}
	defer httpResp.Body.Close()
	respBody, _ := io.ReadAll(httpResp.Body)
	if httpResp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("SPGW returned %d: %s", httpResp.StatusCode, string(respBody))
	}
	var resp S11CreateSessionResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("decoding SPGW response: %w", err)
	}
	c.log.Debugf("created session IMSI=%s UE-IP=%s SGW-TEID=0x%x", req.IMSI, resp.UEIP, resp.SGWTEID)
	return &resp, nil
}

// ModifyBearer invokes POST /api/v1/sessions/{imsi}/modify on the SPGW.
func (c *S11Client) ModifyBearer(imsi string, req *S11ModifyBearerRequest) error {
	if !c.Enabled() {
		return fmt.Errorf("S11 client disabled")
	}
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}
	url := fmt.Sprintf("%s/api/v1/sessions/%s/modify", c.baseURL, imsi)
	httpResp, err := c.httpClient.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("POST %s: %w", url, err)
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(httpResp.Body)
		return fmt.Errorf("SPGW returned %d: %s", httpResp.StatusCode, string(respBody))
	}
	return nil
}

// DeleteSession invokes DELETE /api/v1/sessions/{imsi}.
func (c *S11Client) DeleteSession(imsi string) error {
	if !c.Enabled() {
		return nil
	}
	url := fmt.Sprintf("%s/api/v1/sessions/%s", c.baseURL, imsi)
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("DELETE %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("SPGW returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
}
