// 设置页：主配置按类型渲染成表单，保存时只回写改动过的字段。
// 保存走 SaveConfigForm（后端是候选配置 + config apply 事务，自动 reload）；
// 「校验配置」「配置版本」「对比」「回滚」走 RunCommand 转交 CLI。
import { DesktopService, msgOf } from '../api.js';
import { $, $$, esc, toast } from '../ui/dom.js';
import { state } from '../state.js';
import { askConfirm } from '../dialogs/confirm.js';
import { refreshOverview } from '../refresh.js';
import { refreshOperations } from '../ui/events.js';

function setConfigStatus(text, kind) {
  const el = $('#configStatus');
  el.textContent = text;
  el.className = 'config-status' + (kind ? ' ' + kind : '');
}

function renderConfigField(f) {
  const id = 'cf-' + f.path.replace(/[^\w-]/g, '-');
  let v = f.value || '';
  let control;
  if (f.type === 'multiselect') {
    // 固定候选 + 配置里已有的自定义值，避免勾选时把自定义项丢掉
    const current = String(v).split('\n').map((x) => x.trim()).filter(Boolean);
    const options = [...(f.options || [])];
    for (const x of current) if (!options.includes(x)) options.push(x);
    const checked = options.filter((o) => current.includes(o));
    v = checked.join('\n');
    control = `<div class="choice-group" id="${id}" data-value="${f.path}" data-multi="1">${options.map((o) => `<label class="choice"><input type="checkbox" value="${esc(o)}" ${current.includes(o) ? 'checked' : ''}/><span>${esc(o)}</span></label>`).join('')}</div>`;
  } else if (f.type === 'bool') {
    control = `<label class="form-check"><input type="checkbox" id="${id}" data-value="${f.path}" ${f.value === 'true' ? 'checked' : ''}/><span>开启</span></label>`;
  } else if (f.type === 'select') {
    const opts = (f.options || []).map((o) => `<option value="${esc(o)}" ${o === f.value ? 'selected' : ''}>${o === '' ? '（未设置）' : esc(o)}</option>`).join('');
    control = `<select id="${id}" data-value="${f.path}">${opts}</select>`;
  } else if (f.type === 'list' || f.type === 'textarea') {
    control = `<textarea id="${id}" data-value="${f.path}" rows="3" spellcheck="false">${esc(v)}</textarea>`;
  } else if (f.type === 'email') {
    control = `<input id="${id}" type="email" data-value="${f.path}" value="${esc(v)}" placeholder="you@example.com" spellcheck="false"/>`;
  } else {
    control = `<input id="${id}" type="${f.type === 'number' ? 'number' : 'text'}" data-value="${f.path}" value="${esc(v)}"/>`;
  }
  return `<div class="form-row ${f.missing ? 'missing' : ''}" data-row="${f.path}" data-initial="${esc(v)}">
    <label class="form-label" for="${id}">${esc(f.label)}${f.help ? `<small>${esc(f.help)}</small>` : ''}</label>
    <div class="form-control">${control}</div>
  </div>`;
}

function renderConfigForm(form) {
  const groups = new Map();
  for (const f of form.fields || []) {
    if (!groups.has(f.group)) groups.set(f.group, []);
    groups.get(f.group).push(f);
  }
  let html = '';
  for (const [name, fields] of groups) {
    html += `<div class="config-group"><h4 class="config-group-title">${esc(name)}</h4>${fields.map(renderConfigField).join('')}</div>`;
  }
  if (form.preserved && form.preserved.length) {
    html += `<div class="config-group"><h4 class="config-group-title">未纳入表单的字段</h4><p class="setting-note">${esc(form.preserved.join('、'))} 会在保存时原样保留。</p></div>`;
  }
  $('#configForm').innerHTML = html;
  state.configInitial = {};
  $$('#configForm [data-row]').forEach((row) => { state.configInitial[row.getAttribute('data-row')] = row.getAttribute('data-initial'); });
}

// 统一取一行表单的当前值：多选拼成换行串，布尔取勾选状态，其余取 value
function configRowValue(el) {
  if (el.hasAttribute('data-multi')) {
    return [...el.querySelectorAll('input:checked')].map((x) => x.value).join('\n');
  }
  if (el.type === 'checkbox') return String(el.checked);
  return el.value;
}

function collectChangedValues() {
  const values = {};
  $$('#configForm [data-row]').forEach((row) => {
    const el = row.querySelector('[data-value]');
    if (!el) return;
    const path = row.getAttribute('data-row');
    const v = configRowValue(el);
    if (v !== state.configInitial[path]) values[path] = v;
  });
  return values;
}

