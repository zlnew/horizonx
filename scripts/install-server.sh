#!/bin/bash
set -euo pipefail

# =============================
# Usage
# =============================
usage() {
  cat <<EOF
Usage: $(basename "$0") [OPTIONS]

Options:
  --env-file <path>             Load env overrides from a file (key=value format)
  --api-url <url>               Override HORIZONX_API_URL (used for CORS/allowed origins)
  --http-addr <addr>            Override HTTP_ADDR (e.g. :3000)
  --allowed-origins <origins>   Override ALLOWED_ORIGINS (comma-separated URLs)
  --database-url <url>          Override DATABASE_URL
  --redis-addr <host:port>      Override REDIS_ADDR
  --redis-username <user>       Override REDIS_USERNAME
  --redis-password <pass>       Override REDIS_PASSWORD
  --redis-db <db>               Override REDIS_DB
  --jwt-secret <secret>         Override JWT_SECRET
  --jwt-expiry <duration>       Override JWT_EXPIRY (e.g. 24h)
  --time-zone <tz>              Override TIME_ZONE (e.g. UTC, Local, America/New_York)
  --log-level <level>           Override LOG_LEVEL (debug|info|warn|error)
  --log-format <format>         Override LOG_FORMAT (text|json)
  --app-env <env>               Override APP_ENV (production|development)
  --seed                        Run the seed binary after migrations
  --migrate-only                Run migrations and exit (do not install service)
  -h, --help                    Show this help message

Examples:
  # Fresh install with defaults
  sudo $(basename "$0")

  # Use a custom env file
  sudo $(basename "$0") --env-file /path/to/custom.env

  # Override individual values
  sudo $(basename "$0") --database-url postgres://user:pass@db:5432/horizonx --jwt-secret my_secret

  # Mix both: env file as base, individual flags take precedence
  sudo $(basename "$0") --env-file /path/to/custom.env --log-level debug --seed
EOF
}

# =============================
# Config defaults
# =============================
CFG_APP_ENV="production"
CFG_HTTP_ADDR=":3000"
CFG_ALLOWED_ORIGINS="http://localhost:5173,http://localhost:5174"
CFG_DATABASE_URL="postgres://postgres:@localhost:5432/horizonx?sslmode=disable"
CFG_REDIS_ADDR="localhost:6379"
CFG_REDIS_USERNAME=""
CFG_REDIS_PASSWORD=""
CFG_REDIS_DB="0"
CFG_JWT_SECRET="secret"
CFG_JWT_EXPIRY="24h"
CFG_TIME_ZONE="Local"
CFG_LOG_LEVEL="info"
CFG_LOG_FORMAT="text"

# =============================
# Argument parsing
# =============================
ENV_FILE=""
SEED=false
MIGRATE_ONLY=false

