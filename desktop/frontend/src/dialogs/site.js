// 站点弹窗：添加与详情编辑共用一套表单，另含站点行的详情/删除/访问链接委托。
import { DesktopService, msgOf } from '../api.js';
import { $, $$, esc, toast } from '../ui/dom.js';
import { state } from '../state.js';
import { askConfirm } from './confirm.js';
import { refreshOverview, refreshDetails } from '../refresh.js';

export const template = `<div class="dialog-mask" id="siteDialog" hidden>
  <div class="dialog dialog-wide" role="dialog" aria-modal="true" aria-labelledby="siteDialogTitle">
    <h3 id="siteDialogTitle">添加站点</h3>
    <p class="dialog-source" id="siteSource" hidden></p>
    <div class="site-form">
      <div class="form-row"><label class="form-label" for="siteName">域名 / IP<small>server_name；多个域名用空格分隔，如 example.com www.example.com</small></label><div class="form-control"><input id="siteName" type="text" placeholder="example.com www.example.com 或 192.168.1.10"/></div></div>
      <div class="form-row"><label class="form-label" for="siteListen">监听<small>listen，逗号或换行分隔</small></label><div class="form-control"><input id="siteListen" type="text" placeholder="80 或 443 ssl h2"/></div></div>
      <div class="form-row"><label class="form-label" for="siteType">类型</label><div class="form-control"><select id="siteType"><option value="local">静态站点（local）</option><option value="proxy">反向代理（proxy）</option></select></div></div>
      <div class="form-row" id="siteRootRow"><label class="form-label" for="siteRoot">站点根目录<small>root，相对 config 目录</small></label><div class="form-control with-browse"><input id="siteRoot" type="text" placeholder="./httpdoc/root"/><button class="btn small" type="button" data-browse="dir" data-target="siteRoot">浏览…</button></div></div>
      <div class="form-row" id="siteProxyRow" hidden><label class="form-label" for="siteProxy">反代地址<small>proxy_pass，需 http(s)://</small></label><div class="form-control"><input id="siteProxy" type="text" placeholder="http://127.0.0.1:9000"/></div></div>
      <div class="form-row" id="siteIndexRow"><label class="form-label" for="siteIndex">首页<small>index，可留空</small></label><div class="form-control"><input id="siteIndex" type="text" placeholder="index.html index.htm"/></div></div>
      <div class="form-row"><label class="form-label" for="siteCert">站点证书<small>ssl_certificate；由本地根 CA 签发，留空自动生成</small></label><div class="form-control with-browse"><input id="siteCert" type="text" placeholder="./cert/example.pem"/><button class="btn small" type="button" data-browse="file" data-target="siteCert">浏览…</button></div></div>
      <div class="form-row"><label class="form-label" for="siteCertKey">证书私钥<small>ssl_certificate_key；与站点证书同时留空或同时填写</small></label><div class="form-control with-browse"><input id="siteCertKey" type="text" placeholder="./cert/example-key.pem"/><button class="btn small" type="button" data-browse="file" data-target="siteCertKey">浏览…</button></div></div>
      <div class="form-row" id="siteFileNameRow"><label class="form-label" for="siteFileName">文件名<small>conf.d 下的 JSON 文件名，可留空</small></label><div class="form-control"><input id="siteFileName" type="text" placeholder="默认按域名生成"/></div></div>
    </div>
    <p class="dialog-hint">多个域名用空格分隔（如 example.com www.example.com），它们共享同一组 location 和证书。用 IP、localhost 或别名访问时，默认的 <span class="mono">security.host_check=off</span> 会回落到该端口的站点；改成 <span class="mono">enforce</span> 才会严格校验 Host。</p>
    <p class="dialog-hint">这里填的是<b>站点证书</b>，路径相对配置目录（如 <span class="mono">./cert/&lt;主域名&gt;.pem</span>），由 <span class="mono">ssl_certificate</span> 指向、TLS 握手时发给浏览器；<span class="mono">httpdoc/root/root.crt</span> 是<b>本地根 CA 证书</b>，负责签发这些站点证书，也是要导入系统信任库的那一张。两者都由服务首次启动时自动生成。</p>
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
  $('#siteIndexRow').hidden = proxy;
  $('#siteProxyRow').hidden = !proxy;
}

function setSiteFormEnabled(enabled) {
  $$('#siteDialog input, #siteDialog select').forEach((el) => { el.disabled = !enabled; });
  $$('#siteDialog [data-browse]').forEach((b) => { b.disabled = !enabled; });
  $('#siteSubmit').hidden = !enabled;
}

function resetSiteForm() {
  ['siteName', 'siteListen', 'siteRoot', 'siteProxy', 'siteIndex', 'siteCert', 'siteCertKey', 'siteFileName']
    .forEach((id) => { $('#' + id).value = ''; });
  $('#siteType').value = 'local';
  $('#siteFileNameRow').hidden = false;
  $('#siteSource').hidden = true;
  $('#siteSource').textContent = '';
  $('#siteNote').hidden = true;
  $('#siteError').hidden = true;
  setSiteFormEnabled(true);
  syncSiteType();
}

// 点站点行的「⋯」：读取该站点文件的字段与原文，不可编辑时只读展示原因
async function openSiteDetail(source) {
  state.siteMode = 'edit';
  state.siteEditingSource = source;
  resetSiteForm();
  $('#siteDialog').hidden = false;
  $('#siteDialogTitle').textContent = '站点详情';
  $('#siteSubmit').textContent = '保存修改';
  $('#siteFileNameRow').hidden = true;
  $('#siteSource').hidden = false;
  $('#siteSource').textContent = source;
  try {
    const d = await DesktopService.ReadSiteDetail(source);
    $('#siteName').value = d.name || '';
    $('#siteListen').value = d.listen || '';
    $('#siteType').value = d.type || 'local';
    $('#siteRoot').value = d.root || '';
    $('#siteProxy').value = d.proxyPass || '';
    $('#siteIndex').value = d.index || '';
    $('#siteCert').value = d.cert || '';
    $('#siteCertKey').value = d.certKey || '';
    syncSiteType();
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
    state.siteEditingSource = '';
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
  // 站点行是动态重建的，详情与删除按钮用事件委托
  document.addEventListener('click', async (e) => {
    const more = e.target.closest ? e.target.closest('.site-more') : null;
    if (more) {
      e.preventDefault();
      openSiteDetail(more.getAttribute('data-source'));
      return;
    }
    const toggle = e.target.closest ? e.target.closest('.site-toggle') : null;
    if (toggle) {
      e.preventDefault();
      const source = toggle.getAttribute('data-source');
      const enabled = toggle.getAttribute('data-enabled') === '1';
      const word = enabled ? '启用' : '停用';
      try {
        toast(await DesktopService.SetSiteEnabled(source, enabled));
        await refreshDetails();
        await refreshOverview();
      } catch (err) {
        toast(word + '失败：' + msgOf(err));
      }
      return;
    }
    const del = e.target.closest ? e.target.closest('.site-del') : null;
    if (!del) return;
    e.preventDefault();
    const source = del.getAttribute('data-source');
    const ok = await askConfirm('删除站点？',
      '将删除 ' + source + '（保留 .bak 备份），需点「重载配置」才生效。',
      '删除');
    if (!ok) return;
    try {
      toast(await DesktopService.DeleteSite(source));
      await refreshDetails();
      await refreshOverview();
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
    const payload = {
      name: $('#siteName').value,
      listen: $('#siteListen').value,
      type: $('#siteType').value,
      root: $('#siteRoot').value,
      proxyPass: $('#siteProxy').value,
      index: $('#siteIndex').value,
      cert: $('#siteCert').value,
      certKey: $('#siteCertKey').value,
      fileName: state.siteMode === 'add' ? $('#siteFileName').value : '',
      source: state.siteMode === 'edit' ? state.siteEditingSource : ''
    };
    try {
      const text = state.siteMode === 'add'
        ? await DesktopService.AddSite(payload)
        : await DesktopService.SaveSiteDetail(payload);
      close();
      toast(text);
      await refreshDetails();
      await refreshOverview();
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
