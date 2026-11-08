// Package metrics exposes Prometheus metrics for the HorizonX control plane.
// P2-14: observability — /metrics endpoint with HTTP request counters and
// job queue gauges.
package metrics

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"horizonx/internal/domain"
	"horizonx/internal/logger"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	dto "github.com/prometheus/client_model/go"
)

// RefreshInterval is how often the registry re-reads the queue/server
// gauges from the repos in the background. Scrapes serve the last
// computed values and never touch the database synchronously.
const RefreshInterval = 30 * time.Second

// Registry holds the Prometheus collectors and exposes the /metrics handler.
type Registry struct {
	registry *prometheus.Registry

	httpRequests *prometheus.CounterVec
	httpDuration *prometheus.HistogramVec

	agentDropped *prometheus.GaugeVec

	workerFailures *prometheus.CounterVec

	jobPending   prometheus.Gauge
	jobRunning   prometheus.Gauge
	jobSucceeded prometheus.Gauge
	jobFailed    prometheus.Gauge
	jobTotal     prometheus.Gauge
	serverOnline prometheus.Gauge

	jobRepo    domain.JobRepository
	serverRepo domain.ServerRepository
	log        logger.Logger

	// refreshing guards against overlapping refreshes: a tick that fires
	// while a slow refresh is still running is skipped.
	refreshing atomic.Bool
	// refreshed reports whether at least one refresh attempt completed.
	refreshed atomic.Bool

	stopLoop context.CancelFunc
	stopOnce sync.Once
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

	r := &Registry{
		registry:     reg,
		httpRequests: httpRequests,
		httpDuration: httpDuration,
		jobPending:   promauto.With(reg).NewGauge(prometheus.GaugeOpts{Name: "horizonx_jobs_pending", Help: "Jobs waiting to be picked up."}),
		jobRunning:   promauto.With(reg).NewGauge(prometheus.GaugeOpts{Name: "horizonx_jobs_running", Help: "Jobs currently executing."}),
		jobSucceeded: promauto.With(reg).NewGauge(prometheus.GaugeOpts{Name: "horizonx_jobs_succeeded", Help: "Jobs that finished successfully."}),
		jobFailed:    promauto.With(reg).NewGauge(prometheus.GaugeOpts{Name: "horizonx_jobs_failed", Help: "Jobs that failed."}),
		jobTotal:     promauto.With(reg).NewGauge(prometheus.GaugeOpts{Name: "horizonx_jobs_total", Help: "Total jobs in the system."}),
		serverOnline: promauto.With(reg).NewGauge(prometheus.GaugeOpts{Name: "horizonx_servers_online", Help: "Number of online agent servers."}),
		agentDropped: promauto.With(reg).NewGaugeVec(prometheus.GaugeOpts{
			Name: "horizonx_agent_dropped_messages",
			Help: "WebSocket messages dropped by the agent because its send buffer was full.",
		}, []string{"server_id"}),
		workerFailures: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Name: "horizonx_worker_failures_total",
			Help: "Worker runs that failed after exhausting retries, by worker name.",
		}, []string{"worker"}),
		jobRepo:    jobRepo,
		serverRepo: serverRepo,
		log:        log,
	}

	// Self-scheduling background refresh so no bootstrap/startup wiring is
	// needed. The first refresh happens after the first tick — a fresh
	// registry serves zero gauges until then and scrapes never block on
	// the database.
	ctx, cancel := context.WithCancel(context.Background())
	r.stopLoop = cancel
	go r.loop(ctx, RefreshInterval)
	return r
}

// SetAgentDroppedMessages sets the cumulative send-buffer drop count
// reported by one agent, labelled by its server ID.
func (r *Registry) SetAgentDroppedMessages(serverID string, v float64) {
	if r == nil || r.agentDropped == nil {
		return
	}
	r.agentDropped.WithLabelValues(serverID).Set(v)
}

// AgentDroppedMessagesValue returns the current gauge value for one
// server ID. Test helper; returns 0 when unset.
func (r *Registry) AgentDroppedMessagesValue(serverID string) float64 {
	if r == nil || r.agentDropped == nil {
		return 0
	}
	var m dto.Metric
	if err := r.agentDropped.WithLabelValues(serverID).Write(&m); err != nil {
		return 0
	}
	return m.GetGauge().GetValue()
}

// WorkerRunFailed increments the count of runs that failed after exhausting
// retries, labelled by worker name.
func (r *Registry) WorkerRunFailed(worker string) {
	if r == nil || r.workerFailures == nil {
		return
	}
	r.workerFailures.WithLabelValues(worker).Inc()
}

// WorkerFailuresValue returns the current counter value for one worker.
// Test helper; returns 0 when unset.
func (r *Registry) WorkerFailuresValue(worker string) float64 {
	if r == nil || r.workerFailures == nil {
		return 0
	}
	var m dto.Metric
	if err := r.workerFailures.WithLabelValues(worker).Write(&m); err != nil {
		return 0
	}
	return m.GetCounter().GetValue()
}

// ObserveRequest records an HTTP request against the counters.
func (r *Registry) ObserveRequest(method, path, status string, durationSeconds float64) {
	r.httpRequests.WithLabelValues(method, path, status).Inc()
	r.httpDuration.WithLabelValues(method, path).Observe(durationSeconds)
}

// Refresh collects the current queue/server gauges from the repos.
// It is a synchronous, overlap-guarded helper — it is NOT called by the
// scrape handler. Values are refreshed by the background loop instead.
func (r *Registry) Refresh() {
	r.refresh(context.Background())
}

// Refreshed reports whether at least one background refresh attempt has
// completed. A fresh registry reports false so callers can tell cached
// (zero) gauges apart from live values without blocking on the database.
func (r *Registry) Refreshed() bool {
	return r.refreshed.Load()
}

// Stop terminates the background refresh loop. Safe to call twice.
func (r *Registry) Stop() {
	r.stopOnce.Do(func() { r.stopLoop() })
}

// loop re-reads the gauges every interval until ctx is cancelled. A
// failed refresh is logged and the loop continues on the next tick.
func (r *Registry) loop(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			r.refresh(ctx)
		}
	}
}

func (r *Registry) refresh(ctx context.Context) {
	// Overlap guard: skip this tick when a slow refresh is still running
	// instead of stacking queries behind the ticker.
	if !r.refreshing.CompareAndSwap(false, true) {
		return
	}
	defer r.refreshing.Store(false)
	defer r.refreshed.Store(true)

	if counts, err := r.jobRepo.CountsByStatus(ctx); err == nil {
		r.jobPending.Set(float64(counts.Queued))
		r.jobRunning.Set(float64(counts.Running))
		r.jobSucceeded.Set(float64(counts.Success))
		r.jobFailed.Set(float64(counts.Failed))
		r.jobTotal.Set(float64(counts.Total))
	} else {
		r.logErr("metrics: job counts refresh failed", err)
	}

	if online, err := r.serverRepo.CountOnline(ctx); err == nil {
		r.serverOnline.Set(float64(online))
	} else {
		r.logErr("metrics: server count refresh failed", err)
	}
}

func (r *Registry) logErr(msg string, err error) {
	if r.log == nil {
		return
	}
	r.log.Error(msg, "error", err)
}

// Handler returns the Prometheus text-format handler.
func (r *Registry) Handler() http.Handler {
	return promhttp.HandlerFor(r.registry, promhttp.HandlerOpts{})
}
