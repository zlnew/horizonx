package workers

import (
	"context"
	"fmt"

	"horizonx/internal/domain"
	"horizonx/internal/logger"

	"github.com/google/uuid"
)

type MetricsCollectWorker struct {
	job    domain.JobService
	server domain.ServerService
	log    logger.Logger
}

func NewMetricsCollectWorker(job domain.JobService, server domain.ServerService, log logger.Logger) Worker {
	return &MetricsCollectWorker{
		job:    job,
		server: server,
		log:    log,
	}
}

func (w *MetricsCollectWorker) Name() string {
	return "metrics_collect"
}

func (w *MetricsCollectWorker) Run(ctx context.Context) error {
	isOnline := true
	servers, err := w.server.List(ctx, domain.ServerListOptions{
		IsOnline: &isOnline,
	})
	if err != nil {
		return fmt.Errorf("failed to list online servers: %w", err)
	}

	if len(servers.Data) == 0 {
		w.log.Debug("worker: no online servers found", "name", w.Name())
		return nil
	}

	jobType := domain.JobTypeMetricsCollect

	for _, srv := range servers.Data {
		// Only an open (queued or running) job blocks a new one. A finished job
		// is history and must never be resurrected: re-queueing the finished row
		// wiped started_at/finished_at (so the reaper could never reap it) and
		// re-ran the same job every tick forever. Creating a job only when none
		// is open leaves one row per tick for the retention worker's TTL to bound.
		jobs, err := w.job.List(ctx, domain.JobListOptions{
			ListOptions: domain.ListOptions{
				Limit: 1,
			},
			ServerID: &srv.ID,
			Type:     string(domain.JobTypeMetricsCollect),
			Statuses: []string{string(domain.JobQueued), string(domain.JobRunning)},
		})
		if err != nil {
			w.log.Error("failed to list existing jobs", "job_type", jobType, "server_id", srv.ID.String())
			continue
		}

		// An open job for this server/type will already run; nothing to do.
		if len(jobs.Data) > 0 {
			continue
		}

		data := &domain.Job{
			TraceID:  uuid.New(),
			ServerID: srv.ID,
			Type:     domain.JobTypeMetricsCollect,
		}

		createdJob, err := w.job.Create(ctx, data)
		if err != nil {
			w.log.Error("failed to create job", "job_type", jobType, "server_id", srv.ID.String())
			continue
		}

		w.log.Debug("job created", "job_id", createdJob.ID, "job_type", createdJob.Type)
	}

	return nil
}
