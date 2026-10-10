package app

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	registry *prometheus.Registry
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
	inFlight prometheus.Gauge
}

func NewMetrics(server *Server) *Metrics {
	registry := prometheus.NewRegistry()
	m := &Metrics{
		registry: registry,
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "sameframe_http_requests_total", Help: "HTTP requests by method, route, and status class."}, []string{"method", "route", "status_class"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "sameframe_http_request_duration_seconds", Help: "HTTP request latency by route.", Buckets: prometheus.DefBuckets}, []string{"method", "route"}),
		inFlight: prometheus.NewGauge(prometheus.GaugeOpts{Name: "sameframe_http_requests_in_flight", Help: "Currently executing HTTP requests."}),
	}
	registry.MustRegister(prometheus.NewGoCollector(), prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}), m.requests, m.duration, m.inFlight)
	registry.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "sameframe_websocket_clients", Help: "Connected room WebSocket clients."}, func() float64 { _, clients := server.hub.Counts(); return float64(clients) }))
	registry.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "sameframe_websocket_rooms", Help: "Rooms with connected WebSocket clients."}, func() float64 { rooms, _ := server.hub.Counts(); return float64(rooms) }))
	registry.MustRegister(newBusinessCollector(server.repo))
	return m
}

type businessCollector struct {
	repo         Repository
	descriptions map[string]*prometheus.Desc
}

func newBusinessCollector(repo Repository) *businessCollector {
	names := map[string]string{"active_rooms": "Active rooms.", "pending_reports": "Pending abuse reports.", "pending_deletions": "Pending account deletion requests.", "open_copyright_complaints": "Open copyright complaints.", "overdue_copyright_complaints": "Copyright complaints beyond their response deadline.", "activated_orders": "Activated payment orders.", "revenue_minor": "Activated order revenue in minor currency units."}
	descs := map[string]*prometheus.Desc{}
	for name, help := range names {
		descs[name] = prometheus.NewDesc("sameframe_"+name, help, nil, nil)
	}
	descs["scrape_success"] = prometheus.NewDesc("sameframe_business_metrics_scrape_success", "Whether the business aggregate query succeeded.", nil, nil)
	return &businessCollector{repo: repo, descriptions: descs}
}
func (c *businessCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, desc := range c.descriptions {
		ch <- desc
	}
}
func (c *businessCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	value, err := c.repo.AdminDashboard(ctx)
	success := 1.0
	if err != nil {
		success = 0
	} else {
		values := map[string]int64{"active_rooms": value.ActiveRooms, "pending_reports": value.PendingReports, "pending_deletions": value.PendingDeletions, "open_copyright_complaints": value.OpenCopyrightComplaints, "overdue_copyright_complaints": value.OverdueCopyrightComplaints, "activated_orders": value.ActivatedOrders, "revenue_minor": value.RevenueMinor}
		for name, metricValue := range values {
			ch <- prometheus.MustNewConstMetric(c.descriptions[name], prometheus.GaugeValue, float64(metricValue))
		}
	}
	ch <- prometheus.MustNewConstMetric(c.descriptions["scrape_success"], prometheus.GaugeValue, success)
}

func (m *Metrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}
		m.inFlight.Inc()
		defer m.inFlight.Dec()
		started := time.Now()
		recorder := &metricResponseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		route := r.Pattern
		if route == "" {
			route = "unmatched"
		}
		class := strconv.Itoa(recorder.status/100) + "xx"
		m.requests.WithLabelValues(r.Method, route, class).Inc()
		m.duration.WithLabelValues(r.Method, route).Observe(time.Since(started).Seconds())
	})
}

func (m *Metrics) Handler(token string) http.Handler {
	handler := promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{EnableOpenMetrics: true})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token != "" {
			provided := r.Header.Get("Authorization")
			expected := "Bearer " + token
			if len(provided) != len(expected) || subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
				w.Header().Set("WWW-Authenticate", "Bearer")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		handler.ServeHTTP(w, r)
	})
}

type metricResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *metricResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func (w *metricResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *metricResponseWriter) Status() int                 { return w.status }
