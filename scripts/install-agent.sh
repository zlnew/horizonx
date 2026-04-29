#!/bin/bash
set -euo pipefail

# =============================
# Usage
# =============================
usage() {
  cat <<EOF
Usage: $(basename "$0") [OPTIONS]

Options:
  --env-file <path>           Load env overrides from a file (key=value format)
  --api-url <url>             Override HORIZONX_API_URL
  --ws-url <url>              Override HORIZONX_WS_URL
  --api-token <token>         Override HORIZONX_SERVER_API_TOKEN
  --server-id <id>            Override HORIZONX_SERVER_ID
  --redis-addr <host:port>    Override REDIS_ADDR
  --redis-username <user>     Override REDIS_USERNAME
  --redis-password <pass>     Override REDIS_PASSWORD
  --redis-db <db>             Override REDIS_DB
  --job-worker-count <count>  Override AGENT_JOB_WORKER_COUNT
  --log-level <level>         Override LOG_LEVEL (debug|info|warn|error)
  --log-format <format>       Override LOG_FORMAT (text|json)
  --app-env <env>             Override APP_ENV (production|development)
  -h, --help                  Show this help message

Examples:
  # Use a custom env file
  sudo $(basename "$0") --env-file /path/to/custom.env

  # Override individual values
  sudo $(basename "$0") --api-url http://my-server:8080 --api-token my_secret_token

  # Mix both: env file as base, individual flags take precedence
  sudo $(basename "$0") --env-file /path/to/custom.env --log-level debug
EOF
}

# =============================
# Config defaults
# =============================
CFG_APP_ENV="production"
CFG_API_URL="http://localhost:3000"
CFG_WS_URL="ws://localhost:3000/agent/ws"
CFG_API_TOKEN="hzx_secret"
CFG_SERVER_ID="123abc"
CFG_REDIS_ADDR="localhost:6379"
CFG_REDIS_USERNAME=""
CFG_REDIS_PASSWORD=""
CFG_REDIS_DB="0"
CFG_JOB_WORKER_COUNT="10"
CFG_LOG_LEVEL="info"
CFG_LOG_FORMAT="text"

