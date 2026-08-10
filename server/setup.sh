#!/usr/bin/env bash
# Prepare a server for the first deployment. Secrets are supplied separately.
set -euo pipefail

REPO="https://github.com/gaojingjie11/baoyan.git"
APP_DIR="/opt/baoyan"
WEB_DIR="/var/www/baoyan"

for c in git rsync docker; do
  command -v "$c" >/dev/null 2>&1 || { echo "!! 缺少命令: $c，请先安装"; exit 1; }
done

echo "==> [1/5] 获取项目代码"
if [ -d "$APP_DIR/.git" ]; then
  git -C "$APP_DIR" pull --ff-only
else
  git clone "$REPO" "$APP_DIR"
fi

echo "==> [2/5] 准备服务器环境变量"
cd "$APP_DIR/server"
install -d -m 0700 /etc/baoyan
if [ ! -f /etc/baoyan/baoyan.env ]; then
  install -m 0600 .env.example /etc/baoyan/baoyan.env
  echo "Fill /etc/baoyan/baoyan.env with real server-only values, then rerun this script."
  exit 1
fi

echo "==> [3/5] nginx 配置"
if command -v nginx >/dev/null 2>&1; then
  mkdir -p /etc/nginx/conf.d
  cp nginx.conf /etc/nginx/conf.d/baoyan.conf
  rm -f /etc/nginx/sites-enabled/default
  if nginx -t; then
    (systemctl reload nginx 2>/dev/null || nginx -s reload) && echo "    nginx 已 reload"
  else
    echo "!! nginx -t 失败，请检查 /etc/nginx/conf.d/baoyan.conf"
  fi
else
  echo "!! 未检测到 nginx，请安装后执行："
  echo "   cp $APP_DIR/server/nginx.conf /etc/nginx/conf.d/baoyan.conf && nginx -t && systemctl reload nginx"
fi

echo "==> [4/5] 防火墙放行 26"
command -v ufw >/dev/null 2>&1 && ufw allow 26/tcp
command -v firewall-cmd >/dev/null 2>&1 && { firewall-cmd --add-port=26/tcp --permanent; firewall-cmd --reload; }
echo "    TCP 2026 is loopback-only; do not open it in the security group."

echo
echo "==> [5/5] 首次部署"
chmod 0750 deploy.sh
./deploy.sh
echo "HTTP-only deployment is running. Do not use it over an untrusted network."
