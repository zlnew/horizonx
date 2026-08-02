// Package metrics exposes Prometheus metrics for the HorizonX control plane.
// P2-14: observability — /metrics endpoint with HTTP request counters and
// job queue gauges.
package metrics

import (
	"context"
	"net/http"

	"horizonx/internal/domain"
	"horizonx/internal/logger"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Registry holds the Prometheus collectors and exposes the /metrics handler.
type Registry struct {
	registry *prometheus.Registry

	httpRequests *prometheus.CounterVec
	httpDuration *prometheus.HistogramVec

	jobPending   prometheus.Gauge
	jobRunning   prometheus.Gauge
	jobSucceeded prometheus.Gauge
	jobFailed    prometheus.Gauge
	jobTotal     prometheus.Gauge
	serverOnline prometheus.Gauge

	jobRepo    domain.JobRepository
	serverRepo domain.ServerRepository
	log        logger.Logger
}

func NewRegistry(
	jobRepo domain.JobRepository,
	serverRepo domain.ServerRepository,
	log logger.Logger,
) *Registry {
	reg := prometheus.NewRegistry()

	httpRequests := promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
		Name: "horizonx_http_requests_total",
		Help: "Total HTTP requests by method, path and status.",
	}, []string{"method", "path", "status"})

	httpDuration := promauto.With(reg).NewHistogramVec(prometheus.HistogramOpts{
		Name:    "horizonx_http_request_duration_seconds",
		Help:    "HTTP request duration in seconds by method and path.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path"})

	return &Registry{
		registry:      reg,
		httpRequests:  httpRequests,
		httpDuration:  httpDuration,
		jobPending:    promauto.With(reg).NewGauge(prometheus.GaugeOpts{Name: "horizonx_jobs_pending", Help: "Jobs waiting to be picked up."}),
		jobRunning:    promauto.With(reg).NewGauge(prometheus.GaugeOpts{Name: "horizonx_jobs_running", Help: "Jobs currently executing."}),
		jobSucceeded:  promauto.With(reg).NewGauge(prometheus.GaugeOpts{Name: "horizonx_jobs_succeeded", Help: "Jobs that finished successfully."}),
		jobFailed:     promauto.With(reg).NewGauge(prometheus.GaugeOpts{Name: "horizonx_jobs_failed", Help: "Jobs that failed."}),
		jobTotal:      promauto.With(reg).NewGauge(prometheus.GaugeOpts{Name: "horizonx_jobs_total", Help: "Total jobs in the system."}),
		serverOnline:  promauto.With(reg).NewGauge(prometheus.GaugeOpts{Name: "horizonx_servers_online", Help: "Number of online agent servers."}),
		jobRepo:       jobRepo,
		serverRepo:    serverRepo,
		log:           log,
	}
}

// ObserveRequest records an HTTP request against the counters.
func (r *Registry) ObserveRequest(method, path, status string, durationSeconds float64) {
	r.httpRequests.WithLabelValues(method, path, status).Inc()
	r.httpDuration.WithLabelValues(method, path).Observe(durationSeconds)
}

// Refresh collects the current queue/server gauges from the repos.
// Called before every scrape so the endpoint always shows live state.
func (r *Registry) Refresh() {
	ctx := context.Background()

	if counts, err := r.jobRepo.CountsByStatus(ctx); err == nil {
		r.jobPending.Set(float64(counts.Queued))
		r.jobRunning.Set(float64(counts.Running))
		r.jobSucceeded.Set(float64(counts.Success))
		r.jobFailed.Set(float64(counts.Failed))
		r.jobTotal.Set(float64(counts.Total))
	} else {
		r.log.Error("metrics: job counts refresh failed", "error", err)
	}

	if online, err := r.serverRepo.CountOnline(ctx); err == nil {
		r.serverOnline.Set(float64(online))
	} else {
		r.log.Error("metrics: server count refresh failed", "error", err)
	}
}

// Handler returns the Prometheus text-format handler.
func (r *Registry) Handler() http.Handler {
	return promhttp.HandlerFor(r.registry, promhttp.HandlerOpts{})
}
