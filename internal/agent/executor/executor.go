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

// healthGateTimeout bounds how long a deploy/rollback waits for the app to
// actually come up after `compose up` before failing the job (P1-9).
const healthGateTimeout = 60 * time.Second

// DockerRunner is the subset of docker.Manager the executor needs. Defined as
// an interface so the deploy pipeline can be unit-tested with a fake.
type DockerRunner interface {
	Cmd(ctx context.Context, workDir string, args []string, handlers ...command.StreamHandler) (string, error)
	GetDockerComposeFile(workDir string) (string, error)
	GetDockerfile(workDir string) (string, error)
	WriteEnvFile(workDir string, envVars map[string]string) error
	IsDockerInstalled() bool
	IsDockerComposeAvailable() bool
}

// GitRunner is the subset of git.Manager the executor needs.
type GitRunner interface {
	CloneOrPull(ctx context.Context, workDir, remoteURL, branch string, handlers ...command.StreamHandler) (string, error)
	GetCurrentCommit(ctx context.Context, workDir string, handlers ...command.StreamHandler) (string, error)
	GetCommitMessage(ctx context.Context, workDir string, handlers ...command.StreamHandler) (string, error)
	IsGitInstalled() bool
}

type Executor struct {
	docker DockerRunner
	git    GitRunner
	metrics func() *domain.Metrics

	workDir string

	log logger.Logger
}

func NewExecutor(workDir string, log logger.Logger, metrics func() *domain.Metrics) *Executor {
	return NewExecutorWithDeps(docker.NewManager(), git.NewManager(), workDir, log, metrics)
}

