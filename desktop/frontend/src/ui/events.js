// 概览页右侧的「最近事件」时间线：由状态轮询驱动，只保留最近 5 条。
import { $, esc } from './dom.js';
import { state } from '../state.js';

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
    ? state.events.map((e) => `<li class="${e.kind}"><span class="ev-dot"></span><div><b>${esc(e.text)}</b><small>${esc(e.time)} · 系统消息</small></div></li>`).join('')
    : '<li class="empty">暂无事件</li>';
}

export function pushOverviewEvents(ov) {
  if (state.firstOverview) {
    pushEvent('管理端已启动，开始读取本机状态', 'info');
    state.firstOverview = false;
  }
  if (ov.error && !state.errorPushed) { pushEvent('读取配置文件失败', 'warn'); state.errorPushed = true; }
  if (!ov.error && state.errorPushed) { pushEvent('配置文件读取恢复', 'ok'); state.errorPushed = false; }
  if (state.lastRunning === null) {
    if (!ov.error) pushEvent('实时状态检测完成', 'info');
  } else if (state.lastRunning !== ov.running) {
    pushEvent(ov.running ? '服务已启动，端口开始响应' : '服务已停止', ov.running ? 'ok' : 'warn');
  }
  state.lastRunning = ov.running;
}