OVERRIDE_APP_ENV=""
OVERRIDE_HTTP_ADDR=""
OVERRIDE_ALLOWED_ORIGINS=""
OVERRIDE_DATABASE_URL=""
OVERRIDE_REDIS_ADDR=""
OVERRIDE_REDIS_USERNAME=""
OVERRIDE_REDIS_PASSWORD=""
OVERRIDE_REDIS_DB=""
OVERRIDE_JWT_SECRET=""
OVERRIDE_JWT_EXPIRY=""
OVERRIDE_TIME_ZONE=""
OVERRIDE_LOG_LEVEL=""
OVERRIDE_LOG_FORMAT=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --env-file)
      [[ -z "${2:-}" ]] && { echo "[!] --env-file requires a path argument"; exit 1; }
      ENV_FILE="$2"; shift 2 ;;
    --http-addr)
      [[ -z "${2:-}" ]] && { echo "[!] --http-addr requires a value"; exit 1; }
      OVERRIDE_HTTP_ADDR="$2"; shift 2 ;;
    --allowed-origins)
      [[ -z "${2:-}" ]] && { echo "[!] --allowed-origins requires a value"; exit 1; }
      OVERRIDE_ALLOWED_ORIGINS="$2"; shift 2 ;;
    --database-url)
      [[ -z "${2:-}" ]] && { echo "[!] --database-url requires a value"; exit 1; }
      OVERRIDE_DATABASE_URL="$2"; shift 2 ;;
    --redis-addr)
      [[ -z "${2:-}" ]] && { echo "[!] --redis-addr requires a value"; exit 1; }
      OVERRIDE_REDIS_ADDR="$2"; shift 2 ;;
    --redis-username)
      [[ -z "${2:-}" ]] && { echo "[!] --redis-username requires a value"; exit 1; }
      OVERRIDE_REDIS_USERNAME="$2"; shift 2 ;;
    --redis-password)
      [[ -z "${2:-}" ]] && { echo "[!] --redis-password requires a value"; exit 1; }
      OVERRIDE_REDIS_PASSWORD="$2"; shift 2 ;;
    --redis-db)
      [[ -z "${2:-}" ]] && { echo "[!] --redis-db requires a value"; exit 1; }
      OVERRIDE_REDIS_DB="$2"; shift 2 ;;
    --jwt-secret)
      [[ -z "${2:-}" ]] && { echo "[!] --jwt-secret requires a value"; exit 1; }
      OVERRIDE_JWT_SECRET="$2"; shift 2 ;;
    --jwt-expiry)
      [[ -z "${2:-}" ]] && { echo "[!] --jwt-expiry requires a value"; exit 1; }
      OVERRIDE_JWT_EXPIRY="$2"; shift 2 ;;
    --time-zone)
      [[ -z "${2:-}" ]] && { echo "[!] --time-zone requires a value"; exit 1; }
      OVERRIDE_TIME_ZONE="$2"; shift 2 ;;
    --log-level)
      [[ -z "${2:-}" ]] && { echo "[!] --log-level requires a value"; exit 1; }
      OVERRIDE_LOG_LEVEL="$2"; shift 2 ;;
    --log-format)
      [[ -z "${2:-}" ]] && { echo "[!] --log-format requires a value"; exit 1; }
      OVERRIDE_LOG_FORMAT="$2"; shift 2 ;;
    --app-env)
      [[ -z "${2:-}" ]] && { echo "[!] --app-env requires a value"; exit 1; }
      OVERRIDE_APP_ENV="$2"; shift 2 ;;
    --seed)
      SEED=true; shift ;;
    --migrate-only)
      MIGRATE_ONLY=true; shift ;;
    -h|--help)
      usage; exit 0 ;;
    *)
      echo "[!] Unknown option: $1"
      usage; exit 1 ;;
  esac
done

# =============================
# Load env file (layer 1)
# =============================
load_env_file() {
  local file="$1"

  if [[ ! -f "$file" ]]; then
    echo "[!] Env file not found: $file"
    exit 1
  fi

  if [[ ! -r "$file" ]]; then
    echo "[!] Env file is not readable: $file"
    exit 1
  fi

  echo "[*] Loading env overrides from: $file"

while IFS='=' read -r key val || [[ -n "$key" ]]; do
    # Trim leading/trailing whitespace using internal Bash (faster & safer)
    key="${key#"${key%%[![:space:]]*}"}"
    key="${key%"${key##*[![:space:]]}"}"
    
    # Skip empty lines and comments
    [[ -z "$key" ]] && continue
    [[ "$key" == "#"* ]] && continue

    # Trim whitespace for value
    val="${val#"${val%%[![:space:]]*}"}"
    val="${val%"${val##*[![:space:]]}"}"
    
    # Strip quotes
    val="${val%\"}"; val="${val#\"}"
    val="${val%\'}"; val="${val#\'}"

    case "$key" in
      APP_ENV)          CFG_APP_ENV="$val" ;;
      HTTP_ADDR)        CFG_HTTP_ADDR="$val" ;;
      ALLOWED_ORIGINS)  CFG_ALLOWED_ORIGINS="$val" ;;
      DATABASE_URL)     CFG_DATABASE_URL="$val" ;;
      REDIS_ADDR)       CFG_REDIS_ADDR="$val" ;;
      REDIS_USERNAME)   CFG_REDIS_USERNAME="$val" ;;
      REDIS_PASSWORD)   CFG_REDIS_PASSWORD="$val" ;;
      REDIS_DB)         CFG_REDIS_DB="$val" ;;
      JWT_SECRET)       CFG_JWT_SECRET="$val" ;;
      JWT_EXPIRY)       CFG_JWT_EXPIRY="$val" ;;
      TIME_ZONE)        CFG_TIME_ZONE="$val" ;;
      LOG_LEVEL)        CFG_LOG_LEVEL="$val" ;;
      LOG_FORMAT)       CFG_LOG_FORMAT="$val" ;;
      *) echo "  [!] Unrecognised key in env file, skipping: $key" ;;
    esac
  done < "$file"
}

