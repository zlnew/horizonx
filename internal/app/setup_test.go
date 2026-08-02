package app

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRandomHex(t *testing.T) {
	a, err := randomHex(16)
	if err != nil {
		t.Fatalf("randomHex: %v", err)
	}
	b, err := randomHex(16)
	if err != nil {
		t.Fatalf("randomHex: %v", err)
	}
	if len(a) != 32 || a == b {
		t.Fatalf("expected two distinct 32-char hex strings, got %q vs %q", a, b)
	}
}

func TestSystemdUnitsEmbedded(t *testing.T) {
	for _, unit := range []string{"horizonx-server.service", "horizonx-agent.service"} {
		data, err := systemdUnit(unit)
		if err != nil {
			t.Fatalf("systemdUnit(%s): %v", unit, err)
		}
		if !strings.Contains(string(data), "[Unit]") || !strings.Contains(string(data), "[Service]") {
			t.Fatalf("unit %s looks malformed", unit)
		}
	}
}

func TestSetupWritesFiles(t *testing.T) {
	dir := t.TempDir()

	// RunSetup reads os.Args[2:]; simulate `horizonx setup --dir X --host H`.
	oldArgs := os.Args
	os.Args = []string{"horizonx", "setup", "--dir", dir, "--host", "203.0.113.10"}
	defer func() { os.Args = oldArgs }()

	if err := RunSetup(); err != nil {
		t.Fatalf("RunSetup: %v", err)
	}

	for _, f := range []string{".env", "docker-compose.yml", "systemd/horizonx-server.service", "systemd/horizonx-agent.service"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("expected %s to be written: %v", f, err)
		}
	}

	env, err := os.ReadFile(filepath.Join(dir, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"JWT_SECRET=", "HORIZONX_SERVER_ID=", "HORIZONX_SERVER_API_TOKEN=", "203.0.113.10"} {
		if !strings.Contains(string(env), want) {
			t.Errorf("expected %q in .env", want)
		}
	}

	compose, err := os.ReadFile(filepath.Join(dir, "docker-compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"ghcr.io/zlnew/horizonx", "ghcr.io/zlnew/horizonx-dashboard", "postgres:16-alpine", "redis:7-alpine"} {
		if !strings.Contains(string(compose), want) {
			t.Errorf("expected %q in compose", want)
		}
	}
}

func TestChecksumForAndVerify(t *testing.T) {
	// Build a fake checksum file + file, verify round-trip.
	content := []byte("hello horizonx")
	sum := sha256.Sum256(content)
	wantHex := hex.EncodeToString(sum[:])

	dir := t.TempDir()
	sumsPath := filepath.Join(dir, "SHA256SUMS")
	assetPath := filepath.Join(dir, "horizonx-linux-x86_64.tar.gz")
	if err := os.WriteFile(sumsPath, []byte(wantHex+"  "+assetPath+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(assetPath, content, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := checksumFor(sumsPath, assetPath)
	if err != nil {
		t.Fatalf("checksumFor: %v", err)
	}
	if got != wantHex {
		t.Fatalf("checksumFor: got %s want %s", got, wantHex)
	}

	if err := verifySHA256(assetPath, wantHex); err != nil {
		t.Fatalf("verifySHA256 should pass: %v", err)
	}
	if err := verifySHA256(assetPath, strings.Repeat("0", 64)); err == nil {
		t.Fatal("verifySHA256 should fail on wrong checksum")
	}
}

func TestChecksumForMissing(t *testing.T) {
	sumsPath := filepath.Join(t.TempDir(), "SHA256SUMS")
	if err := os.WriteFile(sumsPath, []byte("abc  some-other-file\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := checksumFor(sumsPath, "horizonx-linux-x86_64.tar.gz"); err == nil {
		t.Fatal("expected error for missing asset checksum")
	}
}
