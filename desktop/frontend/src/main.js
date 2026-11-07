import './style.css';
import { DesktopService } from "../bindings/fast-https-desktop";

/* ---------------------------------------------------------------- 主题
   选中／激活态一律用中性色：亮色淡灰、暗色略亮的灰。
   语义色（ok / warn / danger）写在 CSS 里，不随主题色变化。 */
const ACCENTS = [
  { n: '石墨', v: '#5F6670', vd: '#C2C8D0', s: '#ECEDF0', d: 'rgba(255,255,255,.09)' },
  { n: '暖灰', v: '#6F655C', vd: '#D6C7B7', s: '#F0EDE9', d: 'rgba(214,199,183,.13)' },
  { n: '松绿', v: '#4F6B62', vd: '#8FBCAE', s: '#E8EFEC', d: 'rgba(143,188,174,.13)' },
  { n: '赭石', v: '#8A5A3B', vd: '#D1A183', s: '#F3EBE4', d: 'rgba(209,161,131,.13)' },
  { n: '靛灰', v: '#55607A', vd: '#A7B2C9', s: '#EAECF2', d: 'rgba(167,178,201,.13)' }
];
const root = document.documentElement;
const $ = (s) => document.querySelector(s);
const $$ = (s) => Array.from(document.querySelectorAll(s));
let accentIndex = 0;
let mode = 'light';
let currentLog = 'access';
let overviewBusy = false;
let detailsBusy = false;
let toastTimer = null;
let lastOverview = null;
let lastRunning = null;
let errorPushed = false;
let firstOverview = true;
let events = [];
let configDirty = false;
let configLoaded = false;

function store(k, v) { try { localStorage.setItem(k, v); } catch (e) { /* 忽略 */ } }
function load(k) { try { return localStorage.getItem(k); } catch (e) { return null; } }

function paintTheme() {
  const a = ACCENTS[accentIndex];
  root.style.setProperty('--accent', mode === 'dark' ? a.vd : a.v);
  root.style.setProperty('--accent-soft', mode === 'dark' ? a.d : a.s);
  root.style.setProperty('--accent-ink', mode === 'dark' ? '#14171C' : '#FFFFFF');
  root.setAttribute('data-mode', mode);
  $$('[data-accent]').forEach((b, i) => b.setAttribute('aria-checked', String(i === accentIndex)));
  $$('[data-mode-check]').forEach((b) => {
    const on = b.getAttribute('data-mode-check') === mode;
    b.textContent = on ? '✓' : '';
    if (b.parentElement) b.parentElement.setAttribute('aria-checked', String(on));
  });
}
function setAccent(i) { accentIndex = i; store('fh-accent', String(i)); paintTheme(); }
function setMode(m) { mode = m; store('fh-mode', m); paintTheme(); }

function initTheme() {
  const a = load('fh-accent');
  const m = load('fh-mode');
  if (a !== null && Number(a) >= 0 && Number(a) < ACCENTS.length) accentIndex = Number(a);
  if (m === 'light' || m === 'dark') mode = m;
  else if (window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches) mode = 'dark';
  paintTheme();
  $$('[data-accent]').forEach((b, i) => b.addEventListener('click', () => setAccent(i)));
}

function initAccountMenu() {
  const btn = $('#accountBtn');
  const menu = $('#accountMenu');
  const close = () => { menu.hidden = true; btn.setAttribute('aria-expanded', 'false'); };
  btn.addEventListener('click', (e) => {
    e.stopPropagation();
    menu.hidden = !menu.hidden;
    btn.setAttribute('aria-expanded', String(!menu.hidden));
  });
  document.addEventListener('click', (e) => {
    if (!menu.hidden && !menu.contains(e.target) && !btn.contains(e.target)) close();
  });
  document.addEventListener('keydown', (e) => { if (e.key === 'Escape') close(); });

  $$('[data-menu]').forEach((b) => b.addEventListener('click', async () => {
    const key = b.getAttribute('data-menu');
    if (key === 'light' || key === 'dark') { setMode(key); return; }
    close();
    if (key === 'settings') { activate('settings', '设置'); return; }
    if (key === 'tray') {
      try { await DesktopService.HideWindow(); } catch (e) { toast('窗口操作失败：' + msgOf(e)); }
      return;
    }
    if (key === 'help') {
      try { toast(await DesktopService.OpenHelp()); } catch (e) { toast('打开帮助失败：' + msgOf(e)); }
      return;
    }
  }));
}