# =============================
# Apply CLI overrides (layer 2)
# =============================
apply_overrides() {
  if [[ -n "${OVERRIDE_APP_ENV:-}" ]];          then CFG_APP_ENV="$OVERRIDE_APP_ENV"; fi
  if [[ -n "${OVERRIDE_HTTP_ADDR:-}" ]];        then CFG_HTTP_ADDR="$OVERRIDE_HTTP_ADDR"; fi
  if [[ -n "${OVERRIDE_ALLOWED_ORIGINS:-}" ]];  then CFG_ALLOWED_ORIGINS="$OVERRIDE_ALLOWED_ORIGINS"; fi
  if [[ -n "${OVERRIDE_DATABASE_URL:-}" ]];     then CFG_DATABASE_URL="$OVERRIDE_DATABASE_URL"; fi
  if [[ -n "${OVERRIDE_REDIS_ADDR:-}" ]];       then CFG_REDIS_ADDR="$OVERRIDE_REDIS_ADDR"; fi
  if [[ -n "${OVERRIDE_REDIS_USERNAME:-}" ]];   then CFG_REDIS_USERNAME="$OVERRIDE_REDIS_USERNAME"; fi
  if [[ -n "${OVERRIDE_REDIS_PASSWORD:-}" ]];   then CFG_REDIS_PASSWORD="$OVERRIDE_REDIS_PASSWORD"; fi
  if [[ -n "${OVERRIDE_REDIS_DB:-}" ]];         then CFG_REDIS_DB="$OVERRIDE_REDIS_DB"; fi
  if [[ -n "${OVERRIDE_JWT_SECRET:-}" ]];       then CFG_JWT_SECRET="$OVERRIDE_JWT_SECRET"; fi
  if [[ -n "${OVERRIDE_JWT_EXPIRY:-}" ]];       then CFG_JWT_EXPIRY="$OVERRIDE_JWT_EXPIRY"; fi
  if [[ -n "${OVERRIDE_TIME_ZONE:-}" ]];        then CFG_TIME_ZONE="$OVERRIDE_TIME_ZONE"; fi
  if [[ -n "${OVERRIDE_LOG_LEVEL:-}" ]];        then CFG_LOG_LEVEL="$OVERRIDE_LOG_LEVEL"; fi
  if [[ -n "${OVERRIDE_LOG_FORMAT:-}" ]];       then CFG_LOG_FORMAT="$OVERRIDE_LOG_FORMAT"; fi
}

