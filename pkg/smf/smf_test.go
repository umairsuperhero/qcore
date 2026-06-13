package smf

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/qcore-project/qcore/pkg/logger"
)

func TestHealthRoute(t *testing.T) {
	svc, err := NewService(Config{
		IPPoolCIDR:  "10.45.0.0/16",
		UPFPfcpAddr: "127.0.0.1:8805",
		LocalPfcpIP: "127.0.0.1",
	}, logger.New("error", "console"))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	svc.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%q", rec.Code, http.StatusOK, rec.Body.String())
	}
	if rec.Body.String() != `{"status":"ok"}` {
		t.Fatalf("body = %q, want health JSON", rec.Body.String())
	}
}
