// 概览页渲染：运行指标、主操作按钮、状态矩阵概况与告警条。
// 运行状态按 service status 的四态显示（ov.status），ov.running 仍是 bool。
import { $, bind } from '../ui/dom.js';
import { t } from '../i18n.js';
import { state } from '../state.js';
import { pushOverviewEvents } from '../ui/events.js';

// model.Overview.status 的四态文案（service status 投影）
const STATUS_TEXT = {
  running: 'status.running',
  degraded: 'status.degraded',
  stopped: 'status.stopped',
  unknown: 'status.unknown'
};

const STATUS_SUB = {
  running: 'status.subRunning',
  degraded: 'status.subDegraded',
  stopped: 'status.subStopped',
  unknown: 'status.unknown'
};

const STATUS_CLASS = {
  running: 'state-ok',
  degraded: 'state-warn',
  stopped: 'state-off',
  unknown: ''
};

function statusKey(ov) {
  if (ov.status && STATUS_TEXT[ov.status]) return ov.status;
  return ov.running ? 'running' : 'stopped';
}

// 「运行 N 分钟」：不足 1 分钟按 1 分钟显示，不显示 0 分钟
function uptimeText(seconds) {
  const s = Number(seconds) || 0;
  if (s <= 0) return '';
  const m = Math.floor(s / 60);
  return m > 0 ? t('status.uptimeMinutes', { m }) : t('status.uptimeLess');
}

export function renderPrimary(ov) {
  const btn = $('#primaryAction');
  if (btn) {
    const glyph = btn.querySelector('.glyph');
    if (ov.running) {
      btn.setAttribute('data-action', 'stop');
      $('#primaryLabel').textContent = t('overview.stop');
      if (glyph) glyph.textContent = '■';
    } else {
      btn.setAttribute('data-action', 'start');
      $('#primaryLabel').textContent = t('overview.start');
      if (glyph) glyph.textContent = '▶';
    }
  }
  // 底部状态栏只放图标：随运行状态切换图形与提示
  const foot = $('#footStart');
  if (foot) {
    if (ov.running) {
      foot.setAttribute('data-action', 'stop');
      foot.textContent = '■';
      foot.title = t('overview.stop');
    } else {
      foot.setAttribute('data-action', 'start');
      foot.textContent = '▶';
      foot.title = t('overview.start');
    }
  }
}

export function renderOverview(ov) {
  state.lastOverview = ov;
  const key = statusKey(ov);
  const runningText = t(STATUS_TEXT[key]);
  const uptime = uptimeText(ov.uptimeSeconds);
  bind('running', runningText);
  bind('pid', 'pid ' + (ov.pid || '—'));
  bind('pidNum', ov.pid || '—');
  bind('uptime', t('status.refreshing'));
  bind('statusText', t(STATUS_SUB[key]) + (uptime ? ' · ' + uptime : ''));
  bind('ports', (ov.ports || []).join('　') || '—');
  bind('portCount', t('status.portsCount', { n: (ov.ports || []).length }));
  bind('siteCount', ov.siteCount || 0);
  bind('siteText', t('status.siteText', { sites: ov.siteCount || 0, tls: ov.tlsCount || 0 }));
  bind('certText', ov.nearestCertDays > 0 ? t('status.certNearest', { days: ov.nearestCertDays }) : t('status.certNone'));
  bind('checkedAt', t('status.checkedAt', { time: ov.checkedAt || '—' }));

  const metricRunning = $('#metricRunning');
  metricRunning.textContent = runningText;
  metricRunning.className = 'm-value ' + (STATUS_CLASS[key] || '');
  $('#metricPorts').textContent = (ov.ports || []).length;
  $('#metricSites').textContent = ov.siteCount || 0;

  const cfg = $('#configState');
  const cfgText = ov.configDirty ? t('status.configDirty') : t('status.configReloaded');
  cfg.textContent = cfgText + (ov.configVersion ? t('common.versionParen', { v: ov.configVersion }) : '');
  cfg.className = ov.configDirty ? 'warn-text' : 'ok-text';
  $('#runDot').classList.toggle('on', !!ov.running);
  const matrixLamp = $('#matrixLamp');
  if (matrixLamp) {
    matrixLamp.className = 'ms-lamp ' + (key === 'running' ? 'on' : (key === 'stopped' ? 'off' : ''));
    $('#matrixState').textContent = runningText;
    $('#matrixSub').textContent = t(STATUS_SUB[key]) + (uptime ? ' · ' + uptime : '') + ' · ' + t('status.refreshEvery');
  }
  renderPrimary(ov);

  const alert = $('#overviewError');
  if (ov.error) {
    alert.hidden = false;
    alert.className = 'alert-strip';
    $('#alertTitle').textContent = t('overview.errorTitle');
    $('#alertBody').textContent = t('overview.alertReadErrorBody', { error: ov.error });
  } else if (ov.configDirty) {
    alert.hidden = false;
    alert.className = 'alert-strip is-warn';
    $('#alertTitle').textContent = t('overview.alertDirtyTitle');
    $('#alertBody').textContent = t('overview.alertDirtyBody');
  } else {
    alert.hidden = true;
  }

  pushOverviewEvents(ov);
}
