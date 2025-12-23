// Package executor
package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"horizonx-server/internal/agent/docker"
	"horizonx-server/internal/agent/git"
	"horizonx-server/internal/domain"
	"horizonx-server/internal/logger"
)

type Executor struct {
	log    logger.Logger
	docker *docker.Manager
	git    *git.Manager
}

type ExecuteHandler struct {
	SendLog        func(string)
	SendCommitInfo func(hash string, message string)
}

func NewExecutor(log logger.Logger, workDir string) *Executor {
	return &Executor{
		log:    log,
		docker: docker.NewManager(log, workDir),
		git:    git.NewManager(log),
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

	if err := e.docker.Initialize(); err != nil {
		return err
	}

	e.log.Info("job executor initialized successfully")
	return nil
}

func (e *Executor) Execute(ctx context.Context, job *domain.Job, handler *ExecuteHandler) (string, error) {
	e.log.Info("executing job", "id", job.ID, "type", job.JobType)

	switch job.JobType {
	case domain.JobTypeDeployApp:
		return e.executeDeployApp(ctx, job, handler)
	case domain.JobTypeStartApp:
		return e.executeStartApp(ctx, job)
	case domain.JobTypeStopApp:
		return e.executeStopApp(ctx, job)
	case domain.JobTypeRestartApp:
		return e.executeRestartApp(ctx, job)
	default:
		return "", fmt.Errorf("unknown job type: %s", job.JobType)
	}
}

func (e *Executor) executeDeployApp(ctx context.Context, job *domain.Job, handler *ExecuteHandler) (string, error) {
	if handler == nil {
		return "", fmt.Errorf("missing required deployment handler")
	}

	var payload domain.DeployAppPayload
	if err := json.Unmarshal(job.CommandPayload, &payload); err != nil {
		return "", err
	}

	appID := payload.ApplicationID
	var logs strings.Builder

	// Start
	deployingApplicationLog := "=== Deploying Application ===\n\n"
	logs.WriteString(deployingApplicationLog)
	handler.SendLog(deployingApplicationLog)

	// Fetching source code
	fetchingSourceCodeLog := "Step 1: Fetching source code...\n"
	logs.WriteString(fetchingSourceCodeLog)
	handler.SendLog(fetchingSourceCodeLog)

	repoDir := e.docker.GetAppDir(appID)
	var gitOutput string
	var err error

	if e.git.IsGitRepo(repoDir) {
		e.log.Debug("repository exists, pulling latest", "app_id", appID)
		gitOutput, err = e.git.Pull(ctx, repoDir, payload.Branch)
	} else {
		e.log.Debug("cloning repository", "app_id", appID, "repo", payload.RepoURL)
		gitOutput, err = e.git.Clone(ctx, payload.RepoURL, payload.Branch, repoDir)
	}

	if err != nil {
		gitOperationFailedLog := fmt.Sprintf("❌ Git operation failed: %v\n", err)
		logs.WriteString(gitOperationFailedLog)
		handler.SendLog(gitOperationFailedLog)
		return logs.String(), err
	}

	sourceCodeReadyLog := fmt.Sprintf("%s ✓ Source code ready\n\n", gitOutput)
	logs.WriteString(sourceCodeReadyLog)
	handler.SendLog(sourceCodeReadyLog)

	// Get commit info
	commitHash, _ := e.git.GetCurrentCommit(ctx, repoDir)
	commitMsg, _ := e.git.GetCommitMessage(ctx, repoDir)
	if commitHash != "" {
		commitHashLog := fmt.Sprintf("Commit: %s\n", commitHash[:8])
		commitMessageLog := fmt.Sprintf("Message: %s\n\n", commitMsg)

		logs.WriteString(commitHashLog)
		logs.WriteString(commitMessageLog)

		handler.SendLog(commitHashLog)
		handler.SendLog(commitMessageLog)

		handler.SendCommitInfo(commitHash[:8], commitMsg)
	}

	// Validating compose file
	validatingComposeFileLog := "Step 2: Validating compose file...\n"
	logs.WriteString(validatingComposeFileLog)
	handler.SendLog(validatingComposeFileLog)
	if err := e.docker.ValidateDockerComposeFile(appID); err != nil {
		failedToValidateComposeFileLog := fmt.Sprintf("❌ Failed to validate compose file: %v\n", err)
		logs.WriteString(failedToValidateComposeFileLog)
		handler.SendLog(failedToValidateComposeFileLog)
		return logs.String(), err
	}
	composeFileValidatedLog := "✓ Compose file validated\n\n"
	logs.WriteString(composeFileValidatedLog)
	handler.SendLog(composeFileValidatedLog)

	// Writing environment variables
	if len(payload.EnvVars) > 0 {
		writingEnvVarsLog := fmt.Sprintf("Step 3: Writing environment variables (%d vars)...\n", len(payload.EnvVars))
		logs.WriteString(writingEnvVarsLog)
		handler.SendLog(writingEnvVarsLog)
		if err := e.docker.WriteEnvFile(appID, payload.EnvVars); err != nil {
			failedToWriteEnvFileLog := fmt.Sprintf("❌ Failed to write env file: %v\n", err)
			logs.WriteString(failedToWriteEnvFileLog)
			handler.SendLog(failedToWriteEnvFileLog)
			return logs.String(), err
		}
		envConfiguredLog := "✓ Environment configured\n\n"
		logs.WriteString(envConfiguredLog)
		handler.SendLog(envConfiguredLog)
	}

	// Docker compose down
	stoppingExistingContainersLog := "Step 4: Stopping existing containers...\n"
	logs.WriteString(stoppingExistingContainersLog)
	handler.SendLog(stoppingExistingContainersLog)
	stopOutput, err := e.docker.ComposeDown(ctx, appID, false)
	if err != nil {
		e.log.Warn("failed to stop existing containers (may not exist)", "error", err)
	}
	if stopOutput != "" {
		logs.WriteString(stopOutput + "\n")
		handler.SendLog(stopOutput + "\n")
	}

	// Docker compose up
	buildingAndStartingContainersLog := "Step 5: Building and starting containers...\n"
	logs.WriteString(buildingAndStartingContainersLog)
	handler.SendLog(buildingAndStartingContainersLog)
	upOutput, err := e.docker.ComposeUp(ctx, appID, true, true)
	if upOutput != "" {
		logs.WriteString(upOutput + "\n")
		handler.SendLog(upOutput + "\n")
	}
	if err != nil {
		failLog := fmt.Sprintf("❌ Docker compose up failed: %v\n", err)
		logs.WriteString(failLog)
		handler.SendLog(failLog)
		return logs.String(), err
	}
	logs.WriteString("✓ Containers started\n\n")
	handler.SendLog("✓ Containers started\n\n")

	// Verifying deployment
	verifyingDeploymentLog := "Step 6: Verifying deployment...\n"
	logs.WriteString(verifyingDeploymentLog)
	handler.SendLog(verifyingDeploymentLog)
	psOutput, err := e.docker.ComposePs(ctx, appID)
	if psOutput != "" {
		logs.WriteString(psOutput + "\n")
		handler.SendLog(psOutput + "\n")
	}
	if err != nil {
		warnLog := fmt.Sprintf("⚠ Failed to check container status: %v\n", err)
		logs.WriteString(warnLog)
		handler.SendLog(warnLog)
	}

	// Finish
	deploymentCompleteLog := "\n=== Deployment Complete ===\n"
	logs.WriteString(deploymentCompleteLog)
	handler.SendLog(deploymentCompleteLog)
	return logs.String(), nil
}

func (e *Executor) executeStartApp(ctx context.Context, job *domain.Job) (string, error) {
	var payload domain.StartAppPayload
	if err := json.Unmarshal(job.CommandPayload, &payload); err != nil {
		return "", err
	}

	appID := payload.ApplicationID
	e.log.Info("starting application", "app_id", appID)

	output, err := e.docker.ComposeStart(ctx, appID)
	if err != nil {
		return output, fmt.Errorf("failed to start application: %w", err)
	}

	return fmt.Sprintf("Application %d started successfully\n%s", appID, output), nil
}

func (e *Executor) executeStopApp(ctx context.Context, job *domain.Job) (string, error) {
	var payload domain.StartAppPayload
	if err := json.Unmarshal(job.CommandPayload, &payload); err != nil {
		return "", err
	}

	appID := payload.ApplicationID
	e.log.Info("stopping application", "app_id", appID)

	output, err := e.docker.ComposeStop(ctx, appID)
	if err != nil {
		return output, fmt.Errorf("failed to stop application: %w", err)
	}

	return fmt.Sprintf("Application %d stopped successfully\n%s", appID, output), nil
}

func (e *Executor) executeRestartApp(ctx context.Context, job *domain.Job) (string, error) {
	var payload domain.RestartAppPayload
	if err := json.Unmarshal(job.CommandPayload, &payload); err != nil {
		return "", err
	}

	appID := payload.ApplicationID
	e.log.Info("restarting application", "app_id", appID)

	output, err := e.docker.ComposeRestart(ctx, appID)
	if err != nil {
		return output, fmt.Errorf("failed to restart application: %w", err)
	}

	return fmt.Sprintf("Application %d restarted successfully\n%s", appID, output), nil
}
