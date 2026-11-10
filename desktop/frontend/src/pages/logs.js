// 日志页：logs query 的白名单筛选 + 游标滚动分页 + 实时拉取。
// 常用筛选（类型/级别/状态）一行，高级筛选折叠；实时开关每 2s 拉最新一页
// 并按时间去重 prepend，页面隐藏时暂停。行按字段分栏展示，tooltip 保留 raw。
import { DesktopService, msgOf } from '../api.js';
import { t } from '../i18n.js';
import { $, esc } from '../ui/dom.js';
import { state } from '../state.js';

const LOG_PAGE = 30;
const LIVE_MS = 2000;
let logRows = [];
let logHasMore = false;
let logLoading = false;
let logFilters = { level: '', status: '', host: '', method: '', pathPrefix: '', reason: '', since: '', until: '' };
let liveTimer = null;

function timeText(s) {
  const m = String(s || '').match(/T(\d{2}:\d{2}:\d{2})(?:\.(\d{1,3}))?/);
  if (m) return m[1] + (m[2] ? '.' + m[2] : '');
  return s || '—';
}
function sizeText(v) {
  if (v == null || v === '') return '';
  const s = String(v);
  if (!/^\d+$/.test(s)) return s;
  const n = Number(s);
  if (n < 1024) return n + ' B';
  if (n < 1024 * 1024) return (n / 1024).toFixed(1) + ' KB';
  return (n / 1024 / 1024).toFixed(1) + ' MB';
}

function renderLogRows() {
  const el = $('#logList');
  const keep = el.scrollTop;
  if (!logRows.length) {
    el.innerHTML = `<div class="log-empty">${esc(t('logs.noMatch'))}</div>`;
    return;
  }
  el.innerHTML = logRows.map((e) => {
    const lvl = e.level || '—';
    const code = String(e.status || '');
    const cls = code ? (Number(code) >= 500 || /^5/.test(code) ? 'err' : (Number(code) >= 400 || /^4/.test(code) ? 'warn' : 'ok')) : '';
    return `<div class="log-row" title="${esc(e.raw || '')}">`
      + `<span class="log-time">${esc(timeText(e.time))}</span>`
      + `<span class="log-level lv-${esc(lvl)}">${esc(lvl)}</span>`
      + `<span class="log-method">${esc(e.method || '—')}</span>`
      + `<span class="log-path" title="${esc(e.path || '')}">${esc(e.path || '—')}</span>`
      + `<span class="log-status ${cls}">${esc(e.status || '—')}</span>`
      + `<span class="log-size">${esc(sizeText(e.size) || '—')}</span>`
      + `<span class="log-host" title="${esc(e.host || '')}">${esc(e.host || '—')}</span>`
      + `<span class="log-remote">${esc(e.remoteAddr || '—')}</span>`
      + `<span class="log-reason" title="${esc(e.reason || '')}">${esc(e.reason || '—')}`
      + (e.message ? `<small>${esc(e.message)}</small>` : '')
      + `</span></div>`;
  }).join('');
  el.scrollTop = keep;
}

function setStatusText() {
  const live = $('#logLive')?.checked ? ' · ' + t('logs.autoRefreshOn') : '';
  $('#logStatusText').textContent = (logHasMore
    ? t('logs.statusLoaded', { n: logRows.length })
    : (logRows.length ? t('logs.statusOldest') : t('logs.noMatch'))) + live;
}

// rerenderLogs 在语言切换时用已有数据重绘当前页，不重新查询。
export function rerenderLogs() {
  renderLogRows();
  setStatusText();
}

function queryArgs(cursor) {
  return {
    kind: state.currentLog,
    limit: LOG_PAGE,
    level: logFilters.level,
    status: logFilters.status,
    host: logFilters.host,
    method: logFilters.method,
    pathPrefix: logFilters.pathPrefix,
    reason: logFilters.reason,
    since: logFilters.since,
    until: logFilters.until,
    cursor: cursor || state.logCursor
  };
}

