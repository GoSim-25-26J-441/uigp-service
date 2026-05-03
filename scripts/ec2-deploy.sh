#!/usr/bin/env bash
# EC2 deploy: fetch .env from SSM, install binary from S3, restart systemd.
# Usage: ./ec2-deploy.sh <S3_BUCKET> <AWS_REGION>
#
# Optional env:
#   APP_DIR          install directory (default /opt/uigp-service)
#   SYSTEMD_SERVICE  unit name without path (default uigp-service)
#   DEPLOY_USER      user:group for the service (default ec2-user)
#   SSM_PARAM_ENV    SSM Parameter Store name holding full .env contents (default /uigp/production/env)

set -euo pipefail
export PATH="/usr/local/bin:/usr/bin:${PATH}"

if [ $# -lt 2 ]; then
  echo "Usage: $0 <S3_BUCKET> <AWS_REGION>" >&2
  exit 1
fi

BUCKET="$1"
REGION="$2"
APP_DIR="${APP_DIR:-/opt/uigp-service}"
SYSTEMD_SERVICE="${SYSTEMD_SERVICE:-uigp-service}"
UNIT_FILE="/etc/systemd/system/${SYSTEMD_SERVICE}.service"
DEPLOY_USER="${DEPLOY_USER:-ec2-user}"
SSM_PARAM_ENV="${SSM_PARAM_ENV:-/uigp/production/env}"

mkdir -p "$APP_DIR"

# echo "Fetching .env from Parameter Store (${SSM_PARAM_ENV})..."
# aws ssm get-parameter \
#   --name "$SSM_PARAM_ENV" \
#   --with-decryption \
#   --query "Parameter.Value" \
#   --output text \
#   --region "$REGION" | tee "$APP_DIR/.env" > /dev/null

echo "Downloading app binary from S3..."
aws s3 cp "s3://${BUCKET}/uigp-service/app" "$APP_DIR/app" --region "$REGION"
chmod +x "$APP_DIR/app"

if [ ! -f "$UNIT_FILE" ]; then
  echo "Systemd unit not found; creating ${UNIT_FILE}..."
  sudo tee "$UNIT_FILE" > /dev/null <<EOF
[Unit]
Description=uigp-service API
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=${DEPLOY_USER}
Group=${DEPLOY_USER}
WorkingDirectory=${APP_DIR}
ExecStart=${APP_DIR}/app
Restart=on-failure
RestartSec=5
KillSignal=SIGTERM
TimeoutStopSec=30

[Install]
WantedBy=multi-user.target
EOF
  sudo systemctl daemon-reload
  sudo systemctl enable "${SYSTEMD_SERVICE}"
fi

echo "Restarting ${SYSTEMD_SERVICE}..."
sudo systemctl restart "${SYSTEMD_SERVICE}"

echo "Deploy completed successfully."
