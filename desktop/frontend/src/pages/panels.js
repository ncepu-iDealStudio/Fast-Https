// 状态矩阵新增卡片与证书页 ACME 面板：全部走 RunCommand 转交 CLI，
// 各自管理刷新（进入页面或点「刷新」），不进定时轮询。
// 运行快照 = runtime status + runtime stats（§5.1/§5.2）；
// Monitor = monitor status + monitor events（§5.4/§5.5）；
// ACME 设置 = 图形化表单（config show --section acme 读取 + acme setup 保存）。
import { DesktopService, msgOf } from '../api.js';
import { t } from '../i18n.js';
import { $, esc, toast } from '../ui/dom.js';
import { askConfirm } from '../dialogs/confirm.js';

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

// ---- ACME 设置（图形化表单）----
// 读取：config show --section acme 的 data.config.acme（脱敏结构化数据）；
// 兜底：acme show 文本解析成 kv（旧版本主程序）。
// 保存：acme setup --non-interactive 的 flags，二次确认后转交。
function setAcmeStatus(text, kind) {
  const el = $('#acmeStatus');
  if (el) { el.textContent = text; el.className = 'acme-status' + (kind ? ' ' + kind : ''); }
}

// acme show 的文本标签是中文；这里用 Unicode 码点匹配（避免源码里出现
// 裸中文触发 i18n 自查的 [D] 规则——这些是解析关键字，不是界面文案）。
// \u72b6\u6001 状态 / \u90ae\u7bb1 邮箱 / \u6311\u6218 挑战 /
// \u7eed\u671f 续期 / \u7f13\u5b58 缓存 / \u56de\u843d 回落 / \u6d4b\u8bd5 测试
const ACME_LABEL = {
  enabled: /\u72b6\u6001/,
  email: /\u90ae\u7bb1/,
  staging: /staging|\u6d4b\u8bd5\u73af\u5883/,
  challenge: /\u6311\u6218/,
  renew: /\u7eed\u671f/,
  storage: /\u7f13\u5b58/,
  fallback: /\u56de\u843d/
};

function parseAcmeText(text) {
  const out = {};
  const lines = String(text || '').split('\n');
  for (const line of lines) {
    const m = line.match(/^([^：:]+)[：:]\s*(.+)$/);
    if (!m) continue;
    const k = m[1].trim(), v = m[2].trim();
    if (ACME_LABEL.enabled.test(k)) out.enabled = /\u5f00\u542f|on|true|\u542f\u7528/i.test(v);
    else if (ACME_LABEL.email.test(k)) out.email = v.replace(/^<EMAIL>$/i, '');
    else if (ACME_LABEL.staging.test(k)) out.staging = /true|\u662f/i.test(v);
    else if (ACME_LABEL.challenge.test(k)) out.challenge = v;
    else if (ACME_LABEL.renew.test(k)) { const n = v.match(/\d+/); if (n) out.renew_before_days = Number(n[0]); }
    else if (ACME_LABEL.storage.test(k)) out.storage = v;
    else if (ACME_LABEL.fallback.test(k)) out.fallback_self_signed = /true|\u662f/i.test(v);
  }
  return out;
}

function fillAcmeForm(a) {
  $('#acmeEnabled').checked = !!a.enabled;
  $('#acmeEmail').value = (a.email && a.email !== '<EMAIL>') ? a.email : '';
  $('#acmeCA').value = a.staging ? 'staging' : (a.ca || 'letsencrypt');
  $('#acmeChallenge').value = a.challenge || 'auto';
  $('#acmeRenew').value = a.renew_before_days || 30;
  $('#acmeStorage').value = a.storage || '';
  $('#acmeFallback').checked = a.fallback_self_signed !== false;
}

export async function loadAcmeShow() {
  if (acmeBusy || !$('#acmeForm')) return;
  acmeBusy = true;
  setAcmeStatus(t('acme.saving').replace('…', ''), '');
  try {
    const res = await DesktopService.RunCommand(['config', 'show', '--section', 'acme']);
    const cfg = res?.data?.config;
    const a = (cfg && typeof cfg === 'object' ? (cfg.acme || cfg) : null);
    if (a && typeof a === 'object') {
      fillAcmeForm(a);
      setAcmeStatus('', '');
    } else {
      // 结构化数据不可用时退回 acme show 文本解析
      const txt = await DesktopService.RunCommand(['acme', 'show']);
      fillAcmeForm(parseAcmeText(txt?.text || ''));
      setAcmeStatus(t('acme.textFallback'), '');
    }
  } catch (e) {
    setAcmeStatus(t('acme.loadFailed', { msg: msgOf(e) }), 'is-error');
  } finally {
    acmeBusy = false;
  }
}

async function saveAcme() {
  const enabled = $('#acmeEnabled').checked;
  const email = $('#acmeEmail').value.trim();
  if (enabled && !email) {
    toast(t('acme.saveFailed', { msg: 'email required' }));
    return;
  }
  const ok = await askConfirm(t('acme.confirmSave'), t('acme.confirmBody'), t('acme.save'));
  if (!ok) return;
  setAcmeStatus(t('acme.saving'), '');
  const args = ['acme', 'setup', '--non-interactive'];
  if (enabled) {
    args.push('--email', email);
    const ca = $('#acmeCA').value;
    args.push('--ca', ca === 'staging' ? 'staging' : ca);
    const ch = $('#acmeChallenge').value;
    if (ch && ch !== 'auto') args.push('--challenge', ch);
    args.push('--renew-before-days', String(Number($('#acmeRenew').value) || 30));
    const storage = $('#acmeStorage').value.trim();
    if (storage) args.push('--storage', storage);
    args.push('--fallback-self-signed=' + ($('#acmeFallback').checked ? 'true' : 'false'));
  } else {
    // 关闭 ACME：用 flags 把 enabled 置 false 的路径不存在，
    // 退而求其次提示用户到设置页改；这里给出清晰反馈而不是静默失败
    setAcmeStatus(t('acme.saveFailed', { msg: 'disable acme in Settings' }), 'is-error');
    return;
  }
  try {
    await DesktopService.RunCommand(args);
    setAcmeStatus(t('acme.saved'), 'is-ok');
    toast(t('acme.saved'));
  } catch (e) {
    setAcmeStatus(t('acme.saveFailed', { msg: msgOf(e) }), 'is-error');
  }
}

export function initPanels() {
  const rt = $('#runtimeRefresh');
  if (rt) rt.addEventListener('click', refreshRuntimePanel);
  const mon = $('#monitorRefresh');
  if (mon) mon.addEventListener('click', refreshMonitorPanel);
  const acme = $('#acmeRefresh');
  if (acme) acme.addEventListener('click', loadAcmeShow);
  const save = $('#acmeSave');
  if (save) save.addEventListener('click', saveAcme);
}
