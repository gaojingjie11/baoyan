/* 2027 计算机预推免 · 个人申请追踪器 */
let rows = [];
let filters = { type: 'all', dir: 'all', prog: 'all', status: 'all' };

const list = document.querySelector('#list');
const empty = document.querySelector('#empty');
const search = document.querySelector('#search');
const stats = document.querySelector('#stats');
const reminder = document.querySelector('#reminder');
const focus = document.querySelector('#focus');

/* ---------- 个人进度：localStorage ---------- */
const LS_KEY = 'bjt_progress_v1';
const PROGRESS = [
  { key: '',       label: '未报名', cls: 'p-none' },
  { key: 'applied',label: '已报名', cls: 'p-applied' },
  { key: 'iv',     label: '待复试', cls: 'p-iv' },
  { key: 'adw',    label: '待录取', cls: 'p-adw' },
  { key: 'adm',    label: '已录取', cls: 'p-adm' },
];
let progressMap = {};
try { progressMap = JSON.parse(localStorage.getItem(LS_KEY) || '{}'); } catch (e) { progressMap = {}; }
function getProgress(id) { return progressMap[id] || ''; }
function setProgress(id, val) {
  if (val) progressMap[id] = val; else delete progressMap[id];
  try { localStorage.setItem(LS_KEY, JSON.stringify(progressMap)); } catch (e) {}
}
function progressMeta(key) { return PROGRESS.find(p => p.key === key) || PROGRESS[0]; }

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

/* 排序键：985/211 内，截止越近越前，时间不清的越后
   ① 报名中/即将截止(有日期) → ② 已截止(有日期) → ③ 待发布/暂无数据(无日期)
   同级再按 方向优先(软件工程>计算机>网安>其他) + id 兜底，保证顺序稳定可复现 */
function sortKey(r) {
  const st = stateOf(r);
  const tie = dirRank(r.direction) * 1e9 + r.id;        // 方向优先 + id 兜底
  if (!st.end) return 2e18 + tie;                       // 时间不清 → 最后
  const endMs = st.end.getTime();
  if (st.key === 'closed') return 1e18 + endMs;          // 已截止 → 中间（仍按截止时间）
  return endMs;                                          // 报名中/即将截止 → 最前（越早截止越小）
}
const DIR_CLS = { '软件工程': 'dir-se', '计算机': 'dir-cs', '网安': 'dir-sec', '其他': 'dir-other' };

function esc(s = '') { return String(s).replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c])); }
function link(url, label, secondary = false) {
  if (!/^https?:\/\//.test(url || '')) return `<span class="btn disabled">${label}</span>`;
  return `<a class="btn ${secondary ? 'secondary' : ''}" href="${esc(url)}" target="_blank" rel="noopener">${label}</a>`;
}

/* ---------- 顶部「我的进度」统计（可点击筛选） ---------- */
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

/* ---------- 紧急提醒（7天内截止，实时倒计时） ---------- */
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

/* ---------- 实时倒计时刷新 ---------- */
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

/* ---------- 进度下拉（事件委托） ---------- */
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

/* ---------- 初始化 ---------- */
async function init() {
  try {
    const res = await fetch('./schools.json', { cache: 'no-store' });
    const payload = await res.json();
    rows = payload.schools || [];
    rows.sort((a, b) => typeRank(a.type) - typeRank(b.type) || sortKey(a) - sortKey(b));
    document.querySelector('#updated').textContent = `数据更新：${payload.updated_at || '未注明'}`;
    renderStats(); renderFocus(); renderReminder(); render();
  } catch (e) {
    document.querySelector('#updated').textContent = '数据加载失败';
    document.querySelector('#empty').hidden = false;
    document.querySelector('#empty').textContent = '无法加载 schools.json';
  }
}
setInterval(tick, 30000);
init();
