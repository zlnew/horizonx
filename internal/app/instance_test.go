package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateInstanceLayout(t *testing.T) {
	dir := t.TempDir()
	l, err := GenerateInstance(dir, "myserver")
	if err != nil {
		t.Fatalf("GenerateInstance: %v", err)
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

func TestGenerateInstanceNoGHCR(t *testing.T) {
	dir := t.TempDir()
	if _, err := GenerateInstance(dir, "myserver"); err != nil {
		t.Fatalf("GenerateInstance: %v", err)
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

func TestGenerateInstanceRootIncludeStatic(t *testing.T) {
	dir := t.TempDir()
	if _, err := GenerateInstance(dir, "myserver"); err != nil {
		t.Fatalf("GenerateInstance: %v", err)
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

func TestGenerateInstanceEnvPorts(t *testing.T) {
	dir := t.TempDir()
	if _, err := GenerateInstance(dir, "myserver"); err != nil {
		t.Fatalf("GenerateInstance: %v", err)
	}
	env, err := os.ReadFile(filepath.Join(dir, ".env"))
	if err != nil {
		t.Fatalf("read .env: %v", err)
	}
	s := string(env)
	if !strings.Contains(s, "HTTP_ADDR=:3000") {
		t.Errorf(".env missing HTTP_ADDR=:3000 (internal convention; host port is HORIZONX_PORT)\n%s", s)
	}
	if !strings.Contains(s, "HORIZONX_PORT=4858") {
		t.Errorf(".env missing HORIZONX_PORT=4858 (signature host port)")
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

func TestGenerateInstanceEnvDatabaseURLMatchesPostgresPassword(t *testing.T) {
	dir := t.TempDir()
	if _, err := GenerateInstance(dir, "myserver"); err != nil {
		t.Fatalf("GenerateInstance: %v", err)
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

func TestGenerateInstanceServerCompose(t *testing.T) {
	dir := t.TempDir()
	if _, err := GenerateInstance(dir, "myserver"); err != nil {
		t.Fatalf("GenerateInstance: %v", err)
	}
	sc, err := os.ReadFile(filepath.Join(dir, "server", "docker-compose.yml"))
	if err != nil {
		t.Fatalf("read server compose: %v", err)
	}
	s := string(sc)
	if !strings.Contains(s, "context: .") || !strings.Contains(s, "dockerfile: Dockerfile") {
		t.Errorf("server compose must build from local Dockerfile:\n%s", s)
	}
	// Stale-image bug (2026-08-04): the Dockerfile must accept a pinned
	// HX_VERSION build arg and the compose must pass it through, so each
	// release changes the curl command string and busts the docker layer
	// cache (a fixed "releases/latest" URL was a permanent cache hit and the
	// image kept the first build's binary forever).
	if !strings.Contains(s, "HX_VERSION") {
		t.Errorf("server compose must pass HX_VERSION build arg (stale-image fix):\n%s", s)
	}
	if strings.Contains(s, "ghcr.io") {
		t.Errorf("server compose references ghcr.io")
	}
}

func TestGenerateInstanceDockerfileChecksum(t *testing.T) {
	dir := t.TempDir()
	if _, err := GenerateInstance(dir, "myserver"); err != nil {
		t.Fatalf("GenerateInstance: %v", err)
	}
	df, err := os.ReadFile(filepath.Join(dir, "server", "Dockerfile"))
	if err != nil {
		t.Fatalf("read Dockerfile: %v", err)
	}
	s := string(df)
	if !strings.Contains(s, "sha256sum -c") {
		t.Errorf("server Dockerfile must verify SHA256SUMS:\n%s", s)
	}
	// The Dockerfile must consume HX_VERSION and build a pinned release URL
	// (stale-image fix) — not a hardcoded releases/latest that never busts
	// the docker layer cache.
	if !strings.Contains(s, "HX_VERSION") {
		t.Errorf("server Dockerfile must use HX_VERSION build arg (stale-image fix):\n%s", s)
	}
	// Docker scoping regression (caught live on creatokuserver v0.5.0): an
	// ARG declared before FROM is NOT in scope inside RUN steps — the stage
	// must re-declare it. Without the re-declaration HX_VERSION was empty in
	// the RUN, fell back to releases/latest, and the cache never busted, so
	// `horizonx upgrade` silently left the old binary in the image.
	fromIdx := strings.Index(s, "FROM alpine")
	stageArg := strings.Index(s[fromIdx:], "ARG HX_VERSION")
	if fromIdx < 0 || stageArg < 0 {
		t.Errorf("server Dockerfile must re-declare ARG HX_VERSION inside the stage (Docker ARG scoping, stale-image fix):\n%s", s)
	}
	if strings.Contains(s, "releases/latest/download") {
		t.Errorf("server Dockerfile must NOT hardcode releases/latest (stale-image fix):\n%s", s)
	}
	if strings.Contains(s, "ghcr.io") {
		t.Errorf("server Dockerfile references ghcr.io")
	}
}

func TestGenerateInstancePreservesExistingEnv(t *testing.T) {
	dir := t.TempDir()
	if _, err := GenerateInstance(dir, "myserver"); err != nil {
		t.Fatalf("GenerateInstance: %v", err)
	}
	// Simulate an existing .env (e.g. first install created it) and re-run:
	// re-install must NOT regenerate secrets — volumes depend on them.
	custom := "APP_ENV=production\nJWT_SECRET=keep-me\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(custom), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := GenerateInstance(dir, "myserver"); err != nil {
		t.Fatalf("GenerateInstance re-run: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != custom {
		t.Errorf("re-run must preserve existing .env — got:\n%s", string(got))
	}
}

func TestGenerateInstanceEnvContainsAdminCreds(t *testing.T) {
	dir := t.TempDir()
	if _, err := GenerateInstanceWithAdmin(dir, "myserver", "ops@example.com", "S3cret!", nil); err != nil {
		t.Fatalf("GenerateInstanceWithAdmin: %v", err)
	}
	env, err := os.ReadFile(filepath.Join(dir, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(env)
	if !strings.Contains(s, "ADMIN_EMAIL=ops@example.com") {
		t.Errorf(".env missing ADMIN_EMAIL=ops@example.com\n%s", s)
	}
	if !strings.Contains(s, "ADMIN_PASSWORD=S3cret!") {
		t.Errorf(".env missing ADMIN_PASSWORD\n%s", s)
	}
	if !strings.Contains(s, "AUTO_SEED=true") {
		t.Errorf(".env missing AUTO_SEED=true\n%s", s)
	}
}

func TestNewInstanceEnvUnique(t *testing.T) {
	a, err := newInstanceEnv("h", "", "", nil)
	if err != nil {
		t.Fatalf("newInstanceEnv: %v", err)
	}
	b, err := newInstanceEnv("h", "", "", nil)
	if err != nil {
		t.Fatalf("newInstanceEnv: %v", err)
	}
	if a.JWTSecret == b.JWTSecret || a.PostgresPassword == b.PostgresPassword ||
		a.AgentSecret == b.AgentSecret || a.ServerID == b.ServerID {
		t.Errorf("secrets must be unique per generation")
	}
	if a.HTTPAddr != ":3000" || a.DashboardPort != "4859" {
		t.Errorf("instanceEnv ports wrong: %s %s", a.HTTPAddr, a.DashboardPort)
	}
	if a.AdminEmail != "admin@horizonx.local" || a.AdminPass == "" {
		t.Errorf("instanceEnv must default admin email + random password")
	}
	if a.AdminPass == b.AdminPass {
		t.Errorf("random admin passwords must differ per generation")
	}
}

func TestNewInstanceEnvAdminCreds(t *testing.T) {
	a, err := newInstanceEnv("h", "ops@example.com", "S3cret!", nil)
	if err != nil {
		t.Fatalf("newInstanceEnv: %v", err)
	}
	if a.AdminEmail != "ops@example.com" || a.AdminPass != "S3cret!" {
		t.Errorf("admin creds not honored: %s / %s", a.AdminEmail, a.AdminPass)
	}
}

// TestNewInstanceEnvAllowedOriginsDefault pins the TK-0019 fix: when no origins
// are given, ALLOWED_ORIGINS defaults to the same-box dashboard URL so the
// user WebSocket works without manual config.
func TestNewInstanceEnvAllowedOriginsDefault(t *testing.T) {
	a, err := newInstanceEnv("myhost", "", "", nil)
	if err != nil {
		t.Fatalf("newInstanceEnv: %v", err)
	}
	if len(a.AllowedOrigins) != 1 || a.AllowedOrigins[0] != "http://myhost:4859" {
		t.Errorf("default AllowedOrigins = %v, want [http://myhost:4859]", a.AllowedOrigins)
	}
}

// TestNewInstanceEnvAllowedOriginsExplicit pins that an explicit origin list is
// honored verbatim (e.g. a tunnel/domain the dashboard is served from).
func TestNewInstanceEnvAllowedOriginsExplicit(t *testing.T) {
	a, err := newInstanceEnv("myhost", "", "", []string{"https://horizonx.example.com", "http://myhost:4859"})
	if err != nil {
		t.Fatalf("newInstanceEnv: %v", err)
	}
	if len(a.AllowedOrigins) != 2 || a.AllowedOrigins[0] != "https://horizonx.example.com" {
		t.Errorf("explicit AllowedOrigins = %v, want [https://horizonx.example.com http://myhost:4859]", a.AllowedOrigins)
	}
}

// TestGenerateInstanceEnvAllowedOrigins renders the ALLOWED_ORIGINS key joined
// by comma into the instance .env.
func TestGenerateInstanceEnvAllowedOrigins(t *testing.T) {
	dir := t.TempDir()
	if _, err := GenerateInstanceWithAdmin(dir, "myhost", "", "", []string{"https://horizonx.example.com"}); err != nil {
		t.Fatalf("GenerateInstanceWithAdmin: %v", err)
	}
	env, err := os.ReadFile(filepath.Join(dir, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(env), "ALLOWED_ORIGINS=https://horizonx.example.com") {
		t.Errorf(".env missing expected ALLOWED_ORIGINS:\n%s", string(env))
	}
}
