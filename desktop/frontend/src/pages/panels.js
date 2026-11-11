// 状态矩阵新增卡片与证书页 ACME 面板：全部走 RunCommand 转交 CLI，
// 各自管理刷新（进入页面或点「刷新」），不进定时轮询。
// 运行快照 = runtime status + runtime stats（§5.1/§5.2）；
// Monitor = monitor status + monitor events（§5.4/§5.5）；
// ACME 配置在「设置」页的表单里改（acme.* 分组），本模块不再涉及。
import { DesktopService, msgOf } from '../api.js';
import { t } from '../i18n.js';
import { $, esc } from '../ui/dom.js';
import { withLoading } from '../ui/loading.js';

let runtimeBusy = false;
let monitorBusy = false;

function kvHTML(rows) {
  return rows.map((r) => `<div class="kv-row"><span class="k">${esc(r.k)}</span><span class="v">${esc(r.v)}</span></div>`).join('');
}

function errHTML(e) {
  return `<div class="kv-row"><span class="k">—</span><span class="v warn-text">${esc(msgOf(e))}</span></div>`;
}

function durationText(seconds) {
  const s = Number(seconds) || 0;
  if (s <= 0) return '—';
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  return h > 0 ? t('runtime.durationHM', { h, m }) : t('runtime.durationM', { m });
}

// ---- 运行快照 ----

export async function refreshRuntimePanel() {
  const body = $('#runtimeBody');
  if (!body || runtimeBusy) return;
  runtimeBusy = true;
  body.innerHTML = `<div class="kv-row"><span class="k">—</span><span class="v">${esc(t('runtime.loading'))}</span></div>`;
  try {
    // §5.1：state/pid/uptime_seconds/connections{...}/requests{...}
    const res = await DesktopService.RunCommand(['runtime', 'status']);
    const d = res?.data || {};
    const c = d.connections || {};
    const q = d.requests || {};
    const rows = [
      { k: t('common.status'), v: d.state || '—' },
      { k: 'pid', v: d.pid != null ? String(d.pid) : '—' },
      { k: t('runtime.uptime'), v: durationText(d.uptime_seconds) },
      { k: t('runtime.connActive'), v: String(c.active ?? '—') },
      { k: t('runtime.connAccepted'), v: String(c.accepted_total ?? '—') },
      { k: t('runtime.connClosed'), v: String(c.closed_total ?? '—') },
      { k: t('runtime.connRejected'), v: String(c.rejected_total ?? '—') },
      { k: t('runtime.reqActive'), v: String(q.active ?? '—') },
      { k: t('runtime.reqTotal'), v: String(q.total ?? '—') }
    ];
    body.innerHTML = kvHTML(rows);
    // §5.2：responses（2xx…5xx）与 upstream 小结
    try {
      const st = await DesktopService.RunCommand(['runtime', 'stats']);
      const sd = st?.data || {};
      const r = sd.responses || {};
      const u = sd.upstream || {};
      body.insertAdjacentHTML('beforeend', kvHTML([
        {
          k: t('runtime.responses'),
          v: `2xx ${r['2xx_total'] ?? '—'} · 3xx ${r['3xx_total'] ?? '—'} · 4xx ${r['4xx_total'] ?? '—'} · 5xx ${r['5xx_total'] ?? '—'} · ` +
            t('runtime.other', { n: r.unknown_total ?? '—' })
        },
        {
          k: t('runtime.upstream'),
          v: t('runtime.dialFail', { n: u.dial_fail_total ?? '—' }) + ' · ' +
            t('runtime.sendTimeout', { n: u.send_timeout_total ?? '—' }) + ' · ' +
            t('runtime.readTimeout', { n: u.read_timeout_total ?? '—' }) + ' · ' +
            t('runtime.idleConns', { n: u.idle_conns ?? '—' })
        }
      ]));
    } catch (se) {
      body.insertAdjacentHTML('beforeend', errHTML(se));
    }
  } catch (e) {
    // 服务没跑时是 SERVICE_UNREACHABLE：显示 msgOf 的可读提示
    body.innerHTML = errHTML(e);
  } finally {
    runtimeBusy = false;
  }
}

