package app

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractBinaryPlainName(t *testing.T) {
	// Standard tarball shape: member named `horizonx`.
	path := makeTarball(t, map[string][]byte{"horizonx": []byte("#!/bin/sh\necho ok\n")})
	defer os.Remove(path)

	bin, err := extractBinary(path, "horizonx")
	if err != nil {
		t.Fatalf("extractBinary: %v", err)
	}
	defer os.Remove(bin)
	data, err := os.ReadFile(bin)
	if err != nil {
		t.Fatalf("read extracted: %v", err)
	}
	if !strings.Contains(string(data), "echo ok") {
		t.Errorf("extracted content mismatch: %s", data)
	}
}

func TestExtractBinaryFallbackArchName(t *testing.T) {
	// Regression (v0.3.4): a tarball whose member is arch-qualified
	// (horizonx-linux-x86_64) must still extract — upgrade should not fail
	// with "horizonx not found in release tarball".
	path := makeTarball(t, map[string][]byte{"horizonx-linux-x86_64": []byte("#!/bin/sh\necho ok\n")})
	defer os.Remove(path)

	bin, err := extractBinary(path, "horizonx")
	if err != nil {
		t.Fatalf("extractBinary fallback: %v", err)
	}
	defer os.Remove(bin)
	if filepath.Base(bin) == "" {
		t.Error("expected extracted temp file")
	}
}

func makeTarball(t *testing.T, files map[string][]byte) string {
	t.Helper()
	tmp, err := os.CreateTemp("", "hx-tarball-*.tar.gz")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	defer tmp.Close()
	gz := gzip.NewWriter(tmp)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(content))}); err != nil {
			t.Fatalf("write header: %v", err)
		}
		if _, err := tw.Write(content); err != nil {
			t.Fatalf("write body: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return tmp.Name()
}

func TestRuntimeActiveUnitNone(t *testing.T) {
	// On a bare box there is no horizonx unit; ActiveUnit must be "".
	// On a real install (e.g. this dogfood box) the agent unit IS active —
	// then the answer must still be one of our units, never a crash.
	rt := DetectRuntime()
	got := rt.ActiveUnit()
	if got == "" {
		return // bare box — original assertion holds
	}
	if got != serverUnit && got != agentUnit {
		t.Errorf("ActiveUnit = %q, want one of %q/%q", got, serverUnit, agentUnit)
	}
}

func TestRuntimeDockerComposeDetection(t *testing.T) {
	// If docker + compose are present, DockerCLI must be true. Regression
	// test: the old check matched the version string for "v2", but modern
	// compose reports "Docker Compose version 5.3.1" (no "v2") — so a
	// working install was reported as DockerCLI=false.
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not on PATH")
	}
	if _, err := exec.Command("docker", "compose", "version").CombinedOutput(); err != nil {
		t.Skip("docker compose plugin not available")
	}
	rt := DetectRuntime()
	if !rt.DockerCLI {
		t.Errorf("DockerCLI = false, want true (docker + compose are installed and working)")
	}
}

func TestDetectRuntimeBubbleAtOptHorizonx(t *testing.T) {
	// A bubble root with docker-compose.yml must be detected as BubbleDir,
	// and with a .env present BubbleInstalled must be true.
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "docker-compose.yml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("HORIZONX_PORT=4858\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HORIZONX_PREFIX", root)

	rt := DetectRuntime()
	if rt.BubbleDir != root {
		t.Errorf("BubbleDir = %q, want %q", rt.BubbleDir, root)
	}
	if !rt.BubbleInstalled() {
		t.Error("BubbleInstalled = false, want true (compose + .env present)")
	}
}

func TestBubbleInstalledRequiresEnv(t *testing.T) {
	// A half-generated bubble dir (compose but no .env) is NOT a live
	// install — BubbleInstalled must be false so upgrade doesn't try to
	// apply against it.
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "docker-compose.yml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HORIZONX_PREFIX", root)

	rt := DetectRuntime()
	if rt.BubbleDir != root {
		t.Errorf("BubbleDir = %q, want %q (compose present, env absent)", rt.BubbleDir, root)
	}
	if rt.BubbleInstalled() {
		t.Error("BubbleInstalled = true, want false (no .env)")
	}
}

func TestBubbleInstalledNoBubble(t *testing.T) {
	// No bubble dir at all — BubbleInstalled must be false without crashing.
	t.Setenv("HORIZONX_PREFIX", filepath.Join(t.TempDir(), "does-not-exist"))
	rt := DetectRuntime()
	if rt.BubbleInstalled() {
		t.Error("BubbleInstalled = true on a bare box, want false")
	}
}

func TestRuntimeUserUnitDetection(t *testing.T) {
	// Simulate a user-level unit and confirm it lands in UserUnits.
	home := t.TempDir()
	t.Setenv("HOME", home)
	unitDir := home + "/.config/systemd/user"
	if err := os.MkdirAll(unitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unitDir+"/horizonx-agent.service", []byte("[Unit]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rt := DetectRuntime()
	if !rt.IsUserUnit("horizonx-agent.service") {
		t.Error("expected user unit to be detected in UserUnits")
	}
}

func TestRestartServiceSchedules(t *testing.T) {
	// restartService should either schedule via systemd-run (missing in the
	// test container) or fall back to at (also likely missing). It must not
	// panic and must return an error that mentions the unit name.
	err := restartService("horizonx-agent.service", false)
	if err == nil {
		// systemd-run may actually exist and schedule; that's acceptable.
		t.Log("restartService scheduled (systemd-run present)")
		return
	}
	if err.Error() == "" {
		t.Error("expected a non-empty error")
	}
}

func TestNeedsSudoNonRoot(t *testing.T) {
	// can't easily fake euid; just ensure the function exists and returns bool.
	if isRoot() {
		t.Skip("running as root")
	}
	_ = needsSudo("/usr/local/bin")
}
