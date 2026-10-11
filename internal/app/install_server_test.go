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

	"aead.dev/minisign"
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

func TestDashboardCacheDirDefault(t *testing.T) {
	// The default instance root (/opt/horizonx) must cache the dashboard
	// tarball OUTSIDE the instance — /var/cache/horizonx/dashboard — so the
	// transport artifact never mixes with runtime state or backups.
	if got := dashboardCacheDir(instanceDir); got != "/var/cache/horizonx/dashboard" {
		t.Errorf("dashboardCacheDir(/opt/horizonx) = %q, want /var/cache/horizonx/dashboard", got)
	}
	if got := dashboardCacheDir(""); got != "/var/cache/horizonx/dashboard" {
		t.Errorf("dashboardCacheDir(\"\") = %q, want /var/cache/horizonx/dashboard", got)
	}
}

func TestDashboardCacheDirCustomRoot(t *testing.T) {
	// Custom instance roots (dev/tests via --dir) keep the cache under the
	// instance so tests stay self-contained and don't touch /var/cache.
	root := t.TempDir()
	got := dashboardCacheDir(root)
	want := filepath.Join(root, "dashboard")
	if got != want {
		t.Errorf("dashboardCacheDir(custom) = %q, want %q", got, want)
	}
}

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
	// Regression: `install server` on an existing instance (upgrade path) must
	// not fail when the signature ports are busy — the instance owns them.
	// (v0.3.3→v0.3.4 fix; Maul hit this re-running install over a live instance.)
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
	// Simulate an existing instance: .env present (upgrade, not first install).
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("DATABASE_URL=postgres://x\n"), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	err := RunInstallServer(InstallServerOptions{Dir: dir, Host: "203.0.113.10"})
	if err != nil {
		t.Fatalf("upgrade with busy ports must succeed, got: %v", err)
	}

	// The upgrade path must rebuild + force-recreate the server container so
	// a stale horizonx:latest image (pinned to whatever release it was first
	// built from) never keeps running, and so the boot-time admin auto-seed
	// re-runs after a DELETE (2026-08-04: "fresh install but admin never
	// auto-seeded" — `up -d` never rebuilds an existing image).
	forceRecreated := false
	built := false
	for _, c := range calls {
		for _, a := range c {
			if a == "--force-recreate" {
				forceRecreated = true
			}
			if a == "--build" {
				built = true
			}
		}
	}
	if !forceRecreated {
		t.Errorf("install must --force-recreate the server container; calls: %v", calls)
	}
	if !built {
		t.Errorf("install must --build the server image (stale-image bug); calls: %v", calls)
	}
	// Infra (postgres/redis) still comes up via plain up -d.
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

func TestHasInstanceVolumes(t *testing.T) {
	origRun := runCommand
	runCommand = func(ctx context.Context, name string, args ...string) (string, error) {
		// Real docker compose prefixes with the project name: for a instance at
		// /opt/horizonx the project is `horizonx`, so the volume is
		// `horizonx_horizonx_pgdata`.
		return "horizonx_horizonx_pgdata\nother-volume\n", nil
	}
	defer func() { runCommand = origRun }()

	if !hasInstanceVolumes("/opt/horizonx") {
		t.Error("hasInstanceVolumes should detect horizonx_pgdata")
	}
}

func TestHasInstanceVolumesBareName(t *testing.T) {
	origRun := runCommand
	runCommand = func(ctx context.Context, name string, args ...string) (string, error) {
		// A bare `horizonx_pgdata` (hand-created or old layout) still counts.
		return "horizonx_pgdata\n", nil
	}
	defer func() { runCommand = origRun }()

	if !hasInstanceVolumes("/opt/horizonx") {
		t.Error("hasInstanceVolumes should detect bare horizonx_pgdata")
	}
}

