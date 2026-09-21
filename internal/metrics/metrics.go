// Package metrics implements the lightweight /metrics endpoint
// docs/PHASE_PILOT_RELEASE.md §6 calls for -- a Prometheus text-format
// exposition on the API itself, not a separate always-on collector/
// dashboard stack (explicitly out of scope, see that doc's §8). It covers
// the subset of docs/ARCHITECTURE.md §18's "initial metrics" list that
// actually corresponds to something this codebase does: request/error
// latency by route, database pool usage, login failures, and open review
// counts by type. "push/pull batch success and duration" and "pending
// event age reported by active devices" are deliberately NOT here --
// those describe Phase 4's generic multi-device sync protocol, which was
// never built (see CLAUDE.md's "Phase 4, revisited" section); faking a
// metric for a mechanism that doesn't exist would be worse than omitting
// it.
package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Registry owns a private prometheus.Registry (never the global
// DefaultRegisterer) so this package's metrics are fully self-contained --
// nothing else in the process can accidentally register onto or read from
// it, and tests can construct their own throwaway instance freely.
type Registry struct {
	registry *prometheus.Registry

	httpRequestsTotal   *prometheus.CounterVec
	httpRequestDuration *prometheus.HistogramVec
	loginFailuresTotal  prometheus.Counter
}

func New() *Registry {
	r := &Registry{
		registry: prometheus.NewRegistry(),
		httpRequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "jbm_http_requests_total",
			Help: "Total HTTP requests, by route pattern, method, and status code.",
		}, []string{"route", "method", "status"}),
		httpRequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "jbm_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds, by route pattern and method.",
			Buckets: prometheus.DefBuckets,
		}, []string{"route", "method"}),
		loginFailuresTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "jbm_login_failures_total",
			Help: "Total failed business-user login attempts (wrong credentials, unknown identifier).",
		}),
	}
	r.registry.MustRegister(r.httpRequestsTotal, r.httpRequestDuration, r.loginFailuresTotal)
	return r
}

// ObserveHTTPRequest records one completed request. Called from
// accessLogMiddleware, the same place that already measures status and
// duration for logging -- see internal/httpapi/middleware.go.
func (r *Registry) ObserveHTTPRequest(route, method string, status int, duration time.Duration) {
	r.httpRequestsTotal.WithLabelValues(route, method, strconv.Itoa(status)).Inc()
	r.httpRequestDuration.WithLabelValues(route, method).Observe(duration.Seconds())
}

// IncrementLoginFailure is called from POST /auth/login on any failed
// attempt (unknown identifier or wrong password -- internal/httpapi
// deliberately doesn't distinguish the two in its response, and this
// metric doesn't either).
func (r *Registry) IncrementLoginFailure() {
	r.loginFailuresTotal.Inc()
}

// MustRegister adds an additional prometheus.Collector (DBPoolCollector,
// ReviewsCollector) to this registry. Panics on a duplicate registration,
// matching prometheus.Registry.MustRegister's own contract -- acceptable
// here since every call site is at startup, not on a live request path.
func (r *Registry) MustRegister(collectors ...prometheus.Collector) {
	r.registry.MustRegister(collectors...)
}

// Handler returns the http.Handler for GET /metrics.
func (r *Registry) Handler() http.Handler {
	return promhttp.HandlerFor(r.registry, promhttp.HandlerOpts{})
}
