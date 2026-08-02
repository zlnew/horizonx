package docker

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetDockerComposeFilePrefersProd(t *testing.T) {
	dir := t.TempDir()

	// Only dev compose → picks it.
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := &Manager{}
	got, err := m.GetDockerComposeFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(got) != "docker-compose.yml" {
		t.Fatalf("expected dev compose fallback, got %s", got)
	}

	// Both dev + prod compose → prefers prod.
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.prod.yml"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err = m.GetDockerComposeFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(got) != "docker-compose.prod.yml" {
		t.Fatalf("expected prod compose preferred, got %s", got)
	}
}

func TestGetDockerComposeFileMissing(t *testing.T) {
	m := &Manager{}
	if _, err := m.GetDockerComposeFile(t.TempDir()); err == nil {
		t.Fatal("expected error when no compose file exists")
	}
}
