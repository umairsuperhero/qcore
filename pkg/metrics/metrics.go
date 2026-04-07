package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	registry   *prometheus.Registry
	counters   map[string]*prometheus.CounterVec
	histograms map[string]*prometheus.HistogramVec
	gauges     map[string]*prometheus.GaugeVec
}

func New() *Metrics {
	return &Metrics{
		registry:   prometheus.NewRegistry(),
		counters:   make(map[string]*prometheus.CounterVec),
		histograms: make(map[string]*prometheus.HistogramVec),
		gauges:     make(map[string]*prometheus.GaugeVec),
	}
}

func (m *Metrics) NewCounter(name, help string, labels []string) *prometheus.CounterVec {
	c := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: name,
		Help: help,
	}, labels)
	m.registry.MustRegister(c)
	m.counters[name] = c
	return c
}

func (m *Metrics) NewHistogram(name, help string, labels []string, buckets []float64) *prometheus.HistogramVec {
	if buckets == nil {
		buckets = prometheus.DefBuckets
	}
	h := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    name,
		Help:    help,
		Buckets: buckets,
	}, labels)
	m.registry.MustRegister(h)
	m.histograms[name] = h
	return h
}

func (m *Metrics) NewGauge(name, help string, labels []string) *prometheus.GaugeVec {
	g := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: name,
		Help: help,
	}, labels)
	m.registry.MustRegister(g)
	m.gauges[name] = g
	return g
}

func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

func (m *Metrics) Counter(name string) *prometheus.CounterVec {
	return m.counters[name]
}

func (m *Metrics) Histogram(name string) *prometheus.HistogramVec {
	return m.histograms[name]
}

func (m *Metrics) Gauge(name string) *prometheus.GaugeVec {
	return m.gauges[name]
}

type HSSMetrics struct {
	APIRequests     *prometheus.CounterVec
	APILatency      *prometheus.HistogramVec
	SubscriberTotal *prometheus.GaugeVec
	AuthVectors     *prometheus.CounterVec
}

func RegisterHSSMetrics(m *Metrics) *HSSMetrics {
	return &HSSMetrics{
		APIRequests: m.NewCounter(
			"hss_api_requests_total",
			"Total number of HSS API requests",
			[]string{"method", "path", "status"},
		),
		APILatency: m.NewHistogram(
			"hss_api_request_duration_seconds",
			"HSS API request duration in seconds",
			[]string{"method", "path"},
			[]float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0},
		),
		SubscriberTotal: m.NewGauge(
			"hss_subscribers_total",
			"Total number of subscribers",
			[]string{},
		),
		AuthVectors: m.NewCounter(
			"hss_auth_vectors_generated_total",
			"Total number of authentication vectors generated",
			[]string{},
		),
	}
}
