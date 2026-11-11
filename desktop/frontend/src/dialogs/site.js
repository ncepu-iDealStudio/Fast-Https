// 站点弹窗：添加与详情编辑共用一套表单，另含站点行的详情/删除/访问链接委托。
// 表单按「怎么方便怎么填」设计：协议与端口用下拉/数字框选择，listen 与
// proxy_pass 在下方联动生成（仍可手动微调，如给监听加 h2）；提交时翻译成
// SiteInput（对应 site add / site update 的 flag），证书、首页与文件名由
// 配置事务自动处理。
import { DesktopService, msgOf } from '../api.js';
import { t } from '../i18n.js';
import { $, $$, esc, toast } from '../ui/dom.js';
import { withLoading, withImmediateLoading } from '../ui/loading.js';
import { state } from '../state.js';
import { askConfirm } from './confirm.js';
import { refreshOverview, refreshDetails } from '../refresh.js';
import { refreshOperations } from '../ui/events.js';

export const template = `<div class="dialog-mask" id="siteDialog" hidden>
  <div class="dialog dialog-wide" role="dialog" aria-modal="true" aria-labelledby="siteDialogTitle">
    <h3 id="siteDialogTitle" data-i18n="dialog.site.titleAdd">添加站点</h3>
    <p class="dialog-source" id="siteSource" hidden></p>
    <div class="site-form">
      <div class="form-row"><label class="form-label" for="siteName"><span data-i18n="dialog.site.name">域名 / IP</span><small data-i18n="dialog.site.nameHint">多个域名用空格分隔，如 example.com www.example.com</small></label><div class="form-control"><input id="siteName" type="text" placeholder="example.com 或 192.168.1.10" data-i18n-placeholder="dialog.site.namePh"/></div></div>
      <div class="form-row"><label class="form-label"><span data-i18n="dialog.site.proto">协议与端口</span><small data-i18n="dialog.site.protoHint">选好协议和端口，监听会自动生成</small></label>
        <div class="form-control">
          <div class="inline-fields">
            <select id="siteProto" aria-label="协议" data-i18n-aria="dialog.site.protoAria">
              <option value="http" data-i18n="dialog.site.protoHttp">仅 HTTP</option>
              <option value="https" data-i18n="dialog.site.protoHttps">仅 HTTPS</option>
              <option value="both" data-i18n="dialog.site.protoBoth">HTTP + HTTPS</option>
            </select>
            <label class="mini-field"><span data-i18n="dialog.site.httpPort">HTTP 端口</span><input id="siteHttpPort" type="number" min="1" max="65535" value="80"/></label>
            <label class="mini-field"><span data-i18n="dialog.site.httpsPort">HTTPS 端口</span><input id="siteHttpsPort" type="number" min="1" max="65535" value="443"/></label>
          </div>
          <input id="siteListen" type="text" placeholder="80 或 80, 443 ssl h2" data-i18n-placeholder="dialog.site.listenPh" aria-label="监听" data-i18n-aria="dialog.site.listenAria"/>
          <small class="field-note" data-i18n="dialog.site.listenNote">listen：自动生成，可以再手动微调（例如把 443 ssl 改成 443 ssl h2）</small>
        </div>
      </div>
      <div class="form-row"><label class="form-label" for="siteType" data-i18n="dialog.site.type">类型</label><div class="form-control"><select id="siteType"><option value="local" data-i18n="dialog.site.typeLocal">静态站点（本地文件）</option><option value="proxy" data-i18n="dialog.site.typeProxy">反向代理（转发给后端）</option></select></div></div>
      <div class="form-row" id="siteRootRow"><label class="form-label" for="siteRoot"><span data-i18n="dialog.site.root">站点根目录</span><small data-i18n="dialog.site.rootHint">网页文件放在这里</small></label><div class="form-control with-browse"><input id="siteRoot" type="text" placeholder="./httpdoc/root"/><button class="btn small" type="button" data-browse="dir" data-target="siteRoot" data-i18n="dialog.site.browse">浏览…</button></div></div>
      <div class="form-row" id="siteProxyRow" hidden><label class="form-label"><span data-i18n="dialog.site.proxy">反代地址</span><small data-i18n="dialog.site.proxyHint">后端服务的地址，协议在这里选</small></label>
        <div class="form-control">
          <div class="inline-fields">
            <select id="siteProxyScheme" aria-label="反代协议" data-i18n-aria="dialog.site.proxySchemeAria">
              <option value="http://">http://</option>
              <option value="https://">https://</option>
            </select>
            <input id="siteProxy" type="text" placeholder="127.0.0.1:8080" aria-label="反代主机" data-i18n-aria="dialog.site.proxyHostAria"/>
          </div>
          <small class="field-note" data-i18n="dialog.site.proxyNote">proxy_pass：也可以直接粘贴完整地址（http:// 或 https:// 开头），协议会跟着识别</small>
        </div>
      </div>
    </div>
    <div class="detail-info" id="siteInfo" hidden>
      <div class="section-label"><span data-i18n="dialog.site.readonly">只读信息</span></div>
      <div class="kv-table" id="siteInfoRows"></div>
      <details class="raw-details"><summary data-i18n="dialog.site.raw">原始 server 块（JSON）</summary><pre class="cmd-result" id="siteRaw"></pre></details>
    </div>
    <p class="dialog-hint" data-i18n-html="dialog.site.hint">多个域名共享同一组设置；用 IP、localhost 或别名访问时，默认的 <span class="mono">security.host_check=off</span> 会回落到该端口的站点。证书、首页与文件名由配置事务自动处理（HTTPS 站点证书默认 <span class="mono">./cert/&lt;域名&gt;.pem</span>，文件名即主域名）。</p>
    <p class="dialog-note" id="siteNote" hidden></p>
    <p class="dialog-error" id="siteError" hidden></p>
    <div class="dialog-actions">
      <button class="btn" id="siteCancel" data-i18n="common.cancel">取消</button>
      <button class="btn primary" id="siteSubmit" data-i18n="dialog.site.create">创建站点</button>
    </div>
  </div>
</div>`;

