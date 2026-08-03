package app

// Integration test against the LIVE GitHub release — proves the real
// asset-name resolution + SHA256SUMS verification end-to-end. Runs only when
// HX_LIVE_NET=1 (default off so the suite stays hermetic).
import (
	"os"
	"path/filepath"
	"testing"
)

func TestLiveFetchDashboardTarball(t *testing.T) {
	if os.Getenv("HX_LIVE_NET") != "1" {
		t.Skip("set HX_LIVE_NET=1 to hit the live GitHub release")
	}
	dir := t.TempDir()
	got, err := fetchDashboardTarball(dir)
	if err != nil {
		t.Fatalf("live fetchDashboardTarball: %v", err)
	}
	if filepath.Base(got) != "horizonx-dashboard-v0.3.0-image.tar.gz" {
		t.Errorf("expected live v0.3.0 tarball, got %s", filepath.Base(got))
	}
	info, err := os.Stat(got)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("downloaded %s (%d bytes)", filepath.Base(got), info.Size())
	if info.Size() < 10_000_000 {
		t.Errorf("suspiciously small dashboard tarball: %d bytes", info.Size())
	}
}
