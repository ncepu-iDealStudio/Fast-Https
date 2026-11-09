// 关闭窗口确认：隐藏到托盘或退出桌面端，服务进程不受影响。
import { DesktopService, msgOf } from '../api.js';
import { t } from '../i18n.js';
import { $, toast } from '../ui/dom.js';

export const template = `<div class="dialog-mask" id="closeDialog" hidden>
  <div class="dialog" role="dialog" aria-modal="true" aria-labelledby="closeDialogTitle">
    <h3 id="closeDialogTitle" data-i18n="dialog.close.title">要隐藏到后台还是直接退出？</h3>
    <p data-i18n-html="dialog.close.body">隐藏到后台：桌面端继续在系统托盘运行，点托盘图标可恢复窗口。<br/>直接退出：结束桌面端进程，并停止 <span class="mono">fast-https</span> 服务。</p>
    <div class="dialog-actions">
      <button class="btn" id="closeCancel" data-i18n="common.cancel">取消</button>
      <button class="btn" id="closeQuit" data-i18n="dialog.close.quit">直接退出</button>
      <button class="btn primary" id="closeHide" data-i18n="dialog.close.hide">隐藏到后台</button>
    </div>
  </div>
</div>`;

export function openCloseDialog() {
  const mask = $('#closeDialog');
  if (!mask) return;
  mask.hidden = false;
  $('#closeHide').focus();
}

export function initCloseDialog() {
  const mask = $('#closeDialog');
  if (!mask) return;
  const close = () => { mask.hidden = true; };
  $('#closeCancel').addEventListener('click', close);
  $('#closeHide').addEventListener('click', async () => {
    close();
    try { await DesktopService.HideWindow(); } catch (e) { toast(t('toast.windowOpFailed', { msg: msgOf(e) })); }
  });
  $('#closeQuit').addEventListener('click', async () => {
    close();
    try { await DesktopService.QuitApp(); } catch (e) { toast(t('dialog.close.quitFailed', { msg: msgOf(e) })); }
  });
  mask.addEventListener('click', (e) => { if (e.target === mask) close(); });
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape' && !mask.hidden) { e.preventDefault(); close(); }
  });
}
