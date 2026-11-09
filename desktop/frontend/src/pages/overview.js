// 概览页渲染：运行指标、主操作按钮、状态矩阵概况与告警条。
// 运行状态按 service status 的四态显示（ov.status），ov.running 仍是 bool。
import { $, bind } from '../ui/dom.js';
import { state } from '../state.js';
import { pushOverviewEvents } from '../ui/events.js';

// model.Overview.status 的四态文案（service status 投影）
const STATUS_TEXT = {
  running: '运行中',
  degraded: '运行中（端口不可达）',
  stopped: '已停止',
  unknown: '状态未知'
};

const STATUS_SUB = {
  running: '回环端口可连接',
  degraded: '进程在，端口不可达',
  stopped: '未检测到监听端口',
  unknown: '状态未知'
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
  return m > 0 ? `运行 ${m} 分钟` : '运行不足 1 分钟';
}

export function renderPrimary(ov) {
  const btn = $('#primaryAction');
  if (btn) {
    const glyph = btn.querySelector('.glyph');
    if (ov.running) {
      btn.setAttribute('data-action', 'stop');
      $('#primaryLabel').textContent = '停止服务';
      if (glyph) glyph.textContent = '■';
    } else {
      btn.setAttribute('data-action', 'start');
      $('#primaryLabel').textContent = '启动服务';
      if (glyph) glyph.textContent = '▶';
    }
  }
  // 底部状态栏只放图标：随运行状态切换图形与提示
  const foot = $('#footStart');
  if (foot) {
    if (ov.running) {
      foot.setAttribute('data-action', 'stop');
      foot.textContent = '■';
      foot.title = '停止服务';
    } else {
      foot.setAttribute('data-action', 'start');
      foot.textContent = '▶';
      foot.title = '启动服务';
    }
  }
}

export function renderOverview(ov) {
  state.lastOverview = ov;
  const key = statusKey(ov);
  const runningText = STATUS_TEXT[key];
  const uptime = uptimeText(ov.uptimeSeconds);
  bind('running', runningText);
  bind('pid', 'pid ' + (ov.pid || '—'));
  bind('pidNum', ov.pid || '—');
  bind('uptime', '状态每 2 秒刷新');
  bind('statusText', STATUS_SUB[key] + (uptime ? ' · ' + uptime : ''));
  bind('ports', (ov.ports || []).join('　') || '—');
  bind('portCount', (ov.ports || []).length + ' 个端口');
  bind('siteCount', ov.siteCount || 0);
  bind('siteText', `${ov.siteCount || 0} 个，其中 ${ov.tlsCount || 0} 个启用 TLS`);
  bind('certText', ov.nearestCertDays > 0 ? `最近一张 ${ov.nearestCertDays} 天后到期` : '暂无可读取的证书');
  bind('checkedAt', '最近刷新 ' + (ov.checkedAt || '—'));

  const metricRunning = $('#metricRunning');
  metricRunning.textContent = runningText;
  metricRunning.className = 'm-value ' + (STATUS_CLASS[key] || '');
  $('#metricPorts').textContent = (ov.ports || []).length;
  $('#metricSites').textContent = ov.siteCount || 0;

  const cfg = $('#configState');
  const cfgText = ov.configDirty ? '已修改，尚未重载' : '已重载';
  cfg.textContent = cfgText + (ov.configVersion ? '（' + ov.configVersion + '）' : '');
  cfg.className = ov.configDirty ? 'warn-text' : 'ok-text';
  $('#runDot').classList.toggle('on', !!ov.running);
  const matrixLamp = $('#matrixLamp');
  if (matrixLamp) {
    matrixLamp.className = 'ms-lamp ' + (key === 'running' ? 'on' : (key === 'stopped' ? 'off' : ''));
    $('#matrixState').textContent = runningText;
    $('#matrixSub').textContent = STATUS_SUB[key] + (uptime ? ' · ' + uptime : '') + ' · 每 2 秒刷新';
  }
  renderPrimary(ov);

  const alert = $('#overviewError');
  if (ov.error) {
    alert.hidden = false;
    alert.className = 'alert-strip';
    $('#alertTitle').textContent = '配置文件读取失败';
    $('#alertBody').textContent = ov.error + '。请确认桌面端与 fast-https 主程序放在同一目录。';
  } else if (ov.configDirty) {
    alert.hidden = false;
    alert.className = 'alert-strip is-warn';
    $('#alertTitle').textContent = '配置已修改，尚未重载';
    $('#alertBody').textContent = '手工改过配置文件时才需要点「重载配置」；界面里的保存与站点操作都走配置事务，会自动 reload。';
  } else {
    alert.hidden = true;
  }

  pushOverviewEvents(ov);
}
