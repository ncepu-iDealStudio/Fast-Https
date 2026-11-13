// 运行快照专属页面：runtime status + runtime stats（RunCommand 转交 CLI）。
// 服务没启动时 runtime status 会失败（SERVICE_UNREACHABLE），这里给出人话
// 提示而不是英文 Go 错误。自动刷新 2 秒一次，页面隐藏时暂停。
import { DesktopService, msgOf } from '../api.js';
import { t } from '../i18n.js';
import { $, esc } from '../ui/dom.js';
import { withLoading } from '../ui/loading.js';

let busy = false;
let timer = null;
let ok = false;

function kv(label, value) {
  return `<div class="rt-kv"><span class="rt-k">${esc(label)}</span><span class="rt-v">${esc(value == null || value === '' ? '—' : String(value))}</span></div>`;
}

function card(title, rows) {
  return `<div class="rt-card"><h4>${esc(title)}</h4>${rows.join('')}</div>`;
}

function durationText(seconds) {
  const s = Number(seconds) || 0;
  if (s <= 0) return '—';
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  return h > 0 ? t('runtime.durationHM', { h, m }) : t('runtime.durationM', { m });
}

function latencyHTML(rd) {
  if (!rd || !Array.isArray(rd.buckets)) return '';
  const rows = [kv(t('runtime.latencyUnit', { u: rd.unit || 'ms', n: rd.count ?? '—' }), '')];
  for (const b of rd.buckets) {
    rows.push(kv(t('runtime.latencyBucket', { le: b.le, n: b.count }), ''));
  }
  return card(t('runtime.cardLatency'), rows);
}

function render(d, sd) {
  const c = d.connections || {};
  const q = d.requests || {};
  const r = sd.responses || {};
  const u = sd.upstream || {};
  const html = [
    card(t('runtime.cardState'), [
      kv(t('runtime.state'), d.state || '—'),
      kv('pid', d.pid != null ? d.pid : '—'),
      kv(t('runtime.uptime'), durationText(d.uptime_seconds))
    ]),
    card(t('runtime.cardConn'), [
      kv(t('runtime.connActive'), c.active ?? '—'),
      kv(t('runtime.connAccepted'), c.accepted_total ?? '—'),
      kv(t('runtime.connClosed'), c.closed_total ?? '—'),
      kv(t('runtime.connRejected'), c.rejected_total ?? '—')
    ]),
    card(t('runtime.cardReq'), [
      kv(t('runtime.reqActive'), q.active ?? '—'),
      kv(t('runtime.reqTotal'), q.total ?? '—'),
      kv(t('runtime.reqParseErrors'), sd.requests?.parse_errors_total ?? '—'),
      kv(t('runtime.reqAborted'), sd.requests?.aborted_total ?? '—')
    ]),
    card(t('runtime.cardResp'), [
      kv('2xx', r['2xx_total'] ?? '—'),
      kv('3xx', r['3xx_total'] ?? '—'),
      kv('4xx', r['4xx_total'] ?? '—'),
      kv('5xx', r['5xx_total'] ?? '—'),
      kv(t('runtime.unknownLabel'), r.unknown_total ?? '—')
    ]),
    card(t('runtime.cardUpstream'), [
      kv(t('runtime.dialFailLabel'), u.dial_fail_total ?? '—'),
      kv(t('runtime.sendTimeoutLabel'), u.send_timeout_total ?? '—'),
      kv(t('runtime.readTimeoutLabel'), u.read_timeout_total ?? '—'),
      kv(t('runtime.idleConnsLabel'), u.idle_conns ?? '—')
    ]),
    latencyHTML(sd.request_duration)
  ].filter(Boolean).join('');
  $('#rtPageBody').innerHTML = html || `<div class="runtime-empty">${esc(t('runtime.empty'))}</div>`;
  $('#rtPageFoot').textContent = d.observed_at ? t('runtime.observedAt', { time: d.observed_at }) : '';
}

function unavailable(e) {
  const raw = msgOf(e);
  // 后端 FriendlyError 已翻译成双语；这里再兜底一次（旧版本主程序）
  const hint = /SERVICE_UNREACHABLE|runtime snapshot unreachable|9088|connection refused|actively refused|dial tcp/i.test(raw)
    ? t('runtime.unavailable')
    : t('friendly.runtimeUnavailable');
  $('#rtPageBody').innerHTML = `<div class="runtime-empty is-warn">${esc(hint)}<small>${esc(raw)}</small></div>`;
  $('#rtPageFoot').textContent = '';
}

export async function refreshRuntimePage() {
  if (busy) return;
  busy = true;
  try {
    const res = await DesktopService.RunCommand(['runtime', 'status']);
    const d = res?.data || {};
    let sd = {};
    try {
      const st = await DesktopService.RunCommand(['runtime', 'stats']);
      sd = st?.data || {};
    } catch (e) { /* stats 失败不挡住 status 的展示 */ }
    render(d, sd);
    ok = true;
  } catch (e) {
    ok = false;
    unavailable(e);
  } finally {
    busy = false;
  }
}

function setLive(on) {
  if (timer) { clearInterval(timer); timer = null; }
  if (on) timer = setInterval(() => {
    if (document.hidden) return;
    const pane = document.querySelector('[data-pane="runtime"]');
    if (pane && !pane.hidden) refreshRuntimePage();
  }, 2000);
}

export function initRuntimePage() {
  const btn = $('#rtPageRefresh');
  if (btn) btn.addEventListener('click', (e) => withLoading({ button: e.currentTarget, fn: refreshRuntimePage }));
  const live = $('#runtimeLive');
  live?.addEventListener('change', (e) => setLive(e.target.checked));
  setLive(!!live?.checked);
}
