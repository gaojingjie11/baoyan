/* 2027 计算机预推免 · 个人申请追踪器 */
let rows = [];
let filters = { type: 'all', dir: 'all', prog: 'all', status: 'all' };

const list = document.querySelector('#list');
const empty = document.querySelector('#empty');
const search = document.querySelector('#search');
const stats = document.querySelector('#stats');
const reminder = document.querySelector('#reminder');
const focus = document.querySelector('#focus');

/* ---------- 登录态（JWT 长短 token） ---------- */
const LS_ACCESS = 'bjt_access';
const LS_REFRESH = 'bjt_refresh';
const LS_USER = 'bjt_user';
let accessToken = localStorage.getItem(LS_ACCESS) || '';
let refreshToken = localStorage.getItem(LS_REFRESH) || '';
let currentUser = (() => { try { return JSON.parse(localStorage.getItem(LS_USER) || 'null'); } catch (e) { return null; } })();

const API_BASE = (window.BAOYAN_API || '').replace(/\/+$/, '');
// BAOYAN_API 留空 → 走同源 /api（nginx 同域部署时无需域名）；填了 → 用该绝对地址
const API_URL = API_BASE ? API_BASE + '/api' : '/api';

function setTokens(access, refresh, user) {
  accessToken = access || '';
  refreshToken = refresh || '';
  currentUser = user || null;
  accessToken ? localStorage.setItem(LS_ACCESS, accessToken) : localStorage.removeItem(LS_ACCESS);
  refreshToken ? localStorage.setItem(LS_REFRESH, refreshToken) : localStorage.removeItem(LS_REFRESH);
  user ? localStorage.setItem(LS_USER, JSON.stringify(user)) : localStorage.removeItem(LS_USER);
}
function clearTokens() { setTokens('', '', null); }

// 带鉴权 + 自动刷新（短 token 过期 → 用长 token 换新短 token）的请求
let refreshing = false;
let refreshWaiters = [];
async function apiFetch(url, opts = {}) {
  if (!accessToken) throw new Error('not_logged_in');
  opts.headers = Object.assign({}, opts.headers, { 'Authorization': 'Bearer ' + accessToken });
  let res = await fetch(API_URL + url, opts);
  if (res.status === 401 && refreshToken) {
    const ok = await doRefresh();
    if (ok) {
      opts.headers['Authorization'] = 'Bearer ' + accessToken;
      res = await fetch(API_URL + url, opts);
    } else {
      throw new Error('not_logged_in');
    }
  }
  return res;
}
async function doRefresh() {
  if (refreshing) return new Promise(resolve => refreshWaiters.push(resolve));
  refreshing = true;
  try {
    const res = await fetch(API_URL + '/auth/refresh', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ refresh_token: refreshToken }),
    });
    if (!res.ok) { clearTokens(); return false; }
    const data = await res.json();
    setTokens(data.access_token, data.refresh_token, data.user);
    return true;
  } catch (e) { clearTokens(); return false; }
  finally {
    refreshing = false;
    refreshWaiters.forEach(r => r(true));
    refreshWaiters = [];
  }
}

/* ---------- 个人进度：localStorage 缓存 + 后端（按用户隔离） ---------- */
const LS_PROGRESS = 'bjt_progress_v1';
const PROGRESS = [
  { key: '',       label: '未报名', cls: 'p-none' },
  { key: 'applied',label: '已报名', cls: 'p-applied' },
  { key: 'iv',     label: '待复试', cls: 'p-iv' },
  { key: 'adw',    label: '待录取', cls: 'p-adw' },
  { key: 'adm',    label: '已录取', cls: 'p-adm' },
];
let progressMap = {};
try { progressMap = JSON.parse(localStorage.getItem(LS_PROGRESS) || '{}'); } catch (e) { progressMap = {}; }
function getProgress(id) { return progressMap[id] || ''; }
function setProgress(id, val) {
  if (val) progressMap[id] = val; else delete progressMap[id];
  try { localStorage.setItem(LS_PROGRESS, JSON.stringify(progressMap)); } catch (e) {}
  saveProgressToAPI();
}
function progressMeta(key) { return PROGRESS.find(p => p.key === key) || PROGRESS[0]; }