// ---- Monitor ----

const MONITOR_STATUS = {
  not_installed: 'monitor.statusNotInstalled',
  running: 'monitor.statusRunning',
  stale: 'monitor.statusStale',
  unhealthy: 'monitor.statusUnhealthy',
  unknown: 'status.unknown'
};

export async function refreshMonitorPanel() {
  const body = $('#monitorBody');
  const list = $('#monitorEvents');
  if (!body || monitorBusy) return;
  monitorBusy = true;
  body.innerHTML = `<div class="kv-row"><span class="k">—</span><span class="v">${esc(t('monitor.loading'))}</span></div>`;
  try {
    // §5.4：status/not_installed 时提示未安装（仍是成功查询）
    const res = await DesktopService.RunCommand(['monitor', 'status']);
    const d = res?.data || {};
    const st = d.status || 'unknown';
    if (st === 'not_installed') {
      body.innerHTML = `<div class="kv-row"><span class="k">—</span><span class="v">${esc(t('monitor.notInstalledBody'))}</span></div>`;
      if (list) list.innerHTML = `<li class="empty">${esc(t('monitor.notInstalled'))}</li>`;
      return;
    }
    body.innerHTML = kvHTML([
      { k: t('common.status'), v: MONITOR_STATUS[st] ? t(MONITOR_STATUS[st]) : st },
      { k: t('monitor.version'), v: d.version || '—' },
      { k: 'pid', v: d.pid != null ? String(d.pid) : '—' },
      { k: t('monitor.healthy'), v: d.healthy == null ? '—' : (d.healthy ? t('monitor.ok') : t('monitor.bad')) },
      { k: t('monitor.lastCollect'), v: d.last_collect_at || '—' },
      { k: t('monitor.lastEvent'), v: d.last_event_at || '—' },
      ...(d.last_error ? [{ k: t('monitor.lastError'), v: d.last_error }] : [])
    ]);
  } catch (e) {
    body.innerHTML = errHTML(e);
  }
  try {
    // §5.5：data.events[{time,type,severity,message}]，新到旧
    const ev = await DesktopService.RunCommand(['monitor', 'events', '--limit', '8']);
    const events = ev?.data?.events || [];
    if (list) {
      list.innerHTML = events.length
        ? events.map((x) => {
            const cls = x.severity === 'info' ? 'info' : (x.severity === 'error' || x.severity === 'critical' ? 'err' : 'warn');
            return `<li class="${cls}"><span class="ev-dot"></span><div><b>${esc(x.message || x.type || '—')}</b>`
              + `<small>${esc(x.time || '')} · ${esc(x.type || '')} · ${esc(x.severity || '')}</small></div></li>`;
          }).join('')
        : `<li class="empty">${esc(t('events.empty'))}</li>`;
    }
  } catch (e) {
    if (list) list.innerHTML = `<li class="empty">${esc(t('monitor.eventsFailed', { msg: msgOf(e) }))}</li>`;
  } finally {
    monitorBusy = false;
  }
}

export function refreshMatrixPanels() {
  refreshRuntimePanel();
  refreshMonitorPanel();
}


// ACME 配置属于主配置的 acme 段，统一在「设置」页的表单里改
// （configops 的 configSpecs 已包含 acme.* 分组）。证书页只展示证书清单，
// 不再重复放一份 ACME 表单——两处都能改会让人不知道以哪份为准。
export function initPanels() {
  const rt = $('#runtimeRefresh');
  if (rt) rt.addEventListener('click', (e) => withLoading({ button: e.currentTarget, fn: refreshRuntimePanel }));
  const mon = $('#monitorRefresh');
  if (mon) mon.addEventListener('click', (e) => withLoading({ button: e.currentTarget, fn: refreshMonitorPanel }));
}
