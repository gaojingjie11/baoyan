#!/usr/bin/env bash
# 首次部署：克隆代码 → 生成配置（自动生成 JWT 密钥）→ 起后端 → 配 nginx → 开防火墙。
# 全程无需手动填写任何变量；一条命令即可把 schools.json 与用户 gao 自动入库。
set -euo pipefail

REPO="https://github.com/gaojingjie11/baoyan.git"
APP_DIR="/opt/baoyan"
WEB_DIR="/var/www/baoyan"
ENV_FILE="/etc/baoyan/baoyan.env"

for c in git rsync docker openssl; do
  command -v "$c" >/dev/null 2>&1 || { echo "!! 缺少命令: $c，请先安装"; exit 1; }
done

echo "==> [1/5] 获取项目代码"
if [ -d "$APP_DIR/.git" ]; then
  git -C "$APP_DIR" pull --ff-only
else
  git clone "$REPO" "$APP_DIR"
fi
cd "$APP_DIR/server"

echo "==> [2/5] 准备配置文件（自动生成 JWT 密钥，无需手动填）"
install -d -m 0700 /etc/baoyan
if [ ! -f "$ENV_FILE" ]; then
  install -m 0600 .env.example "$ENV_FILE"
  echo "    已基于 .env.example 生成 $ENV_FILE"
fi
# JWT_SECRET 缺失或仍为占位符时，自动生成 32 字节随机值
if ! grep -q '^JWT_SECRET=' "$ENV_FILE" || grep -Eq '^JWT_SECRET=__AUTO_GENERATE__$|^JWT_SECRET=$' "$ENV_FILE"; then
  SECRET=$(openssl rand -hex 32)
  if grep -q '^JWT_SECRET=' "$ENV_FILE"; then
    sed -i.bak "s/^JWT_SECRET=.*/JWT_SECRET=$SECRET/" "$ENV_FILE"
  else
    echo "JWT_SECRET=$SECRET" >> "$ENV_FILE"
  fi
  echo "    已生成随机 JWT_SECRET"
fi
echo "    数据库与种子用户凭据已就绪（登录账号 gao / gao040907）"

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

echo "==> [4/5] 防火墙放行前端端口"
# 前端经 nginx 对外（当前监听 26）；后端 2026 仅本机回环，切勿在防火墙/安全组放行。
command -v ufw >/dev/null 2>&1 && ufw allow 26/tcp
command -v firewall-cmd >/dev/null 2>&1 && { firewall-cmd --add-port=26/tcp --permanent; firewall-cmd --reload; }
echo "    TCP 2026 为仅本机回环，请勿对外暴露"

echo
echo "==> [5/5] 首次部署（构建后端镜像 + 同步前端 + 自动把 schools.json 与用户入库）"
chmod 0750 deploy.sh
./deploy.sh
echo
echo "完成。浏览器打开 http://<服务器IP>:26 即可，用 gao / gao040907 登录；"
echo "进度按账号自动同步，81 所学校已在启动时由 schools.json 自动写入数据库。"
