// 日志页：JSON 结构化筛选 + 倒序滚动分页。分页状态是本模块私有的，
// 只有当前日志类型从共享状态读取。
import { DesktopService, msgOf } from '../api.js';
import { $, esc } from '../ui/dom.js';
import { state } from '../state.js';

const LOG_PAGE = 30;
let logRows = [];
let logOffset = 0;
let logHasMore = false;
let logLoading = false;
let logFilters = { level: '', status: '', search: '', since: '', until: '' };

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
    const extra = [e.host, e.status ? '状态 ' + e.status : '', e.size ? e.size + ' B' : ''].filter(Boolean).join(' · ');
    return `<div class="log-row" title="${esc(e.raw || '')}"><span class="log-time">${esc(time)}</span><span class="log-level lv-${esc(lvl)}">${esc(lvl)}</span><span class="log-msg">${esc(e.message || '—')}${extra ? `<small>${esc(extra)}</small>` : ''}</span></div>`;
  }).join('');
  el.scrollTop = keep;
}

export async function loadLogs(reset) {
  if (logLoading) return;
  if (reset) { logRows = []; logOffset = 0; logHasMore = false; }
  logLoading = true;
  const more = $('#logMore');
  more.disabled = true;
  $('#logStatusText').textContent = '正在查询…';
  try {
    const page = await DesktopService.QueryLogs({
      name: state.currentLog, offset: logOffset, limit: LOG_PAGE,
      level: logFilters.level, status: logFilters.status,
      search: logFilters.search, since: logFilters.since, until: logFilters.until
    });
    const lines = page.lines || [];
    logRows = logRows.concat(lines);
    logOffset += lines.length;
    logHasMore = !!page.hasMore && lines.length > 0;
    renderLogRows();
    $('#logStatusText').textContent = `已加载 ${logRows.length} 条${logHasMore ? ' · 还有更早的记录' : (logRows.length ? ' · 已到文件头' : '')}`;
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
    level: $('#logLevel').value,
    status: $('#logStatus').value,
    search: $('#logSearch').value.trim(),
    since: $('#logSince').value.trim(),
    until: $('#logUntil').value.trim()
  };
}

export function initLogPage() {
  $('#logApply').addEventListener('click', () => { collectLogFilters(); loadLogs(true); });
  $('#logReset').addEventListener('click', () => {
    ['logLevel', 'logStatus', 'logSearch', 'logSince', 'logUntil'].forEach((id) => { $('#' + id).value = ''; });
    collectLogFilters();
    loadLogs(true);
  });
  $('#logRefresh').addEventListener('click', () => loadLogs(true));
  $('#logMore').addEventListener('click', () => loadLogs(false));
  $('#logSearch').addEventListener('keydown', (e) => { if (e.key === 'Enter') $('#logApply').click(); });
  $('#logList').addEventListener('scroll', () => {
    const el = $('#logList');
    if (logLoading || !logHasMore) return;
    if (el.scrollHeight - el.scrollTop - el.clientHeight < 120) loadLogs(false);
  });
}
