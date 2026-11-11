// 工作区切换：pane 显隐、导航高亮、按需加载页面，并刷新明细数据。
// 切换时给目标 pane 一个短淡入，避免画面硬切；同一 pane 重复点击也重播一次。
import { $$ } from './ui/dom.js';
import { state } from './state.js';
import { loadConfig } from './pages/config.js';
import { refreshDetails } from './refresh.js';
import { refreshRuntimePage } from './pages/runtime.js';
import { refreshOperationsPage } from './pages/operations.js';
import { refreshMatrixPanels } from './pages/panels.js';
import { refreshOperations } from './ui/events.js';

function softReveal(pane) {
  if (!pane) return;
  pane.classList.remove('soft-in');
  // 强制一次 reflow，让移除类生效后再加回来，动画才会重新播放
  void pane.offsetWidth;
  pane.classList.add('soft-in');
}

export function activate(key) {
  let target = null;
  $$('[data-pane]').forEach((p) => {
    const on = p.getAttribute('data-pane') === key;
    p.hidden = !on;
    if (on) target = p;
  });
  $$('[data-nav]').forEach((n) => {
    n.classList.toggle('on', n.getAttribute('data-nav') === key);
  });
  softReveal(target);
  // 切到某页时才拉它的数据：避免后台页面空转轮询
  if (key === 'settings' && !state.configLoaded) loadConfig();
  if (key === 'matrix') refreshMatrixPanels();
  if (key === 'runtime') refreshRuntimePage();
  if (key === 'operations') refreshOperationsPage();
  if (key === 'overview') refreshOperations();
  refreshDetails();
}