// local 与 proxy 显示的字段不同，详情回填后也要重新同步一次
function syncSiteType() {
  const proxy = $('#siteType').value === 'proxy';
  $('#siteRootRow').hidden = proxy;
  $('#siteProxyRow').hidden = !proxy;
}

// composeListen 由协议与端口生成 listen 文本；用户之后仍可手动微调。
function composeListen() {
  const proto = $('#siteProto').value;
  const hp = $('#siteHttpPort').value.trim() || '80';
  const sp = $('#siteHttpsPort').value.trim() || '443';
  const parts = [];
  if (proto === 'http' || proto === 'both') parts.push(hp);
  if (proto === 'https' || proto === 'both') parts.push(sp + ' ssl h2');
  $('#siteListen').value = parts.join(', ');
}

// applyListenToControls 从 listen 文本反推协议与端口（详情回填用）。
// 原文保留在输入框里（可能带 h2 等高级参数），改动上面的控件才会重新生成。
function applyListenToControls(text) {
  let httpPort = '', httpsPort = '';
  for (const part of String(text || '').split(',')) {
    const fields = part.trim().split(/\s+/).filter(Boolean);
    if (!fields.length) continue;
    const port = fields[0].replace(/^:/, '');
    if (!/^\d+$/.test(port)) continue;
    if (/\bssl\b/.test(part)) {
      if (!httpsPort) httpsPort = port;
    } else if (!httpPort) {
      httpPort = port;
    }
  }
  $('#siteHttpPort').value = httpPort || '80';
  $('#siteHttpsPort').value = httpsPort || '443';
  $('#siteProto').value = httpPort && httpsPort ? 'both' : (httpsPort ? 'https' : 'http');
  $('#siteListen').value = text || '';
}

