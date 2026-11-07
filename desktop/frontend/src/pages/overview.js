// 概览页渲染：运行指标、主操作按钮、状态矩阵概况与告警条。
import { $, bind } from '../ui/dom.js';
import { state } from '../state.js';
import { pushOverviewEvents } from '../ui/events.js';

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
  const runningText = ov.running ? '运行中' : '已停止';
  bind('running', runningText);
  bind('pid', 'pid ' + (ov.pid || '—'));
  bind('pidNum', ov.pid || '—');
  bind('uptime', '状态每 2 秒刷新');
  bind('statusText', ov.running ? '回环端口可连接' : '未检测到监听端口');
  bind('ports', (ov.ports || []).join('　') || '—');
  bind('portCount', (ov.ports || []).length + ' 个端口');
  bind('siteCount', ov.siteCount || 0);
  bind('siteText', `${ov.siteCount || 0} 个，其中 ${ov.tlsCount || 0} 个启用 TLS`);
  bind('certText', ov.nearestCertDays > 0 ? `最近一张 ${ov.nearestCertDays} 天后到期` : '暂无可读取的证书');
  bind('checkedAt', '最近刷新 ' + (ov.checkedAt || '—'));

  const metricRunning = $('#metricRunning');
  metricRunning.textContent = runningText;
  metricRunning.className = 'm-value ' + (ov.running ? 'state-ok' : 'state-off');
  $('#metricPorts').textContent = (ov.ports || []).length;
  $('#metricSites').textContent = ov.siteCount || 0;

  const cfg = $('#configState');
  cfg.textContent = ov.configDirty ? '已修改，尚未重载' : '已重载';
  cfg.className = ov.configDirty ? 'warn-text' : 'ok-text';
  $('#runDot').classList.toggle('on', !!ov.running);
  const matrixLamp = $('#matrixLamp');
  if (matrixLamp) {
    matrixLamp.className = 'ms-lamp ' + (ov.running ? 'on' : 'off');
    $('#matrixState').textContent = runningText;
    $('#matrixSub').textContent = ov.running ? '回环端口可连接 · 每 2 秒刷新' : '未检测到监听端口';
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
    $('#alertBody').textContent = '改动 conf.d 后点「重载配置」才生效，重载不会中断已有连接。';
  } else {
    alert.hidden = true;
  }

  pushOverviewEvents(ov);
}