func TestHasInstanceVolumesNoVolumes(t *testing.T) {
	origRun := runCommand
	runCommand = func(ctx context.Context, name string, args ...string) (string, error) {
		return "unrelated-volume\n", nil
	}
	defer func() { runCommand = origRun }()

	if hasInstanceVolumes("/opt/horizonx") {
		t.Error("hasInstanceVolumes should be false when only unrelated volumes exist")
	}
}

func TestHasInstanceVolumesProjectPrefixed(t *testing.T) {
	origRun := runCommand
	runCommand = func(ctx context.Context, name string, args ...string) (string, error) {
		// Real docker prefixes with the compose project name
		// (e.g. instance dir /tmp/test → project `test` → `test_horizonx_pgdata`).
		return "test_horizonx_pgdata\n", nil
	}
	defer func() { runCommand = origRun }()

	if !hasInstanceVolumes("/tmp/test") {
		t.Error("hasInstanceVolumes should detect project-prefixed horizonx volumes")
	}
}

// minisign test helpers (hermetic: pure-Go key generation + signing, no binary).
func minisign_GenerateKeyForTest(t *testing.T) (string, []byte, error) {
	t.Helper()
	pub, _, err := minisign.GenerateKey(nil)
	if err != nil {
		return "", nil, err
	}
	return pub.String(), nil, nil
}

func minisign_SignForTest(t *testing.T, sums []byte) (string, []byte) {
	t.Helper()
	pub, priv, err := minisign.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	return pub.String(), minisign.Sign(priv, sums)
}

func TestFetchDashboardTarballFailsClosedWithoutSignature(t *testing.T) {
	// Fail-closed: with a release public key configured, the dashboard
	// fetch must abort when SHA256SUMS.minisig is missing — on BOTH the
	// fresh-download path and the cached-tarball path.
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

	pub, _, err := minisign_GenerateKeyForTest(t)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(envReleasePubkey, pub)

	if _, err := fetchDashboardTarball(t.TempDir()); err == nil {
		t.Fatal("expected fresh-download path to fail closed without a signature, got nil")
	}

	// Cached path: pre-place a tarball matching the (unsigned) sums.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, tarballName), content, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := fetchDashboardTarball(dir); err == nil {
		t.Fatal("expected cached path to fail closed without a signature, got nil")
	}
}

func TestFetchDashboardTarballVerifiesSignatureWhenKeySet(t *testing.T) {
	// Happy path with a key configured: signed sums verify on download.
	content := []byte("fake dashboard image")
	sum := fmt.Sprintf("%x", sha256.Sum256(content))
	tarballName := "horizonx-dashboard-v0.3.0-image.tar.gz"
	sums := []byte(fmt.Sprintf("%s  %s\n", sum, tarballName))
	pub, sig := minisign_SignForTest(t, sums)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/" + tarballName:
			_, _ = w.Write(content)
		case "/SHA256SUMS":
			_, _ = w.Write(sums)
		case "/SHA256SUMS.minisig":
			_, _ = w.Write(sig)
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

	t.Setenv(envReleasePubkey, pub)
	if _, err := fetchDashboardTarball(t.TempDir()); err != nil {
		t.Fatalf("fetchDashboardTarball with valid signature: %v", err)
	}
}

