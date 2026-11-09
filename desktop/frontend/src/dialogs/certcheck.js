// 证书检查弹窗：cert check --domain <域名>（§6.2）的结果展示。
// passed 大字 + exists/validity/hostname/key_pair/chain 逐项检查。
// 检查结果是数据（证书有问题也退出 0）；域名不在任何 ssl 站点 → NOT_FOUND。
import { DesktopService, msgOf } from '../api.js';
import { $, esc } from '../ui/dom.js';

export const template = `<div class="dialog-mask" id="certCheckDialog" hidden>
  <div class="dialog" role="dialog" aria-modal="true" aria-labelledby="certCheckTitle">
    <h3 id="certCheckTitle">证书检查</h3>
    <div class="diag-overall"><strong id="certCheckPassed">—</strong><span class="muted mono" id="certCheckDomain"></span></div>
    <div class="kv-table" id="certCheckItems"><div class="kv-row"><span class="k">—</span><span class="v">尚未检查</span></div></div>
    <p class="dialog-error" id="certCheckError" hidden></p>
    <div class="dialog-actions">
      <button class="btn primary" id="certCheckClose">关闭</button>
    </div>
  </div>
</div>`;

const CHECK_ITEMS = [
  ['exists', '文件存在'],
  ['validity', '有效期'],
  ['hostname', '域名覆盖'],
  ['key_pair', '私钥配对'],
  ['chain', '证书链']
];

let busy = false;

function renderCheck(d) {
  $('#certCheckDomain').textContent = d.domain || '';
  const passed = $('#certCheckPassed');
  passed.textContent = d.passed ? '检查通过' : '检查未通过';
  passed.className = 'diag-overall-text st-' + (d.passed ? 'ok' : 'err');
  const checks = d.checks || {};
  $('#certCheckItems').innerHTML = CHECK_ITEMS.map(([key, label]) => {
    const c = checks[key] || {};
    // chain.checked 恒为 false（不做链验证）：显示文件内证书张数
    const detail = key === 'chain'
      ? (c.certificates_in_file != null ? `文件内 ${c.certificates_in_file} 张证书` : '')
      : (c.detail || '');
    let stCls = 'off', stLabel = '未检查';
    if (key === 'chain') {
      stCls = 'off'; stLabel = '未验证';
    } else if (c.checked) {
      if (c.ok) { stCls = 'ok'; stLabel = '通过'; }
      else { stCls = 'err'; stLabel = '未通过'; }
    }
    return `<div class="kv-row"><span class="k">${esc(label)}</span>`
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
  $('#certCheckPassed').textContent = '检查中…';
  $('#certCheckPassed').className = 'diag-overall-text';
  $('#certCheckItems').innerHTML = '<div class="kv-row"><span class="k">—</span><span class="v">正在检查…</span></div>';
  try {
    const res = await DesktopService.RunCommand(['cert', 'check', '--domain', domain]);
    renderCheck(res?.data || {});
  } catch (e) {
    $('#certCheckPassed').textContent = '检查失败';
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
