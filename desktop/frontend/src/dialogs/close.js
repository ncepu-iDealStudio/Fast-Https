// 关闭窗口确认：隐藏到托盘或退出桌面端，服务进程不受影响。
import { DesktopService, msgOf } from '../api.js';
import { $, toast } from '../ui/dom.js';

export const template = `<div class="dialog-mask" id="closeDialog" hidden>
  <div class="dialog" role="dialog" aria-modal="true" aria-labelledby="closeDialogTitle">
    <h3 id="closeDialogTitle">要隐藏到后台还是直接退出？</h3>
    <p>隐藏到后台：桌面端继续在系统托盘运行，点托盘图标可恢复窗口。<br/>直接退出：结束桌面端进程，并停止 <span class="mono">fast-https</span> 服务。</p>
    <div class="dialog-actions">
      <button class="btn" id="closeCancel">取消</button>
      <button class="btn" id="closeQuit">直接退出</button>
      <button class="btn primary" id="closeHide">隐藏到后台</button>
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
    try { await DesktopService.HideWindow(); } catch (e) { toast('窗口操作失败：' + msgOf(e)); }
  });
  $('#closeQuit').addEventListener('click', async () => {
    close();
    try { await DesktopService.QuitApp(); } catch (e) { toast('退出失败：' + msgOf(e)); }
  });
  mask.addEventListener('click', (e) => { if (e.target === mask) close(); });
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape' && !mask.hidden) { e.preventDefault(); close(); }
  });
}
