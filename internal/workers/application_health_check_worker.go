package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"horizonx-server/internal/domain"
	"horizonx-server/internal/logger"

	"github.com/google/uuid"
)

type ApplicationHealthCheckWorker struct {
	app domain.ApplicationService
	job domain.JobService
	log logger.Logger
}

func NewApplicationHealthCheckWorker(app domain.ApplicationService, job domain.JobService, server domain.ServerService, log logger.Logger) Worker {
	return &ApplicationHealthCheckWorker{
		app: app,
		job: job,
		log: log,
	}
}

func (w *ApplicationHealthCheckWorker) Name() string {
	return "application_health_check"
}

func (w *ApplicationHealthCheckWorker) Run(ctx context.Context) error {
	applications, err := w.app.List(ctx, domain.ApplicationListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list applications: %w", err)
	}

	jobPayloads := make(map[uuid.UUID]domain.AppHealthCheckPayload)
	for _, app := range applications.Data {
		payload, exists := jobPayloads[app.ServerID]
		if !exists {
			payload = domain.AppHealthCheckPayload{
				ServerID:        app.ServerID,
				ApplicationsIDs: []int64{},
			}
		}

		seen := make(map[int64]bool)
		for _, appID := range payload.ApplicationsIDs {
			seen[appID] = true
		}

		if !seen[app.ID] {
			payload.ApplicationsIDs = append(payload.ApplicationsIDs, app.ID)
		}

		jobPayloads[app.ServerID] = payload
	}

	if len(jobPayloads) == 0 {
		return nil
	}

	jobType := domain.JobTypeAppHealthCheck

	for _, p := range jobPayloads {
		if len(p.ApplicationsIDs) == 0 {
			w.log.Debug("there is no applications to check, skipping...", "server_id", p.ServerID.String())
			continue
		}

		jobs, err := w.job.List(ctx, domain.JobListOptions{
			ListOptions: domain.ListOptions{Limit: 1},
			ServerID:    &p.ServerID,
			Type:        string(jobType),
		})
		if err != nil {
			w.log.Error("failed to list existing jobs", "job_type", jobType, "server_id", p.ServerID.String())
			continue
		}

		payload, err := json.Marshal(p)
		if err != nil {
			return fmt.Errorf("failed to marshal job payload: %w", err)
		}

		// Create new job
		if len(jobs.Data) == 0 {
			data := &domain.Job{
				TraceID:  uuid.New(),
				ServerID: p.ServerID,
				Type:     jobType,
				Payload:  payload,
			}

			createdJob, err := w.job.Create(ctx, data)
			if err != nil {
				w.log.Error("failed to create job", "job_type", jobType, "server_id", p.ServerID.String())
				continue
			}

			w.log.Debug("job created", "job_id", createdJob.ID, "job_type", jobType)
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
				w.log.Error("failed to retry job", "job_type", jobType, "server_id", p.ServerID.String())
				continue
			}

			w.log.Debug("job retried", "job_id", updatedJob.ID, "job_type", jobType)
		}
	}

	return nil
}
