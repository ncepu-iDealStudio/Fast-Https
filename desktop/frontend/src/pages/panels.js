// 状态矩阵新增卡片与证书页 ACME 面板：全部走 RunCommand 转交 CLI，
// 各自管理刷新（进入页面或点「刷新」），不进定时轮询。
// 运行快照 = runtime status + runtime stats（§5.1/§5.2）；
// Monitor = monitor status + monitor events（§5.4/§5.5）；
// ACME 设置 = acme show（纯文本输出，只有 res.text，没有 data）。
import { DesktopService, msgOf } from '../api.js';
import { $, esc } from '../ui/dom.js';

let runtimeBusy = false;
let monitorBusy = false;
let acmeBusy = false;

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
  return h > 0 ? `${h} 小时 ${m} 分钟` : `${m} 分钟`;
}

// ---- 运行快照 ----

export async function refreshRuntimePanel() {
  const body = $('#runtimeBody');
  if (!body || runtimeBusy) return;
  runtimeBusy = true;
  body.innerHTML = '<div class="kv-row"><span class="k">—</span><span class="v">正在读取运行快照…</span></div>';
  try {
    // §5.1：state/pid/uptime_seconds/connections{...}/requests{...}
    const res = await DesktopService.RunCommand(['runtime', 'status']);
    const d = res?.data || {};
    const c = d.connections || {};
    const q = d.requests || {};
    const rows = [
      { k: '状态', v: d.state || '—' },
      { k: 'pid', v: d.pid != null ? String(d.pid) : '—' },
      { k: '运行时长', v: durationText(d.uptime_seconds) },
      { k: '活跃连接', v: String(c.active ?? '—') },
      { k: '累计接受', v: String(c.accepted_total ?? '—') },
      { k: '累计关闭', v: String(c.closed_total ?? '—') },
      { k: '累计拒绝', v: String(c.rejected_total ?? '—') },
      { k: '活跃请求', v: String(q.active ?? '—') },
      { k: '累计请求', v: String(q.total ?? '—') }
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
          k: '响应分布',
          v: `2xx ${r['2xx_total'] ?? '—'} · 3xx ${r['3xx_total'] ?? '—'} · 4xx ${r['4xx_total'] ?? '—'} · 5xx ${r['5xx_total'] ?? '—'} · 其它 ${r.unknown_total ?? '—'}`
        },
        {
          k: '上游',
          v: `拨号失败 ${u.dial_fail_total ?? '—'} · 发送超时 ${u.send_timeout_total ?? '—'} · 读取超时 ${u.read_timeout_total ?? '—'} · 空闲连接 ${u.idle_conns ?? '—'}`
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
  not_installed: '未安装',
  running: '运行中',
  stale: '采集停滞',
  unhealthy: '异常',
  unknown: '状态未知'
};

export async function refreshMonitorPanel() {
  const body = $('#monitorBody');
  const list = $('#monitorEvents');
  if (!body || monitorBusy) return;
  monitorBusy = true;
  body.innerHTML = '<div class="kv-row"><span class="k">—</span><span class="v">正在读取 Monitor 状态…</span></div>';
  try {
    // §5.4：status/not_installed 时提示未安装（仍是成功查询）
    const res = await DesktopService.RunCommand(['monitor', 'status']);
    const d = res?.data || {};
    const st = d.status || 'unknown';
    if (st === 'not_installed') {
      body.innerHTML = '<div class="kv-row"><span class="k">—</span><span class="v">Monitor 未安装：没有采集到本机动态。</span></div>';
      if (list) list.innerHTML = '<li class="empty">Monitor 未安装</li>';
      return;
    }
    body.innerHTML = kvHTML([
      { k: '状态', v: MONITOR_STATUS[st] || st },
      { k: '版本', v: d.version || '—' },
      { k: 'pid', v: d.pid != null ? String(d.pid) : '—' },
      { k: '健康', v: d.healthy == null ? '—' : (d.healthy ? '正常' : '异常') },
      { k: '最近采集', v: d.last_collect_at || '—' },
      { k: '最近事件', v: d.last_event_at || '—' },
      ...(d.last_error ? [{ k: '最近错误', v: d.last_error }] : [])
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
        : '<li class="empty">暂无事件</li>';
    }
  } catch (e) {
    if (list) list.innerHTML = `<li class="empty">读取事件失败：${esc(msgOf(e))}</li>`;
  } finally {
    monitorBusy = false;
  }
}

export function refreshMatrixPanels() {
  refreshRuntimePanel();
  refreshMonitorPanel();
}

// ---- ACME 设置（acme show 纯文本） ----

export async function loadAcmeShow() {
  const box = $('#acmeShow');
  if (!box || acmeBusy) return;
  acmeBusy = true;
  box.className = 'cmd-result';
  box.textContent = '正在读取 ACME 设置…';
  try {
    const res = await DesktopService.RunCommand(['acme', 'show']);
    // 此命令只有文本输出，没有 data
    box.textContent = res.text || JSON.stringify(res.data ?? {}, null, 2);
  } catch (e) {
    box.className = 'cmd-result is-error';
    box.textContent = '读取失败：' + msgOf(e);
  } finally {
    acmeBusy = false;
  }
}

export function initPanels() {
  const rt = $('#runtimeRefresh');
  if (rt) rt.addEventListener('click', refreshRuntimePanel);
  const mon = $('#monitorRefresh');
  if (mon) mon.addEventListener('click', refreshMonitorPanel);
  const acme = $('#acmeRefresh');
  if (acme) acme.addEventListener('click', loadAcmeShow);
}
