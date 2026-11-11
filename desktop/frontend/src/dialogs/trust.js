// 信任本地 CA 引导：给出路径、分步说明、可复制的命令，自动导入前二次确认。
// g.steps / g.command / g.risk 等是 Go 侧 CertTrustGuide 返回的说明文本，
// 随平台不同，由后端生成，这里不做二次翻译。
import { DesktopService, msgOf } from '../api.js';
import { t } from '../i18n.js';
import { $, esc, toast } from '../ui/dom.js';
import { state } from '../state.js';
import { askConfirm } from './confirm.js';

export const template = `<div class="dialog-mask" id="trustDialog" hidden>
  <div class="dialog dialog-wide" role="dialog" aria-modal="true" aria-labelledby="trustTitle">
    <h3 id="trustTitle"><span data-i18n="dialog.trust.title">信任本地 CA</span> <span class="platform-chip" id="trustPlatform">—</span></h3>
    <p class="trust-risk" id="trustRisk"></p>
    <div class="trust-path"><span class="mono" id="trustPath">—</span><span id="trustExists"></span></div>
    <p class="dialog-hint" data-i18n-html="dialog.trust.hint">这张 <span class="mono">root.crt</span> 是<b>本地根 CA 证书</b>，由服务首次启动时自动生成（不是安装包预置），负责签发各站点的证书；信任库要导入的是它，不是站点证书。CA 私钥在 <span class="mono" id="trustKeyPath">—</span>，与证书一样只在文件缺失时创建。</p>
    <ol class="trust-steps" id="trustSteps"></ol>
    <div class="trust-cmd"><code id="trustCommand"></code></div>
    <p class="dialog-hint" id="trustResult" hidden></p>
    <div class="dialog-actions">
      <button class="btn" id="trustCancel" data-i18n="common.cancel">取消</button>
      <button class="btn" id="trustOpenDir" data-i18n="dialog.trust.openDir">打开证书目录</button>
      <button class="btn" id="trustCopy" data-i18n="dialog.trust.copy">复制命令</button>
      <button class="btn primary" id="trustImport" data-i18n="dialog.trust.import">尝试自动导入</button>
    </div>
  </div>
</div>`;

async function openTrustDialog() {
  const mask = $('#trustDialog');
  if (!mask) return;
  $('#trustResult').hidden = true;
  try {
    const g = await DesktopService.CertTrustGuide();
    state.trustGuideData = g;
    $('#trustRisk').textContent = g.risk || '';
    $('#trustPlatform').textContent = g.platformLabel || g.platform || '';
    $('#trustKeyPath').textContent = g.keyPath || '—';
    $('#trustPath').textContent = g.certPath || '—';
    $('#trustExists').textContent = g.exists ? t('dialog.trust.exists') : t('dialog.trust.missing');
    $('#trustSteps').innerHTML = (g.steps || []).map((s) => `<li>${esc(s)}</li>`).join('');
    $('#trustCommand').textContent = g.command || t('dialog.trust.commandFallback');
    $('#trustImport').disabled = !g.exists;
    mask.hidden = false;
  } catch (e) {
    toast(t('dialog.trust.readGuideFailed', { msg: msgOf(e) }));
  }
}

function setTrustResult(text, isError) {
  const r = $('#trustResult');
  r.textContent = text;
  r.className = isError ? 'dialog-error' : 'trust-result-ok';
  r.hidden = false;
}

export function initTrustDialog() {
  const mask = $('#trustDialog');
  if (!mask) return;
  const close = () => { mask.hidden = true; };
  $('#trustCABtn').addEventListener('click', openTrustDialog);
  $('#trustCancel').addEventListener('click', close);
  mask.addEventListener('click', (e) => { if (e.target === mask) close(); });
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape' && !mask.hidden) { e.preventDefault(); close(); }
  });
  $('#trustOpenDir').addEventListener('click', async () => {
    if (!state.trustGuideData) return;
    try { await DesktopService.OpenPath(state.trustGuideData.dirPath); }
    catch (e) { setTrustResult(t('dialog.trust.openDirFailed', { msg: msgOf(e) }), true); }
  });
  $('#trustCopy').addEventListener('click', async () => {
    try {
      await navigator.clipboard.writeText($('#trustCommand').textContent);
      setTrustResult(t('dialog.trust.copied'), false);
    } catch (e) {
      setTrustResult(t('dialog.trust.copyFailed', { msg: msgOf(e) }), true);
    }
  });
  $('#trustImport').addEventListener('click', async () => {
    // 确认期间先收起引导框：两层 dialog-mask 会互相遮挡，确认框容易被
    // 盖住，看起来像「点了没反应」。确认结束后恢复，结果写回引导框。
    mask.hidden = true;
    let ok = false;
    try {
      ok = await askConfirm(t('dialog.trust.confirmTitle'),
        t('dialog.trust.confirmBody'),
        t('dialog.trust.confirmOk'));
    } finally {
      mask.hidden = false;
    }
    if (!ok) return;
    const btn = $('#trustImport');
    btn.disabled = true;
    try {
      setTrustResult(await DesktopService.ImportLocalCA(), false);
      toast(t('dialog.trust.importStarted'));
    } catch (e) {
      setTrustResult(t('dialog.trust.importFailed', { msg: msgOf(e) }), true);
      toast(t('dialog.trust.importFailedToast'));
    } finally {
      btn.disabled = false;
    }
  });
}