// composeProxyPass 拼 proxy_pass：输入框里已有完整地址就原样用，
// 否则拼上选中的协议。
function composeProxyPass() {
  const body = $('#siteProxy').value.trim();
  if (/^https?:\/\//i.test(body)) return body;
  return body ? $('#siteProxyScheme').value + body : '';
}

// applyProxyToControls 从 proxy_pass 反推协议与主机（详情回填用）。
function applyProxyToControls(pass) {
  const m = String(pass || '').match(/^(https?):\/\/(.*)$/i);
  if (m) {
    $('#siteProxyScheme').value = m[1].toLowerCase() + '://';
    $('#siteProxy').value = m[2];
  } else {
    $('#siteProxyScheme').value = 'http://';
    $('#siteProxy').value = pass || '';
  }
}

function setSiteFormEnabled(enabled) {
  $$('#siteDialog input, #siteDialog select').forEach((el) => { el.disabled = !enabled; });
  $$('#siteDialog [data-browse]').forEach((b) => { b.disabled = !enabled; });
  $('#siteSubmit').hidden = !enabled;
}

function resetSiteForm() {
  ['siteName', 'siteRoot', 'siteProxy'].forEach((id) => { $('#' + id).value = ''; });
  $('#siteType').value = 'local';
  $('#siteProto').value = 'http';
  $('#siteHttpPort').value = '80';
  $('#siteHttpsPort').value = '443';
  $('#siteProxyScheme').value = 'http://';
  composeListen();
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

// 点站点行的「⋯」：ReadSiteDetail(id) 返回 model.SiteDetail。表单回填 site update
// 的白名单字段；cert/certKey/index/source 只读展示，Raw 折叠展示 server 块。
async function openSiteDetail(id) {
  state.siteMode = 'edit';
  state.siteEditingId = id;
  state.siteEditingName = id;
  resetSiteForm();
  $('#siteDialog').hidden = false;
  $('#siteDialogTitle').textContent = t('dialog.site.titleDetail');
  $('#siteSubmit').textContent = t('dialog.site.save');
  $('#siteSource').hidden = false;
  $('#siteSource').textContent = id;
  try {
    const d = await DesktopService.ReadSiteDetail(id);
    state.siteEditingName = d.name || id;
    $('#siteSource').textContent = d.source || id;
    $('#siteName').value = d.name || '';
    applyListenToControls(d.listen || '');
    $('#siteType').value = d.type || 'local';
    $('#siteRoot').value = d.root || '';
    applyProxyToControls(d.proxyPass || '');
    syncSiteType();
    // 只读信息：证书、私钥、首页与配置来源
    $('#siteInfoRows').innerHTML = [
      [t('dialog.site.cert'), d.cert],
      [t('dialog.site.certKey'), d.certKey],
      [t('dialog.site.index'), d.index],
      [t('dialog.site.source'), d.source],
      [t('dialog.site.path'), d.path]
    ].filter((r) => r[1])
      .map((r) => `<div class="kv-row"><span class="k">${esc(r[0])}</span><span class="v mono">${esc(r[1])}</span></div>`)
      .join('') || `<div class="kv-row"><span class="k">—</span><span class="v">${esc(t('dialog.site.noExtra'))}</span></div>`;
    $('#siteInfo').hidden = false;
    // Raw 是 server 块 JSON 字符串：能解析就美化展示
    let raw = d.raw || '';
    try { raw = JSON.stringify(JSON.parse(raw), null, 2); } catch (e) { /* 原样展示 */ }
    $('#siteRaw').textContent = raw || t('dialog.site.rawEmpty');
    if (!d.editable) {
      setSiteFormEnabled(false);
      $('#siteNote').textContent = d.note || t('dialog.site.notEditable');
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
    $('#siteDialogTitle').textContent = t('dialog.site.titleAdd');
    $('#siteSubmit').textContent = t('dialog.site.create');
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
      const word = t(enabled ? 'dialog.site.enable' : 'dialog.site.disable');
      // withLoading 会禁用按钮并显示 spinner，结束后自动还原；
      // 不再手动 toggle.disabled = true/false。
      await withLoading({
        button: toggle,
        label: word,
        fn: async () => {
          const msg = await DesktopService.SetSiteEnabled(id, enabled);
          toast(msg || t('toast.actionDone', { action: word }));
          await refreshDetails({ force: true });
          await refreshOverview();
          refreshOperations();
        }
      }).catch((err) => {
        // Go 侧已把 CLI 封套错误翻成人话；这里再按场景补一句可操作的建议
        const raw = msgOf(err);
        let hint = raw;
        if (!enabled && /\u552f\u4e00|\u5168\u90e8\u505c\u7528|no server configured|no usable site/i.test(raw)) {
          hint = t('friendly.siteOnlyOne');
        } else if (/\u5df2\u7ecf\u662f\u542f\u7528|already enabled/i.test(raw)) {
          hint = t('friendly.siteAlreadyOn');
        } else if (/\u5df2\u7ecf\u662f\u505c\u7528|already disabled/i.test(raw)) {
          hint = t('friendly.siteAlreadyOff');
        } else if (/\u627e\u4e0d\u5230|not found/i.test(raw)) {
          hint = t('friendly.siteNotFound');
        } else if (/\u5185\u8054|inline/i.test(raw)) {
          hint = t('friendly.siteInline');
        }
        toast(hint);
      });
      return;
    }
    const del = e.target.closest ? e.target.closest('.site-del') : null;
    if (!del) return;
    e.preventDefault();
    const id = del.getAttribute('data-id');
    const name = del.getAttribute('data-name') || id;
    const ok = await askConfirm(t('dialog.site.deleteTitle'),
      t('dialog.site.deleteBody', { name }),
      t('dialog.site.deleteOk'));
    if (!ok) return;
    await withLoading({
      button: del,
      label: t('common.delete'),
      fn: async () => {
        toast(await DesktopService.DeleteSite(id));
        await refreshDetails({ force: true });
        await refreshOverview();
        refreshOperations();
      }
    }).catch((err) => {
      toast(t('toast.actionFailed', { action: t('common.delete'), msg: msgOf(err) }));
    });
  });
  $$('#siteDialog [data-browse]').forEach((b) => b.addEventListener('click', async () => {
    try {
      const p = await DesktopService.PickPath(b.getAttribute('data-browse'));
      if (p) $('#' + b.getAttribute('data-target')).value = p;
    } catch (e) {
      $('#siteError').textContent = t('dialog.site.pickFailed', { msg: msgOf(e) });
      $('#siteError').hidden = false;
    }
  }));
  $('#siteType').addEventListener('change', syncSiteType);
  // 协议/端口改动重新生成 listen；反代协议与主机联动出 proxy_pass
  ['siteProto', 'siteHttpPort', 'siteHttpsPort'].forEach((id) => {
    $('#' + id).addEventListener('change', composeListen);
    $('#' + id).addEventListener('input', composeListen);
  });
  $('#siteSubmit').addEventListener('click', async () => {
    const btn = $('#siteSubmit');
    $('#siteError').hidden = true;
    // SiteInput：id/name/listen/type/root/proxyPass；name 支持多域名（空格分隔）
    const payload = {
      id: state.siteMode === 'edit' ? state.siteEditingId : '',
      name: $('#siteName').value,
      listen: $('#siteListen').value,
      type: $('#siteType').value,
      root: $('#siteRoot').value,
      proxyPass: composeProxyPass()
    };
    // 配置事务必然较慢，用立即加载（不走 140ms 延迟），按钮立刻变 spinner
    await withImmediateLoading({
      button: btn,
      label: state.siteMode === 'add' ? t('dialog.site.titleAdd') : t('common.save'),
      fn: async () => {
        const text = state.siteMode === 'add'
          ? await DesktopService.AddSite(payload)
          : await DesktopService.SaveSiteDetail(payload);
        close();
        toast(text);
        await refreshDetails({ force: true });
        await refreshOverview();
        refreshOperations();
      }
    }).catch((e) => {
      $('#siteError').textContent = msgOf(e);
      $('#siteError').hidden = false;
    });
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
      toast(t('dialog.site.openLinkFailed', { msg: msgOf(err) }));
    }
  });
}
