package app

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// setExecCompose swaps the compose executor for the duration of a test.
func setExecCompose(f func(args ...string) (string, error)) func() {
	old := execCompose
	execCompose = f
	return func() { execCompose = old }
}

// errExecFailure is the error fake compose executors return to simulate a
// failing docker command.
var errExecFailure = errors.New("exec failed")

func TestInstallServerGenerateOnly(t *testing.T) {
	restore := setExecCompose(func(args ...string) (string, error) {
		t.Fatalf("generate-only must not call docker compose (got %v)", args)
		return "", nil
	})
	defer restore()

	dir := t.TempDir()
	err := RunInstallServer(InstallServerOptions{Dir: dir, Host: "203.0.113.10", GenerateOnly: true})
	if err != nil {
		t.Fatalf("generate-only install: %v", err)
	}
	for _, f := range []string{"docker-compose.yml", "server/docker-compose.yml", "dashboard/docker-compose.yml", ".env"} {
		if _, e := os.Stat(filepath.Join(dir, f)); e != nil {
			t.Errorf("missing %s after generate-only: %v", f, e)
		}
	}
}

func TestInstallServerApplyHealthCheck(t *testing.T) {
	// Fake docker: preflight OK, compose config + up succeed, health passes.
	var composeCalls []string
	restoreCompose := setExecCompose(func(args ...string) (string, error) {
		composeCalls = append(composeCalls, strings.Join(args, " "))
		return "", nil
	})
	defer restoreCompose()

	oldPre := preflightFn
	preflightFn = func() PreflightResult {
		return PreflightResult{DockerAccess: true, ComposeOK: true, ComposeVersion: "5.3.1", PortsFree: true}
	}
	defer func() { preflightFn = oldPre }()

	oldPoll := pollHealthFn
	pollHealthFn = func(url string) bool { return true }
	defer func() { pollHealthFn = oldPoll }()

	dir := t.TempDir()
	err := RunInstallServer(InstallServerOptions{Dir: dir, Host: "203.0.113.10"})
	if err != nil {
		t.Fatalf("apply install: %v", err)
	}
	joined := strings.Join(composeCalls, "\n")
	if !strings.Contains(joined, "config") || !strings.Contains(joined, "up") {
		t.Errorf("expected compose config + up calls, got:\n%s", joined)
	}
}

func TestInstallServerApplyFailsOnBadCompose(t *testing.T) {
	restore := setExecCompose(func(args ...string) (string, error) {
		// Production calls: execCompose("-f", path, "config", "--quiet").
		for _, a := range args {
			if a == "config" {
				return "invalid compose", errExecFailure
			}
		}
		return "", nil
	})
	defer restore()

	oldPre := preflightFn
	preflightFn = func() PreflightResult {
		return PreflightResult{DockerAccess: true, ComposeOK: true, ComposeVersion: "5.3.1", PortsFree: true}
	}
	defer func() { preflightFn = oldPre }()

	dir := t.TempDir()
	err := RunInstallServer(InstallServerOptions{Dir: dir, Host: "203.0.113.10"})
	if err == nil {
		t.Fatal("expected error on invalid compose")
	}
	if !strings.Contains(err.Error(), "compose config invalid") {
		t.Errorf("expected actionable compose error, got: %v", err)
	}
}

func TestPollHealth(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ok.Close()
	if !pollHealth(ok.URL, 3*time.Second) {
		t.Error("pollHealth should succeed when endpoint returns 200")
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer bad.Close()
	if pollHealth(bad.URL, 1*time.Second) {
		t.Error("pollHealth should fail when endpoint returns 500")
	}

	gone := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := gone.URL
	gone.Close()
	if pollHealth(url, 500*time.Millisecond) {
		t.Error("pollHealth should fail when endpoint is unreachable")
	}
}
