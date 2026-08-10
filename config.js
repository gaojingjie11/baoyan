// ===== 后端同步配置 =====
// 把 API_BASE 改成你自托管后端的公网 HTTPS 地址，例如 https://baoyan-api.yourdomain.com
// 留空 '' 则只使用本机浏览器存储（localStorage），不跨设备同步
window.BAOYAN_API = '';

// 可选：若后端设置了 API_TOKEN，填在这里。
// 注意：写在前端会被任何访客看到，仅适合个人使用；更严的做法是在反向代理加 Basic Auth 或限制 IP。
window.BAOYAN_API_TOKEN = '';
