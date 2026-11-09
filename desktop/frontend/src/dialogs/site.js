// 站点弹窗：添加与详情编辑共用一套表单，另含站点行的详情/删除/访问链接委托。
// 表单字段与 site add / site update 的 flag 一一对应（SiteInput）：
// 域名、监听、类型、root / proxyPass；证书路径、首页与文件名由配置事务自动处理。
import { DesktopService, msgOf } from '../api.js';
import { $, $$, esc, toast } from '../ui/dom.js';
import { state } from '../state.js';
import { askConfirm } from './confirm.js';
import { refreshOverview, refreshDetails } from '../refresh.js';
import { refreshOperations } from '../ui/events.js';

export const template = `<div class="dialog-mask" id="siteDialog" hidden>
  <div class="dialog dialog-wide" role="dialog" aria-modal="true" aria-labelledby="siteDialogTitle">
    <h3 id="siteDialogTitle">添加站点</h3>
    <p class="dialog-source" id="siteSource" hidden></p>
    <div class="site-form">
      <div class="form-row"><label class="form-label" for="siteName">域名 / IP<small>server_name；多个域名用空格分隔，如 example.com www.example.com</small></label><div class="form-control"><input id="siteName" type="text" placeholder="example.com www.example.com 或 192.168.1.10"/></div></div>
      <div class="form-row"><label class="form-label" for="siteListen">监听<small>listen，逗号分隔多条，如 80, 443 ssl h2</small></label><div class="form-control"><input id="siteListen" type="text" placeholder="80 或 80, 443 ssl h2"/></div></div>
      <div class="form-row"><label class="form-label" for="siteType">类型</label><div class="form-control"><select id="siteType"><option value="local">静态站点（local）</option><option value="proxy">反向代理（proxy）</option></select></div></div>
      <div class="form-row" id="siteRootRow"><label class="form-label" for="siteRoot">站点根目录<small>root，相对 config 目录</small></label><div class="form-control with-browse"><input id="siteRoot" type="text" placeholder="./httpdoc/root"/><button class="btn small" type="button" data-browse="dir" data-target="siteRoot">浏览…</button></div></div>
      <div class="form-row" id="siteProxyRow" hidden><label class="form-label" for="siteProxy">反代地址<small>proxy_pass，需 http(s)://</small></label><div class="form-control"><input id="siteProxy" type="text" placeholder="http://127.0.0.1:8080"/></div></div>
    </div>
    <div class="detail-info" id="siteInfo" hidden>
      <div class="section-label"><span>只读信息</span></div>
      <div class="kv-table" id="siteInfoRows"></div>
      <details class="raw-details"><summary>原始 server 块（JSON）</summary><pre class="cmd-result" id="siteRaw"></pre></details>
    </div>
    <p class="dialog-hint">多个域名用空格分隔（如 example.com www.example.com），它们共享同一组 location 和证书。用 IP、localhost 或别名访问时，默认的 <span class="mono">security.host_check=off</span> 会回落到该端口的站点；改成 <span class="mono">enforce</span> 才会严格校验 Host。</p>
    <p class="dialog-hint">证书路径、首页与文件名由配置事务自动处理（TLS 站点证书默认 <span class="mono">./cert/&lt;域名&gt;.pem</span>，文件名即主域名）。</p>
    <p class="dialog-note" id="siteNote" hidden></p>
    <p class="dialog-error" id="siteError" hidden></p>
    <div class="dialog-actions">
      <button class="btn" id="siteCancel">取消</button>
      <button class="btn primary" id="siteSubmit">创建站点</button>
    </div>
  </div>
</div>`;

// local 与 proxy 显示的字段不同，详情回填后也要重新同步一次
function syncSiteType() {
  const proxy = $('#siteType').value === 'proxy';
  $('#siteRootRow').hidden = proxy;
  $('#siteProxyRow').hidden = !proxy;
}

function setSiteFormEnabled(enabled) {
  $$('#siteDialog input, #siteDialog select').forEach((el) => { el.disabled = !enabled; });
  $$('#siteDialog [data-browse]').forEach((b) => { b.disabled = !enabled; });
  $('#siteSubmit').hidden = !enabled;
}

function resetSiteForm() {
  ['siteName', 'siteListen', 'siteRoot', 'siteProxy'].forEach((id) => { $('#' + id).value = ''; });
  $('#siteType').value = 'local';
  $('#siteSource').hidden = true;
  $('#siteSource').textContent = '';
  $('#siteInfo').hidden = true;
  $('#siteInfoRows').innerHTML = '';
  $('#siteRaw').textContent = '';
  $('#siteNote').hidden = true;
  $('#siteError').hidden = true;
  setSiteFormEnabled(true);
  syncSiteType();
}

