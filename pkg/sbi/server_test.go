package sbi

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/qcore-project/qcore/pkg/logger"
)

// TestServer_H2C_RoundTrip runs Server in h2c mode against a real TCP
// listener, then uses the matching h2c Client to issue a JSON request and
// verify the full middleware chain (request ID propagation, access log,
// recover) is wired correctly.
func TestServer_H2C_RoundTrip(t *testing.T) {
	log := logger.New("error", "text")

	mux := http.NewServeMux()
	mux.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"method":     r.Method,
			"proto":      r.Proto,
			"request_id": RequestIDFromContext(r.Context()),
			"nf_type":    r.Header.Get("X-Qcore-NFType"),
		})
	})
	mux.HandleFunc("/boom", func(w http.ResponseWriter, r *http.Request) {
		panic("intentional")
	})

	port := pickFreePort(t)
	srv := NewServer(ServerConfig{
		BindAddress: "127.0.0.1",
		Port:        port,
		NFType:      "TEST",
	}, log, mux)

	go func() {
		_ = srv.Serve()
	}()
	// Give the listener a moment to bind before dialing.
	time.Sleep(50 * time.Millisecond)

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})

	baseURL := "http://127.0.0.1:" + strconv.Itoa(port)
	client := NewClient(baseURL, "CALLER", false)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Round-trip happy path.
	var resp map[string]string
	if err := client.DoJSON(ctx, "GET", "/echo", nil, &resp); err != nil {
		t.Fatalf("DoJSON /echo: %v", err)
	}
	if !strings.HasPrefix(resp["proto"], "HTTP/2") {
		t.Errorf("expected HTTP/2 proto, got %q", resp["proto"])
	}
	if resp["nf_type"] != "CALLER" {
		t.Errorf("expected X-Qcore-NFType=CALLER echoed back, got %q", resp["nf_type"])
	}
	if len(resp["request_id"]) != 32 {
		t.Errorf("expected 32-char request id echoed back, got %q", resp["request_id"])
	}

	// Panic → 500 problem+json via Recover middleware.
	err := client.DoJSON(ctx, "GET", "/boom", nil, nil)
	pd, ok := err.(*ProblemDetails)
	if !ok {
		t.Fatalf("expected *ProblemDetails error, got %T: %v", err, err)
	}
	if pd.Status != http.StatusInternalServerError {
		t.Errorf("panic should surface as 500, got %d", pd.Status)
	}
}

// pickFreePort asks the kernel for an ephemeral port, then closes the
// listener so the caller can bind to the same port. Classic TOCTOU race,
// but fine for test fixtures on loopback.
func pickFreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("pick port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port
}
