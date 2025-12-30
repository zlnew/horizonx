#!/bin/bash

set -e

# -----------------------------
# Configurable paths
# -----------------------------
INSTALL_DIR="/usr/local/bin/horizonx-agent"
CONFIG_DIR="/etc/horizonx/agent.env"
LOG_DIR="/var/log/horizonx"
BIN_SOURCE="./bin/agent"
SERVICE_NAME="horizonx-agent"

# -----------------------------
# Create necessary directories
# -----------------------------
echo "[*] Creating directories..."
mkdir -p "$(dirname "$INSTALL_DIR")"
mkdir -p "$(dirname "$CONFIG_DIR")"
mkdir -p "$LOG_DIR"

# Ensure logs exist
touch "$LOG_DIR/agent.log" "$LOG_DIR/agent.error.log"

# Set ownership
chown root:root "$INSTALL_DIR" 2>/dev/null || true
chown root:root "$CONFIG_DIR" 2>/dev/null || true
chown root:root "$LOG_DIR"/*.log

# Stop service
systemctl stop "$SERVICE_NAME" || true

# -----------------------------
# Install binary
# -----------------------------
echo "[*] Installing binary..."
cp "$BIN_SOURCE" "$INSTALL_DIR"
chmod +x "$INSTALL_DIR"

# -----------------------------
# Create env file if missing
# -----------------------------
if [ ! -f "$CONFIG_DIR" ]; then
  echo "[*] Creating default config at $CONFIG_DIR..."
  cat > "$CONFIG_DIR" <<EOF
HORIZONX_API_URL="http://localhost:3000"
HORIZONX_WS_URL="ws://localhost:3000/agent/ws"
HORIZONX_SERVER_API_TOKEN="hzx_secret"
HORIZONX_SERVER_ID="123"
LOG_LEVEL="info"
LOG_FORMAT="text"
EOF
  echo
  echo "!!! IMPORTANT !!!"
  echo "Default environment variables have been created at $CONFIG_DIR."
  echo "You should edit this file to set your actual HorizonX API URL, token, server ID, and any other settings before running the agent in production."
  echo
fi

# -----------------------------
# Install systemd service
# -----------------------------
SERVICE_FILE="/etc/systemd/system/$SERVICE_NAME.service"

echo "[*] Creating systemd service..."
cat > "$SERVICE_FILE" <<EOF
[Unit]
Description=HorizonX Agent
After=network.target

[Service]
Type=simple
ExecStart=$INSTALL_DIR --config $CONFIG_DIR
Restart=always
User=root
Group=root
StandardOutput=file:$LOG_DIR/agent.log
StandardError=file:$LOG_DIR/agent.error.log
EnvironmentFile=$CONFIG_DIR

[Install]
WantedBy=multi-user.target
EOF

# Reload systemd and enable service
echo "[*] Enabling and starting service..."
systemctl daemon-reload
systemctl enable "$SERVICE_NAME"
systemctl restart "$SERVICE_NAME"

echo "[*] Installation complete. Service '$SERVICE_NAME' is running."
echo "[*] Make sure to review and update the environment file: $CONFIG_DIR"

