// 应用内确认框：原生 confirm 在 dev 模式下会把标题显示成 wails.localhost:9245。
import { t } from '../i18n.js';
import { $ } from '../ui/dom.js';

export const template = `<div class="dialog-mask" id="confirmDialog" hidden>
  <div class="dialog dialog-sm" role="dialog" aria-modal="true" aria-labelledby="confirmTitle">
    <h3 id="confirmTitle" data-i18n="dialog.confirm.title">确认操作</h3>
    <p id="confirmBody">—</p>
    <div class="dialog-actions">
      <button class="btn" id="confirmCancel" data-i18n="common.cancel">取消</button>
      <button class="btn primary" id="confirmOk" data-i18n="common.ok">确定</button>
    </div>
  </div>
</div>`;

let confirmResolve = null;

export function askConfirm(title, body, okText) {
  const mask = $('#confirmDialog');
  if (!mask) return Promise.resolve(false);
  $('#confirmTitle').textContent = title;
  $('#confirmBody').textContent = body;
  $('#confirmOk').textContent = okText || t('common.ok');
  mask.hidden = false;
  $('#confirmOk').focus();
  return new Promise((resolve) => { confirmResolve = resolve; });
}

export function initConfirmDialog() {
  const mask = $('#confirmDialog');
  if (!mask) return;
  const settle = (value) => {
    mask.hidden = true;
    const resolve = confirmResolve;
    confirmResolve = null;
    if (resolve) resolve(value);
  };
  $('#confirmCancel').addEventListener('click', () => settle(false));
  $('#confirmOk').addEventListener('click', () => settle(true));
  mask.addEventListener('click', (e) => { if (e.target === mask) settle(false); });
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape' && !mask.hidden) { e.preventDefault(); settle(false); }
  });
}
