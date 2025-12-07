#!/bin/bash
set -e

APP_NAME="horizonx-server"
SERVICE_NAME="horizonx-server"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/horizonx-server"
DATA_DIR="/var/lib/horizonx-server"
LOG_DIR="/var/log/horizonx-server"
USER="horizonx"
GROUP="horizonx"
BINARY_PATH="./bin/${APP_NAME}"
ENV_PATH="./.env.production"

echo "=== HorizonX Deployment Script (Root Mode) ==="

# Build binary
make build

# Stop existing service if exists
if systemctl list-units --full -all | grep -q "${SERVICE_NAME}.service"; then
    echo "Stopping existing systemd service..."
    sudo systemctl stop $SERVICE_NAME || true
    sudo systemctl disable $SERVICE_NAME || true
fi

# Create system user & group if not exist
if ! id -u $USER >/dev/null 2>&1; then
    echo "Creating system user $USER..."
    sudo useradd -r -s /bin/false $USER
fi

# Setup directories
echo "Setting up directories..."
sudo mkdir -p $CONFIG_DIR $DATA_DIR $LOG_DIR
sudo chown -R $USER:$GROUP $CONFIG_DIR $DATA_DIR $LOG_DIR

# Deploy binary (owned by root)
echo "Deploying binary..."
sudo cp $BINARY_PATH $INSTALL_DIR/$APP_NAME
sudo chown root:root $INSTALL_DIR/$APP_NAME
sudo chmod +x $INSTALL_DIR/$APP_NAME

# Copy .env (owned by horizonx)
echo "Copying .env config..."
sudo cp $ENV_PATH $CONFIG_DIR/.env
sudo chown $USER:$GROUP $CONFIG_DIR/.env

# Create systemd service (run as root, but data/log owned by horizonx)
echo "Creating systemd service..."
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"
sudo tee $SERVICE_FILE >/dev/null <<EOF
[Unit]
Description=HorizonX Server (Root Mode)
After=network.target

[Service]
Type=simple
EnvironmentFile=${CONFIG_DIR}/.env
ExecStart=${INSTALL_DIR}/${APP_NAME}
WorkingDirectory=${DATA_DIR}
Restart=on-failure
RestartSec=5
# Run as root for full sensor access
User=root
Group=root
# Data/log still owned by horizonx
StandardOutput=file:${LOG_DIR}/out.log
StandardError=file:${LOG_DIR}/error.log

[Install]
WantedBy=multi-user.target
EOF

# Reload systemd & start service
echo "Reloading systemd and starting service..."
sudo systemctl daemon-reload
sudo systemctl enable $SERVICE_NAME
sudo systemctl start $SERVICE_NAME
sudo systemctl status $SERVICE_NAME --no-pager

echo "=== Deployment complete! HorizonX Server is running as root (data/log safe) ==="
