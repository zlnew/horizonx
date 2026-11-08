package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDashboardVersionFromTarball(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"horizonx-dashboard-v0.7.1-image.tar.gz", "v0.7.1"},
		{"horizonx-dashboard-v0.3.0-image.tar.gz", "v0.3.0"},
		// An operator-provided --tarball-dir file that does not follow the
		// release naming convention is still recorded, under its own name,
		// rather than silently dropped from the manifest.
		{"dashboard-local.tar.gz", "dashboard-local.tar.gz"},
		// Degenerate case: the pattern matches but yields an empty tag, so the
		// fallback must win rather than writing an empty dashboard version.
		{"horizonx-dashboard--image.tar.gz", "horizonx-dashboard--image.tar.gz"},
	}

	for _, c := range cases {
		if got := dashboardVersionFromTarball(c.in); got != c.want {
			t.Errorf("dashboardVersionFromTarball(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestWriteVersionManifestRecordsBothVersions(t *testing.T) {
	dir := t.TempDir()

	if err := writeVersionManifest(dir, "v0.3.8", "v0.7.1"); err != nil {
		t.Fatalf("writeVersionManifest: %v", err)
	}

	body, err := os.ReadFile(filepath.Join(dir, versionManifestName))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	got := string(body)

	if !strings.Contains(got, "server=v0.3.8") {
		t.Errorf("manifest must record the server version, got:\n%s", got)
	}
	if !strings.Contains(got, "dashboard=v0.7.1") {
		t.Errorf("manifest must record the dashboard version, got:\n%s", got)
	}
	// The point of the file is that a mismatched pair is visible, so the two
	// values must be independently derived rather than one copied into both.
	if strings.Contains(got, "server=v0.7.1") {
		t.Errorf("server version was overwritten by the dashboard version:\n%s", got)
	}
}

// TestWriteVersionManifestRecordsMissingDashboardHonestly pins that an install
// without a dashboard says so, instead of omitting the line and leaving the
// reader to guess whether the dashboard is absent or simply unrecorded.
func TestWriteVersionManifestRecordsMissingDashboardHonestly(t *testing.T) {
	dir := t.TempDir()

	if err := writeVersionManifest(dir, "latest", ""); err != nil {
		t.Fatalf("writeVersionManifest: %v", err)
	}

	body, err := os.ReadFile(filepath.Join(dir, versionManifestName))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	got := string(body)

	if !strings.Contains(got, "server=latest") {
		t.Errorf("a dev build installs from the latest release, so it must be recorded as such:\n%s", got)
	}
	if !strings.Contains(got, "dashboard=unknown") {
		t.Errorf("a missing dashboard must be recorded as unknown, not omitted:\n%s", got)
	}
}
