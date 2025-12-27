// Package executor
package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"horizonx-server/internal/agent/docker"
	"horizonx-server/internal/agent/git"
	"horizonx-server/internal/domain"
	"horizonx-server/internal/logger"
)

type EmitHandler = func(event any)

type Executor struct {
	log    logger.Logger
	docker *docker.Manager
	git    *git.Manager
}

func NewExecutor(log logger.Logger, workDir string) *Executor {
	return &Executor{
		log:    log,
		docker: docker.NewManager(workDir),
		git:    git.NewManager(workDir),
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

func (e *Executor) Execute(ctx context.Context, job *domain.Job, onEmit EmitHandler) error {
	e.log.Info("executing job", "id", job.ID, "type", job.Type)

	switch job.Type {
	case domain.JobTypeAppDeploy:
		return e.deployApp(ctx, job, onEmit)
	case domain.JobTypeAppStart:
		return e.startApp(ctx, job, onEmit)
	case domain.JobTypeAppStop:
		return e.stopApp(ctx, job, onEmit)
	case domain.JobTypeAppRestart:
		return e.restartApp(ctx, job, onEmit)
	default:
		return fmt.Errorf("unknown job type: %s", job.Type)
	}
}

func (e *Executor) emitLog(
	level domain.LogLevel,
	source domain.LogSource,
	action domain.LogAction,
	step domain.LogStep,
	stream domain.LogStream,
	line string,
) domain.EventLogEmitted {
	return domain.EventLogEmitted{
		Timestamp: time.Now().UTC(),
		Level:     level,
		Source:    source,
		Action:    action,
		Message:   line,
		Context: &domain.LogContext{
			Step:   step,
			Stream: stream,
			Line:   line,
		},
	}
}

func (e *Executor) deployApp(ctx context.Context, job *domain.Job, emit EmitHandler) error {
	var payload domain.DeployAppPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return err
	}

	appID := payload.ApplicationID
	source := domain.LogAgent
	action := domain.ActionAppDeploy

	// Git clone or pull
	if _, err := e.git.CloneOrPull(ctx, appID, payload.RepoURL, payload.Branch, func(line string, isErr bool) {
		level := domain.LogInfo
		stream := domain.StreamStdout
		if isErr {
			level = domain.LogError
			stream = domain.StreamStderr
		}

		emit(e.emitLog(level, source, action, domain.StepGitClone, stream, line))
	}); err != nil {
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
	if _, err := e.docker.ComposeDown(ctx, appID, false, func(line string, isErr bool) {
		level := domain.LogInfo
		stream := domain.StreamStdout
		if isErr {
			level = domain.LogError
			stream = domain.StreamStderr
		}

		emit(e.emitLog(level, source, action, domain.StepDockerStop, stream, line))
	}); err != nil {
		return err
	}

	// Docker compose up
	if _, err := e.docker.ComposeUp(ctx, appID, true, true, func(line string, isErr bool) {
		level := domain.LogInfo
		stream := domain.StreamStdout
		if isErr {
			level = domain.LogError
			stream = domain.StreamStderr
		}

		emit(e.emitLog(level, source, action, domain.StepDockerBuild, stream, line))
	}); err != nil {
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
	source := domain.LogAgent
	action := domain.ActionAppStart

	if _, err := e.docker.ComposeStart(ctx, appID, func(line string, isErr bool) {
		level := domain.LogInfo
		stream := domain.StreamStdout
		if isErr {
			level = domain.LogError
			stream = domain.StreamStderr
		}

		emit(e.emitLog(level, source, action, domain.StepDockerStart, stream, line))
	}); err != nil {
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
	source := domain.LogAgent
	action := domain.ActionAppStop

	if _, err := e.docker.ComposeStop(ctx, appID, func(line string, isErr bool) {
		level := domain.LogInfo
		stream := domain.StreamStdout
		if isErr {
			level = domain.LogError
			stream = domain.StreamStderr
		}

		emit(e.emitLog(level, source, action, domain.StepDockerStop, stream, line))
	}); err != nil {
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
	source := domain.LogAgent
	action := domain.ActionAppRestart

	if _, err := e.docker.ComposeRestart(ctx, appID, func(line string, isErr bool) {
		level := domain.LogInfo
		stream := domain.StreamStdout
		if isErr {
			level = domain.LogError
			stream = domain.StreamStderr
		}

		emit(e.emitLog(level, source, action, domain.StepDockerRestart, stream, line))
	}); err != nil {
		return err
	}

	return nil
}
