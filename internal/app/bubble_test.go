package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateBubbleLayout(t *testing.T) {
	dir := t.TempDir()
	l, err := GenerateBubble(dir, "myserver")
	if err != nil {
		t.Fatalf("GenerateBubble: %v", err)
	}
	if l.Root == "" || l.ComposeRoot == "" || l.ServerCompose == "" || l.DashboardCompose == "" {
		t.Fatalf("layout not fully populated: %+v", l)
	}
	for _, p := range []string{l.ComposeRoot, l.EnvPath, l.ServerCompose, l.ServerDockerfile, l.DashboardCompose} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("missing generated file %s: %v", p, err)
		}
	}
}

func TestGenerateBubbleNoGHCR(t *testing.T) {
	dir := t.TempDir()
	if _, err := GenerateBubble(dir, "myserver"); err != nil {
		t.Fatalf("GenerateBubble: %v", err)
	}
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), "ghcr.io") {
			t.Errorf("%s references ghcr.io — never allowed", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
}

func TestGenerateBubbleRootIncludeStatic(t *testing.T) {
	dir := t.TempDir()
	if _, err := GenerateBubble(dir, "myserver"); err != nil {
		t.Fatalf("GenerateBubble: %v", err)
	}
	root, err := os.ReadFile(filepath.Join(dir, "docker-compose.yml"))
	if err != nil {
		t.Fatalf("read root compose: %v", err)
	}
	s := string(root)
	if !strings.Contains(s, "server/docker-compose.yml") || !strings.Contains(s, "dashboard/docker-compose.yml") {
		t.Errorf("root include list must reference both sub-projects")
	}
}

func TestGenerateBubbleEnvPorts(t *testing.T) {
	dir := t.TempDir()
	if _, err := GenerateBubble(dir, "myserver"); err != nil {
		t.Fatalf("GenerateBubble: %v", err)
	}
	env, err := os.ReadFile(filepath.Join(dir, ".env"))
	if err != nil {
		t.Fatalf("read .env: %v", err)
	}
	s := string(env)
	if !strings.Contains(s, "HTTP_ADDR=:4858") {
		t.Errorf(".env missing HTTP_ADDR=:4858\n%s", s)
	}
	if !strings.Contains(s, "DASHBOARD_PORT=4859") {
		t.Errorf(".env missing DASHBOARD_PORT=4859")
	}
	if !strings.Contains(s, "HORIZONX_API_URL=http://myserver:4858") {
		t.Errorf(".env missing API URL on signature port")
	}
	if !strings.Contains(s, "HORIZONX_WS_URL=ws://myserver:4858/ws/agent") {
		t.Errorf(".env missing WS URL on signature port")
	}
	if strings.Contains(s, "***") {
		t.Errorf(".env contains literal *** — secrets must be interpolated, never masked")
	}
}

func TestGenerateBubbleEnvDatabaseURLMatchesPostgresPassword(t *testing.T) {
	dir := t.TempDir()
	if _, err := GenerateBubble(dir, "myserver"); err != nil {
		t.Fatalf("GenerateBubble: %v", err)
	}
	env, err := os.ReadFile(filepath.Join(dir, ".env"))
	if err != nil {
		t.Fatalf("read .env: %v", err)
	}
	s := string(env)
	pgPass := extractEnv(s, "POSTGRES_PASSWORD")
	dbURL := extractEnv(s, "DATABASE_URL")
	if pgPass == "" {
		t.Fatalf("POSTGRES_PASSWORD missing")
	}
	if !strings.Contains(dbURL, pgPass) {
		t.Errorf("DATABASE_URL %q does not contain the postgres password %q — server could not authenticate", dbURL, pgPass)
	}
}

func extractEnv(content, key string) string {
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, key+"=") {
			return strings.TrimPrefix(line, key+"=")
		}
	}
	return ""
}

func TestGenerateBubbleServerCompose(t *testing.T) {
	dir := t.TempDir()
	if _, err := GenerateBubble(dir, "myserver"); err != nil {
		t.Fatalf("GenerateBubble: %v", err)
	}
	sc, err := os.ReadFile(filepath.Join(dir, "server", "docker-compose.yml"))
	if err != nil {
		t.Fatalf("read server compose: %v", err)
	}
	s := string(sc)
	if !strings.Contains(s, "context: .") || !strings.Contains(s, "dockerfile: Dockerfile") {
		t.Errorf("server compose must build from local Dockerfile:\n%s", s)
	}
	if strings.Contains(s, "ghcr.io") {
		t.Errorf("server compose references ghcr.io")
	}
}

func TestGenerateBubbleDockerfileChecksum(t *testing.T) {
	dir := t.TempDir()
	if _, err := GenerateBubble(dir, "myserver"); err != nil {
		t.Fatalf("GenerateBubble: %v", err)
	}
	df, err := os.ReadFile(filepath.Join(dir, "server", "Dockerfile"))
	if err != nil {
		t.Fatalf("read Dockerfile: %v", err)
	}
	s := string(df)
	if !strings.Contains(s, "sha256sum -c") {
		t.Errorf("server Dockerfile must verify SHA256SUMS:\n%s", s)
	}
	if strings.Contains(s, "ghcr.io") {
		t.Errorf("server Dockerfile references ghcr.io")
	}
}

func TestNewBubbleEnvUnique(t *testing.T) {
	a, err := newBubbleEnv("h")
	if err != nil {
		t.Fatalf("newBubbleEnv: %v", err)
	}
	b, err := newBubbleEnv("h")
	if err != nil {
		t.Fatalf("newBubbleEnv: %v", err)
	}
	if a.JWTSecret == b.JWTSecret || a.PostgresPassword == b.PostgresPassword ||
		a.AgentSecret == b.AgentSecret || a.ServerID == b.ServerID {
		t.Errorf("secrets must be unique per generation")
	}
	if a.HTTPAddr != ":4858" || a.DashboardPort != "4859" {
		t.Errorf("bubbleEnv ports wrong: %s %s", a.HTTPAddr, a.DashboardPort)
	}
}
