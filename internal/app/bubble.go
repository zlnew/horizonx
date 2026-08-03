package app

// Bubble templates for `horizonx install server`.
//
// The bubble lives at /opt/horizonx and is ONE docker compose project:
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

// BubbleLayout describes the files GenerateBubble writes.
type BubbleLayout struct {
	Root             string // absolute path to the bubble dir (e.g. /opt/horizonx)
	ComposeRoot      string // docker-compose.yml
	EnvPath          string // .env
	ServerDir        string // server/
	ServerCompose    string // server/docker-compose.yml
	ServerDockerfile string // server/Dockerfile
	DashboardDir     string // dashboard/
	DashboardCompose string // dashboard/docker-compose.yml
}

// bubbleEnv is the set of values rendered into the bubble.
type bubbleEnv struct {
	HTTPAddr         string // ":4858"
	DashboardPort    string // "4859"
	JWTSecret        string
	ServerID         string
	AgentSecret      string
	PostgresPassword string
	RedisPassword    string
	Host             string // public host agents/dashboard use (defaults to 127.0.0.1)
}

// GenerateBubble writes the full bubble tree into dir (created if missing).
// It is pure generation — no privileged steps, no docker calls — so it can be
// used by `--generate-only` and by tests. Returns the layout so callers can
// run `docker compose -f <root> config --quiet` afterwards.
func GenerateBubble(dir, host string) (*BubbleLayout, error) {
	root, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	for _, sub := range []string{"", "server", "dashboard"} {
		if err := os.MkdirAll(filepath.Join(root, sub), 0o755); err != nil {
			return nil, fmt.Errorf("create %s: %w", filepath.Join(root, sub), err)
		}
	}

	env, err := newBubbleEnv(host)
	if err != nil {
		return nil, err
	}

	l := &BubbleLayout{
		Root:             root,
		ComposeRoot:      filepath.Join(root, "docker-compose.yml"),
		EnvPath:          filepath.Join(root, ".env"),
		ServerDir:        filepath.Join(root, "server"),
		ServerCompose:    filepath.Join(root, "server", "docker-compose.yml"),
		ServerDockerfile: filepath.Join(root, "server", "Dockerfile"),
		DashboardDir:     filepath.Join(root, "dashboard"),
		DashboardCompose: filepath.Join(root, "dashboard", "docker-compose.yml"),
	}

	files := map[string]string{
		l.EnvPath:          renderBubbleEnv(env),
		l.ComposeRoot:      bubbleComposeRoot,
		l.ServerCompose:    bubbleComposeServer,
		l.ServerDockerfile: bubbleServerDockerfile,
		l.DashboardCompose: bubbleComposeDashboard,
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

const bubbleComposeRoot = `# HorizonX bubble — root compose.
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

const bubbleComposeServer = `# HorizonX control plane — sub-project, included by the root compose.
# Built from the release tarball via the Dockerfile in this dir — no registry
# pulls (the project registry is billing-blocked, so images are built locally).
services:
  server:
    build:
      context: .
      dockerfile: Dockerfile
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

const bubbleComposeDashboard = `# HorizonX dashboard — sub-project, included by the root compose.
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

const bubbleServerDockerfile = `# HorizonX server image — built by the bubble's install.
# Downloads the release tarball + SHA256SUMS from GitHub and verifies the
# checksum BEFORE unpacking. No registry images involved (GHCR is blocked).
FROM alpine:3.20
RUN apk add --no-cache ca-certificates curl
ARG TARGETARCH
RUN curl -fsSL https://github.com/zlnew/horizonx/releases/latest/download/horizonx-linux-${TARGETARCH}.tar.gz -o /tmp/hx.tgz \
 && curl -fsSL https://github.com/zlnew/horizonx/releases/latest/download/SHA256SUMS -o /tmp/SHA256SUMS \
 && cd /tmp && grep "horizonx-linux-${TARGETARCH}.tar.gz" SHA256SUMS | sha256sum -c - \
 && tar -xzf /tmp/hx.tgz -C /usr/local/bin horizonx \
 && rm /tmp/hx.tgz /tmp/SHA256SUMS
WORKDIR /etc/horizonx
ENTRYPOINT ["horizonx"]
CMD ["server"]
`
