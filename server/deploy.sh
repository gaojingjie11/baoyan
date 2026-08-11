#!/usr/bin/env bash
# Deploy the checked-out main branch. Run this script only on the application server.
set -euo pipefail

APP_DIR=/opt/baoyan
WEB_DIR=/var/www/baoyan
ENV_FILE=/etc/baoyan/baoyan.env

test -f "$ENV_FILE" || { echo "missing $ENV_FILE" >&2; exit 1; }
# 自更新后重新执行本脚本：git pull 会改写 deploy.sh 自身，而 bash 执行中已缓冲旧内容，
# 不重新 exec 就会继续跑过期的脚本（曾导致旧的单次 curl 误报 curl 56）。
if [ "${_BAOYAN_PULLED:-}" != "1" ]; then
  git -C "$APP_DIR" fetch origin
  git -C "$APP_DIR" checkout main
  git -C "$APP_DIR" reset --hard origin/main
  _BAOYAN_PULLED=1 exec "$0" "$@"
fi
install -d -m 0755 "$WEB_DIR"
echo "deploying commit: $(git -C "$APP_DIR" rev-parse --short HEAD)  $(git -C "$APP_DIR" log -1 --pretty=%s)"
rsync -a --delete --exclude=.git --exclude=server --exclude=.github "$APP_DIR/" "$WEB_DIR/"
cd "$APP_DIR/server"
docker compose --env-file "$ENV_FILE" up -d --build --remove-orphans --force-recreate

# 等待后端就绪：轮询 /api/health，最多 ~90s（冷启动 + 远程 PG 同步 138 行有延迟）。
# 避免单次 curl 在端口尚未监听时命中 docker-proxy 的 RST，造成误报失败（curl 56）。
HEALTH_URL="http://127.0.0.1:2026/api/health"
ready=0
for i in $(seq 1 45); do
  if curl -fsS --max-time 5 "$HEALTH_URL" >/dev/null 2>&1; then
    ready=1
    echo "backend healthy after ${i} try(ies)"
    break
  fi
  sleep 2
done

if [ "$ready" -ne 1 ]; then
  echo "backend did not become healthy within timeout; dumping container logs:" >&2
  docker compose logs --no-color --tail=120 api >&2 || true
  exit 1
fi

# 只在新版本健康后清理：保留近 7 天的构建缓存，删除无引用旧镜像。
docker image prune -f
docker builder prune -f --filter "until=168h"

echo "deployment complete"
