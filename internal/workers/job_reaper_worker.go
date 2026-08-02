package workers

import (
	"context"
	"fmt"
	"time"

	"horizonx/internal/domain"
	"horizonx/internal/logger"
)

// JobReaperWorker marks long-running jobs as failed when the agent that owned
// them died mid-execution (P0-6). The agent itself enforces per-job timeouts
// (job_worker.go jobTimeouts; the longest is deploy = 15m), so if a job is
// still 'running' after reapAfter it can only mean the agent went away without
// finishing it — no healthy agent would leave a job running that long.
type JobReaperWorker struct {
	job domain.JobService
	log logger.Logger
}

const (
	// Slightly larger than the longest agent-side job timeout (deploy 15m).
	reapAfter = 30 * time.Minute
)

func NewJobReaperWorker(job domain.JobService, log logger.Logger) Worker {
	return &JobReaperWorker{
		job: job,
		log: log,
	}
}

func (w *JobReaperWorker) Name() string {
	return "job_reaper"
}

func (w *JobReaperWorker) Run(ctx context.Context) error {
	statuses := []string{string(domain.JobRunning)}
	result, err := w.job.List(ctx, domain.JobListOptions{
		ListOptions: domain.ListOptions{Limit: 100},
		Statuses:    statuses,
	})
	if err != nil {
		return fmt.Errorf("failed to list running jobs: %w", err)
	}

	reaped := 0
	for _, job := range result.Data {
		if job.StartedAt == nil {
			continue
		}

		if time.Since(*job.StartedAt) <= reapAfter {
			continue
		}

		// The agent is gone or wedged — mark the job failed so it stops
		// blocking the deployment history and queue.
		if _, err := w.job.Finish(ctx, job.ID, domain.JobFailed); err != nil {
			w.log.Error("failed to reap stuck job",
				"job_id", job.ID,
				"error", err.Error(),
			)
			continue
		}

		w.log.Info("reaped stuck job",
			"job_id", job.ID,
			"type", job.Type,
			"started_at", job.StartedAt.Format(time.RFC3339),
		)
		reaped++
	}

	w.log.Debug("job reaper finished", "reaped", reaped, "scanned", len(result.Data))
	return nil
}
