# HorizonX

**The All-in-One Infrastructure Monitoring & Deployment Platform.**

HorizonX serves as the command center for your infrastructure. It allows you to monitor the real-time health of your distributed servers and deploy applications seamlessly using GitOps principles. Whether you have one server or a hundred, HorizonX brings them all into a single, unified view.

> **Note**: This repository contains the **Backend Server** code.  
> The **Frontend Dashboard** can be found here: [https://github.com/zlnew/horizonx-dashboard](https://github.com/zlnew/horizonx-dashboard)

---

## 🧩 How It Works

HorizonX is built on a **Client-Server** model designed for speed and security:

1.  **The Server (Control Plane)**:
    *   This is the central dashboard and API.
    *   It stores all data, manages user accounts, and handles application state.
    *   You interact with this via a Web Dashboard.

2.  **The Agent (Runner)**:
    *   A lightweight, standalone program that you install on your Linux servers.
    *   It securely connects back to your **Server** via a persistent WebSocket connection.
    *   It pushes hardware metrics (CPU, RAM, usage) every second and listens for commands (like "Deploy App").

---

## ✨ Key Features

### 1. 📊 Real-Time Infrastructure Monitoring
Forget about lagging charts. HorizonX provides **second-by-second** telemetry for your hardware.
*   **CPU**: See per-core load, temperatures, and power usage (Watts).
*   **Memory**: Visualize RAM and Swap usage to prevent OOM errors.
*   **Disk & Network**: Monitor I/O throughout, disk space, and network bandwidth in real-time.
*   **GPU Support**: Native monitoring for Nvidia GPUs for AI/ML workloads.

### 2. 🚀 Zero-Downtime Application Deployments
Deploy applications directly from your Git repositories (GitHub, GitLab, etc.).
*   **GitOps**: Push to your branch, and HorizonX pulls the latest code.
*   **Process Management**: HorizonX uses **Docker Compose** to manage the full application lifecycle (Deploy, Start, Stop, Restart), ensuring consistent environments.
*   **Env Vars**: Securely inject API keys and secrets into your running applications.

### 3. 🛡️ Secure & Scalable
*   **Clean Architecture**: Built with a robust Go backend for high performance.
*   **Token Authentication**: Agents require a secure token to join your fleet, preventing unauthorized access.

---

## � Requirements

To run HorizonX components, you need:

- **Operating System**: Linux.
- **Go**: Version 1.25.4 or higher (to compile binaries).
- **Database**: PostgreSQL 13+.
- **Git**: **Required** for cloning repositories and deployment operations. The Agent uses Git to clone your repositories.
- **Docker & Docker Compose**: **Required** for Application Management features (Deploy, Start, Stop, Restart). The Agent uses Docker Compose to manage your deployments.

---

## 📦 Quick Start (Development)

Use these steps to run the project locally for testing or development.

### 1. Setup Control Plane (Server)

```bash
git clone <repo-url>
cd horizonx

# Configure environment
cp .env.example .env
# Open .env and set your DATABASE_URL

# Initialize Database
make migrate-up
make seed # (Optional: Adds dummy data)

# Start Server
make run-server
```

### 2. Connect a Node (Agent)

```bash
# Build binary
make build

# Run Agent (replace credentials with real ones from DB/UI)
export HORIZONX_API_URL="http://localhost:3000"
export HORIZONX_WS_URL="ws://localhost:3000/ws/agent"
export HORIZONX_SERVER_API_TOKEN="hzx_secret"
export HORIZONX_SERVER_ID="123"

./bin/agent
```

---

## 🏭 Production Installation

For production deployments, we provide automated scripts in the `scripts/` directory. These install the binaries as **systemd services**, create dedicated users, and handle log rotation.

### 1. Installing the Server

Run this on the machine that will host the Control Plane.

1.  **Build Binaries**:
    ```bash
    make build
    ```
2.  **Run Installer**:
    ```bash
    sudo ./scripts/install-server.sh
    ```
    *   **Installs to**: `/usr/local/bin/horizonx-server`
    *   **Config**: `/etc/horizonx/server.env`
    *   **Logs**: `/var/log/horizonx/server.log`
    *   **Systemd Service**: `horizonx-server`

3.  **Post-Install**:
    *   Edit `/etc/horizonx/server.env` with your production `DATABASE_URL` and `JWT_SECRET`.
    *   Restart the service: `sudo systemctl restart horizonx-server`

### 2. Installing an Agent

Run this on every remote server you want to monitor and deploy applications to.

1.  **Build Binaries** (or copy `bin/agent` from your build server):
    ```bash
    make build
    ```
2.  **Run Installer**:
    ```bash
    sudo ./scripts/install-agent.sh
    ```
    *   **Installs to**: `/usr/local/bin/horizonx-agent`
    *   **Config**: `/etc/horizonx/agent.env`
    *   **Logs**: `/var/log/horizonx/agent.log`
    *   **Systemd Service**: `horizonx-agent`

3.  **Post-Install**:
    *   Edit `/etc/horizonx/agent.env`. You **MUST** set the `HORIZONX_API_URL`, `HORIZONX_WS_URL`, `HORIZONX_SERVER_API_TOKEN`, and `HORIZONX_SERVER_ID`.
    *   Restart the service: `sudo systemctl restart horizonx-agent`

---

## 🛠️ Development Tools

*   `make build`: Compiles all binaries to `bin/`.
*   `make clean`: Removes the `bin/` directory.
*   `make migrate-up`: Applies database migrations.
*   `make migrate-fresh`: Resets the database (Warning: Data loss).