/* ---------------------------------------------------------------- 通用 UI */
function toast(msg) {
  const el = $('#toast');
  el.textContent = msg;
  el.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => { el.hidden = true; }, 3200);
}
function bind(key, value) {
  $$('[data-bind="' + key + '"]').forEach((el) => { el.textContent = String(value); });
}
function esc(s) {
  return String(s ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
}
function msgOf(e) {
  if (!e) return '未知错误';
  return typeof e === 'string' ? e : (e.message || String(e));
}
/* ---------------------------------------------------------------- 设置页：主配置编辑 */
function setConfigStatus(text, kind) {
  const el = $('#configStatus');
  el.textContent = text;
  el.className = 'config-status' + (kind ? ' ' + kind : '');
}

let configInitial = {};

function renderConfigField(f) {
  const id = 'cf-' + f.path.replace(/[^\w-]/g, '-');
  let v = f.value || '';
  let control;
  if (f.type === 'multiselect') {
    // 固定候选 + 配置里已有的自定义值，避免勾选时把自定义项丢掉
    const current = String(v).split('\n').map((x) => x.trim()).filter(Boolean);
    const options = [...(f.options || [])];
    for (const x of current) if (!options.includes(x)) options.push(x);
    const checked = options.filter((o) => current.includes(o));
    v = checked.join('\n');
    control = `<div class="choice-group" id="${id}" data-value="${f.path}" data-multi="1">${options.map((o) => `<label class="choice"><input type="checkbox" value="${esc(o)}" ${current.includes(o) ? 'checked' : ''}/><span>${esc(o)}</span></label>`).join('')}</div>`;
  } else if (f.type === 'bool') {
    control = `<label class="form-check"><input type="checkbox" id="${id}" data-value="${f.path}" ${f.value === 'true' ? 'checked' : ''}/><span>开启</span></label>`;
  } else if (f.type === 'select') {
    const opts = (f.options || []).map((o) => `<option value="${esc(o)}" ${o === f.value ? 'selected' : ''}>${o === '' ? '（未设置）' : esc(o)}</option>`).join('');
    control = `<select id="${id}" data-value="${f.path}">${opts}</select>`;
  } else if (f.type === 'list' || f.type === 'textarea') {
    control = `<textarea id="${id}" data-value="${f.path}" rows="3" spellcheck="false">${esc(v)}</textarea>`;
  } else if (f.type === 'email') {
    control = `<input id="${id}" type="email" data-value="${f.path}" value="${esc(v)}" placeholder="you@example.com" spellcheck="false"/>`;
  } else {
    control = `<input id="${id}" type="${f.type === 'number' ? 'number' : 'text'}" data-value="${f.path}" value="${esc(v)}"/>`;
  }
  return `<div class="form-row ${f.missing ? 'missing' : ''}" data-row="${f.path}" data-initial="${esc(v)}">
    <label class="form-label" for="${id}">${esc(f.label)}${f.help ? `<small>${esc(f.help)}</small>` : ''}</label>
    <div class="form-control">${control}</div>
  </div>`;
}

function renderConfigForm(form) {
  const groups = new Map();
  for (const f of form.fields || []) {
    if (!groups.has(f.group)) groups.set(f.group, []);
    groups.get(f.group).push(f);
  }
  let html = '';
  for (const [name, fields] of groups) {
    html += `<div class="config-group"><h4 class="config-group-title">${esc(name)}</h4>${fields.map(renderConfigField).join('')}</div>`;
  }
  if (form.preserved && form.preserved.length) {
    html += `<div class="config-group"><h4 class="config-group-title">未纳入表单的字段</h4><p class="setting-note">${esc(form.preserved.join('、'))} 会在保存时原样保留。</p></div>`;
  }
  $('#configForm').innerHTML = html;
  configInitial = {};
  $$('#configForm [data-row]').forEach((row) => { configInitial[row.getAttribute('data-row')] = row.getAttribute('data-initial'); });
}

// 统一取一行表单的当前值：多选拼成换行串，布尔取勾选状态，其余取 value
function configRowValue(el) {
  if (el.hasAttribute('data-multi')) {
    return [...el.querySelectorAll('input:checked')].map((x) => x.value).join('\n');
  }
  if (el.type === 'checkbox') return String(el.checked);
  return el.value;
}

function collectChangedValues() {
  const values = {};
  $$('#configForm [data-row]').forEach((row) => {
    const el = row.querySelector('[data-value]');
    if (!el) return;
    const path = row.getAttribute('data-row');
    const v = configRowValue(el);
    if (v !== configInitial[path]) values[path] = v;
  });
  return values;
}

function markConfigDirty() {
  const n = Object.keys(collectChangedValues()).length;
  configDirty = n > 0;
  $('#configSave').disabled = n === 0;
  setConfigStatus(n ? `已修改 ${n} 个字段，尚未保存` : '已读取，未修改', n ? 'warn' : '');
}

async function loadConfig(force) {
  const box = $('#configForm');
  if (!box) return;
  if (configDirty && force) {
    const ok = await askConfirm('放弃未保存的修改？', '重新读取会用磁盘上的配置覆盖当前编辑内容，未保存的改动会丢失。', '放弃并重新读取');
    if (!ok) return;
  }
  setConfigStatus('正在读取…');
  try {
    const form = await DesktopService.ReadConfigForm();
    renderConfigForm(form);
    $('#configPath').textContent = form.path;
    $('#configMeta').textContent = form.size + ' B · ' + form.modified + (form.writable ? '' : ' · 只读');
    configDirty = false;
    configLoaded = true;
    $('#configSave').disabled = !form.writable;
    setConfigStatus(form.writable ? '已读取，未修改' : '文件不可写，仅可查看', form.writable ? '' : 'warn');
  } catch (e) {
    $('#configPath').textContent = '读取失败';
    $('#configMeta').textContent = '';
    box.innerHTML = '<p class="config-empty">读取失败：请确认桌面端与 fast-https 主程序放在同一目录，或从仓库根运行</p>';
    configInitial = {};
    configLoaded = false;
    $('#configSave').disabled = true;
    setConfigStatus('读取失败：' + msgOf(e), 'err');
  }
}

function initConfigEditor() {
  const box = $('#configForm');
  if (!box) return;
  box.addEventListener('input', markConfigDirty);
  box.addEventListener('change', markConfigDirty);
  $('#configReloadFile').addEventListener('click', () => loadConfig(true));
  $('#configSave').addEventListener('click', async () => {
    const values = collectChangedValues();
    const n = Object.keys(values).length;
    if (!n) { setConfigStatus('没有改动'); return; }
    const btn = $('#configSave');
    btn.disabled = true;
    setConfigStatus('正在保存…');
    try {
      const text = await DesktopService.SaveConfigForm(values);
      configDirty = false;
      $$('#configForm [data-row]').forEach((row) => {
        const el = row.querySelector('[data-value]');
        if (!el) return;
        const v = configRowValue(el);
        row.setAttribute('data-initial', v);
        configInitial[row.getAttribute('data-row')] = v;
      });
      setConfigStatus(`已保存 ${n} 个字段，请点「重载配置」使改动生效`, 'ok');
      toast(text);
      await refreshOverview();
    } catch (e) {
      btn.disabled = false;
      setConfigStatus('保存失败：' + msgOf(e), 'err');
      toast('保存失败：' + msgOf(e));
    }
  });
}

function activate(key, titleOverride) {
  $$('[data-pane]').forEach((p) => { p.hidden = p.getAttribute('data-pane') !== key; });
  let title = '';
  $$('[data-nav]').forEach((n) => {
    const on = n.getAttribute('data-nav') === key;
    n.classList.toggle('on', on);
    if (on) title = n.getAttribute('data-title') || '';
  });
  const finalTitle = titleOverride || title;
  if (finalTitle) $('#pageTitle').textContent = finalTitle;
  if (key === 'settings' && !configLoaded) loadConfig();
  refreshDetails();
}

/* ---------------------------------------------------------------- 最近事件 */
function pushEvent(text, kind) {
  const time = new Date().toLocaleTimeString('zh-CN', { hour12: false }).slice(0, 8);
  events.unshift({ time, text, kind: kind || 'info' });
  if (events.length > 5) events.pop();
  renderEvents();
}
function renderEvents() {
  const el = $('#eventList');
  if (!el) return;
  el.innerHTML = events.length
    ? events.map((e) => `<li class="${e.kind}"><span class="ev-dot"></span><div><b>${esc(e.text)}</b><small>${esc(e.time)} · 系统消息</small></div></li>`).join('')
    : '<li class="empty">暂无事件</li>';
}

function pushOverviewEvents(ov) {
  if (firstOverview) {
    pushEvent('管理端已启动，开始读取本机状态', 'info');
    firstOverview = false;
  }
  if (ov.error && !errorPushed) { pushEvent('读取配置文件失败', 'warn'); errorPushed = true; }
  if (!ov.error && errorPushed) { pushEvent('配置文件读取恢复', 'ok'); errorPushed = false; }
  if (lastRunning === null) {
    if (!ov.error) pushEvent('实时状态检测完成', 'info');
  } else if (lastRunning !== ov.running) {
    pushEvent(ov.running ? '服务已启动，端口开始响应' : '服务已停止', ov.running ? 'ok' : 'warn');
  }
  lastRunning = ov.running;
}

/* ---------------------------------------------------------------- 概览渲染 */
function renderPrimary(ov) {
  const btn = $('#primaryAction');
  if (!btn) return;
  const glyph = btn.querySelector('.glyph');
  if (ov.running) {
    btn.setAttribute('data-action', 'stop');
    $('#primaryLabel').textContent = '停止服务';
    if (glyph) glyph.textContent = '■';
  } else {
    btn.setAttribute('data-action', 'start');
    $('#primaryLabel').textContent = '启动服务';
    if (glyph) glyph.textContent = '▶';
  }
}

function renderOverview(ov) {
  lastOverview = ov;
  const runningText = ov.running ? '运行中' : '已停止';
  bind('running', runningText);
  bind('pid', 'pid ' + (ov.pid || '—'));
  bind('pidNum', ov.pid || '—');
  bind('version', ov.version || '1.6.1');
  bind('uptime', '状态每 2 秒刷新');
  bind('statusText', ov.running ? '回环端口可连接' : '未检测到监听端口');
  bind('ports', (ov.ports || []).join('　') || '—');
  bind('portCount', (ov.ports || []).length + ' 个端口');
  bind('siteCount', ov.siteCount || 0);
  bind('siteText', `${ov.siteCount || 0} 个，其中 ${ov.tlsCount || 0} 个启用 TLS`);
  bind('certText', ov.nearestCertDays > 0 ? `最近一张 ${ov.nearestCertDays} 天后到期` : '暂无可读取的证书');
  bind('checkedAt', '最近刷新 ' + (ov.checkedAt || '—'));

  const metricRunning = $('#metricRunning');
  metricRunning.textContent = runningText;
  metricRunning.className = 'm-value ' + (ov.running ? 'state-ok' : 'state-off');
  $('#metricPorts').textContent = (ov.ports || []).length;
  $('#metricSites').textContent = ov.siteCount || 0;

  const cfg = $('#configState');
  cfg.textContent = ov.configDirty ? '已修改，尚未重载' : '已重载';
  cfg.className = ov.configDirty ? 'warn-text' : 'ok-text';
  $('#stateChip').textContent = runningText;
  $('#runDot').classList.toggle('on', !!ov.running);
  const matrixLamp = $('#matrixLamp');
  if (matrixLamp) {
    matrixLamp.className = 'ms-lamp ' + (ov.running ? 'on' : 'off');
    $('#matrixState').textContent = runningText;
    $('#matrixSub').textContent = ov.running ? '回环端口可连接 · 每 2 秒刷新' : '未检测到监听端口';
  }
  renderPrimary(ov);

  const alert = $('#overviewError');
  if (ov.error) {
    alert.hidden = false;
    alert.className = 'alert-strip';
    $('#alertTitle').textContent = '配置文件读取失败';
    $('#alertBody').textContent = ov.error + '。请确认桌面端与 fast-https 主程序放在同一目录。';
  } else if (ov.configDirty) {
    alert.hidden = false;
    alert.className = 'alert-strip is-warn';
    $('#alertTitle').textContent = '配置已修改，尚未重载';
    $('#alertBody').textContent = '改动 conf.d 后点「重载配置」才生效，重载不会中断已有连接。';
  } else {
    alert.hidden = true;
  }

  pushOverviewEvents(ov);
}

/* ---------------------------------------------------------------- 明细渲染 */
function renderRows(target, rows) {
  $(target).innerHTML = rows.length
    ? rows.map((r) => `<div class="kv-row"><span class="k">${esc(r.k)}</span><span class="v">${esc(r.v)}</span><span class="n">${esc(r.n || '')}</span></div>`).join('')
    : '<div class="kv-row"><span class="k">—</span><span class="v">没有可显示的数据</span></div>';
}
function formatAccessSize(n) {
  if (!Number.isFinite(n)) return '—';
  if (n < 1024) return n + ' B';
  if (n < 1024 * 1024) return (n / 1024).toFixed(1) + ' KB';
  return (n / (1024 * 1024)).toFixed(1) + ' MB';
}

// access.log 每行是结构化 JSON；矩阵里只取时间/方法/路径/状态/大小，
// 不再把原始 JSON 整串折行塞进窄栏。非 JSON 行退化成截断文本。
function parseAccessLine(line) {
  let o;
  try { o = JSON.parse(line); } catch (e) { return null; }
  if (!o || typeof o !== 'object') return null;
  const t = String(o.time || '');
  const m = t.match(/T(\d{2}:\d{2}:\d{2})/);
  return {
    time: m ? m[1] : (t.slice(0, 8) || '—'),
    method: String(o.method || '—'),
    path: String(o.path || '—'),
    status: o.status != null ? String(o.status) : '—',
    size: o.size != null ? formatAccessSize(Number(o.size)) : '—'
  };
}

function renderLogs(lines, target, kind) {
  const el = $(target);
  if (!lines || lines.length === 0) {
    el.innerHTML = kind === 'rail' ? '<div class="log-empty">日志为空或文件不存在</div>' : '<tr><td colspan="3">日志为空或文件不存在</td></tr>';
    return;
  }
  if (kind === 'rail') {
    el.innerHTML = lines.slice(0, 8).map((line) => {
      const a = parseAccessLine(line);
      if (!a) {
        return `<div class="access-row"><span class="a-time">—</span><span class="a-method">—</span><span class="a-path" title="${esc(line)}">${esc(line.slice(0, 90))}</span><span class="a-status">—</span><span class="a-size">—</span></div>`;
      }
      const code = Number(a.status);
      const cls = code >= 400 ? 'err' : (code >= 300 ? 'warn' : (Number.isFinite(code) ? 'ok' : ''));
      return `<div class="access-row"><span class="a-time">${esc(a.time)}</span><span class="a-method">${esc(a.method)}</span><span class="a-path" title="${esc(a.path)}">${esc(a.path)}</span><span class="a-status ${cls}">${esc(a.status)}</span><span class="a-size">${esc(a.size)}</span></div>`;
    }).join('');
    return;
  }
  el.innerHTML = lines.map((l) => `<tr><td colspan="3" class="mono">${esc(l)}</td></tr>`).join('');
}

// 站点访问地址：默认端口省略，其他端口显式带上；server_name 可能是域名或本机 IP。
function siteURL(s) {
  const scheme = s.tls ? 'https' : 'http';
  const port = String(s.port || '').replace(/^:/, '');
  const host = s.name || 'localhost';
  const isDefault = (scheme === 'https' && port === '443') || (scheme === 'http' && port === '80');
  return scheme + '://' + host + (port && !isDefault ? ':' + port : '') + '/';
}

function renderSites(sites, certs) {
  $('#sitesTable').innerHTML = sites.length
    ? sites.map((s) => {
        const url = siteURL(s);
        const off = lastOverview?.running ? '' : ' is-off';
        return `<div class="kv-row"><span class="k">${esc(s.port || '—')}</span><span class="v"><a class="site-link${off}" href="#" data-url="${esc(url)}" title="${esc(url)}">${esc(url)}</a></span><span class="n with-action"><span>${esc(s.source || '')}</span><button class="icon-btn site-more" data-source="${esc(s.source || '')}" title="查看详情">⋯</button></span></div>`;
      }).join('')
    : '<div class="kv-row"><span class="k">—</span><span class="v">没有站点</span></div>';
  $('#matrixBody').innerHTML = sites.length
    ? sites.map((s) => {
        const c = certs.find((x) => x.domain === s.name);
        let dot = 'off', text = '未运行';
        if (lastOverview?.running) {
          if (c?.selfSigned) { dot = 'warn'; text = '自签'; }
          else { dot = 'ok'; text = '正常'; }
        }
        const issuer = c ? (c.selfSigned ? '自签证书' : (c.issuer || '—')) : '—';
        return `<tr>
          <td title="${esc(s.name)}">${esc(s.name)}</td>
          <td class="mono">${esc(s.port || '—')}</td>
          <td><span class="tls-pill ${s.tls ? 'on' : ''}">${s.tls ? 'ssl' : 'HTTP'}</span></td>
          <td class="cert" title="${esc(issuer)}">${esc(issuer)}</td>
          <td class="state-cell"><span class="matrix-state"><i class="dot ${dot}"></i>${text}</span></td>
        </tr>`;
      }).join('')
    : '<tr><td colspan="5">没有站点</td></tr>';
}

function renderCerts(certs) {
  renderRows('#certsTable', certs.map((c) => ({
    k: c.domain,
    v: `${c.issuer || '—'}　·　${c.notAfter || '—'}　·　${c.daysLeft > 0 ? '剩 ' + c.daysLeft + ' 天' : (c.selfSigned ? '自签证书' : c.note)}`,
    n: c.selfSigned ? '自签' : '已签发'
  })));
  $('#metricCerts').textContent = certs.length;
  const valid = certs.filter((c) => c.daysLeft > 0).sort((a, b) => a.daysLeft - b.daysLeft)[0];
  const scale = $('#certScale');
  scale.hidden = !valid;
  if (valid) {
    const pct = Math.max(4, Math.min(96, Math.round(100 - valid.daysLeft / 90 * 100)));
    $('#scaleCursor').style.left = pct + '%';
    $('#scaleToday').style.left = pct + '%';
    $('#scaleToday').textContent = `今天 · 剩 ${valid.daysLeft} 天`;
    $$('[data-bind="certNotAfter"]').forEach((el) => { el.textContent = valid.notAfter; });
  }
}

function renderPaths(p) {
  renderRows('#pathsTable', [
    { k: '主配置', v: p.config, n: 'config' },
    { k: '证书目录', v: p.cert, n: 'cert' },
    { k: '日志目录', v: p.log, n: 'log' },
    { k: 'pid 文件', v: p.pid, n: 'pid' }
  ]);
}

/* ---------------------------------------------------------------- 数据拉取 */
async function refreshOverview() {
  if (overviewBusy) return;
  overviewBusy = true;
  try {
    renderOverview(await DesktopService.GetOverview());
  } catch (e) {
    $('#stateChip').textContent = '未连接后端';
    const alert = $('#overviewError');
    alert.hidden = false;
    alert.className = 'alert-strip';
    $('#alertTitle').textContent = '无法连接 Go 后端';
    $('#alertBody').textContent = msgOf(e) + '。请用 wails3 dev 或构建后的程序打开。';
  } finally {
    overviewBusy = false;
  }
}

async function refreshDetails() {
  if (detailsBusy) return;
  detailsBusy = true;
  try {
    const [sites, certs, paths, lines] = await Promise.all([
      DesktopService.ListSites(),
      DesktopService.ListCerts(),
      DesktopService.Paths(),
      DesktopService.TailLog(currentLog, 12)
    ]);
    renderSites(sites || [], certs || []);
    renderCerts(certs || []);
    renderPaths(paths);
    renderLogs(lines, '#logBody', 'table');
    renderLogs(lines, '#matrixLogs', 'rail');
  } catch (e) {
    console.warn('refreshDetails:', msgOf(e));
  } finally {
    detailsBusy = false;
  }
}

/* ---------------------------------------------------------------- 操作 */
async function openByPath(kind) {
  try {
    const p = await DesktopService.Paths();
    const target = kind === 'config' ? p.config : kind === 'log' ? p.log : (p.config ? p.config.replace(/[\\/][^\\/]+$/, '') : '');
    if (!target) throw new Error('路径为空');
    toast(await DesktopService.OpenPath(target));
  } catch (e) { toast('打开失败：' + msgOf(e)); }
}

async function doAction(name) {
  const simple = {
    start: ['启动', () => DesktopService.StartServer()],
    stop: ['停止', () => DesktopService.StopServer()],
    reload: ['重载配置', () => DesktopService.ReloadServer()],
    status: ['查询状态', () => DesktopService.ServerStatus()]
  };
  if (name === 'open-config' || name === 'open-log' || name === 'open-config-dir') {
    await openByPath(name === 'open-config' ? 'config' : name === 'open-log' ? 'log' : 'dir');
    return;
  }
  const entry = simple[name];
  if (!entry) return;
  const [label, fn] = entry;
  try {
    const text = await fn();
    toast(text || label + '完成');
    await refreshOverview();
    await refreshDetails();
  } catch (e) {
    toast(label + '失败：' + msgOf(e));
  }
}

/* ---------------------------------------------------------------- 启动 */
function openCloseDialog() {
  const mask = $('#closeDialog');
  if (!mask) return;
  mask.hidden = false;
  $('#closeHide').focus();
}

function initCloseDialog() {
  const mask = $('#closeDialog');
  if (!mask) return;
  const close = () => { mask.hidden = true; };
  $('#closeCancel').addEventListener('click', close);
  $('#closeHide').addEventListener('click', async () => {
    close();
    try { await DesktopService.HideWindow(); } catch (e) { toast('窗口操作失败：' + msgOf(e)); }
  });
  $('#closeQuit').addEventListener('click', async () => {
    close();
    try { await DesktopService.QuitApp(); } catch (e) { toast('退出失败：' + msgOf(e)); }
  });
  mask.addEventListener('click', (e) => { if (e.target === mask) close(); });
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape' && !mask.hidden) { e.preventDefault(); close(); }
  });
}

