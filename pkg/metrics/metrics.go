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

type SPGWMetrics struct {
	UplinkPackets   *prometheus.CounterVec
	DownlinkPackets *prometheus.CounterVec
	UplinkBytes     *prometheus.CounterVec
	DownlinkBytes   *prometheus.CounterVec
	Drops           *prometheus.CounterVec
	EchoRequests    *prometheus.CounterVec
	SessionsCreated *prometheus.CounterVec
	SessionsDeleted *prometheus.CounterVec
	ActiveSessions  *prometheus.GaugeVec
	APIRequests     *prometheus.CounterVec
}

func RegisterSPGWMetrics(m *Metrics) *SPGWMetrics {
	return &SPGWMetrics{
		UplinkPackets: m.NewCounter(
			"spgw_uplink_packets_total",
			"Total decapsulated uplink GTP-U packets",
			[]string{},
		),
		DownlinkPackets: m.NewCounter(
			"spgw_downlink_packets_total",
			"Total encapsulated downlink GTP-U packets",
			[]string{},
		),
		UplinkBytes: m.NewCounter(
			"spgw_uplink_bytes_total",
			"Total uplink bytes (inner IP payload)",
			[]string{},
		),
		DownlinkBytes: m.NewCounter(
			"spgw_downlink_bytes_total",
			"Total downlink bytes (inner IP payload)",
			[]string{},
		),
		Drops: m.NewCounter(
			"spgw_drops_total",
			"Total dropped packets, labeled by cause",
			[]string{"cause"},
		),
		EchoRequests: m.NewCounter(
			"spgw_gtpu_echo_requests_total",
			"Total GTP-U Echo Requests received",
			[]string{},
		),
		SessionsCreated: m.NewCounter(
			"spgw_sessions_created_total",
			"Total sessions created via S11",
			[]string{},
		),
		SessionsDeleted: m.NewCounter(
			"spgw_sessions_deleted_total",
			"Total sessions deleted via S11",
			[]string{},
		),
		ActiveSessions: m.NewGauge(
			"spgw_active_sessions",
			"Number of active PDN sessions",
			[]string{},
		),
		APIRequests: m.NewCounter(
			"spgw_api_requests_total",
			"Total HTTP S11 requests",
			[]string{"method", "path", "status"},
		),
	}
}

type MMEMetrics struct {
	S1SetupRequests   *prometheus.CounterVec
	AttachRequests    *prometheus.CounterVec
	AttachSuccess     *prometheus.CounterVec
	AttachFailures    *prometheus.CounterVec
	AuthRequests      *prometheus.CounterVec
	ActiveUEs         *prometheus.GaugeVec
	ConnectedENBs     *prometheus.GaugeVec
	S1APLatency       *prometheus.HistogramVec
}

func RegisterMMEMetrics(m *Metrics) *MMEMetrics {
	return &MMEMetrics{
		S1SetupRequests: m.NewCounter(
			"mme_s1_setup_requests_total",
			"Total S1 Setup requests from eNodeBs",
			[]string{"result"},
		),
		AttachRequests: m.NewCounter(
			"mme_attach_requests_total",
			"Total attach requests from UEs",
			[]string{},
		),
		AttachSuccess: m.NewCounter(
			"mme_attach_success_total",
			"Total successful attaches",
			[]string{},
		),
		AttachFailures: m.NewCounter(
			"mme_attach_failures_total",
			"Total failed attaches",
			[]string{"reason"},
		),
		AuthRequests: m.NewCounter(
			"mme_auth_requests_total",
			"Total authentication requests sent to HSS",
			[]string{"result"},
		),
		ActiveUEs: m.NewGauge(
			"mme_active_ues",
			"Number of UEs in connected state",
			[]string{},
		),
		ConnectedENBs: m.NewGauge(
			"mme_connected_enbs",
			"Number of connected eNodeBs",
			[]string{},
		),
		S1APLatency: m.NewHistogram(
			"mme_s1ap_message_duration_seconds",
			"S1AP message processing duration",
			[]string{"procedure"},
			[]float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0},
		),
	}
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
