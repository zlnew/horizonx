package app

// Instance templates for `horizonx install server`.
//
// The instance lives at /opt/horizonx and is ONE docker compose project:
//   docker-compose.yml        <- root: postgres + redis + include: [server, dashboard]
//   .env                      <- single source of truth (interpolates into sub-projects)
//   server/docker-compose.yml <- the control plane (builds from release tarball, no GHCR)
//   server/Dockerfile         <- downloads the release tarball + verifies SHA256SUMS
//   dashboard/docker-compose.yml <- the SPA (image tarball loaded locally, no GHCR)
//
// Design rules (locked with Maul, 2026-08-03):
//   - Never reference an image we can't prove exists (no ghcr.io, ever).
//   - The root include list is STATIC [server, dashboard] because `install server`
//     writes all three files atomically — no regeneration rule needed.
//   - Internal ports stay conventional (server :3000, dashboard :80); host-exposed
//     ports are the signature ones (4858/4859) via .env.
//   - Requires Docker Compose >= 2.20 (`include:` support) — checked by RunPreflight.

import (
	"fmt"
	"os"
	"path/filepath"
)

// InstanceLayout describes the files GenerateInstance writes.
type InstanceLayout struct {
	Root             string // absolute path to the instance dir (e.g. /opt/horizonx)
	ComposeRoot      string // docker-compose.yml
	EnvPath          string // .env
	ServerDir        string // server/
	ServerCompose    string // server/docker-compose.yml
	ServerDockerfile string // server/Dockerfile
	DashboardDir     string // dashboard/
	DashboardCompose string // dashboard/docker-compose.yml
}

// instanceEnv is the set of values rendered into the instance.
type instanceEnv struct {
	HTTPAddr         string // ":4858"
	DashboardPort    string // "4859"
	JWTSecret        string
	ServerID         string
	AgentSecret      string
	PostgresPassword string
	RedisPassword    string
	Host             string   // public host agents/dashboard use (defaults to 127.0.0.1)
	AdminEmail       string   // first admin user (auto-seeded at boot)
	AdminPass        string   // first admin password (auto-seeded at boot)
	AllowedOrigins   []string // browser origins allowed to open the user WebSocket
}

// GenerateInstance writes the full instance tree into dir (created if missing).
// It is pure generation — no privileged steps, no docker calls — so it can be
// used by `--generate-only` and by tests. Returns the layout so callers can
// run `docker compose -f <root> config --quiet` afterwards.
func GenerateInstance(dir, host string) (*InstanceLayout, error) {
	return generateInstance(dir, host, "", "", nil)
}

// GenerateInstanceWithAdmin is GenerateInstance plus the first admin credentials
// (written into the instance .env; the server auto-seeds the user at boot) and
// an explicit list of browser origins allowed to open the user WebSocket
// (defaults to the same-box dashboard URL when empty — see TK-0019).
func GenerateInstanceWithAdmin(dir, host, adminEmail, adminPass string, allowedOrigins []string) (*InstanceLayout, error) {
	return generateInstance(dir, host, adminEmail, adminPass, allowedOrigins)
}

func generateInstance(dir, host, adminEmail, adminPass string, allowedOrigins []string) (*InstanceLayout, error) {
	root, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	for _, sub := range []string{"", "server", "dashboard"} {
		if err := os.MkdirAll(filepath.Join(root, sub), 0o755); err != nil {
			return nil, fmt.Errorf("create %s: %w", filepath.Join(root, sub), err)
		}
	}

	env, err := newInstanceEnv(host, adminEmail, adminPass, allowedOrigins)
	if err != nil {
		return nil, err
	}

	l := &InstanceLayout{
		Root:             root,
		ComposeRoot:      filepath.Join(root, "docker-compose.yml"),
		EnvPath:          filepath.Join(root, ".env"),
		ServerDir:        filepath.Join(root, "server"),
		ServerCompose:    filepath.Join(root, "server", "docker-compose.yml"),
		ServerDockerfile: filepath.Join(root, "server", "Dockerfile"),
		DashboardDir:     filepath.Join(root, "dashboard"),
		DashboardCompose: filepath.Join(root, "dashboard", "docker-compose.yml"),
	}

	// Idempotency: keep an EXISTING .env on re-run (install = install-or-
	// upgrade). Regenerating secrets would break the postgres/redis volumes
	// already initialized with the old passwords — the server could never
	// authenticate against them.
	envContent := renderInstanceEnv(env)
	if existing, err := os.ReadFile(l.EnvPath); err == nil && len(existing) > 0 {
		envContent = string(existing)
	}

	files := map[string]string{
		l.EnvPath:          envContent,
		l.ComposeRoot:      instanceComposeRoot,
		l.ServerCompose:    instanceComposeServer,
		l.ServerDockerfile: instanceServerDockerfile,
		l.DashboardCompose: instanceComposeDashboard,
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			return nil, fmt.Errorf("write %s: %w", path, err)
		}
	}
	// Dockerfiles and compose files are not secrets — relax to 0644 after write.
	if err := os.Chmod(l.ComposeRoot, 0o644); err == nil {
		_ = os.Chmod(l.ServerCompose, 0o644)
		_ = os.Chmod(l.ServerDockerfile, 0o644)
		_ = os.Chmod(l.DashboardCompose, 0o644)
	}

	return l, nil
}

