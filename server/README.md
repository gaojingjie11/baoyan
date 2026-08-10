# 保研追踪器 · 后端（Go + PostgreSQL）

自托管后端，提供「我的进度」跨设备同步。前端仍部署在 Vercel（或任何静态托管），只把进度读写转发到这个后端，学校数据仍走 Vercel 的 `schools.json`。

## 目录
```
server/
├── main.go              # Go 后端（标准库 net/http + database/sql，零框架依赖）
├── go.mod
├── Dockerfile           # 多阶段构建
├── docker-compose.yml   # 仅后端容器，数据库用你已有的外部 Postgres
├── .env.example         # 环境变量模板（复制为 .env 填写）
├── Caddyfile.example    # 反向代理 + 自动 HTTPS 示例
└── README.md
```

## 一、准备环境变量

复制模板并填入你的 Postgres 连接串：

```bash
cd server
cp .env.example .env
# 编辑 .env：
#   DATABASE_URL=postgres://admin:sdl%40admin@42.193.104.173:5432/baoyan?sslmode=disable
#   （密码里的 @ 必须写成 %40；库名 baoyan 不存在会自动创建）
#   API_TOKEN=          # 可选
```
> `.env` 已加入 `.gitignore`，不会进仓库，密码安全。

## 二、启动后端（Docker）

```bash
docker compose up -d --build
```
- 只起 `api` 一个容器；数据库用你外部已有的 Postgres（上面 DATABASE_URL 指向它）。
- 若 `baoyan` 库不存在，后端启动时会自动 `CREATE DATABASE baoyan`。
- 后端容器内监听 `8080`，映射到宿主机对外端口 `2026`（防火墙/安全组需放行 TCP 2026）。
- 表 `progress_store` 首次启动自动创建。

验证：
```bash
curl http://localhost:2026/api/health          # {"ok":true}
curl http://localhost:2026/api/progress         # {}  （初始为空）
```

> 若后端容器和 Postgres 在同一台服务器、连 `42.193.104.173` 不通，
> 可在 docker-compose.yml 的 api 下加 `network_mode: host`，并把 DATABASE_URL 主机改成 `localhost`。

## 三、前端 HTTPS（必须）

Vercel 前端是 HTTPS，浏览器会拦截它对 `http://` 后端的请求（混合内容）。
所以后端前面必须有 HTTPS。二选一：

- **Nginx（你选的）**：见 `nginx.conf.example`，再 `certbot --nginx -d 你的域名` 申请证书。
- Caddy：见 `Caddyfile.example`，自动申请证书，更省事。
- Cloudflare Tunnel：零反向代理，Cloudflare 直接给 HTTPS 公网地址。

前端 `config.js` 里填 `window.BAOYAN_API = 'https://你的域名'`（域名走 443，
由 Nginx/Caddy 转发到容器 2026），然后 `git push` 让 Vercel 重新部署。


## 二、让外网能访问（必须 HTTPS）

Vercel 前端是 HTTPS，浏览器不允许从 HTTPS 页面向明文 HTTP 后端发请求（混合内容会被拦）。二选一：

### 方案 1：Caddy 反向代理（有公网 IP + 域名，最省事）
在服务器装 Caddy，新建 `/etc/caddy/Caddyfile`：
```
baoyan-api.yourdomain.com {
    reverse_proxy localhost:8080
}
```
`caddy reload` 后自动申请 Let's Encrypt 证书，得到 `https://baoyan-api.yourdomain.com`。
（Caddyfile 也放在仓库 `server/Caddyfile.example` 供参考。）

### 方案 2：Cloudflare Tunnel（家庭宽带 / 无公网 IP）
```bash
cloudflared tunnel --url http://localhost:8080
```
按提示绑定一个 `*.trycloudflare.com` 或你自己的域名子域，得到 HTTPS 公网地址。

### 安全加固（个人站点建议做）
- 反向代理加 **Basic Auth** 或限制来源 IP，避免任何人都能写你的进度。
- 或后端设置 `API_TOKEN`（见上），前端 `config.js` 填同样的值。

## 三、前端指向后端

编辑仓库根的 `config.js`：
```js
window.BAOYAN_API = 'https://baoyan-api.yourdomain.com';  // 改成你的 HTTPS 地址
window.BAOYAN_API_TOKEN = '';                              // 若后端设了 token 就填
```
然后 `git push` 让 Vercel 重新部署。此后：
- 打开网站会自动从后端拉取进度；
- 改任意学校的进度会即时写回后端；
- 桌面和手机打开同一个地址 → 进度完全一致；
- 后端不可达时自动退回本机 localStorage，导出/导入按钮作为兜底仍可用。

## 四、环境变量
| 变量 | 默认 | 说明 |
|------|------|------|
| `PORT` | `8080` | 后端监听端口 |
| `DATABASE_URL` | 必填 | Postgres 连接串（compose 内已配好） |
| `CORS_ORIGIN` | `https://baoyan-one.vercel.app` | 允许的前端源，逗号分隔可多个 |
| `API_TOKEN` | 空 | 设了则 POST 必须带 `Authorization: Bearer <token>` |

## 五、数据模型
```sql
CREATE TABLE progress_store (
  id         TEXT PRIMARY KEY DEFAULT 'default',
  data       JSONB NOT NULL,          -- {"0":"applied","18":"iv",...}
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```
单用户，整份进度存一行 JSON，GET 读取 / POST 整体覆盖。
