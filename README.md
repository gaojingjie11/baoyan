# 2027 计算机预推免追踪

纯静态响应式网站，适合直接部署到 Vercel / Netlify / GitHub Pages。

## 最重要的数据文件
`schools.json`

以后更新学校信息，只需要修改这个文件，网站页面无需改动。

## Vercel 部署（仓库已推送到 GitHub）
本仓库已位于：https://github.com/gaojingjie11/baoyan-tracker-2027

1. 打开一键导入链接：https://vercel.com/new/clone?repository-url=https://github.com/gaojingjie11/baoyan-tracker-2027
2. 首次需授权 Vercel 访问你的 GitHub（OAuth 一次即可）
3. 选中 `baoyan-tracker-2027` → Framework Preset 选 `Other` → 无需 Build Command → Deploy
4. 部署完成后获得 `xxx.vercel.app` 公网地址，手机/电脑均可直接访问

之后每次修改并提交 `schools.json`，Vercel 会自动重新部署。

## 本地预览
因为浏览器直接双击 HTML 时可能禁止读取 JSON，建议在目录运行：
`python -m http.server 8000`
然后打开：
`http://localhost:8000`

## 当前文件
- `index.html` 页面结构
- `styles.css` 手机/电脑响应式样式
- `app.js` 搜索、筛选、截止状态计算、**985/211 排序、实时倒计时提醒、7天内截止紧急提醒条**
- `schools.json` 学校数据

## 功能说明
- 列表按 **985 → 211** 顺序排列（不按截止时间排序）。
- 每张卡片显示「距截止 X天X时X分」实时倒计时，每 30 秒刷新；7 天内截止的条目顶部汇聚成「紧急提醒」条。
- 时间与政策以高校及学院官方通知为准，倒计时仅供参考。
