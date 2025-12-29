// Package executor
package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"horizonx-server/internal/agent/docker"
	"horizonx-server/internal/agent/git"
	"horizonx-server/internal/domain"
	"horizonx-server/internal/logger"
)

type EmitHandler = func(event any)

type Executor struct {
	metrics func() *domain.Metrics
	docker  *docker.Manager
	git     *git.Manager

	log logger.Logger
}

func NewExecutor(workDir string, metrics func() *domain.Metrics, log logger.Logger) *Executor {
	return &Executor{
		docker:  docker.NewManager(workDir),
		git:     git.NewManager(workDir),
		metrics: metrics,

		log: log,
	}
}

func (e *Executor) Initialize() error {
	if !e.docker.IsDockerInstalled() {
		return fmt.Errorf("docker is not installed")
	}

	if !e.docker.IsDockerComposeAvailable() {
		return fmt.Errorf("docker compose is not available")
	}

	if !e.git.IsGitInstalled() {
		return fmt.Errorf("git is not installed")
	}

	return e.docker.Initialize()
}

func (e *Executor) Execute(ctx context.Context, job *domain.Job, emit EmitHandler) error {
	e.log.Debug("executing job", "job_id", job.ID)

	switch job.Type {
	case domain.JobTypeMetricsCollect:
		emit(e.metrics())
		return nil
	case domain.JobTypeAppDeploy:
		return e.deployApp(ctx, job, emit)
	case domain.JobTypeAppStart:
		return e.startApp(ctx, job, emit)
	case domain.JobTypeAppStop:
		return e.stopApp(ctx, job, emit)
	case domain.JobTypeAppRestart:
		return e.restartApp(ctx, job, emit)
	default:
		return fmt.Errorf("unknown job type: %s", job.Type)
	}
}

func (e *Executor) emitLogHandler(
	emit EmitHandler,
	action domain.LogAction,
	step domain.LogStep,
) func(line string, stream domain.LogStream, level domain.LogLevel) {
	return func(line string, stream domain.LogStream, level domain.LogLevel) {
		emit(domain.EventLogEmitted{
			Timestamp: time.Now().UTC(),
			Level:     level,
			Source:    domain.LogAgent,
			Action:    action,
			Message:   line,
			Context: &domain.LogContext{
				Step:   step,
				Stream: stream,
				Line:   line,
			},
		})
	}
}

func (e *Executor) deployApp(ctx context.Context, job *domain.Job, emit EmitHandler) error {
	var payload domain.DeployAppPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return err
	}

	appID := payload.ApplicationID
	appDir := e.git.GetAppDir(appID)
	action := domain.ActionAppDeploy

	// Create app directory
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		return err
	}

	// Git clone or pull
	if _, err := e.git.CloneOrPull(ctx, appID, payload.RepoURL, payload.Branch, e.emitLogHandler(
		emit,
		action,
		domain.StepGitClone,
	),
	); err != nil {
		return err
	}

	// Get git commit info
	if job.DeploymentID != nil {
		hash, err := e.git.GetCurrentCommit(ctx, appID)
		if err != nil {
			return err
		}

		message, err := e.git.GetCommitMessage(ctx, appID)
		if err != nil {
			return err
		}

		emit(domain.EventCommitInfoEmitted{
			DeploymentID: *job.DeploymentID,
			Hash:         hash[:8],
			Message:      message,
		})
	}

	// Validate docker compose file
	if err := e.docker.ValidateDockerComposeFile(appID); err != nil {
		return err
	}

	// Write env
	if len(payload.EnvVars) > 0 {
		if err := e.docker.WriteEnvFile(appID, payload.EnvVars); err != nil {
			return err
		}
	}

	// Docker compose down
	if _, err := e.docker.ComposeDown(ctx, appID, false, e.emitLogHandler(
		emit,
		action,
		domain.StepDockerStop,
	),
	); err != nil {
		return err
	}

	// Docker compose up
	if _, err := e.docker.ComposeUp(ctx, appID, true, true, e.emitLogHandler(
		emit,
		action,
		domain.StepDockerBuild,
	)); err != nil {
		return err
	}

	return nil
}

func (e *Executor) startApp(ctx context.Context, job *domain.Job, emit EmitHandler) error {
	var payload domain.StartAppPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return err
	}

	appID := payload.ApplicationID

	if _, err := e.docker.ComposeStart(ctx, appID, e.emitLogHandler(
		emit,
		domain.ActionAppStart,
		domain.StepDockerStart,
	)); err != nil {
		return err
	}

	return nil
}

func (e *Executor) stopApp(ctx context.Context, job *domain.Job, emit EmitHandler) error {
	var payload domain.StopAppPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return err
	}

	appID := payload.ApplicationID

	if _, err := e.docker.ComposeStop(ctx, appID, e.emitLogHandler(
		emit,
		domain.ActionAppStop,
		domain.StepDockerStop,
	)); err != nil {
		return err
	}

	return nil
}

func (e *Executor) restartApp(ctx context.Context, job *domain.Job, emit EmitHandler) error {
	var payload domain.RestartAppPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return err
	}

	appID := payload.ApplicationID

	if _, err := e.docker.ComposeRestart(ctx, appID, e.emitLogHandler(
		emit,
		domain.ActionAppRestart,
		domain.StepDockerRestart,
	)); err != nil {
		return err
	}

	return nil
}
