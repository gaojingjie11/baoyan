# Server deployment

前端由 Nginx 在 `/var/www/baoyan` 提供，`/api/` 反代至 Docker API。Compose 只将 API 映射到 `127.0.0.1:2026`，PostgreSQL 不由本项目暴露。

## Server-only configuration

创建 `/etc/baoyan/baoyan.env`（权限 `0600`）：

```dotenv
DATABASE_URL=postgres://USER:PASSWORD@HOST:5432/DATABASE?sslmode=disable
JWT_SECRET=a-random-secret-at-least-32-bytes-long
PASSWORD_PEPPER=a-separate-random-secret
BOOTSTRAP_USERNAME=first-account-name
BOOTSTRAP_PASSWORD=first-account-password
```

首次启动会创建缺失的 `users`、`progress` 与 `refresh_tokens` 表。旧版保存原始 refresh token 的会话表会被替换，所有旧会话失效；用户进度保留。首次部署后应从环境文件中移除 `BOOTSTRAP_PASSWORD`，后续不会创建新用户。

## Session model

- 密码使用 Argon2id 哈希；数据库不保存明文密码。
- Access Token 有效期 15 分钟，仅保存在浏览器内存。
- Refresh Token 有效期 7 天，以 HttpOnly cookie 发送，数据库只保存 SHA-256 摘要；每次刷新事务化轮换。
- `progress` 一行对应一名用户与一个学校。`PUT /api/progress/{schoolID}` 的空状态会删除该行。

## Commands

```bash
sudo /opt/baoyan/server/setup.sh
sudo /opt/baoyan/server/deploy.sh
sudo /opt/baoyan/server/verify.sh
```

当前配置为 HTTP-only。请勿在不受信任网络传递账号密码，后续应为 Nginx 配置 HTTPS。
