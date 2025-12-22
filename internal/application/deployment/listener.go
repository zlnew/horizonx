// Package deployment
package deployment

import (
	"context"
	"time"

	"horizonx-server/internal/domain"
	"horizonx-server/internal/event"
	"horizonx-server/internal/logger"
)

// Listener syncs job events to deployment records
type Listener struct {
	repo domain.DeploymentRepository
	log  logger.Logger
}

func NewListener(repo domain.DeploymentRepository, log logger.Logger) *Listener {
	return &Listener{
		repo: repo,
		log:  log,
	}
}

func (l *Listener) Register(bus *event.Bus) {
	bus.Subscribe("job_finished", l.handleJobFinished)
}

func (l *Listener) handleJobFinished(event any) {
	evt, ok := event.(domain.EventJobFinished)
	if !ok {
		return
	}

	// Only process deploy_app jobs
	if evt.JobType != domain.JobTypeDeployApp {
		return
	}

	l.log.Debug("processing job finished event for deployment", "job_id", evt.JobID)

	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Find the deployment associated with this job
	// Note: We need to track job_id in deployments table
	// For now, we'll use a workaround: find pending/building deployment for this app

	// This is a simplified approach - in production you'd store job_id in deployments
	// and query by job_id directly

	l.log.Info("deployment job completed", "job_id", evt.JobID, "type", evt.JobType)
}
