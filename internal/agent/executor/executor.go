// Package executor
package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"horizonx-server/internal/agent/command"
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
	case domain.JobTypeAppHealthCheck:
		return e.checkAppHealths(ctx, job, emit)
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

func (e *Executor) logStreamHandler(emit EmitHandler, action domain.LogAction, step domain.LogStep) command.StreamHandler {
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

func (e *Executor) logFatalHandler(
	message string,
	emit EmitHandler,
	action domain.LogAction,
	step domain.LogStep,
) {
	emit(domain.EventLogEmitted{
		Timestamp: time.Now().UTC(),
		Level:     domain.LogFatal,
		Source:    domain.LogAgent,
		Action:    action,
		Message:   message,
		Context: &domain.LogContext{
			Step:   step,
			Stream: domain.StreamStderr,
			Line:   message,
		},
	})
}

func (e *Executor) checkAppHealths(ctx context.Context, job *domain.Job, emit EmitHandler) error {
	var payload domain.AppHealthCheckPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return err
	}

	action := domain.ActionAppHealthCheck
	step := domain.StepDockerHealthCheck

	reports := make([]domain.ApplicationHealth, 0, len(payload.ApplicationsIDs))

	for _, appID := range payload.ApplicationsIDs {
		output, err := e.docker.ComposePs(ctx, appID, true)
		if err != nil {
			// TODO: implement application docker container status
			e.log.Debug("failed to run docker compose ps",
				"server_id", job.ServerID.String(),
				"app_id", appID,
				"err", err.Error(),
			)

			reports = append(reports, domain.ApplicationHealth{
				ApplicationID: appID,
				Status:        domain.AppStatusFailed,
			})

			continue
		}

		var c docker.Container
		if err := json.Unmarshal([]byte(output), &c); err != nil {
			e.logFatalHandler(
				fmt.Sprintf(
					"failed to parse compose ps output server_id=%s app_id=%d err=%v",
					job.ServerID.String(),
					appID,
					err,
				),
				emit,
				action,
				step,
			)

			reports = append(reports, domain.ApplicationHealth{
				ApplicationID: appID,
				Status:        domain.AppStatusFailed,
			})

			continue
		}

		var status domain.ApplicationStatus

		switch c.State {
		case "running":
			switch c.Health {
			case "unhealthy":
				status = domain.AppStatusFailed
			case "starting":
				status = domain.AppStatusStarting
			default:
				status = domain.AppStatusRunning
			}
		case "restarting":
			status = domain.AppStatusRestarting
		case "exited":
			switch c.ExitCode {
			case 0:
				status = domain.AppStatusStopped
			default:
				status = domain.AppStatusFailed
			}
		case "paused":
			status = domain.AppStatusStopped
		case "dead":
			status = domain.AppStatusFailed
		default:
			status = domain.AppStatusFailed
		}

		reports = append(reports, domain.ApplicationHealth{
			ApplicationID: appID,
			Status:        status,
		})
	}

	emit(reports)

	return nil
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
	if _, err := e.git.CloneOrPull(ctx, appID, payload.RepoURL, payload.Branch, e.logStreamHandler(
		emit,
		action,
		domain.StepGitClone,
	),
	); err != nil {
		e.logFatalHandler(
			fmt.Sprintf("failed to clone or pull repository, %s", err.Error()),
			emit,
			action,
			domain.StepGitClone,
		)
		return err
	}

	// Get git commit info
	if job.DeploymentID != nil {
		hash, err := e.git.GetCurrentCommit(ctx, appID)
		if err != nil {
			e.logFatalHandler(
				fmt.Sprintf("failed to get commit hash, %s", err.Error()),
				emit,
				action,
				domain.StepBuildPrepare,
			)
			return err
		}

		message, err := e.git.GetCommitMessage(ctx, appID)
		if err != nil {
			e.logFatalHandler(
				fmt.Sprintf("failed to get commit message, %s", err.Error()),
				emit,
				action,
				domain.StepBuildPrepare,
			)

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
		e.logFatalHandler(
			fmt.Sprintf("failed to validate docker compose file, %s", err.Error()),
			emit,
			action,
			domain.StepBuildPrepare,
		)
		return err
	}

	// Write env
	if len(payload.EnvVars) > 0 {
		if err := e.docker.WriteEnvFile(appID, payload.EnvVars); err != nil {
			e.logFatalHandler(
				fmt.Sprintf("failed to write env, %s", err.Error()),
				emit,
				action,
				domain.StepBuildPrepare,
			)
			return err
		}
	}

	// Docker compose down
	if _, err := e.docker.ComposeDown(ctx, appID, false, e.logStreamHandler(
		emit,
		action,
		domain.StepDockerStop,
	),
	); err != nil {
		e.logFatalHandler(
			fmt.Sprintf("failed to run docker compose down, %s", err.Error()),
			emit,
			action,
			domain.StepDockerStop,
		)
		return err
	}

	// Docker compose up
	if _, err := e.docker.ComposeUp(ctx, appID, true, true, e.logStreamHandler(
		emit,
		action,
		domain.StepDockerBuild,
	)); err != nil {
		e.logFatalHandler(
			fmt.Sprintf("failed to run docker compose up, %s", err.Error()),
			emit,
			action,
			domain.StepDockerBuild,
		)
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

	if _, err := e.docker.ComposeStart(ctx, appID, e.logStreamHandler(
		emit,
		domain.ActionAppStart,
		domain.StepDockerStart,
	)); err != nil {
		e.logFatalHandler(
			fmt.Sprintf("failed to run docker compose start, %s", err.Error()),
			emit,
			domain.ActionAppStart,
			domain.StepDockerStart,
		)
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

	if _, err := e.docker.ComposeStop(ctx, appID, e.logStreamHandler(
		emit,
		domain.ActionAppStop,
		domain.StepDockerStop,
	)); err != nil {
		e.logFatalHandler(
			fmt.Sprintf("failed to run docker compose stop, %s", err.Error()),
			emit,
			domain.ActionAppStop,
			domain.StepDockerStop,
		)
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

	if _, err := e.docker.ComposeRestart(ctx, appID, e.logStreamHandler(
		emit,
		domain.ActionAppRestart,
		domain.StepDockerRestart,
	)); err != nil {
		e.logFatalHandler(
			fmt.Sprintf("failed to run docker compose restart, %s", err.Error()),
			emit,
			domain.ActionAppRestart,
			domain.StepDockerRestart,
		)
		return err
	}

	return nil
}