let siteMode = 'add';
let siteEditingSource = '';

// local 与 proxy 显示的字段不同，详情回填后也要重新同步一次
function syncSiteType() {
  const proxy = $('#siteType').value === 'proxy';
  $('#siteRootRow').hidden = proxy;
  $('#siteIndexRow').hidden = proxy;
  $('#siteProxyRow').hidden = !proxy;
}

function setSiteFormEnabled(enabled) {
  $$('#siteDialog input, #siteDialog select').forEach((el) => { el.disabled = !enabled; });
  $$('#siteDialog [data-browse]').forEach((b) => { b.disabled = !enabled; });
  $('#siteSubmit').hidden = !enabled;
}

function resetSiteForm() {
  ['siteName', 'siteListen', 'siteRoot', 'siteProxy', 'siteIndex', 'siteCert', 'siteCertKey', 'siteFileName']
    .forEach((id) => { $('#' + id).value = ''; });
  $('#siteType').value = 'local';
  $('#siteFileNameRow').hidden = false;
  $('#siteSource').hidden = true;
  $('#siteSource').textContent = '';
  $('#siteNote').hidden = true;
  $('#siteError').hidden = true;
  setSiteFormEnabled(true);
  syncSiteType();
}

// 点站点行的「⋯」：读取该站点文件的字段与原文，不可编辑时只读展示原因
async function openSiteDetail(source) {
  siteMode = 'edit';
  siteEditingSource = source;
  resetSiteForm();
  $('#siteDialog').hidden = false;
  $('#siteDialogTitle').textContent = '站点详情';
  $('#siteSubmit').textContent = '保存修改';
  $('#siteFileNameRow').hidden = true;
  $('#siteSource').hidden = false;
  $('#siteSource').textContent = source;
  try {
    const d = await DesktopService.ReadSiteDetail(source);
    $('#siteName').value = d.name || '';
    $('#siteListen').value = d.listen || '';
    $('#siteType').value = d.type || 'local';
    $('#siteRoot').value = d.root || '';
    $('#siteProxy').value = d.proxyPass || '';
    $('#siteIndex').value = d.index || '';
    $('#siteCert').value = d.cert || '';
    $('#siteCertKey').value = d.certKey || '';
    syncSiteType();
    if (!d.editable) {
      setSiteFormEnabled(false);
      $('#siteNote').textContent = d.note || '该站点不可在这里编辑';
      $('#siteNote').hidden = false;
    }
  } catch (e) {
    setSiteFormEnabled(false);
    $('#siteError').textContent = msgOf(e);
    $('#siteError').hidden = false;
  }
}

