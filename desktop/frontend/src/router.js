// 工作区切换：pane 显隐、导航高亮、按需加载设置页与各卡片数据，
// 并刷新明细数据。新增卡片（运行快照/Monitor/ACME/配置版本）的刷新
// 各自管理，只在进入页面时拉取，不进定时轮询。
import { $$ } from './ui/dom.js';
import { state } from './state.js';
import { loadConfig, loadConfigVersions } from './pages/config.js';
import { refreshMatrixPanels, loadAcmeShow } from './pages/panels.js';
import { refreshDetails } from './refresh.js';

export function activate(key) {
  $$('[data-pane]').forEach((p) => { p.hidden = p.getAttribute('data-pane') !== key; });
  $$('[data-nav]').forEach((n) => {
    n.classList.toggle('on', n.getAttribute('data-nav') === key);
  });
  if (key === 'settings') {
    if (!state.configLoaded) loadConfig();
    loadConfigVersions();
  }
  if (key === 'matrix') refreshMatrixPanels();
  if (key === 'certs') loadAcmeShow();
  refreshDetails();
}
