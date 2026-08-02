package executor

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"horizonx/internal/agent/command"
	"horizonx/internal/domain"
	"horizonx/internal/logger"
)

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

type fakeDocker struct {
	cmdCalls  []string // "build", "compose up", etc.
	cmdErrs   map[string]error
	psOutput  string
	psErr     error
	envWrites []map[string]string
}

func (f *fakeDocker) Cmd(_ context.Context, _ string, args []string, _ ...command.StreamHandler) (string, error) {
	key := strings.Join(args, " ")
	f.cmdCalls = append(f.cmdCalls, key)
	if f.cmdErrs != nil {
		if err := f.cmdErrs[key]; err != nil {
			return "", err
		}
	}
	if len(args) >= 2 && args[0] == "compose" && args[1] == "ps" {
		if f.psOutput != "" || f.psErr != nil {
			return f.psOutput, f.psErr
		}
		// Default: one healthy running container so the post-deploy health
		// gate (P1-9) passes.
		return `[{"ID":"a","Name":"demo-app-1","State":"running","Health":"","ExitCode":0}]`, nil
	}
	return "ok", nil
}

func (f *fakeDocker) GetDockerComposeFile(string) (string, error)   { return "compose.yml", nil }
func (f *fakeDocker) GetDockerfile(string) (string, error)          { return "Dockerfile", nil }
func (f *fakeDocker) IsDockerInstalled() bool                       { return true }
func (f *fakeDocker) IsDockerComposeAvailable() bool                { return true }

func (f *fakeDocker) WriteEnvFile(_ string, env map[string]string) error {
	f.envWrites = append(f.envWrites, env)
	return nil
}

type fakeGit struct {
	commit string
	msg    string
}

func (g *fakeGit) CloneOrPull(context.Context, string, string, string, ...command.StreamHandler) (string, error) {
	return "cloned", nil
}

func (g *fakeGit) GetCurrentCommit(context.Context, string, ...command.StreamHandler) (string, error) {
	return g.commit, nil
}

func (g *fakeGit) GetCommitMessage(context.Context, string, ...command.StreamHandler) (string, error) {
	return g.msg, nil
}

func (g *fakeGit) IsGitInstalled() bool { return true }

func noopLogger() logger.Logger {
	return noopLog{}
}

type noopLog struct{}

func (noopLog) Debug(string, ...any) {}
func (noopLog) Info(string, ...any)  {}
func (noopLog) Warn(string, ...any)  {}
func (noopLog) Error(string, ...any) {}

func deployJob(t *testing.T) *domain.Job {
	t.Helper()
	payload, _ := json.Marshal(domain.AppDeployPayload{
		ApplicationID: 1,
		DeploymentID:  7,
		AppKey:        "demo-app-1",
		RepoURL:       "git@example.com:demo/demo.git",
		Branch:        "main",
		EnvVars:       map[string]string{"FOO": "bar"},
	})
	deploymentID := int64(7)
	return &domain.Job{
		Type:         domain.JobTypeAppDeploy,
		DeploymentID: &deploymentID,
		Payload:      payload,
	}
}

func emitNoop(any) {}

// ---------------------------------------------------------------------------
// P0-1: build failure must abort the deploy (no compose down/up after it)
// ---------------------------------------------------------------------------

func TestDeployBuildFailureAborts(t *testing.T) {
	docker := &fakeDocker{cmdErrs: map[string]error{
		"build -t demo-app-1:0123456789abcdef0123456789abcdef01234567 -f Dockerfile .": errors.New("build exploded"),
	}}
	git := &fakeGit{commit: "0123456789abcdef0123456789abcdef01234567", msg: "test"}

	ex := NewExecutorWithDeps(docker, git, "/tmp/apps", noopLogger(), nil)

	err := ex.Execute(context.Background(), deployJob(t), emitNoop)
	if err == nil {
		t.Fatal("expected deploy to fail when the build fails")
	}

	for _, call := range docker.cmdCalls {
		if strings.Contains(call, "compose") {
			t.Fatalf("compose command executed after build failure: %q", call)
		}
	}
}

// ---------------------------------------------------------------------------
// P0-3: deploy must NOT run `compose down` (zero-downtime in-place recreate)
// ---------------------------------------------------------------------------

