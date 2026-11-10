// 入口：只做初始化编排。业务逻辑分散在 pages/ 与 dialogs/，
// 数据交互统一走 api.js，跨模块状态在 state.js。
import './styles/index.css';
import { DesktopService, msgOf } from './api.js';
import { initI18n, onLangChange, t } from './i18n.js';
import { state } from './state.js';
import { $, $$, toast } from './ui/dom.js';
import { initIcons } from './ui/icons.js';
import { initTheme } from './theme.js';
import { activate } from './router.js';
import { doAction } from './actions.js';
import { refreshOverview, refreshDetails } from './refresh.js';
import { renderEvents, initEventsCard, refreshOperations } from './ui/events.js';
import { renderOverview } from './pages/overview.js';
import { initConfigEditor, loadConfigVersions } from './pages/config.js';
import { initLogPage, loadLogs, rerenderLogs } from './pages/logs.js';
import { initPanels, refreshMatrixPanels, loadAcmeShow } from './pages/panels.js';
import { template as siteTemplate, initSiteDialog, initSiteLinks } from './dialogs/site.js';
import { template as closeTemplate, initCloseDialog, openCloseDialog } from './dialogs/close.js';
import { template as confirmTemplate, initConfirmDialog } from './dialogs/confirm.js';
import { template as trustTemplate, initTrustDialog } from './dialogs/trust.js';
import { template as diagnoseTemplate, initDiagnoseDialog } from './dialogs/diagnose.js';
import { template as certCheckTemplate, initCertCheckDialog } from './dialogs/certcheck.js';
import { template as helpTemplate, initHelpDialog, openHelp } from './dialogs/help.js';
import { template as accountMenuTemplate, initAccountMenu } from './account-menu.js';

// 弹窗与账户菜单位于 body 末尾：先注入模板，再绑定事件
document.body.insertAdjacentHTML('beforeend',
  [siteTemplate, closeTemplate, confirmTemplate, trustTemplate,
   diagnoseTemplate, certCheckTemplate, helpTemplate, accountMenuTemplate].join(''));

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

// 语言切换的原地重绘：静态文案已由 i18n 的 applyStatic 重刷，这里把动态
// 内容用缓存 / 新数据重画一遍。不整窗 reload，界面无闪烁。
function rerenderAll() {
  // CSS 伪元素（.form-row.missing 的「未配置」角标）文案随语言更新
  document.documentElement.style.setProperty('--i18n-missing', JSON.stringify(t('settings.formMissing')));
  // 先用缓存同步重绘，再静默拉新数据补一轮
  if (state.lastOverview) renderOverview(state.lastOverview);
  renderEvents();
  rerenderLogs();
  refreshOverview();
  refreshDetails();
  refreshOperations();
  refreshMatrixPanels();
  loadAcmeShow();
  if (state.configLoaded) loadConfigVersions();
  const help = $('#helpDialog');
  if (help && !help.hidden) openHelp();
}

function init() {
  // 文案翻译放在最前面：上面注入的弹窗/菜单模板也在静态翻译范围内。
  initI18n();
  onLangChange(rerenderAll);
  // 图标水合：把 <span data-ic="..."> 换成统一的线性 SVG。
  initIcons();
  // CSS 伪元素（.form-row.missing 的「未配置」角标）没法调 t()，
  // 用 CSS 变量把当前语言的文案喂进去（content 需要带引号）。
  document.documentElement.style.setProperty('--i18n-missing', JSON.stringify(t('settings.formMissing')));
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
  initHelpDialog();
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
    } catch (e) { toast(t('toast.windowOpFailed', { msg: msgOf(e) })); }
  }));

  renderEvents();
  initLogPage();
  refreshOverview();
  refreshDetails();
  refreshOperations();
  loadLogs(true);
  setInterval(refreshOverview, 2000);
  setInterval(refreshDetails, 15000);
}

init();
