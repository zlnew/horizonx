// Package deployment
package deployment

import (
	"context"
	"time"

	"horizonx-server/internal/domain"
	"horizonx-server/internal/event"
	"horizonx-server/internal/logger"
)

type Listener struct {
	svc domain.DeploymentService
	log logger.Logger
}

func NewListener(svc domain.DeploymentService, log logger.Logger) *Listener {
	return &Listener{
		svc: svc,
		log: log,
	}
}

func (l *Listener) Register(bus *event.Bus) {
	bus.Subscribe("job_started", l.handleJobStarted)
	bus.Subscribe("job_finished", l.handleJobFinished)
}

func (l *Listener) handleJobStarted(event any) {
	evt, ok := event.(domain.EventJobStarted)
	if !ok {
		l.log.Warn("invalid event payload for job_started", "event", event)
		return
	}

	if evt.DeploymentID == nil || evt.ApplicationID == nil || evt.JobType != domain.JobTypeDeployApp {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_ = l.updateStatus(ctx, *evt.DeploymentID, domain.DeploymentDeploying)

	if err := l.svc.Start(ctx, *evt.DeploymentID); err != nil {
		l.log.Error("failed to start deployment", "deployment_id", *evt.DeploymentID)
		return
	}

	l.log.Debug("deployment started", "deployment_id", *evt.DeploymentID)
}

func (l *Listener) handleJobFinished(event any) {
	evt, ok := event.(domain.EventJobFinished)
	if !ok {
		l.log.Warn("invalid event payload for job_finished", "event", event)
		return
	}

	if evt.DeploymentID == nil || evt.ApplicationID == nil || evt.JobType != domain.JobTypeDeployApp {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	status := domain.DeploymentSuccess
	if evt.Status == domain.JobFailed {
		status = domain.DeploymentFailed
	}

	_ = l.updateStatus(ctx, *evt.DeploymentID, status)

	if err := l.svc.Finish(ctx, *evt.DeploymentID); err != nil {
		l.log.Error("failed to finish deployment", "deployment_id", *evt.DeploymentID)
		return
	}

	l.log.Debug("deployment finished", "deployment_id", *evt.DeploymentID)
}

func (l *Listener) updateStatus(ctx context.Context, deploymentID int64, status domain.DeploymentStatus) error {
	err := l.svc.UpdateStatus(ctx, deploymentID, status)
	if err != nil {
		l.log.Error("failed to update deployment status", "deployment_id", deploymentID, "error", err)
		return err
	}

	l.log.Debug("deployment status updated", "deployment_id", deploymentID)

	return nil
}