// NewExecutorWithDeps wires explicit docker/git runners — used by tests with fakes.
func NewExecutorWithDeps(docker DockerRunner, git GitRunner, workDir string, log logger.Logger, metrics func() *domain.Metrics) *Executor {
	return &Executor{
		docker:  docker,
		git:     git,
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
	case domain.JobTypeAppRollback:
		return e.rollbackApp(ctx, job, emit)
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

		// `docker compose ps --format json` emits a JSON ARRAY when the app
		// has more than one service, and a single object when it has one.
		// Parse the array first, fall back to a single object (P0-5).
		containers, err := parseComposePs(output)
		if err != nil {
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

		reports = append(reports, domain.ApplicationHealth{
			ApplicationID: app.ApplicationID,
			Status:        aggregateContainerHealth(containers),
		})
	}

	emit(reports)

	return nil
}

// parseComposePs parses `docker compose ps --format json` output, which is a
// JSON array for multi-service apps and a single object for one-service apps.
func parseComposePs(output string) ([]docker.Container, error) {
	var many []docker.Container
	if err := json.Unmarshal([]byte(output), &many); err == nil {
		return many, nil
	}

	var one docker.Container
	if err := json.Unmarshal([]byte(output), &one); err != nil {
		return nil, err
	}

	return []docker.Container{one}, nil
}

// aggregateContainerHealth collapses a slice of container states into a single
// app-level status: any failed container fails the app; all running = running;
// anything still starting = starting; otherwise unknown.
func aggregateContainerHealth(containers []docker.Container) domain.ApplicationStatus {
	if len(containers) == 0 {
		return domain.AppStatusUnknown
	}

	hasStarting := false
	for _, c := range containers {
		switch c.State {
		case "running":
			if c.Health == "unhealthy" {
				return domain.AppStatusFailed
			}
			if c.Health == "starting" {
				hasStarting = true
			}
		case "restarting":
			hasStarting = true
		case "exited":
			if c.ExitCode != 0 {
				return domain.AppStatusFailed
			}
		case "dead":
			return domain.AppStatusFailed
		case "paused":
			return domain.AppStatusUnknown
		default:
			// Unknown states (e.g. "created") don't fail the app, but mean
			// it isn't fully up yet.
			hasStarting = true
		}
	}

	if hasStarting {
		return domain.AppStatusStarting
	}

	return domain.AppStatusRunning
}

// waitForAppRunning polls `compose ps` until the app reaches a terminal state
// (P1-9: post-deploy health gate). A deploy that builds and recreates cleanly
// but whose container crashes on boot must be marked FAILED, not success —
// otherwise the control plane flips the app to "running" while it's dead.
// Returns nil when the app is Running, an error when it is Failed/Dead or the
// deadline passes with the app still starting.
func (e *Executor) waitForAppRunning(
	ctx context.Context,
	workDir string,
	appKey string,
	emit EmitHandler,
	action domain.LogAction,
	step domain.LogStep,
	timeout time.Duration,
) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		output, err := e.docker.Cmd(ctx, workDir, []string{"compose", "ps", "--format", "json"})
		if err != nil {
			// compose ps can fail transiently while containers are created.
			if time.Now().After(deadline) {
				e.logFatalHandler(
					fmt.Sprintf("app %s did not become ready: compose ps failed: %v", appKey, err),
					emit, action, step,
				)
				return fmt.Errorf("app %s did not become ready: %w", appKey, err)
			}
			time.Sleep(500 * time.Millisecond)
			continue
		}

		if output != "" {
			if containers, parseErr := parseComposePs(output); parseErr == nil {
				status := aggregateContainerHealth(containers)
				switch status {
				case domain.AppStatusRunning:
					return nil
				case domain.AppStatusFailed:
					e.logFatalHandler(
						fmt.Sprintf("app %s crashed on boot (health gate failed)", appKey),
						emit, action, step,
					)
					return fmt.Errorf("app %s crashed on boot (health gate failed)", appKey)
				}
			}
		}

		if time.Now().After(deadline) {
			e.logFatalHandler(
				fmt.Sprintf("app %s did not become ready within %s (health gate timeout)", appKey, timeout),
				emit, action, step,
			)
			return fmt.Errorf("app %s did not become ready within %s", appKey, timeout)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
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
		// CRITICAL (P0-1): a failed build must abort the deploy. Falling
		// through used to run `compose down` on the RUNNING app and then
		// `compose up` a broken/absent image — a bad build took the old,
		// working app down. Return the error so the job is marked failed
		// and the running stack is left untouched.
		return err
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

	// Docker compose up — in-place recreate (P0-3: zero-downtime). The
	// post-deploy health gate below (P1-9) then waits for the app to
	// actually come up before the job reports success — a build that
	// recreates cleanly but crashes on boot must flip the app to failed,
	// not optimistically claim "running".
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

	// P1-9: post-deploy health gate — only report success once the app is
	// actually running, not merely "compose up returned 0".
	if err := e.waitForAppRunning(ctx, workDir, payload.AppKey, emit, action, domain.StepDockerHealthCheck, healthGateTimeout); err != nil {
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

// rollbackApp re-deploys the stack using a previously built image tag (P0-4).
// The tag was recorded from the last successful deploy, so the image exists in
// the local docker daemon. We rewrite the env file so APP_IMAGE points at the
// old tag, then recreate in place — same zero-downtime path as a normal deploy.
func (e *Executor) rollbackApp(ctx context.Context, job *domain.Job, emit EmitHandler) error {
	var payload domain.AppRollbackPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return err
	}

	workDir := e.getAppWorkDir(payload.AppKey)
	action := domain.ActionAppDeploy
	step := domain.StepDockerStart

	if payload.ImageTag == "" {
		err := fmt.Errorf("rollback image tag is empty")
		e.logFatalHandler(err.Error(), emit, action, step)
		return err
	}

	// Point the stack at the previous image and reuse the app's current env.
	envVars := make(map[string]string)
	if payload.EnvVars != nil {
		for k, v := range payload.EnvVars {
			envVars[k] = v
		}
	}
	envVars["APP_IMAGE"] = payload.ImageTag
	envVars["APP_CONTAINER_NAME"] = payload.AppKey

	if err := e.docker.WriteEnvFile(workDir, envVars); err != nil {
		e.logFatalHandler(
			fmt.Sprintf("failed to write rollback env, %s", err.Error()),
			emit,
			action,
			step,
		)
		return err
	}

	// Recreate with the previous image. Health-gated like a deploy (P1-9):
	// the rollback only succeeds once the stack is actually running again.
	if _, err := e.docker.Cmd(ctx, workDir, []string{"compose", "up", "-d", "--force-recreate"}, e.logStreamHandler(
		emit,
		action,
		step,
	)); err != nil {
		e.logFatalHandler(
			fmt.Sprintf("failed to run docker compose up for rollback, %s", err.Error()),
			emit,
			action,
			step,
		)
		return err
	}

	if err := e.waitForAppRunning(ctx, workDir, payload.AppKey, emit, action, domain.StepDockerHealthCheck, healthGateTimeout); err != nil {
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
		return err
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
		return err
	}

	return nil
}
