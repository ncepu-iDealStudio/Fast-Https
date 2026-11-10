// 操作记录专属页面：operation list（表格） + operation get（点行展开详情）。
// 概览页的「操作记录」小时间线保留不变（ui/events.js）。
import { DesktopService, msgOf } from '../api.js';
import { t } from '../i18n.js';
import { $, esc } from '../ui/dom.js';

let busy = false;
let ops = [];
let openId = '';

const STATUS = {
  succeeded: ['ok', 'ops.succeeded'],
  failed: ['err', 'ops.failed'],
  recovery_required: ['err', 'ops.recoveryRequired'],
  running: ['info', 'ops.running'],
  queued: ['off', 'ops.queued'],
  cancelled: ['off', 'ops.cancelled'],
  unknown: ['off', 'status.unknown']
};

function resultOf(op) {
  const r = op.result || {};
  return { version: r.version || op.version || '', restore: r.restore_point || op.restore_point || '' };
}

function rowHTML(op) {
  const [cls, key] = STATUS[op.status] || ['off', ''];
  const label = key ? t(key) : (op.status || '—');
  const r = resultOf(op);
  const on = openId === (op.operation_id || '');
  return `<div class="ops-row ${on ? 'is-open' : ''}" data-op-id="${esc(op.operation_id || '')}">
    <span title="${esc(op.created_at || '')}">${esc((op.created_at || '—').replace('T', ' ').slice(0, 19))}</span>
    <span class="mono">${esc(op.command || '—')}</span>
    <span class="op-badge st-${cls}">${esc(label)}</span>
    <span class="mono" title="${esc(op.operation_id || '')}">${esc(op.operation_id || '—')}</span>
    <span>${esc(r.version || '—')}</span>
    <span>${esc(r.restore || '—')}</span>
  </div>`;
}

function render() {
  const filter = $('#opsStatusFilter')?.value || '';
  const list = filter ? ops.filter((o) => o.status === filter) : ops;
  const el = $('#opsList');
  if (!list.length) {
    el.innerHTML = `<div class="runtime-empty">${esc(t('ops.empty'))}</div>`;
    return;
  }
  el.innerHTML = list.map(rowHTML).join('');
}

export async function refreshOperationsPage() {
  if (busy) return;
  busy = true;
  try {
    const res = await DesktopService.RunCommand(['operation', 'list', '--limit', '50']);
    ops = res?.data?.operations || [];
    render();
  } catch (e) {
    $('#opsList').innerHTML = `<div class="runtime-empty is-warn">${esc(t('ops.loadFailed', { msg: msgOf(e) }))}</div>`;
  } finally {
    busy = false;
  }
}

async function openDetail(id) {
  const box = $('#opsDetail');
  if (!box || !id) return;
  openId = openId === id ? '' : id;
  render();
  if (!openId) { box.hidden = true; return; }
  box.hidden = false;
  box.innerHTML = `<div class="ops-detail-head"><b>${esc(id)}</b><span data-i18n="ops.detail">${esc(t('ops.detail'))}</span></div><pre class="cmd-result">${esc(t('ops.reading', { id }))}</pre>`;
  try {
    const res = await DesktopService.RunCommand(['operation', 'get', id]);
    const d = res?.data || {};
    const r = d.result || {};
    const summary = `<div class="ops-detail-summary">`
      + `<span>${esc(t('ops.colStatus'))}：${esc(d.status || '—')}</span>`
      + `<span>${esc(t('ops.result'))}：${esc(r.config_state || '—')}</span>`
      + `<span>${esc(t('ops.colVersion'))}：${esc(r.version || d.version || '—')}</span>`
      + `<span>${esc(t('ops.colRestore'))}：${esc(r.restore_point || '—')}</span>`
      + (r.rolled_back ? `<span class="warn-text">${esc(t('ops.rolledBack'))}</span>` : '')
      + (d.error ? `<span class="warn-text">${esc(t('ops.error'))}：${esc(typeof d.error === 'object' ? (d.error.message || JSON.stringify(d.error)) : String(d.error))}</span>` : '')
      + `</div>`;
    box.innerHTML = `<div class="ops-detail-head"><b>${esc(id)}</b><span>${esc(t('ops.detail'))}</span></div>${summary}`
      + `<details open><summary>${esc(t('common.expand'))} JSON</summary><pre class="cmd-result">${esc(JSON.stringify(d, null, 2))}</pre></details>`;
  } catch (e) {
    box.innerHTML = `<div class="ops-detail-head"><b>${esc(id)}</b></div><pre class="cmd-result is-error">${esc(t('ops.readFailed', { msg: msgOf(e) }))}</pre>`;
  }
}

export function initOperationsPage() {
  $('#opsRefresh')?.addEventListener('click', refreshOperationsPage);
  $('#opsStatusFilter')?.addEventListener('change', render);
  $('#opsList')?.addEventListener('click', (e) => {
    const row = e.target.closest ? e.target.closest('.ops-row') : null;
    if (row) openDetail(row.getAttribute('data-op-id') || '');
  });
}
