# 保研追踪器 · 后端 + 部署（Go + PostgreSQL，同源免域名，多用户 + JWT）

个人「2027 计算机预推免」申请追踪器。
- **前端**：纯静态（HTML/CSS/JS + `schools.json`），部署在 Vercel，也同步部署到你的服务器 nginx。
- **后端**：Go（零框架，标准库 `net/http` + `database/sql`），负责「我的进度」**跨设备、按用户**同步与鉴权。
- **数据库**：你已有的外部 PostgreSQL（`42.193.104.173:5432`）。
- **鉴权**：JWT 长短 token（手写 HS256，零外部依赖）。种子用户 `gao` / `gao040907`。

---

## 整体架构

```
浏览器（手机 / 电脑，登录后）
   │
   ├── 学校数据   ── fetch './schools.json' ──► 静态文件（前端自带，只读）
   │
   └── 我的进度   ── /api/progress(Bearer) ──► nginx(/api/) 反代 ──► Go 后端(:2026→:8080) ──► PostgreSQL
        登录/刷新 ── /api/auth/login | /refresh ──► 同上
```
同源部署：nginx 在同一台服务器既提供前端（`/`）又反代后端（`/api/`），无跨域、无混合内容，不用域名、不用证书。

---

## 数据模型（结构化，已弃用单行 JSONB）

进度不再存成整张表的一行 JSON，而是**结构化**存储，便于按用户查询、统计、扩展：

### 1) 用户表 `users`
| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | SERIAL PK | 用户 id |
| `username` | TEXT UNIQUE | 登录名 |
| `password_hash` | TEXT | salt + sha256(+pepper)，常量时间比较 |
| `created_at` | TIMESTAMPTZ | 注册时间 |

种子用户：`gao` / `gao040907`（首次启动自动创建）。

### 2) 进度表 `progress`（结构化，按用户隔离）
| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | SERIAL PK | |
| `user_id` | INT → users(id) | 所属用户 |
| `school_id` | TEXT | 学校 id（与 schools.json 的 id 对应） |
| `status` | TEXT | `''` / `applied` / `iv` / `adw` / `adm` |
| `updated_at` | TIMESTAMPTZ | 更新时间 |

`UNIQUE(user_id, school_id)`，写入用 `INSERT ... ON CONFLICT DO UPDATE`。

### 3) 刷新令牌表 `refresh_tokens`
| 字段 | 类型 | 说明 |
|------|------|------|
| `token` | TEXT PK | 长 token（随机串，入库可吊销） |
| `user_id` | INT → users(id) | 所属用户 |
| `expires_at` | TIMESTAMPTZ | 过期时间（1 周） |
| `created_at` | TIMESTAMPTZ | |

### 三层字段映射
```
前端 progressMap（JS 对象 {schoolId: statusKey}）
   │  fetch /api/progress  ── GET 拉取 / POST 整体 upsert（带 Bearer）──
   ▼
后端：SELECT/INSERT progress 表（按 user_id 隔离）
   │
   ▼
PostgreSQL：每行一个 (user_id, school_id, status)
```
- `statusKey` 取值（5 个）：`''` 未报名 / `applied` 已报名 / `iv` 待复试 / `adw` 待录取 / `adm` 已录取。
- **不同用户看到的是各自独立的进度**（按 `user_id` 隔离），实现「不同用户不同样子」。

---

## JWT 长短 token 机制

| token | 名称 | 有效期 | 用途 |
|-------|------|--------|------|
| Access Token | 短 token | **15 分钟** | 每次请求 `Authorization: Bearer <access>` |
| Refresh Token | 长 token | **1 周** | 用 `/api/auth/refresh` 换取新的短 token |

流程：
1. `POST /api/auth/login {username,password}` → 返回 `access_token` + `refresh_token` + `user`。
2. 前端把两个 token 存 localStorage；短时 token 过期（API 返回 401）时，自动用长 token 调 `/api/auth/refresh` 换新短 token（**刷新时轮换**：删旧长 token、发新长 token，降低泄露风险）。
3. `POST /api/auth/logout {refresh_token}` → 吊销长 token。
4. `GET /api/me` → 当前用户信息。

> 手写 HS256（HMAC-SHA256），签名密钥来自环境变量 `JWT_SECRET`，**不引入任何 JWT 第三方库**，避免构建期拉依赖。

接口汇总：
| 方法 | 路径 | 鉴权 | 说明 |
|------|------|------|------|
| POST | `/api/auth/register` | 否 | 注册新用户（开放，仅本机 IP 可访问） |
| POST | `/api/auth/login` | 否 | 登录，签发双 token |
| POST | `/api/auth/refresh` | 长 token | 换新的短 token（+ 轮换长 token） |
| POST | `/api/auth/logout` | 否 | 吊销长 token |
| GET | `/api/me` | 短 token | 当前用户 |
| GET/POST | `/api/progress` | 短 token | 读取 / 覆盖当前用户进度 |
| GET | `/api/health` | 否 | 健康检查 |

---

## 部署

### 首次部署：一键脚本（推荐）
在**服务器上**执行一次，即完成「拉代码 + 前端同步 + 起后端 + nginx + 防火墙 + 自动生成 JWT_SECRET」：
```bash
curl -fsSL https://raw.githubusercontent.com/gaojingjie11/baoyan/main/server/setup.sh | bash
```
脚本把仓库 clone 到 `/opt/baoyan`，自动生成随机 `JWT_SECRET` 写入 `server/.env`。

### 后续：GitHub 推送自动部署
每次你 `git push` 到 `main`，GitHub Actions（`.github/workflows/deploy.yml`）会：
1. rsync 前端文件到服务器 `/var/www/baoyan`；
2. SSH 到服务器 `git pull` 并 `docker compose up -d --build` 重建后端。
需在 GitHub 仓库 **Settings → Secrets** 添加：`SERVER_HOST` / `SERVER_USER` / `SSH_PRIVATE_KEY`。

### 手动部署（等价步骤）
```bash
# 服务器上
git clone https://github.com/gaojingjie11/baoyan.git /opt/baoyan
cd /opt/baoyan
rsync -avz --delete --exclude='.git' --exclude='server' --exclude='.github' ./ /var/www/baoyan/
cd server && cp .env.example .env && docker compose up -d --build
cp nginx.conf /etc/nginx/conf.d/baoyan.conf
rm -f /etc/nginx/sites-enabled/default
nginx -t && systemctl reload nginx   # 放行 80
```
> 若后端容器连 `42.193.104.173:5432` 不通，在 `docker-compose.yml` 的 `api` 下加 `network_mode: host`，并把 `.env` 的 `DATABASE_URL` 主机改成 `localhost`。

---

## 环境变量
| 变量 | 默认 | 说明 |
|------|------|------|
| `PORT` | `8080` | 后端容器内监听端口 |
| `DATABASE_URL` | 必填 | Postgres 连接串 |
| `CORS_ORIGIN` | `https://baoyan-one.vercel.app` | 允许的前端源（同源部署用不到） |
| `JWT_SECRET` | 空→随机 | JWT 签名密钥，生产请设固定值 |
| `PASSWORD_PEPPER` | 空 | 密码加盐，设置后不可更改 |

---

## （可选）升级到 HTTPS / 域名
同源 `http` 已能跨设备、按用户同步。若以后要域名 + HTTPS：给 nginx 加 `listen 443 ssl` + `certbot` 证书，并把前端 `config.js` 的 `BAOYAN_API` 改成 `https://你的域名`、后端 `CORS_ORIGIN` 加该域名。
