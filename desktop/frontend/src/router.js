// 工作区切换：pane 显隐、导航高亮、按需加载页面，并刷新明细数据。
import { $$ } from './ui/dom.js';
import { state } from './state.js';
import { loadConfig } from './pages/config.js';
import { refreshDetails } from './refresh.js';
import { refreshRuntimePage } from './pages/runtime.js';
import { refreshOperationsPage } from './pages/operations.js';
import { refreshMatrixPanels, loadAcmeShow } from './pages/panels.js';
import { refreshOperations } from './ui/events.js';

export function activate(key) {
  $$('[data-pane]').forEach((p) => { p.hidden = p.getAttribute('data-pane') !== key; });
  $$('[data-nav]').forEach((n) => {
    n.classList.toggle('on', n.getAttribute('data-nav') === key);
  });
  // 切到某页时才拉它的数据：避免后台页面空转轮询
  if (key === 'settings' && !state.configLoaded) loadConfig();
  if (key === 'matrix') refreshMatrixPanels();
  if (key === 'certs') loadAcmeShow();
  if (key === 'runtime') refreshRuntimePage();
  if (key === 'operations') refreshOperationsPage();
  if (key === 'overview') refreshOperations();
  refreshDetails();
}
