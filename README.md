# HorizonX

**Deploy and monitor your apps without babysitting SSH.**

You have a few servers and a handful of apps. Every deploy means `ssh`, `git pull`, `systemctl restart`, and hoping nothing broke while you weren't watching. HorizonX is the control plane that fixes that: connect a lightweight agent to each box, and deploy, roll back, and monitor everything from one dashboard.

> **Note**: This repository contains the **Backend Server** (Go control plane + agent).
> The **Frontend Dashboard** lives here: [https://github.com/zlnew/horizonx-dashboard](https://github.com/zlnew/horizonx-dashboard)

<table>
  <tr>
    <td><img src="screenshots/server-monitoring.png" width="500"></td>
    <td><img src="screenshots/application-deployment-details.png" width="500"></td>
  </tr>
</table>

---

## Why HorizonX?

Because the alternatives are either overkill or a pile of shell scripts.

- **You don't need Kubernetes.** For a handful of apps on a few boxes, K8s is a second full-time job. HorizonX is one binary, one agent per box, one dashboard.
- **You don't need to babysit deploys.** The agent pulls from git, builds the image, and health-gates the rollout — success only counts when the app is actually running. No more "it deployed" followed by a 502.
- **You get real rollback.** Every deploy tags the image; rollback replays the previous tag. One click, no `git revert` ritual.
- **You can see what's happening.** Live CPU/RAM/disk/network per server, plus deploy history and job logs — without another monitoring SaaS.

**The shape of it** — a client-server model:

```
┌─────────────┐   WebSocket    ┌──────────────┐
│  Agent       │ ◄────────────► │  Server      │
│  (per box)   │                │  (control    │
│              │   push metrics │   plane)     │  ◄── Dashboard (Vue 3)
└──────┬───────┘   receive jobs │              │
       │                        └──────────────┘
       │  docker compose
       ▼
   your apps
```

1. **Server (control plane)** — the central API: apps, servers, users, deploy jobs. You talk to it through the dashboard.
2. **Agent (runner)** — a single binary you install on each Linux box. It connects back over a persistent WebSocket, pushes hardware metrics, and executes commands like *Deploy App* using Docker Compose.

---

## Key features

- **Real-time infrastructure monitoring** — CPU, memory, disk, network, GPU, per second, per server.
- **Zero-downtime deployments** — GitOps pull → compose build → health-gated up. In-place recreate, never `compose down`.
- **One-click rollback** — image tags are recorded per deploy; replay any previous tag.
- **Env vars & secrets** — inject per-app environment without touching the repo.
- **Multi-stage builds** — the agent builds through `docker compose build`, so each service gets the right target (php-fpm vs nginx) automatically.
- **Secure by default** — token-authenticated agents, audit log, role-based access.

---

## Quick start

### 1. Install the binary (one-liner)

```bash
curl -fsSL https://raw.githubusercontent.com/zlnew/horizonx/main/install.sh | bash
```

This fetches the latest release tarball, **verifies the SHA256 checksum**, and installs `horizonx` to `/usr/local/bin`.

### 2. Bootstrap the control plane

```bash
horizonx setup --host 203.0.113.10
```

Generates `./horizonx-setup/` with:

- `.env` — strong random `JWT_SECRET`, server ID + agent token, DB/Redis wiring
- `docker-compose.yml` — postgres + redis + server + dashboard in one command
- `systemd/` — unit templates for bare-metal hosts

```bash
cd horizonx-setup
docker compose up -d          # control plane
open http://<host>:8080       # dashboard
```

### 3. Connect an agent to an app host

```bash
curl -fsSL https://raw.githubusercontent.com/zlnew/horizonx/main/install.sh | bash
```

Then, with the credentials from `horizonx setup`:

```bash
HORIZONX_SERVER_ID=<id> HORIZONX_SERVER_API_TOKEN=<token> \
HORIZONX_API_URL=http://<host>:3000 HORIZONX_WS_URL=ws://<host>:3000/ws/agent \
horizonx agent
```

For long-running installs, `horizonx setup` also prints systemd units:
`sudo cp horizonx-setup/systemd/*.service /etc/systemd/system/` then
`sudo systemctl daemon-reload && sudo systemctl enable --now horizonx-server`.

### 4. Deploy your first app

In the dashboard: **Applications → New** → point at a git repo + branch, set env vars → **Deploy**. The agent clones, builds, and health-gates the rollout. Deployments, rollbacks, and job logs are all in the UI.

### 5. Migrations

The compose file ships with a migrate gate — `horizonx migrate -op=up` runs before the server starts, so a fresh control plane always boots on a migrated schema.

---

## Requirements

- **Linux** for the agent and server.
- **PostgreSQL 13+** and **Redis** — or just use the bundled compose (postgres + redis included).
- **Docker + Compose** on each app host (the agent deploys with `docker compose`).
- **Git** on each app host (the agent clones repos).

---

## Development

```bash
# Unified binary (server/agent/setup/upgrade/migrate in one)
go build -o bin/horizonx ./cmd/horizonx
bin/horizonx server         # run the control plane

# Legacy split binaries (still supported)
make build                  # bin/server, bin/agent, bin/migrate, bin/seed

# Tests
go test ./...
```

The dashboard is developed in [horizonx-dashboard](https://github.com/zlnew/horizonx-dashboard) (`npm install && npm run dev`).

---

*HorizonX was built to dogfood itself — the workspace that runs it is deployed through it.*
