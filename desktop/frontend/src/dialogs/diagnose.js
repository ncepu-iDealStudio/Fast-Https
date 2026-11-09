// 健康检查弹窗：diagnose 命令（§5.3）的结果展示。
// overall 大字 + 固定 9 项检查（name/status/message）+ report_id。
import { DesktopService, msgOf } from '../api.js';
import { $, esc } from '../ui/dom.js';

export const template = `<div class="dialog-mask" id="diagnoseDialog" hidden>
  <div class="dialog dialog-wide" role="dialog" aria-modal="true" aria-labelledby="diagnoseTitle">
    <h3 id="diagnoseTitle">健康检查</h3>
    <div class="diag-overall"><span class="diag-label">总体</span><strong id="diagOverall">—</strong><span class="muted mono" id="diagReportId"></span></div>
    <div class="kv-table" id="diagChecks"><div class="kv-row"><span class="k">—</span><span class="v">尚未检查</span></div></div>
    <p class="dialog-error" id="diagError" hidden></p>
    <div class="dialog-actions">
      <button class="btn" id="diagRefresh">重新检查</button>
      <button class="btn primary" id="diagClose">关闭</button>
    </div>
  </div>
</div>`;

const CHECK_NAMES = {
  service: '服务',
  config: '配置',
  config_drift: '配置漂移',
  certs: '证书',
  ports: '端口',
  logs: '日志',
  runtime: '运行快照',
  monitor: '监控',
  state: '管理状态'
};

const CHECK_STATUS = {
  ok: ['ok', '通过'],
  warning: ['warn', '告警'],
  error: ['err', '错误'],
  unavailable: ['off', '不可用']
};

const OVERALL_TEXT = {
  ok: ['ok', '一切正常'],
  warning: ['warn', '存在告警'],
  error: ['err', '存在错误']
};

let busy = false;

function renderReport(d) {
  const [cls, label] = OVERALL_TEXT[d.overall] || ['off', d.overall || '—'];
  const overall = $('#diagOverall');
  overall.textContent = label;
  overall.className = 'diag-overall-text st-' + cls;
  $('#diagReportId').textContent = d.report_id ? 'report_id ' + d.report_id : '';
  $('#diagChecks').innerHTML = (d.checks || []).map((c) => {
    const [stCls, stLabel] = CHECK_STATUS[c.status] || ['off', c.status || '—'];
    return `<div class="kv-row"><span class="k">${esc(CHECK_NAMES[c.name] || c.name)}</span>`
      + `<span class="v">${esc(c.message || '—')}</span>`
      + `<span class="n"><span class="st-badge st-${stCls}" title="${esc(c.status || '')}">${esc(stLabel)}</span>`
      + `<small class="muted">${esc(c.name || '')}</small></span></div>`;
  }).join('');
}

async function runDiagnose() {
  if (busy) return;
  busy = true;
  const btn = $('#diagnoseBtn');
  const refresh = $('#diagRefresh');
  if (btn) btn.disabled = true;
  if (refresh) refresh.disabled = true;
  $('#diagError').hidden = true;
  $('#diagChecks').innerHTML = '<div class="kv-row"><span class="k">—</span><span class="v">正在检查…</span></div>';
  try {
    // §5.3：诊断本身是成功的查询（退出码 0），data.overall/checks/report_id
    const res = await DesktopService.RunCommand(['diagnose']);
    renderReport(res?.data || {});
  } catch (e) {
    $('#diagChecks').innerHTML = '<div class="kv-row"><span class="k">—</span><span class="v">检查失败</span></div>';
    const err = $('#diagError');
    err.textContent = msgOf(e);
    err.hidden = false;
  } finally {
    busy = false;
    if (btn) btn.disabled = false;
    if (refresh) refresh.disabled = false;
  }
}

export function initDiagnoseDialog() {
  const mask = $('#diagnoseDialog');
  if (!mask) return;
  const close = () => { mask.hidden = true; };
  const open = () => {
    mask.hidden = false;
    $('#diagOverall').textContent = '—';
    $('#diagOverall').className = 'diag-overall-text';
    $('#diagReportId').textContent = '';
    runDiagnose();
  };
  const btn = $('#diagnoseBtn');
  if (btn) btn.addEventListener('click', open);
  $('#diagRefresh').addEventListener('click', runDiagnose);
  $('#diagClose').addEventListener('click', close);
  mask.addEventListener('click', (e) => { if (e.target === mask) close(); });
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape' && !mask.hidden) { e.preventDefault(); close(); }
  });
}
