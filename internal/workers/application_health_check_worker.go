package workers

import (
	"context"
	"encoding/json"
	"fmt"

	"horizonx/internal/domain"
	"horizonx/internal/logger"

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
				ServerID:     app.ServerID,
				Applications: []domain.AppInfo{},
			}
		}

		seen := make(map[int64]bool)
		for _, a := range payload.Applications {
			seen[a.ApplicationID] = true
		}

		if !seen[app.ID] {
			payload.Applications = append(payload.Applications, domain.AppInfo{
				ApplicationID: app.ID,
				AppKey:        domain.GetAppKey(app),
			})
		}

		jobPayloads[app.ServerID] = payload
	}

	if len(jobPayloads) == 0 {
		return nil
	}

	jobType := domain.JobTypeAppHealthCheck

	for _, p := range jobPayloads {
		if len(p.Applications) == 0 {
			w.log.Debug("there is no applications to check, skipping...", "server_id", p.ServerID.String())
			continue
		}

		// Only an open (queued or running) job blocks a new one. A finished job
		// is history and must never be resurrected: re-queueing the finished row
		// wiped started_at/finished_at (so the reaper could never reap it) and
		// re-ran the same job every tick forever. Creating a job only when none
		// is open leaves one row per tick for the retention worker's TTL to bound.
		jobs, err := w.job.List(ctx, domain.JobListOptions{
			ListOptions: domain.ListOptions{Limit: 1},
			ServerID:    &p.ServerID,
			Type:        string(jobType),
			Statuses:    []string{string(domain.JobQueued), string(domain.JobRunning)},
		})
		if err != nil {
			w.log.Error("failed to list existing jobs", "job_type", jobType, "server_id", p.ServerID.String())
			continue
		}

		payload, err := json.Marshal(p)
		if err != nil {
			return fmt.Errorf("failed to marshal job payload: %w", err)
		}

		// An open job for this server/type will already run; nothing to do.
		if len(jobs.Data) > 0 {
			continue
		}

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
	}

	return nil
}