// mergeNewer 把实时拉取到的新行 prepend，按 time+message 去重，
// 保持「由新到旧」。返回是否有新增。
function mergeNewer(lines) {
  const seen = new Set(logRows.map((e) => (e.time || '') + '|' + (e.message || '') + '|' + (e.path || '')));
  let added = 0;
  const fresh = [];
  for (const e of lines) {
    const k = (e.time || '') + '|' + (e.message || '') + '|' + (e.path || '');
    if (seen.has(k)) continue;
    seen.add(k);
    fresh.push(e);
    added++;
  }
  if (added) logRows = fresh.concat(logRows).slice(0, 600);
  return added;
}

export async function loadLogs(reset) {
  if (logLoading) return;
  if (reset) {
    logRows = [];
    logHasMore = false;
    state.logCursor = '';
  }
  logLoading = true;
  const more = $('#logMore');
  more.disabled = true;
  if (reset) $('#logStatusText').textContent = t('logs.querying');
  try {
    const page = await DesktopService.QueryLogs(queryArgs());
    const lines = page.lines || [];
    if (!reset) {
      logRows = logRows.concat(lines);
    } else {
      logRows = lines;
    }
    state.logCursor = page.nextUntil || '';
    logHasMore = !!page.hasMore && lines.length > 0;
    renderLogRows();
    setStatusText();
    more.hidden = !logHasMore;
  } catch (e) {
    if (reset) $('#logStatusText').textContent = t('logs.queryFailed', { msg: msgOf(e) });
  } finally {
    logLoading = false;
    more.disabled = false;
  }
}

// liveTick：实时模式只拉「最新一页」（不用游标），有新增才重绘。
async function liveTick() {
  if (document.hidden || !$('#logLive')?.checked || logLoading) return;
  // 分页状态下不自动拉，避免把用户翻到的更早记录冲掉
  if (logRows.length > LOG_PAGE) return;
  try {
    const page = await DesktopService.QueryLogs(queryArgs(''));
    const lines = page.lines || [];
    if (mergeNewer(lines)) {
      renderLogRows();
      setStatusText();
    }
  } catch (e) {
    $('#logStatusText').textContent = t('logs.queryFailed', { msg: msgOf(e) });
  }
}

function setLive(on) {
  if (liveTimer) { clearInterval(liveTimer); liveTimer = null; }
  if (on) liveTimer = setInterval(liveTick, LIVE_MS);
  setStatusText();
}

function collectLogFilters() {
  logFilters = {
    level: $('#logLevel').value.trim(),
    status: $('#logStatus').value,
    host: $('#logHost').value.trim(),
    method: $('#logMethod').value.trim(),
    pathPrefix: $('#logPathPrefix').value.trim(),
    reason: $('#logReason').value.trim(),
    since: $('#logSince').value.trim(),
    until: $('#logUntil').value.trim()
  };
}

const FILTER_IDS = ['logLevel', 'logStatus', 'logHost', 'logMethod', 'logPathPrefix', 'logReason', 'logSince', 'logUntil'];

export function initLogPage() {
  $('#logApply').addEventListener('click', () => { collectLogFilters(); loadLogs(true); });
  $('#logReset').addEventListener('click', () => {
    FILTER_IDS.forEach((id) => { const el = $('#' + id); if (el) el.value = ''; });
    collectLogFilters();
    loadLogs(true);
  });
  $('#logRefresh').addEventListener('click', () => loadLogs(true));
  $('#logMore').addEventListener('click', () => loadLogs(false));
  $('#logMoreFilters').addEventListener('click', () => {
    const box = $('#logAdvanced');
    box.hidden = !box.hidden;
    $('#logMoreFilters').textContent = t(box.hidden ? 'logs.moreFilters' : 'logs.fewerFilters');
  });
  $('#logLive').addEventListener('change', (e) => setLive(e.target.checked));
  FILTER_IDS.forEach((id) => {
    $('#' + id)?.addEventListener('keydown', (e) => { if (e.key === 'Enter') $('#logApply').click(); });
  });
  $('#logList').addEventListener('scroll', () => {
    const el = $('#logList');
    if (logLoading || !logHasMore) return;
    if (el.scrollHeight - el.scrollTop - el.clientHeight < 120) loadLogs(false);
  });
  setLive($('#logLive').checked);
}
