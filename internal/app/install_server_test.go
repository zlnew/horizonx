package app

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
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

	// Dashboard step must not hit the real GitHub API in tests.
	oldRel := latestDashboardRelease
	latestDashboardRelease = func() (dashboardRelease, error) {
		return dashboardRelease{}, errors.New("network disabled in test")
	}
	defer func() { latestDashboardRelease = oldRel }()

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

func TestInstallServerUpgradeAllowsBusyPorts(t *testing.T) {
	// Regression: `install server` on an existing bubble (upgrade path) must
	// not fail when the signature ports are busy — the bubble owns them.
	// (v0.3.3→v0.3.4 fix; Maul hit this re-running install over a live bubble.)
	var calls [][]string
	restore := setExecCompose(func(args ...string) (string, error) {
		calls = append(calls, args)
		return "", nil
	})
	defer restore()

	oldPre := preflightFn
	preflightFn = func() PreflightResult {
		return PreflightResult{DockerAccess: true, ComposeOK: true, ComposeVersion: "5.3.1", PortsFree: false}
	}
	defer func() { preflightFn = oldPre }()

	oldPoll := pollHealthFn
	pollHealthFn = func(url string) bool { return true }
	defer func() { pollHealthFn = oldPoll }()

	oldRel := latestDashboardRelease
	latestDashboardRelease = func() (dashboardRelease, error) {
		return dashboardRelease{}, errors.New("network disabled in test")
	}
	defer func() { latestDashboardRelease = oldRel }()

	dir := t.TempDir()
	// Simulate an existing bubble: .env present (upgrade, not first install).
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("DATABASE_URL=postgres://x\n"), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	err := RunInstallServer(InstallServerOptions{Dir: dir, Host: "203.0.113.10"})
	if err != nil {
		t.Fatalf("upgrade with busy ports must succeed, got: %v", err)
	}

	// The upgrade path must force-recreate ONLY the server container so the
	// boot-time admin auto-seed re-runs (2026-08-04: admin missing after
	// DELETE + re-run install — plain `up -d` never restarts a running
	// container, so the seed never re-ran).
	forceRecreated := false
	for _, c := range calls {
		for _, a := range c {
			if a == "--force-recreate" {
				forceRecreated = true
			}
		}
	}
	if !forceRecreated {
		t.Errorf("upgrade path must --force-recreate the server container; calls: %v", calls)
	}
	// First-install style plain up (postgres/redis/server) still happens.
	plainUp := false
	for _, c := range calls {
		if len(c) >= 4 && c[2] == "up" && c[3] == "-d" && !contains(c, "--force-recreate") {
			plainUp = true
		}
	}
	if !plainUp {
		t.Errorf("expected a plain `up -d postgres redis server` call; calls: %v", calls)
	}
}

// contains reports whether s is present in args.
func contains(args []string, s string) bool {
	for _, a := range args {
		if a == s {
			return true
		}
	}
	return false
}

