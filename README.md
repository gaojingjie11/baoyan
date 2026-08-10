# 2027 计算机预推免追踪

纯静态响应式网站，适合直接部署到 Vercel / Netlify / GitHub Pages。

## 最重要的数据文件
`schools.json`

以后更新学校信息，只需要修改这个文件，网站页面无需改动。

## Vercel 部署
1. 新建 GitHub 仓库，例如 `baoyan-tracker`
2. 把本压缩包中的全部文件上传到仓库根目录
3. 在 Vercel 新建 Project，Import 该 GitHub 仓库
4. Framework Preset 选择 `Other`
5. 无需 Build Command
6. Output Directory 留空
7. Deploy

之后每次修改并提交 `schools.json`，Vercel 会自动重新部署。

## 本地预览
因为浏览器直接双击 HTML 时可能禁止读取 JSON，建议在目录运行：
`python -m http.server 8000`
然后打开：
`http://localhost:8000`

## 当前文件
- `index.html` 页面结构
- `styles.css` 手机/电脑响应式样式
- `app.js` 搜索、筛选、截止状态计算
- `schools.json` 学校数据