// 站点详情：ReadSiteDetail(id) 返回 model.SiteDetail。表单回填 site update
// 的白名单字段；cert/certKey/index/source 只读展示，Raw 折叠展示 server 块。
async function openSiteDetail(id) {
  state.siteMode = 'edit';
  state.siteEditingId = id;
  state.siteEditingName = id;
  resetSiteForm();
  $('#siteDialog').hidden = false;
  $('#siteDialogTitle').textContent = '站点详情';
  $('#siteSubmit').textContent = '保存修改';
  $('#siteSource').hidden = false;
  $('#siteSource').textContent = id;
  try {
    const d = await DesktopService.ReadSiteDetail(id);
    state.siteEditingName = d.name || id;
    $('#siteSource').textContent = d.source || id;
    $('#siteName').value = d.name || '';
    $('#siteListen').value = d.listen || '';
    $('#siteType').value = d.type || 'local';
    $('#siteRoot').value = d.root || '';
    $('#siteProxy').value = d.proxyPass || '';
    syncSiteType();
    // 只读信息：证书、私钥、首页与配置来源
    $('#siteInfoRows').innerHTML = [
      ['站点证书', d.cert],
      ['证书私钥', d.certKey],
      ['首页 index', d.index],
      ['配置来源', d.source],
      ['配置路径', d.path]
    ].filter((r) => r[1])
      .map((r) => `<div class="kv-row"><span class="k">${esc(r[0])}</span><span class="v mono">${esc(r[1])}</span></div>`)
      .join('') || '<div class="kv-row"><span class="k">—</span><span class="v">没有额外信息</span></div>';
    $('#siteInfo').hidden = false;
    // Raw 是 server 块 JSON 字符串：能解析就美化展示
    let raw = d.raw || '';
    try { raw = JSON.stringify(JSON.parse(raw), null, 2); } catch (e) { /* 原样展示 */ }
    $('#siteRaw').textContent = raw || '（空）';
    if (!d.editable) {
      setSiteFormEnabled(false);
      $('#siteNote').textContent = d.note || '该站点不可在这里编辑';
      $('#siteNote').hidden = false;
    }
  } catch (e) {
    setSiteFormEnabled(false);
    $('#siteError').textContent = msgOf(e);
    $('#siteError').hidden = false;
  }
}

export function initSiteDialog() {
  const mask = $('#siteDialog');
  if (!mask) return;
  const close = () => { mask.hidden = true; $('#siteError').hidden = true; };
  $('#addSiteBtn').addEventListener('click', () => {
    state.siteMode = 'add';
    state.siteEditingId = '';
    state.siteEditingName = '';
    resetSiteForm();
    $('#siteDialogTitle').textContent = '添加站点';
    $('#siteSubmit').textContent = '创建站点';
    mask.hidden = false;
    $('#siteName').focus();
  });
  $('#siteCancel').addEventListener('click', close);
  mask.addEventListener('click', (e) => { if (e.target === mask) close(); });
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape' && !mask.hidden) { e.preventDefault(); close(); }
  });
  // 站点行是动态重建的，详情与删除按钮用事件委托（data-id = 站点名）
  document.addEventListener('click', async (e) => {
    const more = e.target.closest ? e.target.closest('.site-more') : null;
    if (more) {
      e.preventDefault();
      openSiteDetail(more.getAttribute('data-id'));
      return;
    }
    const toggle = e.target.closest ? e.target.closest('.site-toggle') : null;
    if (toggle) {
      e.preventDefault();
      const id = toggle.getAttribute('data-id');
      const enabled = toggle.getAttribute('data-enabled') === '1';
      const word = enabled ? '启用' : '停用';
      try {
        toast(await DesktopService.SetSiteEnabled(id, enabled));
        await refreshDetails();
        await refreshOverview();
        refreshOperations();
      } catch (err) {
        toast(word + '失败：' + msgOf(err));
      }
      return;
    }
    const del = e.target.closest ? e.target.closest('.site-del') : null;
    if (!del) return;
    e.preventDefault();
    const id = del.getAttribute('data-id');
    const name = del.getAttribute('data-name') || id;
    const ok = await askConfirm('删除站点？',
      `将删除站点 ${name}。删除走配置事务（校验、恢复点、reload、可回滚），提交后立即生效，无需手动重载。`,
      '删除');
    if (!ok) return;
    try {
      toast(await DesktopService.DeleteSite(id));
      await refreshDetails();
      await refreshOverview();
      refreshOperations();
    } catch (err) {
      toast('删除失败：' + msgOf(err));
    }
  });
  $$('#siteDialog [data-browse]').forEach((b) => b.addEventListener('click', async () => {
    try {
      const p = await DesktopService.PickPath(b.getAttribute('data-browse'));
      if (p) $('#' + b.getAttribute('data-target')).value = p;
    } catch (e) {
      $('#siteError').textContent = '选择失败：' + msgOf(e);
      $('#siteError').hidden = false;
    }
  }));
  $('#siteType').addEventListener('change', syncSiteType);
  $('#siteSubmit').addEventListener('click', async () => {
    const btn = $('#siteSubmit');
    btn.disabled = true;
    $('#siteError').hidden = true;
    // SiteInput：id/name/listen/type/root/proxyPass；name 支持多域名（空格分隔）
    const payload = {
      id: state.siteMode === 'edit' ? state.siteEditingId : '',
      name: $('#siteName').value,
      listen: $('#siteListen').value,
      type: $('#siteType').value,
      root: $('#siteRoot').value,
      proxyPass: $('#siteProxy').value
    };
    try {
      const text = state.siteMode === 'add'
        ? await DesktopService.AddSite(payload)
        : await DesktopService.SaveSiteDetail(payload);
      close();
      toast(text);
      await refreshDetails();
      await refreshOverview();
      refreshOperations();
    } catch (e) {
      $('#siteError').textContent = msgOf(e);
      $('#siteError').hidden = false;
    } finally {
      btn.disabled = false;
    }
  });
}

export function initSiteLinks() {
  // 站点地址是 <a href="#">，统一委托到 Go 侧的 OpenPath，用系统默认浏览器打开。
  document.addEventListener('click', async (e) => {
    const a = e.target.closest ? e.target.closest('.site-link') : null;
    if (!a) return;
    e.preventDefault();
    try {
      await DesktopService.OpenPath(a.getAttribute('data-url'));
    } catch (err) {
      toast('打开链接失败：' + msgOf(err));
    }
  });
}
