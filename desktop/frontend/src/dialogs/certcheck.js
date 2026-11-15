// 证书检查弹窗：cert check --domain <域名>（§6.2）的结果展示。
// passed 大字 + exists/validity/hostname/key_pair/chain 逐项检查。
// 检查结果是数据（证书有问题也退出 0）；域名不在任何 ssl 站点 → NOT_FOUND。
import { DesktopService, msgOf } from '../api.js';
import { t } from '../i18n.js';
import { $, esc } from '../ui/dom.js';

export const template = `<div class="dialog-mask" id="certCheckDialog" hidden>
  <div class="dialog" role="dialog" aria-modal="true" aria-labelledby="certCheckTitle">
    <h3 id="certCheckTitle" data-i18n="dialog.certcheck.title">证书检查</h3>
    <div class="diag-overall"><strong id="certCheckPassed">—</strong><span class="muted mono" id="certCheckDomain"></span></div>
    <div class="kv-table" id="certCheckItems"><div class="kv-row"><span class="k">—</span><span class="v" data-i18n="dialog.certcheck.notChecked">尚未检查</span></div></div>
    <p class="dialog-error" id="certCheckError" hidden></p>
    <div class="dialog-actions">
      <button class="btn primary" id="certCheckClose" data-i18n="common.close">关闭</button>
    </div>
  </div>
</div>`;

const CHECK_ITEMS = [
  ['exists', 'dialog.certcheck.iExists'],
  ['validity', 'dialog.certcheck.iValidity'],
  ['hostname', 'dialog.certcheck.iHostname'],
  ['key_pair', 'dialog.certcheck.iKeyPair'],
  ['chain', 'dialog.certcheck.iChain']
];

let busy = false;

function renderCheck(d) {
  $('#certCheckDomain').textContent = d.domain || '';
  const passed = $('#certCheckPassed');
  passed.textContent = d.passed ? t('dialog.certcheck.passed') : t('dialog.certcheck.failed');
  passed.className = 'diag-overall-text st-' + (d.passed ? 'ok' : 'err');
  const checks = d.checks || {};
  $('#certCheckItems').innerHTML = CHECK_ITEMS.map(([key, labelKey]) => {
    const c = checks[key] || {};
    // chain.checked 恒为 false（不做链验证）：显示文件内证书张数
    const detail = key === 'chain'
      ? (c.certificates_in_file != null ? t('dialog.certcheck.chainCount', { n: c.certificates_in_file }) : '')
      : (c.detail || '');
    let stCls = 'off', stLabel = t('dialog.certcheck.stNotChecked');
    if (key === 'chain') {
      stCls = 'off'; stLabel = t('dialog.certcheck.stNotVerified');
    } else if (c.checked) {
      if (c.ok) { stCls = 'ok'; stLabel = t('dialog.certcheck.stPass'); }
      else { stCls = 'err'; stLabel = t('dialog.certcheck.stFail'); }
    }
    return `<div class="kv-row"><span class="k">${esc(t(labelKey))}</span>`
      + `<span class="v">${esc(detail || '—')}</span>`
      + `<span class="n"><span class="st-badge st-${stCls}">${esc(stLabel)}</span></span></div>`;
  }).join('');
}

export async function openCertCheck(domain) {
  const mask = $('#certCheckDialog');
  if (!mask || busy) return;
  busy = true;
  mask.hidden = false;
  $('#certCheckError').hidden = true;
  $('#certCheckDomain').textContent = domain || '';
  $('#certCheckPassed').textContent = t('dialog.certcheck.checking');
  $('#certCheckPassed').className = 'diag-overall-text';
  $('#certCheckItems').innerHTML = `<div class="kv-row"><span class="k">—</span><span class="v">${esc(t('dialog.certcheck.checking'))}</span></div>`;
  try {
    const res = await DesktopService.RunCommand(['cert', 'check', '--domain', domain]);
    renderCheck(res?.data || {});
  } catch (e) {
    $('#certCheckPassed').textContent = t('dialog.certcheck.checkFailed');
    $('#certCheckPassed').className = 'diag-overall-text st-err';
    $('#certCheckItems').innerHTML = '';
    const err = $('#certCheckError');
    err.textContent = msgOf(e);
    err.hidden = false;
  } finally {
    busy = false;
  }
}

export function initCertCheckDialog() {
  const mask = $('#certCheckDialog');
  if (!mask) return;
  const close = () => { mask.hidden = true; };
  $('#certCheckClose').addEventListener('click', close);
  mask.addEventListener('click', (e) => { if (e.target === mask) close(); });
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape' && !mask.hidden) { e.preventDefault(); close(); }
  });
  // 证书行是动态重建的，「检查」按钮用事件委托
  document.addEventListener('click', (e) => {
    const b = e.target.closest ? e.target.closest('.cert-check') : null;
    if (!b) return;
    e.preventDefault();
    openCertCheck(b.getAttribute('data-domain'));
  });
}
