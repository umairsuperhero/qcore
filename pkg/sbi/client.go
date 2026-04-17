package sbi

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"golang.org/x/net/http2"
)

// Client is the outbound counterpart to Server — an HTTP/2 client with sane
// timeouts, RFC 7807 problem-body awareness, and request-ID propagation.
// Every NF uses this when calling another NF over SBI.
type Client struct {
	BaseURL string
	NFType  string // who we are, for X-Qcore-NFType header
	H       *http.Client
}

// NewClient returns a Client that speaks plaintext HTTP/2 (h2c) when baseURL
// uses http://, or H2 over TLS when https://. InsecureSkipVerify is dev-only.
//
// The h2c client-side pattern is: use http2.Transport, set AllowHTTP=true so
// the transport honours the http:// scheme, and override DialTLSContext with
// a plain TCP dialer so no TLS is attempted. This mirrors the server's
// h2c.NewHandler posture on the other side.
func NewClient(baseURL, nfType string, insecureSkipVerify bool) *Client {
	tr := &http2.Transport{
		AllowHTTP: true,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: insecureSkipVerify, //nolint:gosec // dev-only; callers opt in
			NextProtos:         []string{"h2"},
		},
		DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
			// Plain TCP; h2c framing sits directly on top.
			var d net.Dialer
			return d.DialContext(ctx, network, addr)
		},
	}

	return &Client{
		BaseURL: baseURL,
		NFType:  nfType,
		H: &http.Client{
			Transport: tr,
			Timeout:   30 * time.Second,
		},
	}
}

// DoJSON performs an HTTP request with a JSON body. `in` and `out` are
// optional: pass nil to skip marshaling/unmarshaling. On non-2xx we try to
// decode a ProblemDetails and return it as the error.
func (c *Client) DoJSON(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("marshal %T: %w", in, err)
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json, application/problem+json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if rid := RequestIDFromContext(ctx); rid != "" {
		req.Header.Set(HeaderRequestID, rid)
	}
	if c.NFType != "" {
		req.Header.Set("X-Qcore-NFType", c.NFType)
	}

	resp, err := c.H.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var pd ProblemDetails
		_ = json.NewDecoder(resp.Body).Decode(&pd) // best effort — may be empty body
		if pd.Status == 0 {
			pd.Status = resp.StatusCode
		}
		if pd.Title == "" {
			pd.Title = http.StatusText(resp.StatusCode)
		}
		return &pd
	}

	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil && err != io.EOF {
			return fmt.Errorf("decode %T: %w", out, err)
		}
	}
	return nil
}
