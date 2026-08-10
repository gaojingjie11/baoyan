# Server deployment

前端由 Nginx 在 `/var/www/baoyan` 提供，`/api/` 反代至 Docker 后端。Compose 只把后端映射到 `127.0.0.1:2026`（仅本机回环），PostgreSQL 不由本项目暴露。

## 全自动首次部署（无需手动填任何变量）

在服务器上执行一条命令即可，脚本会：克隆代码 → 生成配置（自动随机生成 `JWT_SECRET`）→ 构建后端 → 同步前端 → 把 `schools.json` 与种子用户自动写入数据库。

```bash
curl -fsSL https://raw.githubusercontent.com/gaojingjie11/baoyan/main/server/setup.sh | sudo bash
```

完成后浏览器打开 `http://<服务器IP>:26`，用 `gao` / `gao040907` 登录。

> 脚本读取仓库内的 `.env.example`（已内置你的 Postgres 连接串与 `gao/gao040907` 凭据），并自动补上随机 `JWT_SECRET`，因此**全程不需要你手动编辑任何环境文件**，也不需要对数据库做手工 INSERT。

## 数据如何“直接放进去”（自动入库，无需手动 SQL）

| 数据 | 来源 | 入库方式 | 备注 |
| --- | --- | --- | --- |
| 81 所学校 | 仓库根 `schools.json` | `docker-compose` 以只读卷挂载到容器 `/app/schools.json`，后端 `syncSchools()` 在**每次启动** upsert 进 `schools` 表 | 改了 `schools.json` 重新部署即刷新，不用碰数据库 |
| 用户 `gao` | 环境变量 `BOOTSTRAP_USERNAME/PASSWORD` | 后端 `bootstrap()` 首次启动自动建用户（已存在则跳过，不覆盖你改过的密码） | 密码用 Argon2id 哈希，库里不存明文 |
| 个人进度 | 前端操作 | `PUT /api/progress/{schoolID}` 写入 `progress` 表（按 `user_id` 隔离） | 一行 = 一个用户 × 一所学校 |

建表也在启动时自动完成（`migrate()` 创建 `users` / `schools` / `progress` / `refresh_tokens`）。所以**数据库完全由代码托管，你不需要手动建表或插数据**。

## 表结构

- `users(id, username, password_hash, theme_config, created_at)`
- `schools(id TEXT PK, school, tier, college, direction, major, start_text, end_text, status, site, admit, source, remark, source_updated_at, updated_at)`
- `progress(id, user_id → users, school_id → schools, status, updated_at, UNIQUE(user_id, school_id))`
- `refresh_tokens(token_hash PK, user_id → users, family_id, expires_at, revoked_at, created_at)`

## 会话模型（长短 token）

- 密码 Argon2id 哈希；`PASSWORD_PEPPER` 为可选额外部署密钥。
- **短 token（Access）15 分钟**，仅存浏览器内存，每次请求带 `Authorization: Bearer`。
- **长 token（Refresh）7 天**，HttpOnly cookie，数据库只存 SHA-256 摘要；每次刷新事务化轮换（删旧发新），一处泄露可整族吊销。
- `progress` 一行对应一名用户与一个学校；空状态会删除该行。

## 进阶：GitHub 自动部署

推送 `main` 后，GitHub Actions（`.github/workflows/deploy.yml`）会 SSH 进服务器执行 `deploy.sh`：同步前端 + `docker compose up -d --build` 重建后端。需在仓库 Secrets 配置 `SERVER_HOST` / `SERVER_USER` / `SSH_PRIVATE_KEY`。

## 命令

```bash
sudo /opt/baoyan/server/setup.sh         # 首次部署（全自动）
sudo /opt/baoyan/server/deploy.sh        # 更新部署（拉代码 + 重建后端）
sudo /opt/baoyan/server/verify.sh        # 健康检查
```

`deploy.sh` 会在新版本通过健康检查后自动删除无引用的旧镜像，并清理 7 天前的 Docker 构建缓存；当前使用中的容器和近 7 天缓存会保留。

当前为 HTTP-only 配置。请勿在不受信任网络上传输账号密码，后续应给 Nginx 加 HTTPS。