async function loadProgressFromAPI() {
  try {
    const res = await apiFetch('/progress', { cache: 'no-store' });
    if (!res.ok) return false;
    const data = await res.json();
    if (data && typeof data === 'object') {
      progressMap = data;
      try { localStorage.setItem(LS_PROGRESS, JSON.stringify(progressMap)); } catch (e) {}
      return true;
    }
  } catch (e) { /* 未登录 / 失败 */ }
  return false;
}
async function saveProgressToAPI() {
  try {
    await apiFetch('/progress', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(progressMap),
    });
  } catch (e) { /* 忽略（本地已有缓存） */ }
}

/* ---------- 登录 / 登出 ---------- */
const loginEl = document.querySelector('#login');
const loginForm = document.querySelector('#login-form');
const loginErr = document.querySelector('#login-err');
const userbar = document.querySelector('#userbar');
const userNameEl = document.querySelector('#user-name');
const btnLogout = document.querySelector('#btn-logout');

function showLogin() { loginEl.hidden = false; document.body.classList.add('locked'); }
function hideLogin() { loginEl.hidden = true; document.body.classList.remove('locked'); }

loginForm.addEventListener('submit', async (e) => {
  e.preventDefault();
  loginErr.textContent = '';
  const username = loginForm.username.value.trim();
  const password = loginForm.password.value;
  try {
    const res = await fetch(API_URL + '/auth/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username, password }),
    });
    const data = await res.json();
    if (!res.ok) { loginErr.textContent = data.error || '登录失败'; return; }
    setTokens(data.access_token, data.refresh_token, data.user);
    hideLogin();
    await bootstrap();
  } catch (e) { loginErr.textContent = '无法连接后端'; }
});
btnLogout.addEventListener('click', async () => {
  try {
    await fetch(API_URL + '/auth/logout', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ refresh_token: refreshToken }),
    });
  } catch (e) {}
  clearTokens();
  showLogin();
});

function updateSyncBadge() {
  const tip = document.querySelector('.sync-tip');
  if (!tip) return;
  if (currentUser) {
    tip.innerHTML = `已登录 <b>${esc(currentUser.username)}</b> · 进度自动同步到手机 / 电脑`;
    tip.classList.add('ok');
  } else {
    tip.innerHTML = '进度仅存本设备 · 换设备前先「导出」，到新设备「导入」';
    tip.classList.remove('ok');
  }
}

/* ---------- 时间与官方报名状态 ---------- */
function todayCN() {
  const now = new Date();
  return new Date(now.toLocaleString('en-US', { timeZone: 'Asia/Shanghai' }));
}
function parseEnd(s) {
  if (!s || s.includes('待') || s.includes('另行')) return null;
  const m = s.match(/^(\d{4})-(\d{2})-(\d{2})(?:\s+(\d{1,2}):(\d{2}))?/);
  if (!m) return null;
  return new Date(`${m[1]}-${m[2]}-${m[3]}T${(m[4] || '23').padStart(2, '0')}:${m[5] || '59'}:00+08:00`);
}
function stateOf(r) {
  const end = parseEnd(r.end), now = todayCN();
  if (r.status === '已截止') return { key: 'closed', label: '已截止', end };
  if (r.status === '待发布') return { key: 'pending', label: '待发布', end };
  if (end) {
    const days = Math.ceil((end - now) / 86400000);
    if (days < 0) return { key: 'closed', label: '已截止', end };
    if (days <= 7) return { key: 'soon', label: `${days}天内截止`, end };
  }
  return { key: 'open', label: '报名中', end };
}
function countdownParts(ms) {
  if (!ms) return null;
  const diff = ms - todayCN().getTime();
  if (diff <= 0) return { done: true };
  const t = Math.floor(diff / 60000);
  const d = Math.floor(t / 1440), h = Math.floor((t % 1440) / 60), m = t % 60;
  return { done: false, text: d > 0 ? `${d}天${h}时${m}分` : h > 0 ? `${h}时${m}分` : `${m}分` };
}