# =============================
# Validation
# =============================
validate_config() {
  local errors=0

  # APP_ENV
  if [[ ! "$CFG_APP_ENV" =~ ^(production|development)$ ]]; then
    echo "[!] Invalid APP_ENV: '$CFG_APP_ENV'. Must be: production, development"
    errors=$((errors + 1))
  fi

  # HTTP_ADDR must be :port or host:port
  if [[ ! "$CFG_HTTP_ADDR" =~ ^[^:]*:[0-9]+$ ]]; then
    echo "[!] Invalid HTTP_ADDR: '$CFG_HTTP_ADDR'. Expected format: :3000 or host:3000"
    errors=$((errors + 1))
  fi

  # DATABASE_URL must not be empty and must look like a DSN
  if [[ -z "$CFG_DATABASE_URL" ]]; then
    echo "[!] DATABASE_URL must not be empty"
    errors=$((errors + 1))
  elif [[ ! "$CFG_DATABASE_URL" =~ ^(postgres|postgresql):// ]]; then
    echo "[!] Invalid DATABASE_URL: '$CFG_DATABASE_URL'. Must start with postgres:// or postgresql://"
    errors=$((errors + 1))
  fi

  # REDIS_ADDR must be host:port
  if [[ ! "$CFG_REDIS_ADDR" =~ ^.+:[0-9]+$ ]]; then
    echo "[!] Invalid REDIS_ADDR: '$CFG_REDIS_ADDR'. Expected format: host:port"
    errors=$((errors + 1))
  fi

  # REDIS_DB must be a non-negative integer
  if [[ ! "$CFG_REDIS_DB" =~ ^[0-9]+$ ]]; then
    echo "[!] Invalid REDIS_DB: '$CFG_REDIS_DB'. Must be a non-negative integer"
    errors=$((errors + 1))
  fi

  # JWT_SECRET must not be empty
  if [[ -z "$CFG_JWT_SECRET" ]]; then
    echo "[!] JWT_SECRET must not be empty"
    errors=$((errors + 1))
  fi

  # Warn if JWT_SECRET is still the default placeholder
  if [[ "$CFG_JWT_SECRET" == "secret" && "$CFG_APP_ENV" == "production" ]]; then
    echo "[!] WARNING: JWT_SECRET is set to the default placeholder 'secret' in production!"
    errors=$((errors + 1))
  fi

  # JWT_EXPIRY must look like a Go duration (e.g. 24h, 30m, 7d)
  if [[ ! "$CFG_JWT_EXPIRY" =~ ^[0-9]+(s|m|h|d)$ ]]; then
    echo "[!] Invalid JWT_EXPIRY: '$CFG_JWT_EXPIRY'. Expected a duration like 24h, 30m, 7d"
    errors=$((errors + 1))
  fi

  # LOG_LEVEL
  if [[ ! "$CFG_LOG_LEVEL" =~ ^(debug|info|warn|error)$ ]]; then
    echo "[!] Invalid LOG_LEVEL: '$CFG_LOG_LEVEL'. Must be: debug, info, warn, error"
    errors=$((errors + 1))
  fi

  # LOG_FORMAT
  if [[ ! "$CFG_LOG_FORMAT" =~ ^(text|json)$ ]]; then
    echo "[!] Invalid LOG_FORMAT: '$CFG_LOG_FORMAT'. Must be: text, json"
    errors=$((errors + 1))
  fi

  if [[ $errors -gt 0 ]]; then
    echo ""
    echo "[!] $errors validation error(s) found. Aborting installation."
    exit 1
  fi
}

# =============================
# Pre-flight checks
# =============================
preflight_checks() {
  if [[ $EUID -ne 0 ]]; then
    echo "[!] This script must be run as root (use sudo)"
    exit 1
  fi

  if [[ ! -f "$BIN_SOURCE" ]]; then
    echo "[!] Server binary not found at $BIN_SOURCE"
    exit 1
  fi
}

# =============================
# Paths
# =============================
INSTALL_BIN="/usr/local/bin/horizonx-server"
BIN_SOURCE="./bin/server"
MIGRATE_BIN="./bin/migrate"
SEED_BIN="./bin/seed"
CONFIG_DIR="/var/lib/horizonx/server.env"
LOG_DIR="/var/log/horizonx"
DATA_DIR="/var/lib/horizonx"
USER_NAME="horizonx"
GROUP_NAME="horizonx"
SERVICE_NAME="horizonx-server"
SERVICE_FILE="/etc/systemd/system/$SERVICE_NAME.service"

# =============================
# Run pre-flight, load config
# =============================
preflight_checks

[[ -n "$ENV_FILE" ]] && load_env_file "$ENV_FILE"

apply_overrides

validate_config

# =============================
# Print resolved config (mask secrets)
# =============================
echo ""
echo "[*] Resolved configuration:"
echo "    APP_ENV           = $CFG_APP_ENV"
echo "    HTTP_ADDR         = $CFG_HTTP_ADDR"
echo "    ALLOWED_ORIGINS   = $CFG_ALLOWED_ORIGINS"
MASKED_DB=$(echo "$CFG_DATABASE_URL" | sed 's/:\/\/[^:]*:[^@]*@/:\/\/***:***@/')
echo "    DATABASE_URL      = $MASKED_DB"
echo "    REDIS_ADDR        = $CFG_REDIS_ADDR"
echo "    REDIS_USERNAME    = ${CFG_REDIS_USERNAME:-<empty>}"
if [[ -n "${CFG_REDIS_PASSWORD:-}" ]]; then
  echo "    REDIS_PASSWORD    = ********"
else
  echo "    REDIS_PASSWORD    = <empty>"
fi
echo "    REDIS_DB          = $CFG_REDIS_DB"
echo "    JWT_SECRET        = ${CFG_JWT_SECRET:0:4}***"
echo "    JWT_EXPIRY        = $CFG_JWT_EXPIRY"
echo "    TIME_ZONE         = $CFG_TIME_ZONE"
echo "    LOG_LEVEL         = $CFG_LOG_LEVEL"
echo "    LOG_FORMAT        = $CFG_LOG_FORMAT"
echo "    --seed            = $SEED"
echo "    --migrate-only    = $MIGRATE_ONLY"
echo ""

# =============================
# User & Group
# =============================
if ! id -u "$USER_NAME" >/dev/null 2>&1; then
  echo "[*] Creating user $USER_NAME"
  groupadd -f "$GROUP_NAME"
  useradd -r \
    -g "$GROUP_NAME" \
    -d "$DATA_DIR" \
    -s /usr/sbin/nologin \
    "$USER_NAME"
else
  echo "[*] User exists, skipping"
fi

# =============================
# Directories
# =============================
echo "[*] Creating directories"
mkdir -p "$DATA_DIR" "$LOG_DIR"
chmod 750 "$DATA_DIR"
chown -R "$USER_NAME:$GROUP_NAME" "$DATA_DIR" "$LOG_DIR"
touch "$LOG_DIR/server.log" "$LOG_DIR/server.error.log"
chown "$USER_NAME:$GROUP_NAME" "$LOG_DIR/"*.log

# =============================
# Stop service if running
# =============================
if systemctl is-active --quiet "$SERVICE_NAME" 2>/dev/null; then
  echo "[*] Stopping $SERVICE_NAME to update binary..."
  systemctl stop "$SERVICE_NAME"
fi

# =============================
# Install binary
# =============================
echo "[*] Installing server binary"
cp "$BIN_SOURCE" "$INSTALL_BIN"
chmod 755 "$INSTALL_BIN"
chown root:root "$INSTALL_BIN"

# =============================
# Write env file (always overwrite with resolved config)
# =============================
echo "[*] Writing env file"
cat > "$CONFIG_DIR" <<EOF
APP_ENV=$CFG_APP_ENV
TIME_ZONE="$CFG_TIME_ZONE"
HTTP_ADDR="$CFG_HTTP_ADDR"
ALLOWED_ORIGINS="$CFG_ALLOWED_ORIGINS"
DATABASE_URL="$CFG_DATABASE_URL"
REDIS_ADDR="$CFG_REDIS_ADDR"
REDIS_USERNAME="$CFG_REDIS_USERNAME"
REDIS_PASSWORD="$CFG_REDIS_PASSWORD"
REDIS_DB="$CFG_REDIS_DB"
JWT_SECRET="$CFG_JWT_SECRET"
JWT_EXPIRY="$CFG_JWT_EXPIRY"
LOG_LEVEL=$CFG_LOG_LEVEL
LOG_FORMAT=$CFG_LOG_FORMAT
EOF
chown "$USER_NAME:$GROUP_NAME" "$CONFIG_DIR"
chmod 600 "$CONFIG_DIR"

# =============================
# Run migrations
# =============================
if [[ -f "$MIGRATE_BIN" ]]; then
  chmod +x "$MIGRATE_BIN"
  echo "[*] Running migrations..."
  "$MIGRATE_BIN" -op=up -dsn="$CFG_DATABASE_URL" || { echo "[!] Migration failed!"; exit 1; }
  echo "  [✓] Migrations complete"
else
  echo "[!] Migration binary NOT FOUND at $MIGRATE_BIN. This is probably bad."
fi

# =============================
# Optional seeding
# =============================
if [[ "$SEED" == true ]]; then
  if [[ -x "$SEED_BIN" ]]; then
    echo "[*] Seeding data..."
    "$SEED_BIN" -dsn="$CFG_DATABASE_URL"
    echo "  [✓] Seeding complete"
  else
    echo "[!] Seed binary not found or not executable at $SEED_BIN, skipping"
  fi
fi

# =============================
# Migrate-only mode: exit here
# =============================
if [[ "$MIGRATE_ONLY" == true ]]; then
  echo ""
  echo "[✓] Migrate-only mode: done. Service not installed or started."
  exit 0
fi

# =============================
# systemd service
# =============================
echo "[*] Writing systemd service"
cat > "$SERVICE_FILE" <<EOF
[Unit]
Description=HorizonX Server
After=network-online.target postgresql.service redis.service
Wants=network-online.target

[Service]
Type=simple
User=$USER_NAME
Group=$GROUP_NAME
EnvironmentFile=$CONFIG_DIR
ExecStart=$INSTALL_BIN
Restart=always
RestartSec=5

# --- Security Hardening ---
AmbientCapabilities=CAP_NET_BIND_SERVICE
NoNewPrivileges=true
ProtectSystem=full
ProtectHome=true
PrivateTmp=true
PrivateDevices=true
ReadWritePaths=$DATA_DIR $LOG_DIR

# --- Resource Limit ---
LimitNOFILE=65535

StandardOutput=append:$LOG_DIR/server.log
StandardError=append:$LOG_DIR/server.error.log
SyslogIdentifier=horizonx-server

[Install]
WantedBy=multi-user.target
EOF

# =============================
# Enable & start
# =============================
systemctl daemon-reload
systemctl enable "$SERVICE_NAME"

echo "  [*] Starting $SERVICE_NAME..."
systemctl start "$SERVICE_NAME"

sleep 2
if systemctl is-active --quiet "$SERVICE_NAME"; then
  echo "  [✓] Service started successfully"

  echo "[*] Verifying API Health..."
  if curl -s "http://localhost${CFG_HTTP_ADDR#*:}"/health > /dev/null; then
    echo "    [✓] API is healthy!"
  else
    echo "    [!] API is not responding on /health (check logs)"
  fi
else
  echo "  [!] Service failed to start, check logs:"
  echo "      journalctl -u $SERVICE_NAME -n 20 --no-pager"
fi

echo ""
echo "[✓] HorizonX Server installed"
echo ""
echo "[*] Service status:"
systemctl status "$SERVICE_NAME" --no-pager -l || true
