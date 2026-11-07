// 设置页：主配置按类型渲染成表单，保存时只回写改动过的字段。
import { DesktopService, msgOf } from '../api.js';
import { $, $$, esc, toast } from '../ui/dom.js';
import { state } from '../state.js';
import { askConfirm } from '../dialogs/confirm.js';
import { refreshOverview } from '../refresh.js';

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

export function initConfigEditor() {
  const box = $('#configForm');
  if (!box) return;
  box.addEventListener('input', markConfigDirty);
  box.addEventListener('change', markConfigDirty);
  $('#configReloadFile').addEventListener('click', () => loadConfig(true));
  $('#configSave').addEventListener('click', async () => {
    const values = collectChangedValues();
    const n = Object.keys(values).length;
    if (!n) { setConfigStatus('没有改动'); return; }
    const btn = $('#configSave');
    btn.disabled = true;
    setConfigStatus('正在保存…');
    try {
      const text = await DesktopService.SaveConfigForm(values);
      state.configDirty = false;
      $$('#configForm [data-row]').forEach((row) => {
        const el = row.querySelector('[data-value]');
        if (!el) return;
        const v = configRowValue(el);
        row.setAttribute('data-initial', v);
        state.configInitial[row.getAttribute('data-row')] = v;
      });
      setConfigStatus(`已保存 ${n} 个字段，请点「重载配置」使改动生效`, 'ok');
      toast(text);
      await refreshOverview();
    } catch (e) {
      btn.disabled = false;
      setConfigStatus('保存失败：' + msgOf(e), 'err');
      toast('保存失败：' + msgOf(e));
    }
  });
}
