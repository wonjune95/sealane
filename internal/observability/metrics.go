// Package observability provides Prometheus metrics for Sealane.
package observability

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Labels are intentionally low-cardinality (method + status, no raw path) to avoid
// metric explosion once P2 adds variable path segments ({ns}/{name}).
var (
	httpRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total HTTP requests by method and status.",
	}, []string{"method", "status"})

	httpDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request duration by method.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method"})
)

func MetricsHandler() http.Handler { return promhttp.Handler() }

// Instrument records request count and duration for every request through next.
func Instrument(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		httpRequests.WithLabelValues(r.Method, strconv.Itoa(rec.status)).Inc()
		httpDuration.WithLabelValues(r.Method).Observe(time.Since(start).Seconds())
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}
