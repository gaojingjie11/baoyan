#!/usr/bin/env bash
# 保研追踪器 · 一键初始化部署（在服务器上运行一次）
# 完成：拉代码 → 同步前端 → 起后端(Docker) → 放 nginx → 开防火墙 → 健康检查
set -euo pipefail

REPO="https://github.com/gaojingjie11/baoyan.git"
APP_DIR="/opt/baoyan"
WEB_DIR="/var/www/baoyan"

for c in git rsync docker; do
  command -v "$c" >/dev/null 2>&1 || { echo "!! 缺少命令: $c，请先安装"; exit 1; }
done

echo "==> [1/6] 获取项目代码"
if [ -d "$APP_DIR/.git" ]; then
  git -C "$APP_DIR" pull --ff-only
else
  git clone "$REPO" "$APP_DIR"
fi

echo "==> [2/6] 同步前端到 $WEB_DIR"
mkdir -p "$WEB_DIR"
rsync -avz --delete \
  --exclude='.git' --exclude='server' --exclude='.github' \
  "$APP_DIR/" "$WEB_DIR/"

echo "==> [3/6] 后端环境变量"
cd "$APP_DIR/server"
[ -f .env ] || cp .env.example .env
# 若 JWT_SECRET 为空，生成一个稳定的随机密钥写入 .env（避免每次重启失效）
if grep -q '^JWT_SECRET=$' .env; then
  JWT_VAL=$(head -c 32 /dev/urandom | base64 | tr -d '/+' | head -c 43)
  sed -i "s#^JWT_SECRET=#JWT_SECRET=$JWT_VAL#" .env
  echo "    已生成随机 JWT_SECRET 写入 .env"
fi
echo "    .env 已就绪（含外部 Postgres 连接串，密码 @ 已转 %40）"

echo "==> [4/6] 启动后端（Docker）"
docker compose up -d --build

echo "==> [5/6] nginx 配置"
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

echo "==> [6/6] 防火墙放行 80"
command -v ufw >/dev/null 2>&1 && ufw allow 80/tcp
command -v firewall-cmd >/dev/null 2>&1 && { firewall-cmd --add-port=80/tcp --permanent; firewall-cmd --reload; }
echo "    （腾讯云安全组请在控制台放行 TCP 80；2026 端口只给本机 nginx 用，不必对外暴露）"

echo
echo "==> 健康检查"
sleep 3
if curl -fsS http://localhost:2026/api/health >/dev/null 2>&1; then
  echo "    后端 OK：http://localhost:2026/api/health"
else
  echo "    !! 后端未就绪。常见原因：容器内连 42.193.104.173:5432 被安全组/PG 绑定拦截。"
  echo "       解决：编辑 server/docker-compose.yml 给 api 加 'network_mode: host'，"
  echo "       并把 .env 的 DATABASE_URL 主机改成 localhost，再 'docker compose up -d --build'"
fi
PUB="$(curl -fsS icanhazip.com 2>/dev/null || echo 你的服务器IP)"
echo "完成。浏览器打开 http://$PUB 即可访问（手机/电脑同一地址，进度自动同步）。"
