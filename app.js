
let rows = [];
let typeFilter = 'all';
let statusFilter = '';
const list = document.querySelector('#list');
const empty = document.querySelector('#empty');
const search = document.querySelector('#search');
const stats = document.querySelector('#stats');
const reminder = document.querySelector('#reminder');

function todayCN(){
  const now = new Date();
  return new Date(now.toLocaleString('en-US',{timeZone:'Asia/Shanghai'}));
}
function parseEnd(s){
  if(!s || s.includes('待') || s.includes('另行')) return null;
  const m=s.match(/^(\d{4})-(\d{2})-(\d{2})(?:\s+(\d{1,2}):(\d{2}))?/);
  if(!m)return null;
  return new Date(`${m[1]}-${m[2]}-${m[3]}T${(m[4]||'23').padStart(2,'0')}:${m[5]||'59'}:00+08:00`);
}
function stateOf(r){
  const end=parseEnd(r.end), now=todayCN();
  if(r.status==='已截止') return {key:'closed',label:'已截止',end};
  if(r.status==='待发布') return {key:'pending',label:'待发布',end};
  if(end){
    const days=Math.ceil((end-now)/86400000);
    if(days<0)return {key:'closed',label:'已截止',end};
    if(days<=7)return {key:'soon',label:`${days}天内截止`,end};
  }
  return {key:'open',label:'报名中',end};
}
// 985 优先，211 其次，其余最后；同类型保持原顺序
function typeRank(t){return t==='985'?0:t==='211'?1:2;}

// 根据绝对截止时间戳计算剩余「天/时/分」
function countdownParts(ms){
  if(!ms) return null;
  const diff = ms - todayCN().getTime();
  if(diff<=0) return {done:true};
  const totalMin=Math.floor(diff/60000);
  const d=Math.floor(totalMin/1440);
  const h=Math.floor((totalMin%1440)/60);
  const m=totalMin%60;
  return {done:false, d, h, m, text: d>0?`${d}天${h}时${m}分`:h>0?`${h}时${m}分`:`${m}分`};
}
function esc(s=''){return String(s).replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]))}
function link(url,label,secondary=false){
  if(!/^https?:\/\//.test(url||''))return `<span class="btn disabled">${label}</span>`;
  return `<a class="btn ${secondary?'secondary':''}" href="${esc(url)}" target="_blank" rel="noopener">${label}</a>`;
}
function renderStats(){
  const open=rows.filter(r=>['open','soon'].includes(stateOf(r).key)).length;
  const soon=rows.filter(r=>stateOf(r).key==='soon').length;
  stats.innerHTML=[
    ['学校总数',rows.length],['985',rows.filter(r=>r.type==='985').length],['报名中',open],['7天内截止',soon]
  ].map(([k,v])=>`<div class="stat"><b>${v}</b><span>${k}</span></div>`).join('');
}
// 顶部紧急提醒：列出 7 天内截止的学校，带实时倒计时
function renderReminder(){
  const soon = rows.filter(r=>stateOf(r).key==='soon')
    .sort((a,b)=> (parseEnd(a.end)||0)-(parseEnd(b.end)||0));
  if(!soon.length){ reminder.hidden=true; reminder.innerHTML=''; return; }
  reminder.hidden=false;
  reminder.innerHTML = `<div class="reminder-head">⏰ 紧急提醒 · ${soon.length} 所学校将于 7 天内截止</div>` +
    soon.map(r=>{
      const p=countdownParts(parseEnd(r.end)?.getTime());
      return `<div class="reminder-item" data-end="${p&&p.done?'':(parseEnd(r.end)?.getTime()||'')}"><span class="r-school">${esc(r.school)}</span><span class="r-time">${p?p.text:'已结束'}</span></div>`;
    }).join('');
}
function render(){
  const q=search.value.trim().toLowerCase();
  const filtered=rows.filter(r=>{
    const st=stateOf(r), hay=`${r.school} ${r.college} ${r.remark}`.toLowerCase();
    return (!q||hay.includes(q)) &&
      (typeFilter==='all'||r.type===typeFilter) &&
      (!statusFilter || (statusFilter==='报名中'?['open','soon'].includes(st.key):(r.status===statusFilter||st.label===statusFilter)));
  });
  list.innerHTML=filtered.map(r=>{
    const st=stateOf(r);
    const endMs = st.end? st.end.getTime():null;
    const cd = (st.key==='open'||st.key==='soon')
      ? `<div class="countdown" data-end="${endMs||''}">⏳ 距截止 ${(countdownParts(endMs)||{text:'—'}).text}</div>`
      : `<div class="countdown muted">${st.key==='closed'?'已结束':'待发布'}</div>`;
    return `<article class="card ${st.key}">
      <div>
        <div class="title-row"><div class="school">${esc(r.school)}</div><span class="badge ${r.type==='211'?'type-211':''}">${esc(r.type)}</span><span class="status ${st.key}">${esc(st.label)}</span></div>
        <div class="college">${esc(r.college)}</div>
        <div class="meta"><span><strong>开始：</strong>${esc(r.start)}</span><span><strong>截止：</strong>${esc(r.end)}</span><span><strong>类型：</strong>${esc(r.admit)}</span></div>
        ${cd}
        ${r.remark?`<div class="remark">${esc(r.remark)}</div>`:''}
      </div>
      <div class="actions">${link(r.site,'报名入口')}${link(r.source,'官方通知',true)}</div>
    </article>`;
  }).join('');
  empty.hidden=filtered.length!==0;
  tick();
}
// 实时刷新所有倒计时（页面每 30 秒刷新一次）
function tick(){
  document.querySelectorAll('.countdown[data-end], .reminder-item[data-end]').forEach(el=>{
    const ms=el.getAttribute('data-end');
    if(!ms) return;
    const p=countdownParts(Number(ms));
    if(!p){ el.textContent = el.classList.contains('reminder-item') ? '已结束' : '⏳ 已结束'; return; }
    el.textContent = el.classList.contains('reminder-item') ? p.text : `⏳ 距截止 ${p.text}`;
  });
}
async function init(){
  try{
    const res=await fetch('./schools.json',{cache:'no-store'});
    const payload=await res.json();
    rows=payload.schools||[];
    rows.sort((a,b)=> typeRank(a.type)-typeRank(b.type));
    document.querySelector('#updated').textContent=`数据更新：${payload.updated_at||'未注明'}`;
    renderStats();renderReminder();render();
  }catch(e){
    document.querySelector('#updated').textContent='数据加载失败';
    document.querySelector('#empty').hidden=false;
    document.querySelector('#empty').textContent='无法加载 schools.json';
  }
}
document.querySelectorAll('[data-filter-type]').forEach(btn=>btn.addEventListener('click',()=>{
  document.querySelectorAll('[data-filter-type]').forEach(x=>x.classList.remove('active'));
  btn.classList.add('active');typeFilter=btn.dataset.filterType;render();
}));
document.querySelectorAll('[data-filter-status]').forEach(btn=>btn.addEventListener('click',()=>{
  const same=statusFilter===btn.dataset.filterStatus;
  document.querySelectorAll('[data-filter-status]').forEach(x=>x.classList.remove('active'));
  statusFilter=same?'':btn.dataset.filterStatus;if(!same)btn.classList.add('active');render();
}));
search.addEventListener('input',render);
setInterval(tick,30000);
init();
