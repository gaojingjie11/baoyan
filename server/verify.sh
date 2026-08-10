#!/usr/bin/env bash
set -euo pipefail

ENV_FILE=/etc/baoyan/baoyan.env
test -r "$ENV_FILE" || { echo "missing $ENV_FILE" >&2; exit 1; }
curl -fsS http://127.0.0.1:2026/api/health >/dev/null
ss -ltn | awk '$4 ~ /127\.0\.0\.1:2026$/ { found=1 } END { exit !found }'
set -a
source "$ENV_FILE"
set +a
psql "$DATABASE_URL" -X -v ON_ERROR_STOP=1 -Atc "SELECT 'schools=' || count(*) FROM schools UNION ALL SELECT 'users=' || count(*) FROM users UNION ALL SELECT 'progress=' || count(*) FROM progress UNION ALL SELECT 'refresh_tokens=' || count(*) FROM refresh_tokens ORDER BY 1"
echo 'health=ok listener=127.0.0.1:2026'
