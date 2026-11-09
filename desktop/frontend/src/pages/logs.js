// 日志页：logs query 的白名单筛选 + 游标滚动分页。
// QueryLogs({kind, limit, level, status, host, method, pathPrefix, reason,
// since, until, cursor})；until 专指筛选里的「止」，cursor 是分页游标：
// 「加载更早」把上一页的 page.nextUntil 传到 cursor（后端会剔掉与游标
// 同时间的重复行）。
import { DesktopService, msgOf } from '../api.js';
import { $, esc } from '../ui/dom.js';
import { state } from '../state.js';

const LOG_PAGE = 30;
let logRows = [];
let logHasMore = false;
let logLoading = false;
let logFilters = { level: '', status: '', host: '', method: '', pathPrefix: '', reason: '', since: '', until: '' };

function renderLogRows() {
  const el = $('#logList');
  const keep = el.scrollTop;
  if (!logRows.length) {
    el.innerHTML = '<div class="log-empty">没有匹配的日志</div>';
    return;
  }
  el.innerHTML = logRows.map((e) => {
    const m = String(e.time || '').match(/T(\d{2}:\d{2}:\d{2})/);
    const time = m ? m[1] : (e.time || '—');
    const lvl = e.level || '—';
    const size = String(e.size ?? '');
    const extra = [e.host, e.status ? '状态 ' + e.status : '', size ? (/^\d+$/.test(size) ? size + ' B' : size) : '']
      .filter(Boolean).join(' · ');
    return `<div class="log-row" title="${esc(e.raw || '')}"><span class="log-time">${esc(time)}</span><span class="log-level lv-${esc(lvl)}">${esc(lvl)}</span><span class="log-msg">${esc(e.message || '—')}${extra ? `<small>${esc(extra)}</small>` : ''}</span></div>`;
  }).join('');
  el.scrollTop = keep;
}

function setStatusText() {
  $('#logStatusText').textContent = logHasMore
    ? `已加载 ${logRows.length} 条 · 还有更早的记录`
    : (logRows.length ? '已到最早' : '没有匹配的日志');
}

export async function loadLogs(reset) {
  if (logLoading) return;
  if (reset) {
    // 切换 tab、改筛选、点刷新都重置：游标清空，从最新一页开始
    logRows = [];
    logHasMore = false;
    state.logCursor = '';
  }
  logLoading = true;
  const more = $('#logMore');
  more.disabled = true;
  $('#logStatusText').textContent = '正在查询…';
  try {
    const page = await DesktopService.QueryLogs({
      kind: state.currentLog,
      limit: LOG_PAGE,
      level: logFilters.level,
      status: logFilters.status,
      host: logFilters.host,
      method: logFilters.method,
      pathPrefix: logFilters.pathPrefix,
      reason: logFilters.reason,
      since: logFilters.since,
      // until 是筛选输入，原样传；分页游标单独走 cursor
      until: logFilters.until,
      cursor: state.logCursor
    });
    const lines = page.lines || [];
    logRows = logRows.concat(lines);
    state.logCursor = page.nextUntil || '';
    logHasMore = !!page.hasMore && lines.length > 0;
    renderLogRows();
    setStatusText();
    more.hidden = !logHasMore;
  } catch (e) {
    $('#logStatusText').textContent = '查询失败：' + msgOf(e);
  } finally {
    logLoading = false;
    more.disabled = false;
  }
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
    FILTER_IDS.forEach((id) => { $('#' + id).value = ''; });
    collectLogFilters();
    loadLogs(true);
  });
  $('#logRefresh').addEventListener('click', () => loadLogs(true));
  $('#logMore').addEventListener('click', () => loadLogs(false));
  FILTER_IDS.forEach((id) => {
    $('#' + id).addEventListener('keydown', (e) => { if (e.key === 'Enter') $('#logApply').click(); });
  });
  $('#logList').addEventListener('scroll', () => {
    const el = $('#logList');
    if (logLoading || !logHasMore) return;
    if (el.scrollHeight - el.scrollTop - el.clientHeight < 120) loadLogs(false);
  });
}
