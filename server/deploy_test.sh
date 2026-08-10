#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT_DIR"

grep -Fq '127.0.0.1:2026:8080' server/docker-compose.yml
test -f server/go.sum
if git ls-files --error-unmatch server/.env >/dev/null 2>&1; then
  echo 'server/.env must not be tracked' >&2
  exit 1
fi
if rg -g '!deploy_test.sh' -q 'vercel|CORS_ORIGIN|API_TOKEN' README.md server app.js config.js .github; then
  echo 'obsolete deployment configuration found' >&2
  exit 1
fi
echo 'deployment configuration checks passed'
