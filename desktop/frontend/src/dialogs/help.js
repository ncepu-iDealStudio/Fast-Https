// 帮助弹窗：帮助文档直接嵌在应用里，不再跳浏览器。
// 内容在 src/help-content.js（中英双语，可扩展）；底部保留一个
// 「完整命令参考」链接，那本手册（usage.html）才用系统浏览器打开。
import { DesktopService, msgOf } from '../api.js';
import { $, esc, toast } from '../ui/dom.js';
import { getLang, t } from '../i18n.js';
import { helpContent } from '../help-content.js';

export const template = `<div class="dialog-mask" id="helpDialog" hidden>
  <div class="dialog dialog-doc" role="dialog" aria-modal="true" aria-labelledby="helpTitle">
    <h3 id="helpTitle">使用帮助</h3>
    <p class="dialog-source" id="helpNote">帮助就在应用里，不用跳到浏览器。</p>
    <div class="help-layout">
      <nav class="help-toc" id="helpToc"></nav>
      <div class="help-body" id="helpBody"></div>
    </div>
    <div class="dialog-actions">
      <button class="btn" id="helpFull">完整命令参考…</button>
      <button class="btn primary" id="helpClose">知道了</button>
    </div>
  </div>
</div>`;

function renderHelp() {
  const sections = helpContent(getLang());
  $('#helpToc').innerHTML = sections.map((s) =>
    `<a class="help-toc-item" href="#help-${esc(s.id)}">${esc(s.title)}</a>`).join('');
  $('#helpBody').innerHTML = sections.map((s) =>
    `<section class="help-section" id="help-${esc(s.id)}"><h4>${esc(s.title)}</h4>${s.html}</section>`).join('');
}

export function openHelp() {
  renderHelp();
  $('#helpDialog').hidden = false;
}

export function initHelpDialog() {
  const mask = $('#helpDialog');
  if (!mask) return;
  $('#helpTitle').textContent = t('help.title');
  $('#helpNote').textContent = t('help.builtInNote');
  $('#helpFull').textContent = t('help.full');
  $('#helpClose').textContent = t('common.close');
  const close = () => { mask.hidden = true; };
  $('#helpClose').addEventListener('click', close);
  mask.addEventListener('click', (e) => { if (e.target === mask) close(); });
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape' && !mask.hidden) { e.preventDefault(); close(); }
  });
  $('#helpFull').addEventListener('click', async () => {
    try { toast(await DesktopService.OpenHelp()); } catch (e) { toast(msgOf(e)); }
  });
}
