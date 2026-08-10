#!/usr/bin/env bash
# Deploy the checked-out main branch. Run this script only on the application server.
set -euo pipefail

APP_DIR=/opt/baoyan
WEB_DIR=/var/www/baoyan
ENV_FILE=/etc/baoyan/baoyan.env

test -f "$ENV_FILE" || { echo "missing $ENV_FILE" >&2; exit 1; }
git -C "$APP_DIR" pull --ff-only
install -d -m 0755 "$WEB_DIR"
rsync -a --delete --exclude=.git --exclude=server --exclude=.github "$APP_DIR/" "$WEB_DIR/"
cd "$APP_DIR/server"
docker compose --env-file "$ENV_FILE" up -d --build --remove-orphans
curl -fsS http://127.0.0.1:2026/api/health >/dev/null
echo "deployment complete"
