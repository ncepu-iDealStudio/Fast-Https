// 工作区切换：pane 显隐、导航高亮、按需加载设置页，并刷新明细数据。
import { $$ } from './ui/dom.js';
import { state } from './state.js';
import { loadConfig } from './pages/config.js';
import { refreshDetails } from './refresh.js';

export function activate(key) {
  $$('[data-pane]').forEach((p) => { p.hidden = p.getAttribute('data-pane') !== key; });
  $$('[data-nav]').forEach((n) => {
    n.classList.toggle('on', n.getAttribute('data-nav') === key);
  });
  if (key === 'settings' && !state.configLoaded) loadConfig();
  refreshDetails();
}