function initSiteDialog() {
  const mask = $('#siteDialog');
  if (!mask) return;
  const close = () => { mask.hidden = true; $('#siteError').hidden = true; };
  $('#addSiteBtn').addEventListener('click', () => {
    siteMode = 'add';
    siteEditingSource = '';
    resetSiteForm();
    $('#siteDialogTitle').textContent = '添加站点';
    $('#siteSubmit').textContent = '创建站点';
    mask.hidden = false;
    $('#siteName').focus();
  });
  $('#siteCancel').addEventListener('click', close);
  mask.addEventListener('click', (e) => { if (e.target === mask) close(); });
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape' && !mask.hidden) { e.preventDefault(); close(); }
  });
  // 站点行是动态重建的，详情按钮用事件委托
  document.addEventListener('click', (e) => {
    const more = e.target.closest ? e.target.closest('.site-more') : null;
    if (!more) return;
    e.preventDefault();
    openSiteDetail(more.getAttribute('data-source'));
  });
  $$('#siteDialog [data-browse]').forEach((b) => b.addEventListener('click', async () => {
    try {
      const p = await DesktopService.PickPath(b.getAttribute('data-browse'));
      if (p) $('#' + b.getAttribute('data-target')).value = p;
    } catch (e) {
      $('#siteError').textContent = '选择失败：' + msgOf(e);
      $('#siteError').hidden = false;
    }
  }));
  $('#siteType').addEventListener('change', syncSiteType);
  $('#siteSubmit').addEventListener('click', async () => {
    const btn = $('#siteSubmit');
    btn.disabled = true;
    $('#siteError').hidden = true;
    const payload = {
      name: $('#siteName').value,
      listen: $('#siteListen').value,
      type: $('#siteType').value,
      root: $('#siteRoot').value,
      proxyPass: $('#siteProxy').value,
      index: $('#siteIndex').value,
      cert: $('#siteCert').value,
      certKey: $('#siteCertKey').value,
      fileName: siteMode === 'add' ? $('#siteFileName').value : '',
      source: siteMode === 'edit' ? siteEditingSource : ''
    };
    try {
      const text = siteMode === 'add'
        ? await DesktopService.AddSite(payload)
        : await DesktopService.SaveSiteDetail(payload);
      close();
      toast(text);
      await refreshDetails();
      await refreshOverview();
    } catch (e) {
      $('#siteError').textContent = msgOf(e);
      $('#siteError').hidden = false;
    } finally {
      btn.disabled = false;
    }
  });
}

