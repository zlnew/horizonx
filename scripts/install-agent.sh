#!/bin/bash
set -e
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

# =============================
# Directories
# =============================
echo "[*] Creating directories"
mkdir -p "$SSH_DIR" "$LOG_DIR"
touch "$LOG_DIR/agent.log" "$LOG_DIR/agent.error.log"
chown -R "$USER_NAME:$GROUP_NAME" "$DATA_DIR" "$LOG_DIR"
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
    ssh-keyscan -H -t rsa,ed25519 "$provider" >> "$KNOWN_HOSTS_FILE" 2>/dev/null || \
      echo "  [!] Failed to scan $provider (skipping)"
  else
    echo "  [√] $provider already in known_hosts"
  fi
done

# Verify known_hosts populated
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
cat > "$GIT_SSH_WRAPPER" <<'EOF'
#!/bin/bash
exec ssh -i "$SSH_KEY" -F "$SSH_CONFIG" "$@"
EOF
# Replace placeholders
sed -i "s|\$SSH_KEY|$SSH_KEY|g" "$GIT_SSH_WRAPPER"
sed -i "s|\$SSH_CONFIG|$SSH_CONFIG|g" "$GIT_SSH_WRAPPER"
chmod 700 "$GIT_SSH_WRAPPER"
chown "$USER_NAME:$GROUP_NAME" "$GIT_SSH_WRAPPER"

# =============================
# Install binary
# =============================
echo "[*] Installing agent binary"

# Stop service if running
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
setcap cap_dac_read_search,cap_sys_ptrace+ep "$INSTALL_BIN" || \
  echo "[!] setcap skipped"

# =============================
# Hardware Monitoring Access
# =============================
echo "[*] Setting up hardware monitoring access"

# udev rules for persistence
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

# Reload udev
udevadm control --reload-rules 2>/dev/null || true
udevadm trigger --subsystem-match=powercap 2>/dev/null || true
udevadm trigger --subsystem-match=hwmon 2>/dev/null || true
udevadm trigger --subsystem-match=thermal 2>/dev/null || true
udevadm trigger --subsystem-match=block 2>/dev/null || true

# Immediate fixes (apply now, udev handles future)
echo "  [*] Applying immediate permissions..."

# Intel RAPL (power)
if [ -d /sys/class/powercap/intel-rapl ]; then
  chmod 444 /sys/class/powercap/intel-rapl/intel-rapl:*/energy_uj 2>/dev/null || true
  chmod 444 /sys/class/powercap/intel-rapl/intel-rapl:*/max_energy_range_uj 2>/dev/null || true
  echo "    [✓] Intel RAPL"
fi

# AMD/Intel hwmon (power, temp, fan, voltage)
if [ -d /sys/class/hwmon ]; then
  chmod -R a+r /sys/class/hwmon/hwmon* 2>/dev/null || true
  echo "    [✓] Hardware monitors (hwmon)"
fi

# CPU frequency
if [ -d /sys/devices/system/cpu/cpu0/cpufreq ]; then
  chmod 444 /sys/devices/system/cpu/cpu*/cpufreq/scaling_cur_freq 2>/dev/null || true
  chmod 444 /sys/devices/system/cpu/cpu*/cpufreq/scaling_max_freq 2>/dev/null || true
  chmod 444 /sys/devices/system/cpu/cpu*/cpufreq/scaling_min_freq 2>/dev/null || true
  echo "    [✓] CPU frequency scaling"
fi

# Thermal zones
if [ -d /sys/class/thermal ]; then
  chmod 444 /sys/class/thermal/thermal_zone*/temp 2>/dev/null || true
  echo "    [✓] Thermal zones"
fi

# Block device stats
if [ -d /sys/block ]; then
  chmod 444 /sys/block/*/stat 2>/dev/null || true
  echo "    [✓] Block device stats"
fi

# Network stats (usually already readable, but just in case)
if [ -d /sys/class/net ]; then
  chmod 444 /sys/class/net/*/statistics/* 2>/dev/null || true
  echo "    [✓] Network statistics"
fi

# GPU (DRM) if available
if [ -d /sys/class/drm ]; then
  chmod 444 /sys/class/drm/card*/device/power_state 2>/dev/null || true
  chmod 444 /sys/class/drm/card*/device/gpu_busy_percent 2>/dev/null || true
  echo "    [✓] GPU metrics (if available)"
fi

echo "  [✓] Hardware monitoring configured"

# Verify critical metrics accessible
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
# Env file
# =============================
if [ ! -f "$CONFIG_DIR" ]; then
  echo "[*] Creating env file"
  cat > "$CONFIG_DIR" <<EOF
HORIZONX_API_URL=http://localhost:3000
HORIZONX_WS_URL=ws://localhost:3000/agent/ws
HORIZONX_SERVER_API_TOKEN=hzx_secret
HORIZONX_SERVER_ID=123
LOG_LEVEL=info
LOG_FORMAT=text
EOF
  chown "$USER_NAME:$GROUP_NAME" "$CONFIG_DIR"
  chmod 600 "$CONFIG_DIR"
fi

# =============================
# systemd service
# =============================
echo "[*] Writing systemd service"
cat > "$SERVICE_FILE" <<EOF
[Unit]
Description=HorizonX Agent
After=network.target docker.service
Wants=docker.service

[Service]
Type=simple
User=$USER_NAME
Group=$GROUP_NAME
Environment=HOME=$HOME_DIR
Environment=GIT_SSH=$GIT_SSH_WRAPPER
EnvironmentFile=$CONFIG_DIR
ExecStart=$INSTALL_BIN --config $CONFIG_DIR
Restart=always
RestartSec=5
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=true
ProtectHome=false
ReadWritePaths=$DATA_DIR $LOG_DIR
StandardOutput=file:$LOG_DIR/agent.log
StandardError=file:$LOG_DIR/agent.error.log

[Install]
WantedBy=multi-user.target
EOF

# =============================
# Enable service
# =============================
systemctl daemon-reload
systemctl enable "$SERVICE_NAME"

echo "  [*] Starting $SERVICE_NAME..."
systemctl start "$SERVICE_NAME"

# Wait a bit and check status
sleep 2
if systemctl is-active --quiet "$SERVICE_NAME"; then
  echo "  [✓] Service started successfully"
else
  echo "  [!] Service failed to start, check logs:"
  echo "      journalctl -u $SERVICE_NAME -n 20 --no-pager"
fi

echo
echo "[✓] HorizonX Agent installed"
echo "[*] Public SSH key (add to Git provider):"
echo "----------------------------------------"
cat "$SSH_KEY.pub"
echo "----------------------------------------"
echo
echo "[*] Service status:"
systemctl status "$SERVICE_NAME" --no-pager -l || true