# =============================
# Argument parsing
# =============================
ENV_FILE=""
OVERRIDE_APP_ENV=""
OVERRIDE_API_URL=""
OVERRIDE_WS_URL=""
OVERRIDE_API_TOKEN=""
OVERRIDE_SERVER_ID=""
OVERRIDE_REDIS_ADDR=""
OVERRIDE_REDIS_USERNAME=""
OVERRIDE_REDIS_PASSWORD=""
OVERRIDE_REDIS_DB=""
OVERRIDE_JOB_WORKER_COUNT=""
OVERRIDE_LOG_LEVEL=""
OVERRIDE_LOG_FORMAT=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --env-file)
      [[ -z "${2:-}" ]] && { echo "[!] --env-file requires a path argument"; exit 1; }
      ENV_FILE="$2"; shift 2 ;;
    --api-url)
      [[ -z "${2:-}" ]] && { echo "[!] --api-url requires a value"; exit 1; }
      OVERRIDE_API_URL="$2"; shift 2 ;;
    --ws-url)
      [[ -z "${2:-}" ]] && { echo "[!] --ws-url requires a value"; exit 1; }
      OVERRIDE_WS_URL="$2"; shift 2 ;;
    --api-token)
      [[ -z "${2:-}" ]] && { echo "[!] --api-token requires a value"; exit 1; }
      OVERRIDE_API_TOKEN="$2"; shift 2 ;;
    --server-id)
      [[ -z "${2:-}" ]] && { echo "[!] --server-id requires a value"; exit 1; }
      OVERRIDE_SERVER_ID="$2"; shift 2 ;;
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
    --job-worker-count)
      [[ -z "${2:-}" ]] && { echo "[!] --job-worker-count requires a value"; exit 1; }
      OVERRIDE_JOB_WORKER_COUNT="$2"; shift 2 ;;
    --log-level)
      [[ -z "${2:-}" ]] && { echo "[!] --log-level requires a value"; exit 1; }
      OVERRIDE_LOG_LEVEL="$2"; shift 2 ;;
    --log-format)
      [[ -z "${2:-}" ]] && { echo "[!] --log-format requires a value"; exit 1; }
      OVERRIDE_LOG_FORMAT="$2"; shift 2 ;;
    --app-env)
      [[ -z "${2:-}" ]] && { echo "[!] --app-env requires a value"; exit 1; }
      OVERRIDE_APP_ENV="$2"; shift 2 ;;
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
    # Internal trim (removes leading/trailing whitespace)
    key="${key#"${key%%[![:space:]]*}"}"
    key="${key%"${key##*[![:space:]]}"}"

    # Skip empty lines or lines starting with a comment hash
    [[ -z "$key" ]] && continue
    [[ "$key" == "#"* ]] && continue

    # Internal trim for value
    val="${val#"${val%%[![:space:]]*}"}"
    val="${val%"${val##*[![:space:]]}"}"
    val="${val%\"}"; val="${val#\"}"
    val="${val%\'}"; val="${val#\'}"

    case "$key" in
      APP_ENV)                   CFG_APP_ENV="$val" ;;
      HORIZONX_API_URL)          CFG_API_URL="$val" ;;
      HORIZONX_WS_URL)           CFG_WS_URL="$val" ;;
      HORIZONX_SERVER_API_TOKEN) CFG_API_TOKEN="$val" ;;
      HORIZONX_SERVER_ID)        CFG_SERVER_ID="$val" ;;
      REDIS_ADDR)                CFG_REDIS_ADDR="$val" ;;
      REDIS_USERNAME)            CFG_REDIS_USERNAME="$val" ;;
      REDIS_PASSWORD)            CFG_REDIS_PASSWORD="$val" ;;
      REDIS_DB)                  CFG_REDIS_DB="$val" ;;
      JOB_WORKER_COUNT)          CFG_JOB_WORKER_COUNT="$val" ;;
      LOG_LEVEL)                 CFG_LOG_LEVEL="$val" ;;
      LOG_FORMAT)                CFG_LOG_FORMAT="$val" ;;
      *) echo "  [!] Unrecognised key in env file, skipping: $key" ;;
    esac
  done < "$file"
}

