#!/bin/bash
set -e

APP_NAME="horizonx-server"
AGENT_NAME="horizonx-agent"
MIGRATE_TOOL="horizonx-migrate"
SEED_TOOL="horizonx-seed"

SERVER_SERVICE="horizonx-server"
AGENT_SERVICE="horizonx-agent"

INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/horizonx"
LOG_DIR="/var/log/horizonx"

SYS_USER="horizonx"
SYS_GROUP="horizonx"

BIN_SRC="./bin"
ENV_SERVER_SRC="./.env.server.prod"
ENV_AGENT_SRC="./.env.agent.prod"

DEPLOY_SERVER=false
DEPLOY_AGENT=false

echo "=== HorizonX Deployment ==="

deploy_server() {
    echo ""
    echo "--- Server Setup ---"

    echo "📄 Copying server.env..."
    sudo cp $ENV_SERVER_SRC $CONFIG_DIR/server.env
    sudo chown root:$SYS_GROUP $CONFIG_DIR/server.env
    sudo chmod 640 $CONFIG_DIR/server.env

    echo "📦 Running migrations..."
    sudo sh -c "set -a; source $CONFIG_DIR/server.env; set +a; $INSTALL_DIR/$MIGRATE_TOOL -op=up"

    echo "⚙️ Creating systemd service for server..."
    sudo tee /etc/systemd/system/${SERVER_SERVICE}.service >/dev/null <<EOF
[Unit]
Description=HorizonX Core Server
After=network.target postgresql.service

[Service]
Type=simple
EnvironmentFile=$CONFIG_DIR/server.env
ExecStart=$INSTALL_DIR/$APP_NAME
Restart=always
User=$SYS_USER
Group=$SYS_GROUP
StandardOutput=append:${LOG_DIR}/server.log
StandardError=append:${LOG_DIR}/server.error.log

[Install]
WantedBy=multi-user.target
EOF

    echo "🔥 Starting server..."
    sudo systemctl daemon-reload
    sudo systemctl enable $SERVER_SERVICE
    sudo systemctl start $SERVER_SERVICE
}

deploy_agent() {
    echo ""
    echo "--- Agent Setup ---"

    echo "📄 Copying agent.env..."
    sudo cp $ENV_AGENT_SRC $CONFIG_DIR/agent.env
    sudo chown root:root $CONFIG_DIR/agent.env
    sudo chmod 600 $CONFIG_DIR/agent.env

    echo "⚙️ Creating systemd service for agent..."
    sudo tee /etc/systemd/system/${AGENT_SERVICE}.service >/dev/null <<EOF
[Unit]
Description=HorizonX Agent
After=network.target

[Service]
Type=simple
EnvironmentFile=$CONFIG_DIR/agent.env
ExecStart=$INSTALL_DIR/$AGENT_NAME
Restart=always
User=root
Group=root
StandardOutput=append:${LOG_DIR}/agent.log
StandardError=append:${LOG_DIR}/agent.error.log

[Install]
WantedBy=multi-user.target
EOF

    echo "🔥 Starting agent..."
    sudo systemctl daemon-reload
    sudo systemctl enable $AGENT_SERVICE
    sudo systemctl start $AGENT_SERVICE
}

# ───────────────────────────────────────────────
# MENU
# ───────────────────────────────────────────────
echo ""
echo "Choose deployment type:"
echo "1) Server"
echo "2) Agent"
echo "3) All"
echo "4) Exit"
echo -n "> "
read choice

case "$choice" in
    1) DEPLOY_SERVER=true ;;
    2) DEPLOY_AGENT=true ;;
    3) DEPLOY_SERVER=true; DEPLOY_AGENT=true ;;
    4) exit 0 ;;
    *) echo "Invalid choice."; exit 1 ;;
esac

echo ""
echo "🔧 Preparing environment..."

# ───────────────────────────────────────────────
# SELECTIVE: BUILD
# ───────────────────────────────────────────────
make build

# ───────────────────────────────────────────────
# SELECTIVE: STOPPING SERVICES
# ───────────────────────────────────────────────
if $DEPLOY_SERVER; then
    echo "🛑 Stopping server..."
    sudo systemctl stop $SERVER_SERVICE || true
fi

if $DEPLOY_AGENT; then
    echo "🛑 Stopping agent..."
    sudo systemctl stop $AGENT_SERVICE || true
fi

# ───────────────────────────────────────────────
# SYSTEM USER & DIR ONLY ONCE
# ───────────────────────────────────────────────
if ! id -u $SYS_USER >/dev/null 2>&1; then
    echo "👤 Creating user..."
    sudo useradd -r -s /bin/false $SYS_USER
fi

sudo mkdir -p $CONFIG_DIR $LOG_DIR
sudo chown -R $SYS_USER:$SYS_GROUP $LOG_DIR

# ───────────────────────────────────────────────
# SELECTIVE: DEPLOY BINARIES
# ───────────────────────────────────────────────
if $DEPLOY_SERVER; then
    echo "🚀 Deploying server binary..."
    sudo cp $BIN_SRC/server $INSTALL_DIR/$APP_NAME
    sudo cp $BIN_SRC/migrate $INSTALL_DIR/$MIGRATE_TOOL
    sudo cp $BIN_SRC/seed $INSTALL_DIR/$SEED_TOOL
    sudo chmod +x $INSTALL_DIR/$APP_NAME $INSTALL_DIR/$MIGRATE_TOOL $INSTALL_DIR/$SEED_TOOL
fi

if $DEPLOY_AGENT; then
    echo "🚀 Deploying agent binary..."
    sudo cp $BIN_SRC/agent $INSTALL_DIR/$AGENT_NAME
    sudo chmod +x $INSTALL_DIR/$AGENT_NAME
fi

# ───────────────────────────────────────────────
# RUN DEPLOYMENT
# ───────────────────────────────────────────────
if $DEPLOY_SERVER; then deploy_server; fi
if $DEPLOY_AGENT; then deploy_agent; fi

echo ""
echo "🎉 Deployment done!"
