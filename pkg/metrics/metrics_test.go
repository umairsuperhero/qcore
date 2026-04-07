package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	m := New()
	require.NotNil(t, m)
	require.NotNil(t, m.registry)
}

func TestMetrics_Counter(t *testing.T) {
	m := New()
	c := m.NewCounter("test_counter", "A test counter", []string{"label"})
	require.NotNil(t, c)

	c.WithLabelValues("value1").Inc()
	c.WithLabelValues("value1").Inc()

	// Verify via /metrics endpoint
	handler := m.Handler()
	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	body, _ := io.ReadAll(w.Result().Body)
	assert.Contains(t, string(body), "test_counter")
	assert.Contains(t, string(body), `label="value1"`)
}

func TestMetrics_Histogram(t *testing.T) {
	m := New()
	h := m.NewHistogram("test_histogram", "A test histogram", []string{}, nil)
	require.NotNil(t, h)

	h.WithLabelValues().Observe(0.5)

	handler := m.Handler()
	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	body, _ := io.ReadAll(w.Result().Body)
	assert.Contains(t, string(body), "test_histogram")
}

func TestMetrics_Gauge(t *testing.T) {
	m := New()
	g := m.NewGauge("test_gauge", "A test gauge", []string{})
	require.NotNil(t, g)

	g.WithLabelValues().Set(42)

	handler := m.Handler()
	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	body, _ := io.ReadAll(w.Result().Body)
	assert.Contains(t, string(body), "test_gauge")
	assert.Contains(t, string(body), "42")
}

func TestRegisterHSSMetrics(t *testing.T) {
	m := New()
	hm := RegisterHSSMetrics(m)
	require.NotNil(t, hm)
	require.NotNil(t, hm.APIRequests)
	require.NotNil(t, hm.APILatency)
	require.NotNil(t, hm.SubscriberTotal)
	require.NotNil(t, hm.AuthVectors)

	// Increment and verify
	hm.APIRequests.WithLabelValues("GET", "/subscribers", "200").Inc()
	hm.AuthVectors.WithLabelValues().Inc()

	handler := m.Handler()
	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	content := string(body)
	assert.True(t, strings.Contains(content, "hss_api_requests_total"))
	assert.True(t, strings.Contains(content, "hss_auth_vectors_generated_total"))
}
