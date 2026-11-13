// 概览页「最近事件」卡片：双 tab ——「操作记录」（operation list/get，
// 走 RunCommand 转交 CLI）与「本机动态」（pushEvent 由状态轮询合成的本地时间线）。
import { DesktopService, msgOf } from '../api.js';
import { t } from '../i18n.js';
import { $, esc } from './dom.js';
import { state } from '../state.js';

// ---- 本机动态：保留合成时间线 ----

export function pushEvent(text, kind) {
  const time = new Date().toLocaleTimeString('zh-CN', { hour12: false }).slice(0, 8);
  state.events.unshift({ time, text, kind: kind || 'info' });
  if (state.events.length > 5) state.events.pop();
  renderEvents();
}

export function renderEvents() {
  const el = $('#eventList');
  if (!el) return;
  el.innerHTML = state.events.length
    ? state.events.map((e) => `<li class="${e.kind}"><span class="ev-dot"></span><div><b>${esc(e.text)}</b><small>${esc(e.time)} · ${esc(t('events.systemMsg'))}</small></div></li>`).join('')
    : `<li class="empty">${esc(t('events.empty'))}</li>`;
}

export function pushOverviewEvents(ov) {
  if (state.firstOverview) {
    pushEvent(t('events.started'), 'info');
    state.firstOverview = false;
  }
  if (ov.error && !state.errorPushed) { pushEvent(t('events.configError'), 'warn'); state.errorPushed = true; }
  if (!ov.error && state.errorPushed) { pushEvent(t('events.configRecovered'), 'ok'); state.errorPushed = false; }
  if (state.lastRunning === null) {
    if (!ov.error) pushEvent(t('events.statusDone'), 'info');
  } else if (state.lastRunning !== ov.running) {
    pushEvent(ov.running ? t('events.serverUp') : t('events.serverDown'), ov.running ? 'ok' : 'warn');
  }
  state.lastRunning = ov.running;
}

// ---- 操作记录：operation list / operation get ----

const OP_STATUS = {
  succeeded: ['ok', 'ops.succeeded'],
  failed: ['err', 'ops.failed'],
  recovery_required: ['err', 'ops.recoveryRequired'],
  running: ['info', 'ops.running'],
  queued: ['off', 'ops.queued'],
  cancelled: ['off', 'ops.cancelled'],
  unknown: ['off', 'status.unknown']
};

let opsBusy = false;

export function renderOperations(ops) {
  const el = $('#opList');
  if (!el) return;
  if (!ops.length) {
    el.innerHTML = `<li class="empty">${esc(t('ops.empty'))}</li>`;
    return;
  }
  el.innerHTML = ops.map((op) => {
    const [cls, key] = OP_STATUS[op.status] || ['off', ''];
    const label = key ? t(key) : (op.status || '—');
    return `<li class="op-item st-${cls}" data-op-id="${esc(op.operation_id || '')}"><span class="ev-dot"></span><div><b>${esc(op.command || '—')}</b><small>${esc(op.created_at || '')}${op.operation_id ? ' · ' + esc(op.operation_id) : ''}</small></div><span class="op-badge st-${cls}">${esc(label)}</span></li>`;
  }).join('');
}

export async function refreshOperations() {
  const el = $('#opList');
  if (!el || opsBusy) return;
  opsBusy = true;
  try {
    // §8.1：data.operations 为 operation.Record[]（新到旧）
    const res = await DesktopService.RunCommand(['operation', 'list', '--limit', '8']);
    renderOperations(res?.data?.operations || []);
  } catch (e) {
    el.innerHTML = `<li class="empty">${esc(t('ops.loadFailed', { msg: msgOf(e) }))}</li>`;
  } finally {
    opsBusy = false;
  }
}

async function openOperationDetail(id) {
  const box = $('#opDetail');
  if (!box || !id) return;
  box.hidden = false;
  box.className = 'cmd-result';
  box.textContent = t('ops.reading', { id });
  try {
    // §8.2：data 直接是 operation.Record 对象
    const res = await DesktopService.RunCommand(['operation', 'get', id]);
    box.textContent = JSON.stringify(res?.data ?? {}, null, 2);
  } catch (e) {
    box.className = 'cmd-result is-error';
    box.textContent = t('ops.readFailed', { msg: msgOf(e) });
  }
}

export function initEventsCard() {
  const tabs = document.querySelectorAll('[data-event-tab]');
  if (!tabs.length) return;
  tabs.forEach((b) => b.addEventListener('click', () => {
    const key = b.getAttribute('data-event-tab');
    tabs.forEach((x) => x.classList.toggle('on', x === b));
    const ops = key === 'ops';
    $('#opList').hidden = !ops;
    $('#opDetail').hidden = true;
    $('#eventList').hidden = ops;
    if (ops) refreshOperations();
    else renderEvents();
  }));
  // 操作行是动态重建的，用事件委托打开详情
  $('#opList')?.addEventListener('click', (e) => {
    const li = e.target.closest ? e.target.closest('.op-item') : null;
    if (li) openOperationDetail(li.getAttribute('data-op-id'));
  });
}
