// 信任本地 CA 引导：给出路径、分步说明、可复制的命令，自动导入前二次确认。
import { DesktopService, msgOf } from '../api.js';
import { $, esc, toast } from '../ui/dom.js';
import { state } from '../state.js';
import { askConfirm } from './confirm.js';

export const template = `<div class="dialog-mask" id="trustDialog" hidden>
  <div class="dialog dialog-wide" role="dialog" aria-modal="true" aria-labelledby="trustTitle">
    <h3 id="trustTitle">信任本地 CA <span class="platform-chip" id="trustPlatform">—</span></h3>
    <p class="trust-risk" id="trustRisk"></p>
    <div class="trust-path"><span class="mono" id="trustPath">—</span><span id="trustExists"></span></div>
    <p class="dialog-hint">这张 <span class="mono">root.crt</span> 是<b>本地根 CA 证书</b>，由服务首次启动时自动生成（不是安装包预置），负责签发各站点的证书；信任库要导入的是它，不是站点证书。CA 私钥在 <span class="mono" id="trustKeyPath">—</span>，与证书一样只在文件缺失时创建。</p>
    <ol class="trust-steps" id="trustSteps"></ol>
    <div class="trust-cmd"><code id="trustCommand"></code></div>
    <p class="dialog-hint" id="trustResult" hidden></p>
    <div class="dialog-actions">
      <button class="btn" id="trustCancel">取消</button>
      <button class="btn" id="trustOpenDir">打开证书目录</button>
      <button class="btn" id="trustCopy">复制命令</button>
      <button class="btn primary" id="trustImport">尝试自动导入</button>
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
    $('#trustExists').textContent = g.exists ? '证书已存在' : '证书尚未生成：请先启动一次服务';
    $('#trustSteps').innerHTML = (g.steps || []).map((s) => `<li>${esc(s)}</li>`).join('');
    $('#trustCommand').textContent = g.command || '当前平台请手动导入 root.crt 到系统信任库';
    $('#trustImport').disabled = !g.exists;
    mask.hidden = false;
  } catch (e) {
    toast('读取引导失败：' + msgOf(e));
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
    catch (e) { setTrustResult('打开目录失败：' + msgOf(e), true); }
  });
  $('#trustCopy').addEventListener('click', async () => {
    try {
      await navigator.clipboard.writeText($('#trustCommand').textContent);
      setTrustResult('命令已复制到剪贴板', false);
    } catch (e) {
      setTrustResult('复制失败，请手动选中命令：' + msgOf(e), true);
    }
  });
  $('#trustImport').addEventListener('click', async () => {
    // 确认期间先收起引导框：两层 dialog-mask 会互相遮挡，确认框容易被
    // 盖住，看起来像「点了没反应」。确认结束后恢复，结果写回引导框。
    mask.hidden = true;
    let ok = false;
    try {
      ok = await askConfirm('确认信任本地根 CA？',
        '这会把它加入系统受信任的根证书颁发机构，之后本机所有 HTTPS 站点都会信任由它签发的证书；系统可能弹出 UAC 或管理员密码框。仅限本机开发使用。',
        '确认并导入');
    } finally {
      mask.hidden = false;
    }
    if (!ok) return;
    const btn = $('#trustImport');
    btn.disabled = true;
    try {
      setTrustResult(await DesktopService.ImportLocalCA(), false);
      toast('本地 CA 导入请求已发起，详情见弹窗');
    } catch (e) {
      setTrustResult('导入未完成：' + msgOf(e) + '\n可点「复制命令」在管理员终端里手动执行。', true);
      toast('导入未完成，详情见弹窗');
    } finally {
      btn.disabled = false;
    }
  });
}
