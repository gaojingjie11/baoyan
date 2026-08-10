# 2027 计算机预推免追踪

这是一个部署在单台服务器上的个人申请追踪器：Nginx 提供静态前端，Go API 通过同源 `/api/` 提供账号、主题和跨设备进度同步，PostgreSQL 保存用户数据。

## 日常维护

- 学校目录：编辑 `schools.json`。
- 前端：`index.html`、`styles.css`、`app.js`。
- 后端：`server/`。
- 发布：推送 `main` 后 GitHub Actions 在服务器执行 `sudo /opt/baoyan/server/deploy.sh`。

## 首次部署

1. 在服务器克隆仓库至 `/opt/baoyan`，安装 Docker、Compose、Nginx、rsync 与 git。
2. 将 `server/.env.example` 复制为服务器专用的 `/etc/baoyan/baoyan.env`，填入真实值，并设为 `0600`。不要提交该文件。
3. 运行 `sudo /opt/baoyan/server/setup.sh`，随后在 GitHub 配置 `SERVER_HOST`、`SERVER_USER`、`SSH_PRIVATE_KEY` 和固定的 `SERVER_KNOWN_HOSTS` Secrets。
4. 用 `sudo /opt/baoyan/server/verify.sh` 检查健康状态和表结构。

此部署按当前要求使用 HTTP。HTTP 会暴露登录密码和会话给网络监听者，不能用于不受信任的网络；启用公网使用前应迁移至 HTTPS。
