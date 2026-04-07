#!/bin/bash
set -e

# -----------------------------
# Config
# -----------------------------
INSTALL_DIR="/usr/local/bin/horizonx-server"
CONFIG_DIR="/var/lib/horizonx/server.env"
LOG_DIR="/var/log/horizonx"
DATA_DIR="/var/lib/horizonx"
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
  useradd -r \
    -g "$GROUP_NAME" \
    -d "$DATA_DIR" \
    -s /usr/sbin/nologin \
    "$USER_NAME"
fi

# -----------------------------
# Create directories
# -----------------------------
echo "[*] Creating directories..."
mkdir -p "$(dirname "$INSTALL_DIR")"
mkdir -p "$LOG_DIR"
mkdir -p "$DATA_DIR"

touch "$LOG_DIR/server.log" "$LOG_DIR/server.error.log"

chown -R "$USER_NAME:$GROUP_NAME" \
  "$LOG_DIR" \
  "$DATA_DIR"

# -----------------------------
# Stop service
# -----------------------------
systemctl stop "$SERVICE_NAME" || true

# -----------------------------
# Install server binary
# -----------------------------
echo "[*] Installing server binary..."
cp "$BIN_SOURCE" "$INSTALL_DIR"
chmod 755 "$INSTALL_DIR"
chown root:root "$INSTALL_DIR"

# -----------------------------
# Create env file if missing
# -----------------------------
if [ ! -f "$CONFIG_DIR" ]; then
  echo "[*] Creating default server env at $CONFIG_DIR..."
  cat > "$CONFIG_DIR" <<EOF
APP_ENV=production
TIME_ZONE="Local"
HTTP_ADDR=":3000"
ALLOWED_ORIGINS="http://localhost:5173,http://localhost:5174"
DATABASE_URL="postgres://postgres:@localhost:5432/horizonx?sslmode=disable"
REDIS_ADDR="localhost:6379"
REDIS_USERNAME=""
REDIS_PASSWORD=""
REDIS_DB="0"
JWT_SECRET="secret"
JWT_EXPIRY="24h"
LOG_LEVEL="info"
LOG_FORMAT="text"
EOF

  chown "$USER_NAME:$GROUP_NAME" "$CONFIG_DIR"
  chmod 600 "$CONFIG_DIR"
else
  echo "[*] Server env already exists, skipping."
fi

# -----------------------------
# Load env (for migrate/seed)
# -----------------------------
set -a
source "$CONFIG_DIR"
set +a

# -----------------------------
# Run migrations
# -----------------------------
if [ -x "$MIGRATE_BIN" ]; then
  echo "[*] Running migrations..."
  "$MIGRATE_BIN" -op=up -dsn="$DATABASE_URL"
fi

# -----------------------------
# Optional seeding
# -----------------------------
if [ "$SEED" = true ] && [ -x "$SEED_BIN" ]; then
  echo "[*] Seeding data..."
  "$SEED_BIN" -dsn="$DATABASE_URL"
fi

# -----------------------------
# systemd service
# -----------------------------
SERVICE_FILE="/etc/systemd/system/$SERVICE_NAME.service"

echo "[*] Writing systemd service..."
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

# -----------------------------
# Enable & start
# -----------------------------
systemctl daemon-reload
systemctl enable "$SERVICE_NAME"
systemctl restart "$SERVICE_NAME"

echo "[✓] HorizonX Server installed"