func TestDeployUsesInPlaceRecreateWithoutDown(t *testing.T) {
	docker := &fakeDocker{}
	git := &fakeGit{commit: "0123456789abcdef0123456789abcdef01234567", msg: "test"}

	ex := NewExecutorWithDeps(docker, git, "/tmp/apps", noopLogger(), nil)

	if err := ex.Execute(context.Background(), deployJob(t), emitNoop); err != nil {
		t.Fatalf("unexpected deploy error: %v", err)
	}

	var hasUp, hasDown, hasGate bool
	for _, call := range docker.cmdCalls {
		if strings.Contains(call, "compose up -d --force-recreate") {
			hasUp = true
		}
		if strings.Contains(call, "compose down") {
			hasDown = true
		}
		if strings.Contains(call, "compose ps --format json") {
			hasGate = true
		}
	}

	if !hasUp {
		t.Fatalf("expected compose up -d --force-recreate, got calls: %v", docker.cmdCalls)
	}
	if hasDown {
		t.Fatalf("deploy must not run compose down (zero-downtime), got: %v", docker.cmdCalls)
	}
	if !hasGate {
		t.Fatalf("deploy must run the post-deploy health gate (compose ps), got: %v", docker.cmdCalls)
	}
}

// ---------------------------------------------------------------------------
// P0-5: health check parses multi-service JSON array
// ---------------------------------------------------------------------------

func TestHealthCheckParsesArray(t *testing.T) {
	docker := &fakeDocker{psOutput: `[
		{"ID":"a","Name":"demo-app-1","State":"running","Health":"","ExitCode":0},
		{"ID":"b","Name":"demo-app-1-db","State":"running","Health":"","ExitCode":0}
	]`}
	git := &fakeGit{commit: "0123456789abcdef0123456789abcdef01234567", msg: "test"}

	ex := NewExecutorWithDeps(docker, git, "/tmp/apps", noopLogger(), nil)

	payload, _ := json.Marshal(domain.AppHealthCheckPayload{
		Applications: []domain.AppInfo{{ApplicationID: 1, AppKey: "demo-app-1"}},
	})
	job := &domain.Job{Type: domain.JobTypeAppHealthCheck, Payload: payload}

	var reports []domain.ApplicationHealth
	ex.Execute(context.Background(), job, func(evt any) {
		if r, ok := evt.([]domain.ApplicationHealth); ok {
			reports = r
		}
	})

	if len(reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(reports))
	}
	if reports[0].Status != domain.AppStatusRunning {
		t.Fatalf("expected running for healthy multi-service app, got %s", reports[0].Status)
	}
}

func TestHealthCheckFailsIfAnyContainerUnhealthy(t *testing.T) {
	docker := &fakeDocker{psOutput: `[
		{"ID":"a","Name":"demo-app-1","State":"running","Health":"","ExitCode":0},
		{"ID":"b","Name":"demo-app-1-db","State":"exited","Health":"","ExitCode":1}
	]`}
	git := &fakeGit{commit: "0123456789abcdef0123456789abcdef01234567", msg: "test"}

	ex := NewExecutorWithDeps(docker, git, "/tmp/apps", noopLogger(), nil)

	payload, _ := json.Marshal(domain.AppHealthCheckPayload{
		Applications: []domain.AppInfo{{ApplicationID: 1, AppKey: "demo-app-1"}},
	})
	job := &domain.Job{Type: domain.JobTypeAppHealthCheck, Payload: payload}

	var reports []domain.ApplicationHealth
	ex.Execute(context.Background(), job, func(evt any) {
		if r, ok := evt.([]domain.ApplicationHealth); ok {
			reports = r
		}
	})

	if reports[0].Status != domain.AppStatusFailed {
		t.Fatalf("expected failed when a container exited non-zero, got %s", reports[0].Status)
	}
}

func TestHealthCheckSingleObjectFallback(t *testing.T) {
	docker := &fakeDocker{psOutput: `{"ID":"a","Name":"demo-app-1","State":"running","Health":"","ExitCode":0}`}
	git := &fakeGit{commit: "0123456789abcdef0123456789abcdef01234567", msg: "test"}

	ex := NewExecutorWithDeps(docker, git, "/tmp/apps", noopLogger(), nil)

	payload, _ := json.Marshal(domain.AppHealthCheckPayload{
		Applications: []domain.AppInfo{{ApplicationID: 1, AppKey: "demo-app-1"}},
	})
	job := &domain.Job{Type: domain.JobTypeAppHealthCheck, Payload: payload}

	var reports []domain.ApplicationHealth
	ex.Execute(context.Background(), job, func(evt any) {
		if r, ok := evt.([]domain.ApplicationHealth); ok {
			reports = r
		}
	})

	if len(reports) != 1 || reports[0].Status != domain.AppStatusRunning {
		t.Fatalf("single-object fallback failed: %+v", reports)
	}
}