function markConfigDirty() {
  const n = Object.keys(collectChangedValues()).length;
  state.configDirty = n > 0;
  $('#configSave').disabled = n === 0;
  setConfigStatus(n ? `已修改 ${n} 个字段，尚未保存` : '已读取，未修改', n ? 'warn' : '');
}

export async function loadConfig(force) {
  const box = $('#configForm');
  if (!box) return;
  if (state.configDirty && force) {
    const ok = await askConfirm('放弃未保存的修改？', '重新读取会用磁盘上的配置覆盖当前编辑内容，未保存的改动会丢失。', '放弃并重新读取');
    if (!ok) return;
  }
  setConfigStatus('正在读取…');
  try {
    const form = await DesktopService.ReadConfigForm();
    renderConfigForm(form);
    $('#configPath').textContent = form.path;
    $('#configMeta').textContent = form.size + ' B · ' + form.modified + (form.writable ? '' : ' · 只读');
    state.configDirty = false;
    state.configLoaded = true;
    $('#configSave').disabled = !form.writable;
    setConfigStatus(form.writable ? '已读取，未修改' : '文件不可写，仅可查看', form.writable ? '' : 'warn');
  } catch (e) {
    $('#configPath').textContent = '读取失败';
    $('#configMeta').textContent = '';
    box.innerHTML = '<p class="config-empty">读取失败：请确认桌面端与 fast-https 主程序放在同一目录，或从仓库根运行</p>';
    state.configInitial = {};
    state.configLoaded = false;
    $('#configSave').disabled = true;
    setConfigStatus('读取失败：' + msgOf(e), 'err');
  }
}

// ---- 配置版本：config versions / diff / rollback ----

const OP_LABEL = { added: '新增', removed: '删除', modified: '修改' };

function renderVersionRows(data) {
  const versions = data.versions || [];
  $('#configDriftNote').hidden = !data.drift;
  $('#configVersions').innerHTML = versions.length
    ? versions.map((v) => {
        const active = v.active ? '<span class="st-badge st-ok">当前</span>' : '';
        const meta = [v.created_at, v.created_by, v.summary, v.parent ? '来自 ' + v.parent : '']
          .filter(Boolean).join('　·　');
        return `<div class="kv-row" data-version="${esc(v.version)}">`
          + `<span class="k">${esc(v.version)}${active}</span>`
          + `<span class="v">${esc(meta)}</span>`
          + `<span class="n with-action">`
          + `<button class="btn small ver-diff" data-version="${esc(v.version)}">对比</button>`
          + (v.active ? '' : `<button class="btn small ver-rollback" data-version="${esc(v.version)}">回滚</button>`)
          + `</span></div>`;
      }).join('')
    : '<div class="kv-row"><span class="k">—</span><span class="v">还没有配置版本快照（保存一次配置后生成）</span></div>';
}

export async function loadConfigVersions() {
  const box = $('#configVersions');
  if (!box) return;
  try {
    // §2.4：data.versions[{version,created_at,created_by,summary,parent,active}] + data.drift
    const res = await DesktopService.RunCommand(['config', 'versions']);
    renderVersionRows(res?.data || {});
    $('#configDiffBox').hidden = true;
  } catch (e) {
    box.innerHTML = `<div class="kv-row"><span class="k">—</span><span class="v">读取失败：${esc(msgOf(e))}</span></div>`;
  }
}

function renderDiff(version, data) {
  const box = $('#configDiffBox');
  box.hidden = false;
  if (data.identical || !(data.changes || []).length) {
    box.innerHTML = `<div class="diff-head"><b>${esc(version)}</b><span>无差异</span>`
      + `<button class="btn small" id="diffClose">收起</button></div>`;
    return;
  }
  const rows = data.changes.map((c) => {
    // changed:true 是敏感键：只有变更标记，不显示新旧值
    const values = c.changed
      ? '<i>敏感键已变更（不显示值）</i>'
      : [c.op !== 'added' && c.old !== undefined ? '旧 ' + JSON.stringify(c.old) : '',
         c.op !== 'removed' && c.new !== undefined ? '新 ' + JSON.stringify(c.new) : '',
         c.op === 'added' && c.value !== undefined ? '值 ' + JSON.stringify(c.value) : '']
        .filter(Boolean).join('　→　');
    return `<div class="diff-row"><span class="diff-path mono">${esc(c.path)}</span>`
      + `<span class="diff-op st-badge ${c.op === 'added' ? 'st-ok' : (c.op === 'removed' ? 'st-err' : 'st-warn')}">${esc(OP_LABEL[c.op] || c.op)}</span>`
      + `<span class="diff-val">${values}</span></div>`;
  }).join('');
  box.innerHTML = `<div class="diff-head"><b>${esc(version)}</b><span class="muted">与当前磁盘配置的差异</span>`
    + `<button class="btn small" id="diffClose">收起</button></div>${rows}`;
}

