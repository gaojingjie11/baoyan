// ===== 后端同步配置 =====
// 留空 '' → 走同源 /api（推荐：nginx 在同服务器上同时提供前端并反代 /api，无需域名/证书）
// 若后端在别的地址（如 https://baoyan-api.yourdomain.com），填在这里
window.BAOYAN_API = '';

// 可选：若后端设置了 API_TOKEN，填在这里。
// 注意：写在前端会被任何访客看到，仅适合个人使用；更严的做法是在反向代理加 Basic Auth 或限制 IP。
window.BAOYAN_API_TOKEN = '';