// writeLocalDashboardArtifacts writes a dashboard tarball and optional
// SHA256SUMS / SHA256SUMS.minisig into dir, returning the tarball path.
func writeLocalDashboardArtifacts(t *testing.T, dir string, content, sums, sig []byte) string {
	t.Helper()
	name := "horizonx-dashboard-v0.3.0-image.tar.gz"
	if err := os.WriteFile(filepath.Join(dir, name), content, 0o644); err != nil {
		t.Fatal(err)
	}
	if sums != nil {
		if err := os.WriteFile(filepath.Join(dir, "SHA256SUMS"), sums, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if sig != nil {
		if err := os.WriteFile(filepath.Join(dir, "SHA256SUMS.minisig"), sig, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(dir, name)
}

func TestArtifactBaseResolution(t *testing.T) {
	// Unset -> the historical GitHub default, and the produced URL keeps exactly
	// one slash before releases/.
	t.Setenv(envArtifactBase, "")
	if got := artifactBaseFromEnv(); got != defaultArtifactBase {
		t.Errorf("unset artifactBaseFromEnv() = %q, want %q", got, defaultArtifactBase)
	}
	wantDefault := "https://github.com/zlnew/horizonx/releases/download/v1.2.3/horizonx-linux-x86_64.tar.gz"
	if got := releaseArtifactURL(artifactBaseFromEnv(), "v1.2.3", "horizonx-linux-x86_64.tar.gz"); got != wantDefault {
		t.Errorf("default release URL = %q, want %q", got, wantDefault)
	}

	// Set -> used verbatim.
	t.Setenv(envArtifactBase, "https://mirror.example.com")
	if got := artifactBaseFromEnv(); got != "https://mirror.example.com" {
		t.Errorf("set artifactBaseFromEnv() = %q, want the mirror verbatim", got)
	}
	wantMirror := "https://mirror.example.com/releases/latest/download/SHA256SUMS"
	if got := releaseArtifactURL(artifactBaseFromEnv(), "latest", "SHA256SUMS"); got != wantMirror {
		t.Errorf("mirror latest URL = %q, want %q", got, wantMirror)
	}

	// Trailing slash normalised: exactly one slash before releases/.
	t.Setenv(envArtifactBase, "https://mirror.example.com/")
	if got := artifactBaseFromEnv(); got != "https://mirror.example.com" {
		t.Errorf("trailing-slash artifactBaseFromEnv() = %q, want %q", got, "https://mirror.example.com")
	}
	gotURL := releaseArtifactURL(artifactBaseFromEnv(), "v1.2.3", "x")
	wantURL := "https://mirror.example.com/releases/download/v1.2.3/x"
	if gotURL != wantURL {
		t.Errorf("trailing-slash URL = %q, want %q", gotURL, wantURL)
	}
	if strings.Contains(gotURL, "//releases") {
		t.Errorf("doubled slash before releases/: %q", gotURL)
	}
}

func TestServerDockerfileUsesArtifactBase(t *testing.T) {
	// Unset must reproduce the historical default: the ARG default and the
	// compose pass-through both resolve to the GitHub repo URL, and the curl
	// lines build their URLs from that ARG instead of a hard-coded host.
	if !strings.Contains(instanceServerDockerfile, "ARG HX_ARTIFACT_BASE="+defaultArtifactBase) {
		t.Errorf("generated Dockerfile missing the artifact-base ARG default:\n%s", instanceServerDockerfile)
	}
	if !strings.Contains(instanceServerDockerfile, `ARTIFACT_BASE="${HX_ARTIFACT_BASE%/}"`) {
		t.Error("generated Dockerfile does not strip a trailing slash from HX_ARTIFACT_BASE")
	}
	if !strings.Contains(instanceServerDockerfile, `curl -fsSL $ARTIFACT_BASE/$BASE/horizonx-linux-$ARCH.tar.gz`) {
		t.Error("generated Dockerfile does not build the tarball URL from $ARTIFACT_BASE")
	}
	if !strings.Contains(instanceServerDockerfile, `curl -fsSL $ARTIFACT_BASE/$BASE/SHA256SUMS`) {
		t.Error("generated Dockerfile does not build the SHA256SUMS URL from $ARTIFACT_BASE")
	}
	if strings.Contains(instanceServerDockerfile, "curl -fsSL https://github.com/zlnew/horizonx/$BASE") {
		t.Error("generated Dockerfile still hard-codes the GitHub host in a curl line")
	}
	if !strings.Contains(instanceComposeServer, "HX_ARTIFACT_BASE: ${HX_ARTIFACT_BASE:-"+defaultArtifactBase+"}") {
		t.Error("server compose does not pass HX_ARTIFACT_BASE through to the build")
	}
}

func TestGeneratedDockerfileIndependentOfArtifactBaseEnv(t *testing.T) {
	// "Unset preserves today's behaviour byte-for-byte": the generated server
	// Dockerfile must be identical whether HX_ARTIFACT_BASE is set or not. Only
	// the build arg passed by compose changes at run time, never the file.
	generate := func(base string) string {
		t.Helper()
		t.Setenv(envArtifactBase, base)
		l, err := GenerateInstanceWithAdmin(t.TempDir(), "127.0.0.1", "admin@horizonx.local", "pw", nil)
		if err != nil {
			t.Fatalf("GenerateInstanceWithAdmin: %v", err)
		}
		b, err := os.ReadFile(l.ServerDockerfile)
		if err != nil {
			t.Fatalf("read generated Dockerfile: %v", err)
		}
		return string(b)
	}
	unset := generate("")
	set := generate("https://mirror.example.com")
	if unset != set {
		t.Errorf("generated Dockerfile bytes changed with HX_ARTIFACT_BASE:\nunset: %q\nset:   %q", unset, set)
	}
	if !strings.Contains(unset, "ARG HX_ARTIFACT_BASE="+defaultArtifactBase) {
		t.Errorf("unset generated Dockerfile does not carry the GitHub default:\n%s", unset)
	}
}

func TestTarballDirOrDefault(t *testing.T) {
	// An explicit flag wins; an empty one falls back to HX_TARBALL_DIR. This is
	// what lets the upgrade path (which never parses the flag) honour the env.
	t.Setenv(envTarballDir, "/tmp/from-env")
	if got := tarballDirOrDefault(""); got != "/tmp/from-env" {
		t.Errorf("tarballDirOrDefault(\"\") = %q, want the env value", got)
	}
	if got := tarballDirOrDefault("/tmp/from-flag"); got != "/tmp/from-flag" {
		t.Errorf("tarballDirOrDefault(flag) = %q, want the flag value", got)
	}
	t.Setenv(envTarballDir, "")
	if got := tarballDirOrDefault(""); got != "" {
		t.Errorf("tarballDirOrDefault with nothing set = %q, want empty", got)
	}
}

func TestTarballDirPreferredWithoutReleaseAPI(t *testing.T) {
	tarballDir := t.TempDir()
	cacheDir := t.TempDir()
	content := []byte("local dashboard image")
	sum := fmt.Sprintf("%x", sha256.Sum256(content))
	name := "horizonx-dashboard-v9.9.9-image.tar.gz"
	if err := os.WriteFile(filepath.Join(tarballDir, name), content, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tarballDir, "SHA256SUMS"), []byte(fmt.Sprintf("%s  %s\n", sum, name)), 0o644); err != nil {
		t.Fatal(err)
	}

	apiCalled := false
	oldRel := latestDashboardRelease
	latestDashboardRelease = func() (dashboardRelease, error) {
		apiCalled = true
		return dashboardRelease{}, errors.New("release API down")
	}
	defer func() { latestDashboardRelease = oldRel }()

	got, err := fetchDashboardTarballFrom(cacheDir, tarballDir)
	if err != nil {
		t.Fatalf("--tarball-dir install should succeed offline: %v", err)
	}
	if got != filepath.Join(tarballDir, name) {
		t.Errorf("got %q, want the local %q", got, filepath.Join(tarballDir, name))
	}
	if apiCalled {
		t.Error("release API was contacted despite a complete --tarball-dir; local artifacts must be used without GitHub")
	}
}

func TestResolveDashboardTarballPrefersLocalDir(t *testing.T) {
	tarballDir := t.TempDir()
	name := "horizonx-dashboard-v9.9.9-image.tar.gz"
	if err := os.WriteFile(filepath.Join(tarballDir, name), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	pick, _, err := resolveDashboardTarball(tarballDir, t.TempDir(), func() (dashboardRelease, error) {
		t.Error("release lookup must not run when --tarball-dir holds a tarball")
		return dashboardRelease{}, errors.New("unreachable")
	})
	if err != nil {
		t.Fatalf("resolveDashboardTarball: %v", err)
	}
	if !pick.VerifyLocal || pick.Path != filepath.Join(tarballDir, name) {
		t.Errorf("pick = %+v, want a verified local path", pick)
	}

	// No local tarball + failed lookup => the pre-existing best-effort cache
	// fallback, unverified.
	cacheDir := t.TempDir()
	cacheName := "horizonx-dashboard-old.tar.gz"
	if err := os.WriteFile(filepath.Join(cacheDir, cacheName), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	pick, _, err = resolveDashboardTarball("", cacheDir, func() (dashboardRelease, error) {
		return dashboardRelease{}, errors.New("api down")
	})
	if err == nil {
		t.Error("expected the release lookup error to be returned")
	}
	if pick.VerifyLocal || pick.Path != filepath.Join(cacheDir, cacheName) {
		t.Errorf("fallback pick = %+v, want the unverified cache path", pick)
	}
}

func TestTarballDirTamperedTarballFailsClosed(t *testing.T) {
	// A tampered tarball plus a matching, attacker-supplied SHA256SUMS (the
	// checksum is self-consistent). With a real release public key configured
	// only the signature gate can reject it — which is exactly what must
	// happen: --tarball-dir must not bypass signature verification.
	tarballDir := t.TempDir()
	content := []byte("tampered dashboard image")
	sum := fmt.Sprintf("%x", sha256.Sum256(content))
	sums := []byte(fmt.Sprintf("%s  horizonx-dashboard-v0.3.0-image.tar.gz\n", sum))
	writeLocalDashboardArtifacts(t, tarballDir, content, sums, nil)

	pub, _, err := minisign_GenerateKeyForTest(t)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(envReleasePubkey, pub)

	if _, err := fetchDashboardTarballFrom(t.TempDir(), tarballDir); err == nil {
		t.Fatal("tampered --tarball-dir tarball accepted: local dir bypassed signature verification")
	}
}

func TestTarballDirSignedLocalTarballAccepted(t *testing.T) {
	content := []byte("local dashboard image")
	sum := fmt.Sprintf("%x", sha256.Sum256(content))
	name := "horizonx-dashboard-v0.3.0-image.tar.gz"
	sums := []byte(fmt.Sprintf("%s  %s\n", sum, name))
	pub, sig := minisign_SignForTest(t, sums)

	dir := t.TempDir()
	writeLocalDashboardArtifacts(t, dir, content, sums, sig)
	t.Setenv(envReleasePubkey, pub)

	got, err := fetchDashboardTarballFrom(t.TempDir(), dir)
	if err != nil {
		t.Fatalf("signed local tarball should be accepted: %v", err)
	}
	if got != filepath.Join(dir, name) {
		t.Errorf("got %q, want %q", got, filepath.Join(dir, name))
	}
}

func TestTarballDirChecksumMismatchRejected(t *testing.T) {
	// Validly signed sums that list a hash which does NOT match the tarball must
	// still be rejected — the checksum step is not implied by the signature step.
	content := []byte("dashboard image")
	name := "horizonx-dashboard-v0.3.0-image.tar.gz"
	sums := []byte(fmt.Sprintf("%s  %s\n", strings.Repeat("0", 64), name))
	pub, sig := minisign_SignForTest(t, sums)

	dir := t.TempDir()
	writeLocalDashboardArtifacts(t, dir, content, sums, sig)
	t.Setenv(envReleasePubkey, pub)

	if _, err := fetchDashboardTarballFrom(t.TempDir(), dir); err == nil {
		t.Fatal("expected a local tarball failing the checksum to be rejected")
	}
}
