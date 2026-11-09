// 入口：只做初始化编排。业务逻辑分散在 pages/ 与 dialogs/，
// 数据交互统一走 api.js，跨模块状态在 state.js。
import './styles/index.css';
import { DesktopService, msgOf } from './api.js';
import { state } from './state.js';
import { $, $$, toast } from './ui/dom.js';
import { initTheme } from './theme.js';
import { activate } from './router.js';
import { doAction } from './actions.js';
import { refreshOverview, refreshDetails } from './refresh.js';
import { renderEvents, initEventsCard, refreshOperations } from './ui/events.js';
import { initConfigEditor } from './pages/config.js';
import { initLogPage, loadLogs } from './pages/logs.js';
import { initCommandPanel } from './pages/commands.js';
import { initPanels } from './pages/panels.js';
import { template as siteTemplate, initSiteDialog, initSiteLinks } from './dialogs/site.js';
import { template as closeTemplate, initCloseDialog, openCloseDialog } from './dialogs/close.js';
import { template as confirmTemplate, initConfirmDialog } from './dialogs/confirm.js';
import { template as trustTemplate, initTrustDialog } from './dialogs/trust.js';
import { template as diagnoseTemplate, initDiagnoseDialog } from './dialogs/diagnose.js';
import { template as certCheckTemplate, initCertCheckDialog } from './dialogs/certcheck.js';
import { template as accountMenuTemplate, initAccountMenu } from './account-menu.js';

// 弹窗与账户菜单位于 body 末尾：先注入模板，再绑定事件
document.body.insertAdjacentHTML('beforeend',
  [siteTemplate, closeTemplate, confirmTemplate, trustTemplate,
   diagnoseTemplate, certCheckTemplate, accountMenuTemplate].join(''));

// F5 / Ctrl+R 刷新窗口：Frameless 窗口里 WebView 默认快捷键不可靠，
// wails dev 下改了前端代码可用它强制重载最新模块。
document.addEventListener('keydown', (e) => {
  if (e.key === 'F5' || ((e.ctrlKey || e.metaKey) && (e.key === 'r' || e.key === 'R'))) {
    e.preventDefault();
    window.location.reload();
  }
});

// 桌面端禁用浏览器上下文菜单（返回 / 刷新 / 另存为 / 打印 / 检查）。
// 窗口层已设 DefaultContextMenuDisabled，这里兜底 WebKit 等平台差异；
// 输入框保留原生菜单，方便复制粘贴。
document.addEventListener('contextmenu', (e) => {
  const tag = String(e.target?.tagName || '').toLowerCase();
  if (tag === 'input' || tag === 'textarea') return;
  e.preventDefault();
});

function init() {
  initTheme();
  initAccountMenu();
  initConfigEditor();
  initCloseDialog();
  initSiteDialog();
  initSiteLinks();
  initConfirmDialog();
  initTrustDialog();
  initDiagnoseDialog();
  initCertCheckDialog();
  initPanels();
  initEventsCard();

  $$('[data-nav]').forEach((n) => n.addEventListener('click', () => activate(n.getAttribute('data-nav'))));
  $$('[data-goto]').forEach((b) => b.addEventListener('click', () => activate(b.getAttribute('data-goto'))));
  $$('[data-action]').forEach((b) => b.addEventListener('click', () => doAction(b.getAttribute('data-action'))));
  $$('[data-log]').forEach((b) => b.addEventListener('click', () => {
    state.currentLog = b.getAttribute('data-log');
    $$('[data-log]').forEach((x) => x.classList.toggle('on', x === b));
    loadLogs(true);
  }));
  $$('[data-window]').forEach((b) => b.addEventListener('click', async () => {
    const action = b.getAttribute('data-window');
    try {
      if (action === 'minimise') await DesktopService.MinimiseWindow();
      else if (action === 'maximise') await DesktopService.ToggleMaximiseWindow();
      else if (action === 'hide') openCloseDialog();
    } catch (e) { toast('窗口操作失败：' + msgOf(e)); }
  }));

  renderEvents();
  initLogPage();
  initCommandPanel();
  refreshOverview();
  refreshDetails();
  refreshOperations();
  loadLogs(true);
  setInterval(refreshOverview, 2000);
  setInterval(refreshDetails, 15000);
}

init();