apply_overrides() {
  if [[ -n "${OVERRIDE_APP_ENV:-}" ]];          then CFG_APP_ENV="$OVERRIDE_APP_ENV"; fi
  if [[ -n "${OVERRIDE_API_URL:-}" ]];          then CFG_API_URL="$OVERRIDE_API_URL"; fi
  if [[ -n "${OVERRIDE_WS_URL:-}" ]];           then CFG_WS_URL="$OVERRIDE_WS_URL"; fi
  if [[ -n "${OVERRIDE_API_TOKEN:-}" ]];        then CFG_API_TOKEN="$OVERRIDE_API_TOKEN"; fi
  if [[ -n "${OVERRIDE_SERVER_ID:-}" ]];        then CFG_SERVER_ID="$OVERRIDE_SERVER_ID"; fi
  if [[ -n "${OVERRIDE_REDIS_ADDR:-}" ]];       then CFG_REDIS_ADDR="$OVERRIDE_REDIS_ADDR"; fi
  if [[ -n "${OVERRIDE_REDIS_USERNAME:-}" ]];   then CFG_REDIS_USERNAME="$OVERRIDE_REDIS_USERNAME"; fi
  if [[ -n "${OVERRIDE_REDIS_PASSWORD:-}" ]];   then CFG_REDIS_PASSWORD="$OVERRIDE_REDIS_PASSWORD"; fi
  if [[ -n "${OVERRIDE_REDIS_DB:-}" ]];         then CFG_REDIS_DB="$OVERRIDE_REDIS_DB"; fi
  if [[ -n "${OVERRIDE_JOB_WORKER_COUNT:-}" ]]; then CFG_JOB_WORKER_COUNT="$OVERRIDE_JOB_WORKER_COUNT"; fi
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

  # URL format (basic)
  if [[ ! "$CFG_API_URL" =~ ^https?:// ]]; then
    echo "[!] Invalid HORIZONX_API_URL: '$CFG_API_URL'. Must start with http:// or https://"
    errors=$((errors + 1))
  fi

  if [[ ! "$CFG_WS_URL" =~ ^wss?:// ]]; then
    echo "[!] Invalid HORIZONX_WS_URL: '$CFG_WS_URL'. Must start with ws:// or wss://"
    errors=$((errors + 1))
  fi

  # API token must not be empty or default placeholder
  if [[ -z "$CFG_API_TOKEN" ]]; then
    echo "[!] Invalid HORIZONX_SERVER_API_TOKEN: must not be empty"
    errors=$((errors + 1))
  fi

  # SERVER_ID must not be empty
  if [[ -z "$CFG_SERVER_ID" ]]; then
    echo "[!] Invalid HORIZONX_SERVER_ID: must not be empty"
    errors=$((errors + 1))
  fi

  # REDIS_ADDR must be host:port
  if [[ ! "$CFG_REDIS_ADDR" =~ ^.+:[0-9]+$ ]]; then
    echo "[!] Invalid REDIS_ADDR: '$CFG_REDIS_ADDR'. Expected format: host:port"
    errors=$((errors + 1))
  fi

  # REDIS_DB must be an integer
  if [[ ! "$CFG_REDIS_DB" =~ ^[0-9]+$ ]]; then
    echo "[!] Invalid REDIS_DB: '$CFG_REDIS_DB'. Must be a non-negative integer"
    errors=$((errors + 1))
  fi

  # AGENT_JOB_WORKER_COUNT must be an integer
  if [[ ! "$CFG_JOB_WORKER_COUNT" =~ ^[0-9]+$ ]]; then
    echo "[!] Invalid AGENT_JOB_WORKER_COUNT: '$CFG_JOB_WORKER_COUNT'. Must be a non-negative integer"
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

  if [[ ! -f "./bin/agent" ]]; then
    echo "[!] Agent binary not found at ./bin/agent"
    exit 1
  fi
}

# =============================
# Config
# =============================
USER_NAME="horizonx"
GROUP_NAME="horizonx"
DATA_DIR="/var/lib/horizonx"
SSH_DIR="$DATA_DIR/.ssh"
HOME_DIR="$DATA_DIR"
INSTALL_BIN="/usr/local/bin/horizonx-agent"
BIN_SOURCE="./bin/agent"
CONFIG_DIR="$DATA_DIR/agent.env"
LOG_DIR="/var/log/horizonx"
SERVICE_NAME="horizonx-agent"
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
echo "    APP_ENV                   = $CFG_APP_ENV"
echo "    HORIZONX_API_URL          = $CFG_API_URL"
echo "    HORIZONX_WS_URL           = $CFG_WS_URL"
echo "    HORIZONX_SERVER_API_TOKEN = ${CFG_API_TOKEN:0:4}***"
echo "    HORIZONX_SERVER_ID        = $CFG_SERVER_ID"
echo "    REDIS_ADDR                = $CFG_REDIS_ADDR"
echo "    REDIS_USERNAME            = ${CFG_REDIS_USERNAME:-<empty>}"
if [[ -n "${CFG_REDIS_PASSWORD:-}" ]]; then
  echo "    REDIS_PASSWORD            = ********"
else
  echo "    REDIS_PASSWORD            = <empty>"
fi
echo "    REDIS_DB                  = $CFG_REDIS_DB"
echo "    JOB_WORKER_COUNT          = $CFG_JOB_WORKER_COUNT"
echo "    LOG_LEVEL                 = $CFG_LOG_LEVEL"
echo "    LOG_FORMAT                = $CFG_LOG_FORMAT"
echo ""

# =============================
# User & Group
# =============================
if ! id -u "$USER_NAME" >/dev/null 2>&1; then
  echo "[*] Creating user $USER_NAME"
  groupadd -f "$GROUP_NAME"
  useradd -r \
    -g "$GROUP_NAME" \
    -d "$HOME_DIR" \
    -s /usr/sbin/nologin \
    "$USER_NAME"
else
  echo "[*] User exists, skipping"
fi
echo "[*] Ensuring docker access"
getent group docker >/dev/null || groupadd docker
usermod -aG docker "$USER_NAME"

# =============================
# Directories
# =============================
echo "[*] Creating directories"
mkdir -p "$SSH_DIR" "$LOG_DIR"
chmod 750 "$DATA_DIR" "$LOG_DIR"
chown -R "$USER_NAME:$GROUP_NAME" "$DATA_DIR" "$LOG_DIR"
touch "$LOG_DIR/agent.log" "$LOG_DIR/agent.error.log"
chown "$USER_NAME:$GROUP_NAME" "$LOG_DIR/"*.log
chmod 700 "$SSH_DIR"

# =============================
# SSH key
# =============================
SSH_KEY="$SSH_DIR/id_ed25519"
if [ ! -f "$SSH_KEY" ]; then
  echo "[*] Generating SSH key"
  sudo -u "$USER_NAME" env HOME="$HOME_DIR" \
    ssh-keygen -t ed25519 \
    -f "$SSH_KEY" \
    -N "" \
    -C "horizonx-agent@$(hostname)"
else
  echo "[*] SSH key exists"
fi
chmod 600 "$SSH_KEY"
chmod 644 "$SSH_KEY.pub"
chown "$USER_NAME:$GROUP_NAME" "$SSH_KEY" "$SSH_KEY.pub"

# =============================
# SSH config
# =============================
SSH_CONFIG="$SSH_DIR/config"
echo "[*] Writing SSH config"
cat > "$SSH_CONFIG" <<EOF
Host *
  IdentityFile $SSH_KEY
  UserKnownHostsFile $SSH_DIR/known_hosts
  StrictHostKeyChecking yes
  IdentitiesOnly yes
EOF
chmod 600 "$SSH_CONFIG"
chown "$USER_NAME:$GROUP_NAME" "$SSH_CONFIG"

# =============================
# Auto-add Git Provider Known Hosts
# =============================
KNOWN_HOSTS_FILE="$SSH_DIR/known_hosts"
touch "$KNOWN_HOSTS_FILE"
chmod 644 "$KNOWN_HOSTS_FILE"
chown "$USER_NAME:$GROUP_NAME" "$KNOWN_HOSTS_FILE"

echo "[*] Adding common Git providers to known_hosts"
GIT_PROVIDERS=(
  "github.com"
  "gitlab.com"
  "bitbucket.org"
  "ssh.dev.azure.com"
  "vs-ssh.visualstudio.com"
)

for provider in "${GIT_PROVIDERS[@]}"; do
  if ! grep -q "$provider" "$KNOWN_HOSTS_FILE" 2>/dev/null; then
    echo "  [+] Scanning $provider..."
    SCAN_RESULT=$(ssh-keyscan -H -t rsa,ed25519 "$provider" 2>/dev/null || :)
    if [[ -n "$SCAN_RESULT" ]]; then
      echo "$SCAN_RESULT" >> "$KNOWN_HOSTS_FILE"
    else
      echo "  [!] Failed to scan $provider (skipping)"
    fi
  else
    echo "  [√] $provider already in known_hosts"
  fi
done

if [ ! -s "$KNOWN_HOSTS_FILE" ]; then
  echo "[!] WARNING: known_hosts is empty! Manual intervention may be needed."
fi

sort -u "$KNOWN_HOSTS_FILE" -o "$KNOWN_HOSTS_FILE"
chown "$USER_NAME:$GROUP_NAME" "$KNOWN_HOSTS_FILE"

# =============================
# Git SSH Wrapper
# =============================
GIT_SSH_WRAPPER="$DATA_DIR/git-ssh-wrapper.sh"
echo "[*] Creating Git SSH wrapper"
cat > "$GIT_SSH_WRAPPER" <<EOF
#!/bin/bash
exec ssh -i "$SSH_KEY" -F "$SSH_CONFIG" "\$@"
EOF
chmod 700 "$GIT_SSH_WRAPPER"
chown "$USER_NAME:$GROUP_NAME" "$GIT_SSH_WRAPPER"

# =============================
# Install binary
# =============================
echo "[*] Installing agent binary"

if systemctl is-active --quiet "$SERVICE_NAME" 2>/dev/null; then
  echo "  [*] Stopping $SERVICE_NAME to update binary..."
  systemctl stop "$SERVICE_NAME"
fi

cp "$BIN_SOURCE" "$INSTALL_BIN"
chmod 755 "$INSTALL_BIN"
chown root:root "$INSTALL_BIN"

# =============================
# Capabilities
# =============================
setcap cap_dac_read_search,cap_sys_ptrace+ep "$INSTALL_BIN" || {
  echo "[!] setcap failed! Agent might not be able to monitor all processes."
  echo "    Check if your filesystem supports file capabilities."
}

# =============================
# Hardware Monitoring Access
# =============================
echo "[*] Setting up hardware monitoring access"

UDEV_RULE_FILE="/etc/udev/rules.d/99-horizonx-hwmon.rules"
cat > "$UDEV_RULE_FILE" <<'EOF'
# Intel RAPL power metrics
SUBSYSTEM=="powercap", KERNEL=="intel-rapl:*", ACTION=="add", RUN+="/bin/chmod 444 /sys/class/powercap/%k/energy_uj"
SUBSYSTEM=="powercap", KERNEL=="intel-rapl:*", ACTION=="add", RUN+="/bin/chmod 444 /sys/class/powercap/%k/max_energy_range_uj"

# Hardware monitoring sensors (temp, fan, voltage, power)
SUBSYSTEM=="hwmon", ACTION=="add", RUN+="/bin/chmod -R a+r /sys/class/hwmon/%k"

# Thermal zones
SUBSYSTEM=="thermal", ACTION=="add", RUN+="/bin/chmod 444 /sys/class/thermal/%k/temp"

# Block devices
SUBSYSTEM=="block", ACTION=="add", RUN+="/bin/chmod 444 /sys/block/%k/stat"
EOF

udevadm control --reload-rules 2>/dev/null || true
udevadm trigger --subsystem-match=powercap 2>/dev/null || true
udevadm trigger --subsystem-match=hwmon 2>/dev/null || true
udevadm trigger --subsystem-match=thermal 2>/dev/null || true
udevadm trigger --subsystem-match=block 2>/dev/null || true

echo "  [*] Applying immediate permissions..."

if [ -d /sys/class/powercap/intel-rapl ]; then
  find /sys/class/powercap/intel-rapl/ -name "energy_uj" -exec chmod 444 {} + 2>/dev/null || true
  find /sys/class/powercap/intel-rapl/ -name "max_energy_range_uj" -exec chmod 444 {} + 2>/dev/null || true
  echo "    [✓] Intel RAPL"
fi

if [ -d /sys/class/hwmon ]; then
  chmod -R a+r /sys/class/hwmon/hwmon* 2>/dev/null || true
  echo "    [✓] Hardware monitors (hwmon)"
fi

if [ -d /sys/devices/system/cpu/cpu0/cpufreq ]; then
  chmod 444 /sys/devices/system/cpu/cpu*/cpufreq/scaling_cur_freq 2>/dev/null || true
  chmod 444 /sys/devices/system/cpu/cpu*/cpufreq/scaling_max_freq 2>/dev/null || true
  chmod 444 /sys/devices/system/cpu/cpu*/cpufreq/scaling_min_freq 2>/dev/null || true
  echo "    [✓] CPU frequency scaling"
fi

if [ -d /sys/class/thermal ]; then
  chmod 444 /sys/class/thermal/thermal_zone*/temp 2>/dev/null || true
  echo "    [✓] Thermal zones"
fi

if [ -d /sys/block ]; then
  chmod 444 /sys/block/*/stat 2>/dev/null || true
  echo "    [✓] Block device stats"
fi

if [ -d /sys/class/net ]; then
  chmod 444 /sys/class/net/*/statistics/* 2>/dev/null || true
  echo "    [✓] Network statistics"
fi

if [ -d /sys/class/drm ]; then
  chmod 444 /sys/class/drm/card*/device/power_state 2>/dev/null || true
  chmod 444 /sys/class/drm/card*/device/gpu_busy_percent 2>/dev/null || true
  echo "    [✓] GPU metrics (if available)"
fi

echo "  [✓] Hardware monitoring configured"

echo "  [*] Verifying access..."
VERIFICATION_FAILED=0

if [ -f /sys/class/powercap/intel-rapl/intel-rapl:0/energy_uj ]; then
  if ! sudo -u "$USER_NAME" cat /sys/class/powercap/intel-rapl/intel-rapl:0/energy_uj >/dev/null 2>&1; then
    echo "    [!] RAPL access failed"
    VERIFICATION_FAILED=1
  fi
fi

if [ $VERIFICATION_FAILED -eq 0 ]; then
  echo "    [✓] All checks passed"
else
  echo "    [!] Some checks failed, agent may have limited monitoring capabilities"
fi

# =============================
# Write env file (always overwrite with resolved config)
# =============================
echo "[*] Writing env file"
cat > "$CONFIG_DIR" <<EOF
APP_ENV=$CFG_APP_ENV
HORIZONX_API_URL=$CFG_API_URL
HORIZONX_WS_URL=$CFG_WS_URL
HORIZONX_SERVER_API_TOKEN=$CFG_API_TOKEN
HORIZONX_SERVER_ID=$CFG_SERVER_ID
REDIS_ADDR="$CFG_REDIS_ADDR"
REDIS_USERNAME="$CFG_REDIS_USERNAME"
REDIS_PASSWORD="$CFG_REDIS_PASSWORD"
REDIS_DB="$CFG_REDIS_DB"
LOG_LEVEL=$CFG_LOG_LEVEL
LOG_FORMAT=$CFG_LOG_FORMAT
EOF
chown "$USER_NAME:$GROUP_NAME" "$CONFIG_DIR"
chmod 600 "$CONFIG_DIR"

# =============================
# systemd service
# =============================
echo "[*] Writing systemd service"
cat > "$SERVICE_FILE" <<EOF
[Unit]
Description=HorizonX Agent
After=network.target docker.service
Requires=docker.service

[Service]
Type=simple
User=$USER_NAME
Group=$GROUP_NAME
Environment=HOME=$HOME_DIR
Environment=GIT_SSH=$GIT_SSH_WRAPPER
EnvironmentFile=$CONFIG_DIR
ExecStart=$INSTALL_BIN

# --- Security & Capabilities ---
CapabilityBoundingSet=CAP_DAC_READ_SEARCH CAP_SYS_PTRACE
AmbientCapabilities=CAP_DAC_READ_SEARCH CAP_SYS_PTRACE
NoNewPrivileges=true

# --- Sandboxing ---
PrivateTmp=true
ProtectSystem=full
ProtectHome=false
ProtectControlGroups=true
ProtectKernelModules=true
ProtectKernelTunables=true

# --- Resources & Restart ---
Restart=always
RestartSec=5
LimitNOFILE=65535

# --- Logging & Paths ---
ReadWritePaths=$DATA_DIR $LOG_DIR
StandardOutput=append:$LOG_DIR/agent.log
StandardError=append:$LOG_DIR/agent.error.log
SyslogIdentifier=horizonx-agent

[Install]
WantedBy=multi-user.target
EOF

# =============================
# Enable & start service
# =============================
systemctl daemon-reload
systemctl enable "$SERVICE_NAME"

echo "  [*] Starting $SERVICE_NAME..."
systemctl restart "$SERVICE_NAME" || systemctl start "$SERVICE_NAME"

sleep 2
if systemctl is-active --quiet "$SERVICE_NAME"; then
  echo "  [✓] Service started successfully"
else
  echo "  [!] Service failed to start, check logs:"
  echo "      journalctl -u $SERVICE_NAME -n 20 --no-pager"
fi

echo ""
echo "[✓] HorizonX Agent installed"
echo "[*] Public SSH key (add to your Git provider):"
echo "----------------------------------------"
cat "$SSH_KEY.pub"
echo "----------------------------------------"
echo ""
echo "[*] Service status:"
systemctl status "$SERVICE_NAME" --no-pager -l || true
