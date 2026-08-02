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

That's it — no `sudo` needed. The installer:

- Fetches the latest release tarball + `SHA256SUMS`, **verifies the checksum**
- Installs `horizonx` to `/usr/local/bin` — and **auto-elevates via sudo itself** if your user can't write there (re-running under `sudo` transparently)
- For a rootless install instead: `HORIZONX_PREFIX=$HOME/.local curl -fsSL … | bash`

### 2. Bootstrap the control plane (interactive wizard)

```bash
horizonx setup
```

The wizard walks you through everything — no manual config:

1. **Mode** — full (server + dashboard + agent) · server only · agent only · dashboard only
2. **Preflight** — checks git, docker, compose, postgres/redis reachability; tells you exactly what's missing and how to install it
3. **Install method** — probes the box and recommends `docker compose` or bare-metal systemd (overridable)
4. **Environment** — public host/IP, admin email
5. **Secrets** — generates JWT secret, server ID, agent token
6. **Apply** — writes `.env` + `docker-compose.yml` + systemd units, then starts the stack (or prints the exact commands, with `--generate-only`)

Non-interactive for scripts/CI: `horizonx setup --mode full --yes` or the classic
`horizonx setup --host 203.0.113.10` (generates `./horizonx-setup/` only).

### 3. Connect an agent to an app host

```bash
curl -fsSL https://raw.githubusercontent.com/zlnew/horizonx/main/install.sh | bash
horizonx setup --mode agent --yes
```

Agent mode creates the `horizonx` system user, adds it to the docker group,
generates a git SSH key (prints the public key to add to GitHub), writes the
env file, and installs + starts the systemd unit. It reuses the control
plane's credentials from the server `.env`, so no token juggling.

### 4. Deploy your first app

In the dashboard: **Applications → New** → point at a git repo + branch, set env vars → **Deploy**. The agent clones, builds, and health-gates the rollout. Deployments, rollbacks, and job logs are all in the UI.

### 5. Upgrading (self-contained)

```bash
horizonx upgrade
```

Downloads + checksum-verifies the new release, swaps the binary, and
**restarts the running service itself** (systemd unit or compose stack) — you
don't touch anything afterwards.

### 6. Migrations

Migrations run **automatically at server boot** — versioned, idempotent
(no-op when up to date), and concurrency-safe (Postgres advisory lock, so two
racing boots never conflict). Disable with `AUTO_MIGRATE=false`. Manual
control stays available: `horizonx migrate -op=up|down|version|force`.

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