func TestInstallServerFirstInstallFailsOnBusyPorts(t *testing.T) {
	// First install (no .env) + busy ports → still an error.
	oldPre := preflightFn
	preflightFn = func() PreflightResult {
		return PreflightResult{DockerAccess: true, ComposeOK: true, ComposeVersion: "5.3.1", PortsFree: false}
	}
	defer func() { preflightFn = oldPre }()

	dir := t.TempDir()
	err := RunInstallServer(InstallServerOptions{Dir: dir, Host: "203.0.113.10"})
	if err == nil {
		t.Fatal("first install with busy ports must fail")
	}
	if !strings.Contains(err.Error(), "signature ports") {
		t.Errorf("expected signature-ports error, got: %v", err)
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

// TestInstallServerAuthFailureDiagnosis pins the stale-volume remediation:
// when the health poll fails AND the server log shows the postgres auth
// signature, the error must tell the user to reset volumes (not just dump a
// generic "did not become healthy").
func TestInstallServerAuthFailureDiagnosis(t *testing.T) {
	restore := setExecCompose(func(args ...string) (string, error) {
		// config --quiet OK, up OK, but `compose logs server` shows the auth
		// crash-loop signature.
		for _, a := range args {
			if a == "logs" {
				return "server-1  | FATAL: auto-migrate: password authentication failed for user \"postgres\" (SQLSTATE 28P01)\n", nil
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

	oldPoll := pollHealthFn
	pollHealthFn = func(url string) bool { return false }
	defer func() { pollHealthFn = oldPoll }()

	oldRel := latestDashboardRelease
	latestDashboardRelease = func() (dashboardRelease, error) {
		return dashboardRelease{}, errors.New("network disabled in test")
	}
	defer func() { latestDashboardRelease = oldRel }()

	dir := t.TempDir()
	err := RunInstallServer(InstallServerOptions{Dir: dir, Host: "203.0.113.10"})
	if err == nil {
		t.Fatal("expected error when health never becomes OK")
	}
	if !strings.Contains(err.Error(), "password") || !strings.Contains(err.Error(), "down -v") {
		t.Errorf("expected stale-volume remediation, got: %v", err)
	}
}

func TestFindDashboardTarball(t *testing.T) {
	dir := t.TempDir()
	if got := findDashboardTarball(dir); got != "" {
		t.Fatalf("expected no tarball in empty dir, got %q", got)
	}
	path := filepath.Join(dir, "horizonx-dashboard-0.2.0.tar.gz")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := findDashboardTarball(dir); got != path {
		t.Fatalf("expected %q, got %q", path, got)
	}
}

func TestVerifySHA256SUMS(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "horizonx-dashboard-v0.3.0-image.tar.gz")
	content := []byte("fake image tarball bytes")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := fmt.Sprintf("%x", sha256.Sum256(content))
	sums := sum + "  " + filepath.Base(path) + "\n"

	if err := verifySHA256SUMS(path, sums); err != nil {
		t.Errorf("expected match, got: %v", err)
	}
	// Wrong hash must fail.
	if err := verifySHA256SUMS(path, strings.Repeat("0", 64)+"  "+filepath.Base(path)+"\n"); err == nil {
		t.Error("expected checksum mismatch error")
	}
	// Missing entry must fail.
	if err := verifySHA256SUMS(path, "deadbeef  some-other-file.tar.gz\n"); err == nil {
		t.Error("expected missing-entry error")
	}
}

func TestFetchDashboardTarballDownloadsAndVerifies(t *testing.T) {
	// A fake "release server": serves the tarball + SHA256SUMS.
	content := []byte("fake dashboard image")
	sum := fmt.Sprintf("%x", sha256.Sum256(content))
	tarballName := "horizonx-dashboard-v0.3.0-image.tar.gz"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/" + tarballName:
			_, _ = w.Write(content)
		case "/SHA256SUMS":
			fmt.Fprintf(w, "%s  %s\n", sum, tarballName)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	oldRel := latestDashboardRelease
	latestDashboardRelease = func() (dashboardRelease, error) {
		return dashboardRelease{
			Tag:        "v0.3.0",
			TarballURL: srv.URL + "/" + tarballName,
			SHAURL:     srv.URL + "/SHA256SUMS",
		}, nil
	}
	defer func() { latestDashboardRelease = oldRel }()

	dir := t.TempDir()
	got, err := fetchDashboardTarball(dir)
	if err != nil {
		t.Fatalf("fetchDashboardTarball: %v", err)
	}
	if filepath.Base(got) != tarballName {
		t.Errorf("expected %s, got %s", tarballName, filepath.Base(got))
	}
	b, err := os.ReadFile(got)
	if err != nil || string(b) != string(content) {
		t.Errorf("downloaded content mismatch (err=%v)", err)
	}
	// SHA256SUMS companion should also be cached in the dir.
	if _, err := os.Stat(filepath.Join(dir, "SHA256SUMS")); err != nil {
		t.Errorf("SHA256SUMS not cached: %v", err)
	}
}

func TestFetchDashboardTarballReusesVerifiedLocal(t *testing.T) {
	// Pre-place a tarball that matches the release checksum → no re-download.
	dir := t.TempDir()
	content := []byte("existing dashboard image")
	sum := fmt.Sprintf("%x", sha256.Sum256(content))
	tarballName := "horizonx-dashboard-v0.3.0-image.tar.gz"
	if err := os.WriteFile(filepath.Join(dir, tarballName), content, 0o644); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/SHA256SUMS" {
			fmt.Fprintf(w, "%s  %s\n", sum, tarballName)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	oldRel := latestDashboardRelease
	latestDashboardRelease = func() (dashboardRelease, error) {
		return dashboardRelease{Tag: "v0.3.0", TarballURL: srv.URL + "/nope", SHAURL: srv.URL + "/SHA256SUMS"}, nil
	}
	defer func() { latestDashboardRelease = oldRel }()

	got, err := fetchDashboardTarball(dir)
	if err != nil {
		t.Fatalf("fetchDashboardTarball: %v", err)
	}
	if got != filepath.Join(dir, tarballName) {
		t.Errorf("expected reuse of %s, got %s", tarballName, got)
	}
}

func TestFetchDashboardTarballFallsBackToLocalWhenAPIUnreachable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "horizonx-dashboard-old.tar.gz")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	oldRel := latestDashboardRelease
	latestDashboardRelease = func() (dashboardRelease, error) {
		return dashboardRelease{}, errors.New("api down")
	}
	defer func() { latestDashboardRelease = oldRel }()

	got, err := fetchDashboardTarball(dir)
	if err != nil {
		t.Fatalf("expected local fallback, got error: %v", err)
	}
	if got != path {
		t.Errorf("expected %s, got %s", path, got)
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

func TestHasBubbleVolumes(t *testing.T) {
	origRun := runCommand
	runCommand = func(ctx context.Context, name string, args ...string) (string, error) {
		// Real docker compose prefixes with the project name: for a bubble at
		// /opt/horizonx the project is `horizonx`, so the volume is
		// `horizonx_horizonx_pgdata`.
		return "horizonx_horizonx_pgdata\nother-volume\n", nil
	}
	defer func() { runCommand = origRun }()

	if !hasBubbleVolumes("/opt/horizonx") {
		t.Error("hasBubbleVolumes should detect horizonx_pgdata")
	}
}

func TestHasBubbleVolumesBareName(t *testing.T) {
	origRun := runCommand
	runCommand = func(ctx context.Context, name string, args ...string) (string, error) {
		// A bare `horizonx_pgdata` (hand-created or old layout) still counts.
		return "horizonx_pgdata\n", nil
	}
	defer func() { runCommand = origRun }()

	if !hasBubbleVolumes("/opt/horizonx") {
		t.Error("hasBubbleVolumes should detect bare horizonx_pgdata")
	}
}

func TestHasBubbleVolumesNoVolumes(t *testing.T) {
	origRun := runCommand
	runCommand = func(ctx context.Context, name string, args ...string) (string, error) {
		return "unrelated-volume\n", nil
	}
	defer func() { runCommand = origRun }()

	if hasBubbleVolumes("/opt/horizonx") {
		t.Error("hasBubbleVolumes should be false when only unrelated volumes exist")
	}
}

func TestHasBubbleVolumesProjectPrefixed(t *testing.T) {
	origRun := runCommand
	runCommand = func(ctx context.Context, name string, args ...string) (string, error) {
		// Real docker prefixes with the compose project name
		// (e.g. bubble dir /tmp/test → project `test` → `test_horizonx_pgdata`).
		return "test_horizonx_pgdata\n", nil
	}
	defer func() { runCommand = origRun }()

	if !hasBubbleVolumes("/tmp/test") {
		t.Error("hasBubbleVolumes should detect project-prefixed horizonx volumes")
	}
}