function initSiteLinks() {
  // 站点地址是 <a href="#">，统一委托到 Go 侧的 OpenPath，用系统默认浏览器打开。
  document.addEventListener('click', async (e) => {
    const a = e.target.closest ? e.target.closest('.site-link') : null;
    if (!a) return;
    e.preventDefault();
    try {
      await DesktopService.OpenPath(a.getAttribute('data-url'));
    } catch (err) {
      toast('打开链接失败：' + msgOf(err));
    }
  });
}

// 应用内确认框：原生 confirm 在 dev 模式下会把标题显示成 wails.localhost:9245
let confirmResolve = null;

function askConfirm(title, body, okText) {
  const mask = $('#confirmDialog');
  if (!mask) return Promise.resolve(false);
  $('#confirmTitle').textContent = title;
  $('#confirmBody').textContent = body;
  $('#confirmOk').textContent = okText || '确定';
  mask.hidden = false;
  $('#confirmOk').focus();
  return new Promise((resolve) => { confirmResolve = resolve; });
}

function initConfirmDialog() {
  const mask = $('#confirmDialog');
  if (!mask) return;
  const settle = (value) => {
    mask.hidden = true;
    const resolve = confirmResolve;
    confirmResolve = null;
    if (resolve) resolve(value);
  };
  $('#confirmCancel').addEventListener('click', () => settle(false));
  $('#confirmOk').addEventListener('click', () => settle(true));
  mask.addEventListener('click', (e) => { if (e.target === mask) settle(false); });
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape' && !mask.hidden) { e.preventDefault(); settle(false); }
  });
}