/* ---------- 排序：985→211，再按 软件工程>计算机>网安>其他 ---------- */
function typeRank(t) { return t === '985' ? 0 : t === '211' ? 1 : 2; }
const DIR_RANK = { '软件工程': 0, '计算机': 1, '网安': 2, '其他': 3 };
function dirRank(d) { return DIR_RANK[d] !== undefined ? DIR_RANK[d] : 3; }

/* 排序键：985/211 内，截止越近越前，时间不清的越后 */
function sortKey(r) {
  const st = stateOf(r);
  const tie = dirRank(r.direction) * 1e9 + r.id;
  if (!st.end) return 2e18 + tie;
  const endMs = st.end.getTime();
  if (st.key === 'closed') return 1e18 + endMs;
  return endMs;
}
const DIR_CLS = { '软件工程': 'dir-se', '计算机': 'dir-cs', '网安': 'dir-sec', '其他': 'dir-other' };

function esc(s = '') { return String(s).replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c])); }
function link(url, label, secondary = false) {
  if (!/^https?:\/\//.test(url || '')) return `<span class="btn disabled">${label}</span>`;
  return `<a class="btn ${secondary ? 'secondary' : ''}" href="${esc(url)}" target="_blank" rel="noopener">${label}</a>`;
}

/* ---------- 顶部「我的进度」统计 ---------- */
function renderStats() {
  const counts = { '': 0, applied: 0, iv: 0, adw: 0, adm: 0 };
  rows.forEach(r => { counts[getProgress(r.id)] = (counts[getProgress(r.id)] || 0) + 1; });
  const open = rows.filter(r => ['open', 'soon'].includes(stateOf(r).key)).length;
  const soon = rows.filter(r => stateOf(r).key === 'soon').length;
  const items = PROGRESS.map(p => ({ key: p.key || 'none', label: p.label, val: counts[p.key] || 0, cls: p.cls }));
  stats.innerHTML =
    items.map(it => `<button class="stat ${it.cls}" data-prog="${it.key}"><b>${it.val}</b><span>${it.label}</span></button>`).join('') +
    `<button class="stat s-open" data-status="open"><b>${open}</b><span>报名中</span></button>` +
    `<button class="stat s-soon" data-status="soon"><b>${soon}</b><span>7天内截止</span></button>`;
  stats.querySelectorAll('[data-prog]').forEach(b => b.addEventListener('click', () => setFilter('prog', b.dataset.prog)));
  stats.querySelectorAll('[data-status]').forEach(b => b.addEventListener('click', () => setFilter('status', b.dataset.status)));
}

/* ---------- 今日关注 ---------- */
function renderFocus() {
  const soon = rows.filter(r => stateOf(r).key === 'soon').sort((a, b) => (parseEnd(a.end) || 0) - (parseEnd(b.end) || 0));
  const open = rows.filter(r => stateOf(r).key === 'open');
  const mine = rows.filter(r => ['applied', 'iv', 'adw'].includes(getProgress(r.id)));
  const col = (title, cls, arr, fmt) => `
    <div class="focus-col ${cls}">
      <div class="focus-head">${title} <em>${arr.length}</em></div>
      ${arr.length ? arr.slice(0, 8).map(fmt).join('') : '<div class="focus-empty">暂无</div>'}
    </div>`;
  focus.innerHTML =
    col('⏰ 即将截止', 'f-soon', soon, r => {
      const p = countdownParts(parseEnd(r.end)?.getTime());
      return `<div class="focus-item"><span>${esc(r.school)}</span><b>${p ? p.text : '已结束'}</b></div>`;
    }) +
    col('🟢 正在报名', 'f-open', open, r => `<div class="focus-item"><span>${esc(r.school)}</span><b>${esc(r.direction)}</b></div>`) +
    col('📌 我的进行中', 'f-mine', mine, r => `<div class="focus-item"><span>${esc(r.school)}</span><b>${progressMeta(getProgress(r.id)).label}</b></div>`);
}

/* ---------- 紧急提醒 ---------- */
function renderReminder() {
  const soon = rows.filter(r => stateOf(r).key === 'soon').sort((a, b) => (parseEnd(a.end) || 0) - (parseEnd(b.end) || 0));
  if (!soon.length) { reminder.hidden = true; reminder.innerHTML = ''; return; }
  reminder.hidden = false;
  reminder.innerHTML = `<div class="reminder-head">⏰ 紧急提醒 · ${soon.length} 所将于 7 天内截止</div>` +
    soon.map(r => {
      const end = parseEnd(r.end)?.getTime();
      const p = countdownParts(end);
      return `<div class="reminder-item" data-end="${p && p.done ? '' : (end || '')}"><span class="r-school">${esc(r.school)}</span><span class="r-time">${p ? p.text : '已结束'}</span></div>`;
    }).join('');
}

/* ---------- 列表 ---------- */
function render() {
  const q = search.value.trim().toLowerCase();
  const filtered = rows.filter(r => {
    const st = stateOf(r);
    const hay = `${r.school} ${r.college} ${r.direction} ${r.remark}`.toLowerCase();
    if (q && !hay.includes(q)) return false;
    if (filters.type !== 'all' && r.type !== filters.type) return false;
    if (filters.dir !== 'all' && r.direction !== filters.dir) return false;
    if (filters.prog !== 'all') {
      const key = getProgress(r.id);
      if (filters.prog === 'none') { if (key !== '') return false; }
      else if (key !== filters.prog) return false;
    }
    if (filters.status !== 'all') {
      if (filters.status === '报名中' || filters.status === 'open') { if (!['open', 'soon'].includes(st.key)) return false; }
      else if (filters.status === 'soon') { if (st.key !== 'soon') return false; }
      else if (filters.status === '待发布') { if (r.status !== '待发布') return false; }
      else if (filters.status === '已截止') { if (st.key !== 'closed') return false; }
    }
    return true;
  });

  list.innerHTML = filtered.map(r => {
    const st = stateOf(r);
    const pm = progressMeta(getProgress(r.id));
    const endMs = st.end ? st.end.getTime() : null;
    const cd = (st.key === 'open' || st.key === 'soon')
      ? `<div class="countdown" data-end="${endMs || ''}">⏳ 距截止 ${(countdownParts(endMs) || { text: '—' }).text}</div>`
      : `<div class="countdown muted">${st.key === 'closed' ? '已结束' : '待发布'}</div>`;
    const opts = PROGRESS.map(p => `<option value="${p.key}" ${getProgress(r.id) === p.key ? 'selected' : ''}>${p.label}</option>`).join('');
    return `<article class="card ${st.key} ${pm.cls}" data-id="${r.id}">
      <div class="card-main">
        <div class="title-row">
          <div class="school">${esc(r.school)}</div>
          <span class="badge ${r.type === '211' ? 'type-211' : ''}">${esc(r.type)}</span>
          <span class="badge ${DIR_CLS[r.direction] || 'dir-other'}">${esc(r.direction)}</span>
          <span class="status ${st.key}">${esc(st.label)}</span>
        </div>
        <div class="college">${esc(r.college)}</div>
        <div class="meta"><span><strong>开始：</strong>${esc(r.start)}</span><span><strong>截止：</strong>${esc(r.end)}</span><span><strong>类型：</strong>${esc(r.admit)}</span></div>
        ${cd}
        ${r.remark ? `<div class="remark">${esc(r.remark)}</div>` : ''}
      </div>
      <div class="card-side">
        <label class="p-label">我的进度</label>
        <select class="progress ${pm.cls}" data-id="${r.id}">${opts}</select>
        <div class="actions">${link(r.site, '报名入口')}${link(r.source, '官方通知', true)}</div>
      </div>
    </article>`;
  }).join('');
  empty.hidden = filtered.length !== 0;
  tick();
}

/* ---------- 实时倒计时 ---------- */
function tick() {
  document.querySelectorAll('.countdown[data-end], .reminder-item[data-end]').forEach(el => {
    const ms = el.getAttribute('data-end');
    if (!ms) return;
    const p = countdownParts(Number(ms));
    if (!p || p.done) { el.textContent = el.classList.contains('reminder-item') ? '已结束' : '⏳ 已结束'; return; }
    el.textContent = el.classList.contains('reminder-item') ? p.text : `⏳ 距截止 ${p.text}`;
  });
}

/* ---------- 筛选 ---------- */
function setFilter(group, value) {
  filters[group] = value;
  document.querySelectorAll(`.chip[data-group="${group}"]`).forEach(x => {
    x.classList.toggle('active', x.dataset.value === value);
  });
  render();
}
document.querySelectorAll('.chip[data-group]').forEach(btn => btn.addEventListener('click', () => {
  setFilter(btn.dataset.group, btn.dataset.value);
}));

/* ---------- 进度下拉 ---------- */
list.addEventListener('change', e => {
  const sel = e.target.closest('select.progress');
  if (!sel) return;
  const id = sel.dataset.id;
  setProgress(id, sel.value);
  const pm = progressMeta(sel.value);
  const card = sel.closest('article.card');
  PROGRESS.forEach(p => { card.classList.remove(p.cls); sel.classList.remove(p.cls); });
  card.classList.add(pm.cls); sel.classList.add(pm.cls);
  renderStats(); renderFocus();
});

search.addEventListener('input', render);

/* ---------- 进度导出 / 导入（备份，无需后端） ---------- */
function exportProgress() {
  const blob = new Blob([JSON.stringify(progressMap, null, 2)], { type: 'application/json' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  const d = new Date();
  const pad = n => String(n).padStart(2, '0');
  a.href = url;
  a.download = `baoyan-progress-${d.getFullYear()}${pad(d.getMonth() + 1)}${pad(d.getDate())}.json`;
  document.body.appendChild(a); a.click(); a.remove();
  URL.revokeObjectURL(url);
  flashSync(`已导出 ${Object.keys(progressMap).length} 条进度`);
}
function importProgress(file) {
  const reader = new FileReader();
  reader.onload = () => {
    try {
      const data = JSON.parse(reader.result);
      if (typeof data !== 'object' || !data) throw new Error('格式不对');
      let n = 0;
      Object.keys(data).forEach(id => {
        const v = data[id];
        if (PROGRESS.some(p => p.key === v)) { progressMap[id] = v; n++; }
      });
      localStorage.setItem(LS_PROGRESS, JSON.stringify(progressMap));
      renderStats(); renderFocus(); render(); saveProgressToAPI();
      flashSync(`已导入 ${n} 条进度`);
    } catch (e) {
      flashSync('导入失败：不是有效的进度文件', true);
    }
  };
  reader.readAsText(file);
}
let syncTimer = null;
function flashSync(msg, isErr) {
  const tip = document.querySelector('.sync-tip');
  if (!tip) return;
  tip.textContent = msg;
  tip.classList.toggle('err', !!isErr);
  clearTimeout(syncTimer);
  syncTimer = setTimeout(() => {
    if (currentUser) { updateSyncBadge(); }
    else { tip.textContent = '进度仅存本设备 · 换设备前先「导出」，到新设备「导入」'; tip.classList.remove('err'); }
  }, 2600);
}
document.querySelector('#btn-export').addEventListener('click', exportProgress);
document.querySelector('#btn-import').addEventListener('click', () => document.querySelector('#file-import').click());
document.querySelector('#file-import').addEventListener('change', e => {
  if (e.target.files && e.target.files[0]) importProgress(e.target.files[0]);
  e.target.value = '';
});

/* ---------- 初始化 ---------- */
async function bootstrap() {
  const res = await fetch('./schools.json', { cache: 'no-store' });
  const payload = await res.json();
  rows = payload.schools || [];
  rows.sort((a, b) => typeRank(a.type) - typeRank(b.type) || sortKey(a) - sortKey(b));
  document.querySelector('#updated').textContent = `数据更新：${payload.updated_at || '未注明'}`;
  await loadProgressFromAPI();
  if (!accessToken) { showLogin(); return; } // 刷新失败被清空 → 回到登录
  updateSyncBadge();
  if (currentUser && userNameEl) userNameEl.textContent = currentUser.username;
  if (userbar) userbar.hidden = false;
  renderStats(); renderFocus(); renderReminder(); render();
}

async function init() {
  if (!accessToken) { showLogin(); return; }
  try {
    await bootstrap();
  } catch (e) {
    document.querySelector('#updated').textContent = '数据加载失败';
    document.querySelector('#empty').hidden = false;
    document.querySelector('#empty').textContent = '无法加载 schools.json 或后端不可用';
    showLogin();
  }
}
setInterval(tick, 30000);
init();
