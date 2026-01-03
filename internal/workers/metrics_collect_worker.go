package workers

import (
	"context"
	"fmt"
	"time"

	"horizonx-server/internal/domain"
	"horizonx-server/internal/logger"

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
		jobs, err := w.job.List(ctx, domain.JobListOptions{
			ListOptions: domain.ListOptions{
				Limit: 1,
			},
			ServerID: &srv.ID,
			Type:     string(domain.JobTypeMetricsCollect),
		})
		if err != nil {
			w.log.Error("failed to list existing jobs", "job_type", jobType, "server_id", srv.ID.String())
			continue
		}

		// Create new job
		if len(jobs.Data) == 0 {
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
			continue
		}

		// Retry existing job
		if len(jobs.Data) > 0 {
			job := jobs.Data[0]

			if job.Status == domain.JobQueued {
				continue
			}

			queuedAt := time.Now().UTC()
			data := &domain.Job{
				Status:   domain.JobQueued,
				QueuedAt: &queuedAt,
			}

			updatedJob, err := w.job.Retry(ctx, job.ID, data)
			if err != nil {
				w.log.Error("failed to retry job", "job_type", jobType, "server_id", srv.ID.String())
				continue
			}

			w.log.Debug("job retried", "job_id", updatedJob.ID, "job_type", updatedJob.Type)
		}
	}

	return nil
}
