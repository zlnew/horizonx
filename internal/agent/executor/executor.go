// Package executor
package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"time"

	"horizonx/internal/agent/command"
	"horizonx/internal/agent/docker"
	"horizonx/internal/agent/git"
	"horizonx/internal/domain"
	"horizonx/internal/logger"
)

type EmitHandler = func(event any)

type Executor struct {
	docker  *docker.Manager
	git     *git.Manager
	metrics func() *domain.Metrics

	workDir string

	log logger.Logger
}

func NewExecutor(workDir string, log logger.Logger, metrics func() *domain.Metrics) *Executor {
	return &Executor{
		docker:  docker.NewManager(),
		git:     git.NewManager(),
		metrics: metrics,

		workDir: workDir,

		log: log,
	}
}

func (e *Executor) Init() error {
	if !e.docker.IsDockerInstalled() {
		return fmt.Errorf("docker is not installed")
	}

	if !e.docker.IsDockerComposeAvailable() {
		return fmt.Errorf("docker compose is not installed")
	}

	if !e.git.IsGitInstalled() {
		return fmt.Errorf("git is not installed")
	}

	return e.createWorkDir()
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
	case domain.JobTypeAppDestroy:
		return e.destroyApp(ctx, job, emit)
	default:
		return fmt.Errorf("unknown job type: %s", job.Type)
	}
}

func (e *Executor) getAppWorkDir(dirName string) string {
	return filepath.Join(e.workDir, dirName)
}

func (e *Executor) createWorkDir() error {
	if err := os.MkdirAll(e.workDir, 0o755); err != nil {
		return fmt.Errorf("failed to create apps work directory: %w", err)
	}

	return nil
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

	reports := make([]domain.ApplicationHealth, 0, len(payload.Applications))

	for _, app := range payload.Applications {
		workDir := e.getAppWorkDir(app.AppKey)

		output, err := e.docker.Cmd(ctx, workDir, []string{"compose", "ps", "--format", "json"})
		if err != nil {
			// TODO: implement application docker container status
			e.log.Debug("failed to run docker compose ps",
				"server_id", job.ServerID.String(),
				"app_id", app.ApplicationID,
				"err", err.Error(),
			)

			reports = append(reports, domain.ApplicationHealth{
				ApplicationID: app.ApplicationID,
				Status:        domain.AppStatusFailed,
			})

			continue
		}

		if output == "" {
			reports = append(reports, domain.ApplicationHealth{
				ApplicationID: app.ApplicationID,
				Status:        domain.AppStatusUnknown,
			})
			continue
		}

		var c docker.Container
		if err := json.Unmarshal([]byte(output), &c); err != nil {
			e.logFatalHandler(
				fmt.Sprintf(
					"failed to parse compose ps output server_id=%s app_id=%d err=%v",
					job.ServerID.String(),
					app.ApplicationID,
					err,
				),
				emit,
				action,
				step,
			)

			reports = append(reports, domain.ApplicationHealth{
				ApplicationID: app.ApplicationID,
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
			status = domain.AppStatusUnknown
		case "dead":
			status = domain.AppStatusFailed
		default:
			status = domain.AppStatusUnknown
		}

		reports = append(reports, domain.ApplicationHealth{
			ApplicationID: app.ApplicationID,
			Status:        status,
		})
	}

	emit(reports)

	return nil
}

func (e *Executor) deployApp(ctx context.Context, job *domain.Job, emit EmitHandler) error {
	var payload domain.AppDeployPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return err
	}

	workDir := e.getAppWorkDir(payload.AppKey)
	action := domain.ActionAppDeploy

	// Create app work directory
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return err
	}

	// Git clone or pull
	if _, err := e.git.CloneOrPull(ctx, workDir, payload.RepoURL, payload.Branch, e.logStreamHandler(
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
	commitHash, err := e.git.GetCurrentCommit(ctx, workDir)
	if err != nil {
		e.logFatalHandler(
			fmt.Sprintf("failed to get commit hash, %s", err.Error()),
			emit,
			action,
			domain.StepBuildPrepare,
		)
		return err
	}

	// Get git commit message
	commitMessage, err := e.git.GetCommitMessage(ctx, workDir)
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
		Hash:         commitHash[:8],
		Message:      commitMessage,
	})

	// Get docker compose file
	if _, err := e.docker.GetDockerComposeFile(workDir); err != nil {
		e.logFatalHandler(
			fmt.Sprintf("failed to get docker compose file, %s", err.Error()),
			emit,
			action,
			domain.StepBuildPrepare,
		)
		return err
	}

	// Get Dockerfile
	dockerfilePath, err := e.docker.GetDockerfile(workDir)
	if err != nil {
		e.logFatalHandler(
			fmt.Sprintf("failed to get Dockerfile, %s", err.Error()),
			emit,
			action,
			domain.StepBuildPrepare,
		)
		return err
	}

	// Write user env
	userEnvVars := payload.EnvVars
	if len(userEnvVars) > 0 {
		if err := e.docker.WriteEnvFile(workDir, userEnvVars); err != nil {
			e.logFatalHandler(
				fmt.Sprintf("failed to write user env, %s", err.Error()),
				emit,
				action,
				domain.StepBuildPrepare,
			)
			return err
		}
	}

	// Define docker's image and container name
	appImage := fmt.Sprintf("%s:%s", payload.AppKey, commitHash)
	appContainerName := payload.AppKey

	// Docker build image
	if _, err := e.docker.Cmd(ctx, workDir, []string{"build", "-t", appImage, "-f", dockerfilePath, "."}, e.logStreamHandler(
		emit,
		action,
		domain.StepDockerBuild,
	)); err != nil {
		e.logFatalHandler(
			fmt.Sprintf("failed to build image, %s", err.Error()),
			emit,
			action,
			domain.StepDockerBuild,
		)
	}

	// Write user and build env
	envVars := make(map[string]string)

	maps.Copy(envVars, userEnvVars)

	envVars["APP_IMAGE"] = appImage
	envVars["APP_CONTAINER_NAME"] = appContainerName

	if err := e.docker.WriteEnvFile(workDir, envVars); err != nil {
		e.logFatalHandler(
			fmt.Sprintf("failed to write user and build env, %s", err.Error()),
			emit,
			action,
			domain.StepDockerBuild,
		)
		return err
	}

	// Docker compose down
	if _, err := e.docker.Cmd(ctx, workDir, []string{"compose", "down"}, e.logStreamHandler(
		emit,
		action,
		domain.StepDockerStart,
	),
	); err != nil {
		e.logFatalHandler(
			fmt.Sprintf("failed to run docker compose down, %s", err.Error()),
			emit,
			action,
			domain.StepDockerStart,
		)
		return err
	}

	// Docker compose up
	if _, err := e.docker.Cmd(ctx, workDir, []string{"compose", "up", "-d", "--force-recreate"}, e.logStreamHandler(
		emit,
		action,
		domain.StepDockerStart,
	)); err != nil {
		e.logFatalHandler(
			fmt.Sprintf("failed to run docker compose up, %s", err.Error()),
			emit,
			action,
			domain.StepDockerStart,
		)
		return err
	}

	return nil
}

