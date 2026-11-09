// 左下角「本机管理」账户菜单：外观切换、语言切换、设置入口、隐藏到托盘、帮助文档。
// 语言列表从 i18n.js 的 LANGS 注册表生成，label 用各语言自己的写法；
// 新增语言只要在注册表加一行，这里不用改。
import { DesktopService, msgOf } from './api.js';
import { LANGS, getLang, setLang, t } from './i18n.js';
import { $, $$, toast } from './ui/dom.js';
import { setMode } from './theme.js';
import { activate } from './router.js';
import { openHelp } from './dialogs/help.js';

const langItems = Object.entries(LANGS).map(([code, l]) =>
  `<button class="menu-item" data-lang="${code}" role="menuitemradio"><span>${l.label}</span><span class="menu-check" data-lang-check="${code}"></span></button>`
).join('');

export const template = `<div class="account-menu" id="accountMenu" hidden role="menu">
  <div class="menu-head">
    <img class="menu-logo" src="/appicon.png" alt=""/>
    <div class="menu-head-text"><b>Fast-Https Desktop</b><small data-i18n="app.console">本机管理端</small></div>
  </div>

  <div class="menu-title" data-i18n="menu.appearance">外观</div>
  <button class="menu-item" data-menu="light" role="menuitemradio"><span data-i18n="menu.light">亮色</span><span class="menu-check" data-mode-check="light"></span></button>
  <button class="menu-item" data-menu="dark" role="menuitemradio"><span data-i18n="menu.dark">暗色</span><span class="menu-check" data-mode-check="dark"></span></button>
  <div class="menu-title" data-i18n="menu.neutral">中性色</div>
  <div class="menu-swatches">
    <button class="menu-swatch" data-accent="0" aria-label="石墨" data-i18n-aria="menu.accentGraphite"><i style="background:#5F6670"></i></button>
    <button class="menu-swatch" data-accent="1" aria-label="暖灰" data-i18n-aria="menu.accentWarm"><i style="background:#6F655C"></i></button>
    <button class="menu-swatch" data-accent="2" aria-label="松绿" data-i18n-aria="menu.accentPine"><i style="background:#4F6B62"></i></button>
    <button class="menu-swatch" data-accent="3" aria-label="赭石" data-i18n-aria="menu.accentOchre"><i style="background:#8A5A3B"></i></button>
    <button class="menu-swatch" data-accent="4" aria-label="靛灰" data-i18n-aria="menu.accentIndigo"><i style="background:#55607A"></i></button>
  </div>

  <div class="menu-title" data-i18n="common.language">语言</div>
  ${langItems}

  <div class="menu-sep"></div>
  <button class="menu-item" data-menu="settings"><span data-i18n="menu.settings">设置</span><span class="menu-hint" data-i18n="menu.settingsHint">本机管理端</span></button>
  <button class="menu-item" data-menu="tray"><span data-i18n="menu.tray">隐藏到后台托盘</span><span class="menu-hint" data-i18n="menu.trayHint">保留窗口</span></button>

  <div class="menu-sep"></div>
  <button class="menu-item" data-menu="help"><span data-i18n="help.open">帮助文档</span><span class="menu-hint" data-i18n="menu.helpHint">应用内</span></button>
</div>`;

// 当前语言打勾（与 data-mode-check 同一套 menu-check 样式）
function paintLangChecks() {
  const lang = getLang();
  $$('[data-lang-check]').forEach((b) => {
    const on = b.getAttribute('data-lang-check') === lang;
    b.textContent = on ? '✓' : '';
    if (b.parentElement) b.parentElement.setAttribute('aria-checked', String(on));
  });
}

export function initAccountMenu() {
  const btn = $('#accountBtn');
  const menu = $('#accountMenu');
  const close = () => { menu.hidden = true; btn.setAttribute('aria-expanded', 'false'); };
  paintLangChecks();
  btn.addEventListener('click', (e) => {
    e.stopPropagation();
    menu.hidden = !menu.hidden;
    btn.setAttribute('aria-expanded', String(!menu.hidden));
  });
  document.addEventListener('click', (e) => {
    if (!menu.hidden && !menu.contains(e.target) && !btn.contains(e.target)) close();
  });
  document.addEventListener('keydown', (e) => { if (e.key === 'Escape') close(); });

  $$('[data-lang]').forEach((b) => b.addEventListener('click', () => {
    // setLang 会写 localStorage 并整窗 reload，菜单不用手动收起
    setLang(b.getAttribute('data-lang'));
  }));

  $$('[data-menu]').forEach((b) => b.addEventListener('click', async () => {
    const key = b.getAttribute('data-menu');
    if (key === 'light' || key === 'dark') { setMode(key); return; }
    close();
    if (key === 'settings') { activate('settings'); return; }
    if (key === 'tray') {
      try { await DesktopService.HideWindow(); } catch (e) { toast(t('toast.windowOpFailed', { msg: msgOf(e) })); }
      return;
    }
    if (key === 'help') {
      openHelp();
      return;
    }
  }));
}