function init() {
  initTheme();
  initAccountMenu();
  initConfigEditor();
  initCloseDialog();
  initSiteDialog();
  initSiteLinks();
  initConfirmDialog();
  $$('[data-nav]').forEach((n) => n.addEventListener('click', () => activate(n.getAttribute('data-nav'))));
  $$('[data-goto]').forEach((b) => b.addEventListener('click', () => activate(b.getAttribute('data-goto'))));
  $$('[data-action]').forEach((b) => b.addEventListener('click', () => doAction(b.getAttribute('data-action'))));
  $$('[data-log]').forEach((b) => b.addEventListener('click', () => {
    currentLog = b.getAttribute('data-log');
    $$('[data-log]').forEach((x) => x.classList.toggle('on', x === b));
    refreshDetails();
  }));
  $$('[data-window]').forEach((b) => b.addEventListener('click', async () => {
    const action = b.getAttribute('data-window');
    try {
      if (action === 'minimise') await DesktopService.MinimiseWindow();
      else if (action === 'maximise') await DesktopService.ToggleMaximiseWindow();
      else if (action === 'hide') openCloseDialog();
    } catch (e) { toast('窗口操作失败：' + msgOf(e)); }
  }));
  renderEvents();
  refreshOverview();
  refreshDetails();
  setInterval(refreshOverview, 2000);
  setInterval(refreshDetails, 15000);
}

init();
