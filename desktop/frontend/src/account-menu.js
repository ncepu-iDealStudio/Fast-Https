// 左下角「本机管理」账户菜单：外观切换、设置入口、隐藏到托盘、帮助文档。
import { DesktopService, msgOf } from './api.js';
import { $, $$, toast } from './ui/dom.js';
import { setMode } from './theme.js';
import { activate } from './router.js';

export const template = `<div class="account-menu" id="accountMenu" hidden role="menu">
  <div class="menu-head">
    <img class="menu-logo" src="/appicon.png" alt=""/>
    <div class="menu-head-text"><b>Fast-Https Desktop</b><small>本机管理端</small></div>
  </div>

  <div class="menu-title">外观</div>
  <button class="menu-item" data-menu="light" role="menuitemradio"><span>亮色</span><span class="menu-check" data-mode-check="light"></span></button>
  <button class="menu-item" data-menu="dark" role="menuitemradio"><span>暗色</span><span class="menu-check" data-mode-check="dark"></span></button>
  <div class="menu-title">中性色</div>
  <div class="menu-swatches">
    <button class="menu-swatch" data-accent="0" aria-label="石墨"><i style="background:#5F6670"></i></button>
    <button class="menu-swatch" data-accent="1" aria-label="暖灰"><i style="background:#6F655C"></i></button>
    <button class="menu-swatch" data-accent="2" aria-label="松绿"><i style="background:#4F6B62"></i></button>
    <button class="menu-swatch" data-accent="3" aria-label="赭石"><i style="background:#8A5A3B"></i></button>
    <button class="menu-swatch" data-accent="4" aria-label="靛灰"><i style="background:#55607A"></i></button>
  </div>

  <div class="menu-sep"></div>
  <button class="menu-item" data-menu="settings"><span>设置</span><span class="menu-hint">本机管理端</span></button>
  <button class="menu-item" data-menu="tray"><span>隐藏到后台托盘</span><span class="menu-hint">保留窗口</span></button>

  <div class="menu-sep"></div>
  <button class="menu-item" data-menu="help"><span>帮助文档</span><span class="menu-hint">usage.html</span></button>
</div>`;

export function initAccountMenu() {
  const btn = $('#accountBtn');
  const menu = $('#accountMenu');
  const close = () => { menu.hidden = true; btn.setAttribute('aria-expanded', 'false'); };
  btn.addEventListener('click', (e) => {
    e.stopPropagation();
    menu.hidden = !menu.hidden;
    btn.setAttribute('aria-expanded', String(!menu.hidden));
  });
  document.addEventListener('click', (e) => {
    if (!menu.hidden && !menu.contains(e.target) && !btn.contains(e.target)) close();
  });
  document.addEventListener('keydown', (e) => { if (e.key === 'Escape') close(); });

  $$('[data-menu]').forEach((b) => b.addEventListener('click', async () => {
    const key = b.getAttribute('data-menu');
    if (key === 'light' || key === 'dark') { setMode(key); return; }
    close();
    if (key === 'settings') { activate('settings'); return; }
    if (key === 'tray') {
      try { await DesktopService.HideWindow(); } catch (e) { toast('窗口操作失败：' + msgOf(e)); }
      return;
    }
    if (key === 'help') {
      try { toast(await DesktopService.OpenHelp()); } catch (e) { toast('打开帮助失败：' + msgOf(e)); }
      return;
    }
  }));
}
