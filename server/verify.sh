#!/usr/bin/env bash
set -euo pipefail

ENV_FILE=/etc/baoyan/baoyan.env
test -r "$ENV_FILE" || { echo "missing $ENV_FILE" >&2; exit 1; }
curl -fsS http://127.0.0.1:2026/api/health >/dev/null
ss -ltn | awk '$4 ~ /127\.0\.0\.1:2026$/ { found=1 } END { exit !found }'
set -a
source "$ENV_FILE"
set +a
psql "$DATABASE_URL" -X -v ON_ERROR_STOP=1 -Atc "SELECT table_name FROM information_schema.tables WHERE table_schema='public' AND table_name IN ('users','progress','refresh_tokens') ORDER BY table_name" | sed -n '1,3p'
echo 'health=ok listener=127.0.0.1:2026'
