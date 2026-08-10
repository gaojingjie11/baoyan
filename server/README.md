# 保研追踪器 · 后端 + 部署（Go + PostgreSQL，同源免域名）

个人「2027 计算机预推免」申请追踪器。
- **前端**：纯静态（HTML/CSS/JS + `schools.json`），部署在 Vercel，也同步部署到你的服务器 nginx。
- **后端**：Go（零框架，标准库 `net/http` + `database/sql`），负责「我的进度」跨设备同步。
- **数据库**：你已有的外部 PostgreSQL（`42.193.104.173:5432`）。

---

## 整体架构

```
浏览器（手机 / 电脑）
   │
   ├── 学校数据   ── fetch './schools.json' ──► 静态文件（前端自带，只读）
   │
   └── 我的进度   ── /api/progress ──►  nginx(/api/) 反代  ──►  Go 后端(容器:8080→宿主:2026)  ──►  PostgreSQL
```

**同源部署**：nginx 在同一台服务器既提供前端（`/`）又反代后端（`/api/`），
所以前端与 `/api` 同来源（`http://服务器IP`），无跨域、无混合内容，不用域名、不用证书。

---

## 字段与数据库设计（你问的）

### 1) 进度状态字段（5 个）
前端 `progressMap` 是 `{ [schoolId]: statusKey }`，`statusKey` 取值：

| statusKey | 含义   |
|-----------|--------|
| `""`      | 未报名 |
| `applied` | 已报名 |
| `iv`      | 待复试 |
| `adw`     | 待录取 |
| `adm`     | 已录取 |

### 2) 三层映射
学校数据（`schools.json`）与「我的进度」**完全分离**：
- `schools.json`：只读，含 `id / school / type / direction / start / end / status / site / source / remark` 等。
- 进度：用户操作产生，存在后端。

```
前端 progressMap（JS 对象）
   │  fetch /api/progress  ── GET 拉取 / POST 整体覆盖 ──
   ▼
后端响应体 JSON：{ "0":"applied", "18":"iv", ... }   （与前端 progressMap 同结构）
   │  存入 progress_store.data（JSONB）
   ▼
PostgreSQL 表 progress_store：
CREATE TABLE progress_store (
  id         TEXT PRIMARY KEY DEFAULT 'default',
  data       JSONB NOT NULL,            -- 整份 progressMap
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

- **单用户**：整份进度只占一行（`id='default'`）。
- **GET** 全量读、**POST** 全量覆盖（乐观更新）。
- **本机 `localStorage` 仍做缓存/兜底**：后端不可达时自动退回；导出/导入按钮作为无后端时的跨设备方案。

---

## 一、准备环境变量

```bash
cd server
cp .env.example .env
# .env 里：
#   DATABASE_URL=postgres://admin:sdl%40admin@42.193.104.173:5432/baoyan?sslmode=disable
#   （密码里的 @ 必须写成 %40；库名 baoyan 不存在后端会自动 CREATE DATABASE）
#   API_TOKEN=        # 可选；填了则 POST 必须带 Authorization: Bearer <token>
# .env 已加入 .gitignore，不会进仓库。
```

## 二、启动后端（Docker）

```bash
docker compose up -d --build
```
- 只起 `api` 容器；数据库用外部已有的 Postgres。
- 容器内监听 `8080`，映射到宿主机 `2026`（安全组放行 TCP 2026 仅本机 nginx 用，不必对外暴露）。
- 表 `progress_store` 首次启动自动创建；库 `baoyan` 不存在也会自动创建。
- 验证：
  ```bash
  curl http://localhost:2026/api/health      # {"ok":true}
  curl http://localhost:2026/api/progress     # {} （初始为空）
  ```

> 若后端容器和 Postgres 同机、连 `42.193.104.173` 不通，
> 在 `docker-compose.yml` 的 `api` 下加 `network_mode: host`，并把 `DATABASE_URL` 主机改成 `localhost`。

## 三、前端部署（同源，免域名/证书）★ 推荐

### 方式 A：GitHub Actions 自动部署（推荐）
每次 push 到 `main`，GitHub Actions 把前端文件 rsync 到服务器 `/var/www/baoyan`。
需要先在 GitHub 仓库 **Settings → Secrets** 添加：
- `SERVER_HOST` = `42.193.104.173`
- `SERVER_USER` = （你的 SSH 用户名，如 `root`）
- `SSH_PRIVATE_KEY` = （部署私钥；公钥已写入服务器 `~/.ssh/authorized_keys`）

未配置前工作流会失败（正常），配置后即生效。工作流文件：`.github/workflows/deploy.yml`。

### 方式 B：手动部署
```bash
rsync -avz --delete ./ user@42.193.104.173:/var/www/baoyan
```

### nginx（同服务器）
- 把 `server/nginx.conf` 放到 `/etc/nginx/conf.d/baoyan.conf`
  （若 `/etc/nginx/sites-enabled/default` 存在，删掉避免冲突）
- `sudo nginx -t && sudo systemctl reload nginx`
- nginx 在 `/` 提供前端，在 `/api/` 反代到 `localhost:2026`
- 防火墙放行 `80`（nginx）。`2026` 只给本机 nginx 用，不必对外暴露。

## 四、前端 config.js

```js
window.BAOYAN_API = '';       // 留空 → 走同源 /api（nginx 同服务器）
window.BAOYAN_API_TOKEN = '';
```

（以后若把后端放到别的域名，再填 `https://你的域名`；此时需要在后端 `CORS_ORIGIN` 加该域名，并上 HTTPS。）

## 五、环境变量表
| 变量 | 默认 | 说明 |
|------|------|------|
| `PORT` | `8080` | 后端容器内监听端口 |
| `DATABASE_URL` | 必填 | Postgres 连接串（compose 内已配） |
| `CORS_ORIGIN` | `https://baoyan-one.vercel.app` | 允许的前端源，逗号分隔可多个（同源部署用不到） |
| `API_TOKEN` | 空 | 设了则 POST 必须带 `Bearer` |

---

## （可选）升级到 HTTPS / 域名
同源 `http` 已能跨设备同步。若以后想用域名 + HTTPS：
- nginx 增加 `listen 443 ssl` 的 server 块 + `certbot` 签证书；或换 Caddy 自动签证书；或 Cloudflare Tunnel 零反向代理给 HTTPS 公网地址。
- 同时把前端 `config.js` 的 `BAOYAN_API` 改成 `https://你的域名`，并在 `CORS_ORIGIN` 加上该域名。