const instanceComposeRoot = `# HorizonX instance — root compose.
# ` + "`install server`" + ` owns this whole tree; do not hand-edit the include list.
#
#   docker compose up -d     # brings up postgres + redis + server + dashboard
#
# postgres + redis are PRIVATE to this project (no host ports). The server and
# dashboard are separate sub-projects included below — each with its own
# docker-compose.yml so ` + "`install server`" + ` can update them independently.
include:
  - server/docker-compose.yml
  - dashboard/docker-compose.yml

services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
      POSTGRES_DB: horizonx
    volumes:
      - horizonx_pgdata:/var/lib/postgresql/data
    restart: unless-stopped
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U postgres"]
      interval: 5s
      timeout: 3s
      retries: 10

  redis:
    image: redis:7-alpine
    command: ["redis-server", "--requirepass", "${REDIS_PASSWORD}"]
    volumes:
      - horizonx_redisdata:/data
    restart: unless-stopped

volumes:
  horizonx_pgdata:
  horizonx_redisdata:
`

const instanceComposeServer = `# HorizonX control plane — sub-project, included by the root compose.
# Built from the release tarball via the Dockerfile in this dir — no registry
# pulls (the project registry is billing-blocked, so images are built locally).
services:
  server:
    build:
      context: .
      dockerfile: Dockerfile
      args:
        HX_VERSION: ${HX_VERSION:-latest}
    image: ${APP_IMAGE:-horizonx:latest}
    env_file: ../.env
    ports:
      - '127.0.0.1:${HORIZONX_PORT:-4858}:3000'
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_started
    restart: unless-stopped
`

const instanceComposeDashboard = `# HorizonX dashboard — sub-project, included by the root compose.
# The image is loaded locally from the release tarball (docker load) — never
# pulled from a registry. nginx serves the SPA and proxies /api + WebSocket
# to the server at 'server:3000' on this project's network.
services:
  dashboard:
    image: ${DASHBOARD_IMAGE:-horizonx-dashboard:latest}
    ports:
      - '127.0.0.1:${DASHBOARD_PORT:-4859}:80'
    depends_on:
      - server
    restart: unless-stopped
`

const instanceServerDockerfile = `# HorizonX server image — built by the instance's install.
# Downloads the release tarball + SHA256SUMS from GitHub and verifies the
# checksum BEFORE unpacking. No registry images involved (the registry is
# billing-blocked).
#
# HX_VERSION pins the release (e.g. v0.3.8). It is passed as a build arg by
# ` + "`install server`" + ` so the curl command string CHANGES every release —
# Docker layer cache is keyed on (parent layer + command text), so an
# unchanged "releases/latest" URL was a permanent cache hit and the image
# kept the FIRST build's binary forever (the stale-image bug, 2026-08-04).
# With a pinned version the layer cache misses each release -> fresh download.
ARG HX_VERSION=latest
FROM alpine:3.20
# Docker scoping: an ARG before FROM is only in scope for FROM lines. The
# build-arg value (passed by compose) is invisible to RUN steps unless the
# ARG is re-declared inside the stage. Without this re-declaration the RUN
# below sees an empty HX_VERSION, falls back to releases/latest, and the
# layer cache never busts — the stale-image bug, silently (caught live on
# creatokuserver v0.5.0: upgrade left the old binary in the image; a manual
# no-cache rebuild was the tell).
ARG HX_VERSION
RUN apk add --no-cache ca-certificates curl
# Detect the build arch from uname (release tarballs use x86_64/arm64 —
# BuildKit's automatic TARGETARCH arg is unreliable across builders).
RUN ARCH=$(uname -m); \
    case "$ARCH" in \
      x86_64|amd64) ARCH=x86_64 ;; \
      aarch64|arm64) ARCH=arm64 ;; \
      *) echo "unsupported arch: $ARCH" >&2; exit 1 ;; \
    esac; \
    if [ "$HX_VERSION" = "latest" ] || [ -z "$HX_VERSION" ]; then \
      BASE="releases/latest"; \
    else \
      BASE="releases/download/$HX_VERSION"; \
    fi; \
    curl -fsSL https://github.com/zlnew/horizonx/$BASE/download/horizonx-linux-$ARCH.tar.gz -o /tmp/horizonx-linux-$ARCH.tar.gz \
 && curl -fsSL https://github.com/zlnew/horizonx/$BASE/download/SHA256SUMS -o /tmp/SHA256SUMS \
 && cd /tmp && grep "horizonx-linux-$ARCH.tar.gz" SHA256SUMS | sha256sum -c - \
 && tar -xzf /tmp/horizonx-linux-$ARCH.tar.gz -C /usr/local/bin horizonx \
 && rm /tmp/horizonx-linux-$ARCH.tar.gz /tmp/SHA256SUMS
WORKDIR /etc/horizonx
ENTRYPOINT ["horizonx"]
CMD ["server"]
`