func (e *Executor) startApp(ctx context.Context, job *domain.Job, emit EmitHandler) error {
	var payload domain.AppStartPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return err
	}

	workDir := e.getAppWorkDir(payload.AppKey)

	if _, err := e.docker.Cmd(ctx, workDir, []string{"compose", "start"}, e.logStreamHandler(
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
	var payload domain.AppStopPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return err
	}

	workDir := e.getAppWorkDir(payload.AppKey)

	if _, err := e.docker.Cmd(ctx, workDir, []string{"compose", "stop"}, e.logStreamHandler(
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
	var payload domain.AppRestartPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return err
	}

	workDir := e.getAppWorkDir(payload.AppKey)

	if _, err := e.docker.Cmd(ctx, workDir, []string{"compose", "up", "-d", "--force-recreate"}, e.logStreamHandler(
		emit,
		domain.ActionAppRestart,
		domain.StepDockerRestart,
	)); err != nil {
		e.logFatalHandler(
			fmt.Sprintf("failed to run docker compose up, %s", err.Error()),
			emit,
			domain.ActionAppRestart,
			domain.StepDockerRestart,
		)
		return err
	}

	return nil
}

func (e *Executor) destroyApp(ctx context.Context, job *domain.Job, emit EmitHandler) error {
	var payload domain.AppDestroyPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return err
	}

	workDir := e.getAppWorkDir(payload.AppKey)
	imageName := payload.AppKey

	// Stopping container
	if _, err := e.docker.Cmd(ctx, workDir, []string{"stop", imageName}, e.logStreamHandler(
		emit,
		domain.ActionAppDestroy,
		domain.StepDockerStop,
	)); err != nil {
		e.logFatalHandler(
			fmt.Sprintf("failed to stopping container, %s", err.Error()),
			emit,
			domain.ActionAppDestroy,
			domain.StepDockerStop,
		)
		return err
	}

	backupName := fmt.Sprintf("%s:backup", imageName)

	// Commiting backup
	if _, err := e.docker.Cmd(ctx, workDir, []string{"commit", imageName, backupName}, e.logStreamHandler(
		emit,
		domain.ActionAppDestroy,
		domain.StepDockerCommit,
	)); err != nil {
		e.logFatalHandler(
			fmt.Sprintf("failed to commiting backup, %s", err.Error()),
			emit,
			domain.ActionAppDestroy,
			domain.StepDockerCommit,
		)
		return nil
	}

	// Remove container
	if _, err := e.docker.Cmd(ctx, workDir, []string{"rm", imageName}, e.logStreamHandler(
		emit,
		domain.ActionAppDestroy,
		domain.StepDockerRemove,
	)); err != nil {
		e.logFatalHandler(
			fmt.Sprintf("failed to removing container, %s", err.Error()),
			emit,
			domain.ActionAppDestroy,
			domain.StepDockerRemove,
		)
		return nil
	}

	return nil
}