// ---------------------------------------------------------------------------
// P0-4: rollback rewrites env to the previous image and recreates in place
// ---------------------------------------------------------------------------

func TestRollbackUsesPreviousImageTag(t *testing.T) {
	docker := &fakeDocker{}
	git := &fakeGit{commit: "0123456789abcdef0123456789abcdef01234567", msg: "test"}

	ex := NewExecutorWithDeps(docker, git, "/tmp/apps", noopLogger(), nil)

	payload, _ := json.Marshal(domain.AppRollbackPayload{
		ApplicationID: 1,
		DeploymentID:  9,
		AppKey:        "demo-app-1",
		ImageTag:      "demo-app-1:deadbeef",
		EnvVars:       map[string]string{"FOO": "bar"},
	})
	job := &domain.Job{Type: domain.JobTypeAppRollback, Payload: payload}

	if err := ex.Execute(context.Background(), job, emitNoop); err != nil {
		t.Fatalf("unexpected rollback error: %v", err)
	}

	if len(docker.envWrites) == 0 {
		t.Fatal("rollback did not write env file")
	}
	lastEnv := docker.envWrites[len(docker.envWrites)-1]
	if lastEnv["APP_IMAGE"] != "demo-app-1:deadbeef" {
		t.Fatalf("expected APP_IMAGE=demo-app-1:deadbeef, got %q", lastEnv["APP_IMAGE"])
	}
	if lastEnv["FOO"] != "bar" {
		t.Fatalf("rollback dropped app env vars: %+v", lastEnv)
	}

	found := false
	for _, call := range docker.cmdCalls {
		if strings.Contains(call, "compose up -d --force-recreate") {
			found = true
		}
	}
	if !found {
		t.Fatalf("rollback did not run compose up, calls: %v", docker.cmdCalls)
	}
}

func TestRollbackRejectsEmptyImageTag(t *testing.T) {
	docker := &fakeDocker{}
	git := &fakeGit{commit: "abc123", msg: "test"}

	ex := NewExecutorWithDeps(docker, git, "/tmp/apps", noopLogger(), nil)

	payload, _ := json.Marshal(domain.AppRollbackPayload{
		ApplicationID: 1,
		DeploymentID:  9,
		AppKey:        "demo-app-1",
	})
	job := &domain.Job{Type: domain.JobTypeAppRollback, Payload: payload}

	if err := ex.Execute(context.Background(), job, emitNoop); err == nil {
		t.Fatal("expected rollback to fail with empty image tag")
	}
	if len(docker.cmdCalls) != 0 {
		t.Fatalf("rollback must not run docker when image tag is empty: %v", docker.cmdCalls)
	}
}

// ---------------------------------------------------------------------------
// P1-9: post-deploy health gate — deploy must FAIL when the app crashes on boot
// ---------------------------------------------------------------------------

func TestDeployFailsWhenAppCrashesOnBoot(t *testing.T) {
	docker := &fakeDocker{psOutput: `[{"ID":"a","Name":"demo-app-1","State":"exited","Health":"","ExitCode":1}]`}
	git := &fakeGit{commit: "0123456789abcdef0123456789abcdef01234567", msg: "test"}

	ex := NewExecutorWithDeps(docker, git, "/tmp/apps", noopLogger(), nil)

	err := ex.Execute(context.Background(), deployJob(t), emitNoop)
	if err == nil {
		t.Fatal("expected deploy to fail when the app crashes on boot")
	}
	if !strings.Contains(err.Error(), "crashed on boot") {
		t.Fatalf("expected crash-on-boot error, got: %v", err)
	}
}

func TestDeployHealthGateTimesOutOnUnknownState(t *testing.T) {
	// Container stays "created" (not running, not failed) — the gate must
	// time out and fail the deploy rather than hang forever.
	docker := &fakeDocker{psOutput: `[{"ID":"a","Name":"demo-app-1","State":"created","Health":"","ExitCode":0}]`}
	git := &fakeGit{commit: "0123456789abcdef0123456789abcdef01234567", msg: "test"}

	ex := NewExecutorWithDeps(docker, git, "/tmp/apps", noopLogger(), nil)

	workDir := ex.getAppWorkDir("demo-app-1")
	err := ex.waitForAppRunning(context.Background(), workDir, "demo-app-1", emitNoop, domain.ActionAppDeploy, domain.StepDockerHealthCheck, 200*time.Millisecond)
	if err == nil {
		t.Fatal("expected health gate to time out for app stuck in 'created' state")
	}
	if !strings.Contains(err.Error(), "did not become ready") {
		t.Fatalf("expected timeout error, got: %v", err)
	}
}