async function showVersionDiff(version) {
  const box = $('#configDiffBox');
  box.hidden = false;
  box.innerHTML = `<div class="diff-head"><b>${esc(version)}</b><span class="muted">正在对比…</span></div>`;
  try {
    // §2.5：--candidate 可以是版本号；data.changes[{path,op,old,new,value,changed}]
    const res = await DesktopService.RunCommand(['config', 'diff', '--candidate', version]);
    renderDiff(version, res?.data || {});
  } catch (e) {
    box.innerHTML = `<div class="diff-head"><b>${esc(version)}</b><span class="dialog-error">对比失败：${esc(msgOf(e))}</span>`
      + `<button class="btn small" id="diffClose">收起</button></div>`;
  }
}

async function rollbackVersion(version) {
  const ok = await askConfirm('回滚配置版本？',
    `将回滚到 ${version}。回滚是一次新的事务操作（校验、恢复点、切换、自动 reload），可再回滚。`,
    '回滚');
  if (!ok) return;
  try {
    // §2.7：config rollback --version vNNN --yes，data 为事务结果（§0.5）
    const res = await DesktopService.RunCommand(['config', 'rollback', '--version', version, '--yes']);
    const d = res?.data || {};
    toast(`回滚完成${d.operation_id ? ' · ' + d.operation_id : ''}${d.version ? ' · 当前版本 ' + d.version : ''}`);
    await loadConfigVersions();
    await loadConfig();
    await refreshOverview();
    refreshOperations();
  } catch (e) {
    toast('回滚失败：' + msgOf(e));
  }
}

// ---- 校验配置 ----

async function validateConfig() {
  try {
    // §2.3：data.valid 成功即通过；失败是错误封套（CONFIG_INVALID）
    const res = await DesktopService.RunCommand(['config', 'validate']);
    if (res?.data?.valid) {
      toast('配置校验通过');
      setConfigStatus('配置校验通过', 'ok');
    } else {
      toast('配置校验未通过');
      setConfigStatus('配置校验未通过', 'warn');
    }
  } catch (e) {
    toast('配置校验失败：' + msgOf(e));
    setConfigStatus('配置校验失败：' + msgOf(e), 'err');
  }
}

export function initConfigEditor() {
  const box = $('#configForm');
  if (!box) return;
  box.addEventListener('input', markConfigDirty);
  box.addEventListener('change', markConfigDirty);
  $('#configReloadFile').addEventListener('click', () => loadConfig(true));
  $('#configValidate').addEventListener('click', validateConfig);
  $('#configVersionsRefresh').addEventListener('click', loadConfigVersions);
  // 版本行按钮动态重建，用事件委托
  $('#configVersions').addEventListener('click', (e) => {
    const diff = e.target.closest ? e.target.closest('.ver-diff') : null;
    if (diff) { showVersionDiff(diff.getAttribute('data-version')); return; }
    const rb = e.target.closest ? e.target.closest('.ver-rollback') : null;
    if (rb) rollbackVersion(rb.getAttribute('data-version'));
  });
  $('#configDiffBox').addEventListener('click', (e) => {
    if (e.target.closest && e.target.closest('#diffClose')) $('#configDiffBox').hidden = true;
  });
  $('#configSave').addEventListener('click', async () => {
    const values = collectChangedValues();
    const n = Object.keys(values).length;
    if (!n) { setConfigStatus('没有改动'); return; }
    const btn = $('#configSave');
    btn.disabled = true;
    setConfigStatus('正在保存…');
    try {
      // 后端返回的文案已含操作 id 与版本号，事务自动 reload
      const text = await DesktopService.SaveConfigForm(values);
      state.configDirty = false;
      $$('#configForm [data-row]').forEach((row) => {
        const el = row.querySelector('[data-value]');
        if (!el) return;
        const v = configRowValue(el);
        row.setAttribute('data-initial', v);
        state.configInitial[row.getAttribute('data-row')] = v;
      });
      setConfigStatus(`已保存 ${n} 个字段`, 'ok');
      toast(text);
      await refreshOverview();
      await loadConfigVersions();
      refreshOperations();
    } catch (e) {
      btn.disabled = false;
      setConfigStatus('保存失败：' + msgOf(e), 'err');
      toast('保存失败：' + msgOf(e));
    }
  });
}
