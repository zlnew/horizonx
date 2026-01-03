#!/bin/bash

set -e

# -----------------------------
# Configurable paths
# -----------------------------
INSTALL_DIR="/usr/local/bin/horizonx-server"
CONFIG_DIR="/etc/horizonx/server.env"
LOG_DIR="/var/log/horizonx"
BIN_SOURCE="./bin/server"
MIGRATE_BIN="./bin/migrate"
SEED_BIN="./bin/seed"
SERVICE_NAME="horizonx-server"
USER_NAME="horizonx"
GROUP_NAME="horizonx"

# -----------------------------
# Parse options
# -----------------------------
SEED=false
while [[ $# -gt 0 ]]; do
    case $1 in
        --seed)
            SEED=true
            shift
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# -----------------------------
# Create user/group if missing
# -----------------------------
if ! id -u "$USER_NAME" >/dev/null 2>&1; then
    echo "[*] Creating user and group '$USER_NAME'..."
    groupadd "$GROUP_NAME"
    useradd -r -g "$GROUP_NAME" -s /bin/false "$USER_NAME"
fi

# -----------------------------
# Create necessary directories
# -----------------------------
echo "[*] Creating directories..."
mkdir -p "$(dirname "$INSTALL_DIR")"
mkdir -p "$(dirname "$CONFIG_DIR")"
mkdir -p "$LOG_DIR"

# Ensure logs exist
touch "$LOG_DIR/server.log" "$LOG_DIR/server.error.log"

# Set ownership
chown -R "$USER_NAME:$GROUP_NAME" "$(dirname "$INSTALL_DIR")" "$(dirname "$CONFIG_DIR")" "$LOG_DIR"

# Stop service
systemctl stop "$SERVICE_NAME" || true

# -----------------------------
# Install server binary
# -----------------------------
echo "[*] Installing server binary..."
cp "$BIN_SOURCE" "$INSTALL_DIR"
chmod +x "$INSTALL_DIR"
chown "$USER_NAME:$GROUP_NAME" "$INSTALL_DIR"

# -----------------------------
# Create env file if missing
# -----------------------------
if [ ! -f "$CONFIG_DIR" ]; then
  echo "[*] Creating default config at $CONFIG_DIR..."
  cat > "$CONFIG_DIR" <<EOF
TIME_ZONE="Local"
HTTP_ADDR=":3000"
ALLOWED_ORIGINS="http://localhost:5173,http://localhost:5174"
DATABASE_URL="postgres://postgres:@localhost:5432/horizonx?sslmode=disable"
JWT_SECRET="secret"
JWT_EXPIRY="24h"
DB_ADMIN_EMAIL="admin@horizonx.local"
DB_ADMIN_PASSWORD="password"
APP_HEALTH_CHECK_INTERVAL="5m"
APP_HEALTH_CHECK_TIMEOUT="1m"
METRICS_COLLECT_INTERVAL="10s"
METRICS_FLUSH_INTERVAL="15s"
METRICS_BUFFER_SIZE=50
METRICS_BATCH_SIZE=10
LOG_LEVEL="info"
LOG_FORMAT="text"
EOF

  echo
  echo "!!! IMPORTANT !!!"
  echo "Default environment variables have been created at $CONFIG_DIR."
  echo "Edit this file before running in production to set your database credentials and server port."
  echo
fi

# -----------------------------
# Load environment variables safely
# -----------------------------
set -o allexport
source "$CONFIG_DIR"
set +o allexport

# -----------------------------
# Run migration
# -----------------------------
if [ -x "$MIGRATE_BIN" ]; then
    echo "[*] Running migrations..."
    "$MIGRATE_BIN" -op=up -dsn="$DATABASE_URL"
fi

# -----------------------------
# Optional seeding
# -----------------------------
if [ "$SEED" = true ] && [ -x "$SEED_BIN" ]; then
    echo "[*] Seeding dummy admin user..."
    "$SEED_BIN" -dsn="$DATABASE_URL"
fi

# -----------------------------
# Install systemd service
# -----------------------------
SERVICE_FILE="/etc/systemd/system/$SERVICE_NAME.service"

echo "[*] Creating systemd service..."
cat > "$SERVICE_FILE" <<EOF
[Unit]
Description=HorizonX Server
After=network.target postgresql.service
Requires=postgresql.service

[Service]
Type=simple
ExecStart=$INSTALL_DIR --config $CONFIG_DIR
Restart=always
User=$USER_NAME
Group=$GROUP_NAME
EnvironmentFile=$CONFIG_DIR
StandardOutput=file:$LOG_DIR/server.log
StandardError=file:$LOG_DIR/server.error.log

[Install]
WantedBy=multi-user.target
EOF

# Reload systemd and enable/start service
echo "[*] Enabling and starting service..."
systemctl daemon-reload
systemctl enable "$SERVICE_NAME"
systemctl restart "$SERVICE_NAME"

echo "[*] Installation complete. Service '$SERVICE_NAME' is running."
echo "[*] Make sure to review and update the environment file: $CONFIG_DIR"

