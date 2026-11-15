// 设置页：主配置按类型渲染成表单，保存时只回写改动过的字段。
// 保存走 SaveConfigForm（后端是候选配置 + config apply 事务，自动 reload）；
// 「校验配置」「配置版本」「对比」「回滚」走 RunCommand 转交 CLI。
import { DesktopService, msgOf } from '../api.js';
import { t } from '../i18n.js';
import { $, $$, esc, toast } from '../ui/dom.js';
import { state } from '../state.js';
import { askConfirm } from '../dialogs/confirm.js';
import { withImmediateLoading } from '../ui/loading.js';
import { refreshOverview } from '../refresh.js';
import { refreshOperations } from '../ui/events.js';

function setConfigStatus(text, kind) {
  const el = $('#configStatus');
  el.textContent = text;
  el.className = 'config-status' + (kind ? ' ' + kind : '');
}

// renderConfigField 按字段的业务含义选控件，不要一律用文本框：
//   select      枚举值 → 下拉（手打容易错）
//   bool        开关 → 勾选框
//   multiselect 多选（挑战方式、日志字段）→ 勾选组
//   pathlist    路径列表（include 目录）→ 每行一个输入 + 浏览 + 删除，可追加
//   browse 非空 单一路径 → 输入框 + 「浏览…」按钮（dir 选目录 / file 选文件）
//   textarea    CSP 这类长策略 → 多行
// 其余（HSTS 策略、MIME 类型等自由文本）才用普通输入框。
function renderConfigField(f) {
  const id = 'cf-' + f.path.replace(/[^\w-]/g, '-');
  let v = f.value || '';
  let control;
  const browseBtn = f.browse
    ? `<button type="button" class="btn small path-browse" data-browse-for="${id}" data-browse-kind="${esc(f.browse)}" title="${esc(t('settings.browse'))}">…</button>`
    : '';

  if (f.type === 'multiselect') {
    // 固定候选 + 配置里已有的自定义值，避免勾选时把自定义项丢掉
    const current = String(v).split('\n').map((x) => x.trim()).filter(Boolean);
    const options = [...(f.options || [])];
    for (const x of current) if (!options.includes(x)) options.push(x);
    const checked = options.filter((o) => current.includes(o));
    v = checked.join('\n');
    control = `<div class="choice-group" id="${id}" data-value="${f.path}" data-multi="1">${options.map((o) => `<label class="choice"><input type="checkbox" value="${esc(o)}" ${current.includes(o) ? 'checked' : ''}/><span>${esc(o)}</span></label>`).join('')}</div>`;
  } else if (f.type === 'bool') {
    control = `<label class="form-check"><input type="checkbox" id="${id}" data-value="${f.path}" ${f.value === 'true' ? 'checked' : ''}/><span>${esc(t('settings.formOn'))}</span></label>`;
  } else if (f.type === 'select') {
    const opts = (f.options || []).map((o) => `<option value="${esc(o)}" ${o === f.value ? 'selected' : ''}>${o === '' ? esc(t('settings.formUnset')) : esc(o)}</option>`).join('');
    control = `<select id="${id}" data-value="${f.path}">${opts}</select>`;
  } else if (f.type === 'pathlist') {
    // 路径列表：一行一个，可浏览、可删除、可追加。值仍是换行分隔的字符串，
    // 与后端 list 类型的解析保持一致。
    const rows = String(v).split('\n').map((x) => x.trim()).filter(Boolean);
    if (!rows.length) rows.push('');
    control = `<div class="path-list" id="${id}" data-value="${f.path}" data-pathlist="1" data-browse-kind="${esc(f.browse || 'dir')}">
      ${rows.map((r) => pathRowHTML(r, f.browse)).join('')}
      <button type="button" class="btn small path-add" data-add-for="${id}">${esc(t('settings.pathAdd'))}</button>
    </div>`;
  } else if (f.type === 'list' || f.type === 'textarea') {
    control = `<textarea id="${id}" data-value="${f.path}" rows="3" spellcheck="false">${esc(v)}</textarea>`;
  } else if (f.type === 'email') {
    control = `<input id="${id}" type="email" data-value="${f.path}" value="${esc(v)}" placeholder="you@example.com" spellcheck="false"/>`;
  } else if (f.browse) {
    control = `<div class="path-single"><input id="${id}" type="text" data-value="${f.path}" value="${esc(v)}" spellcheck="false"/>${browseBtn}</div>`;
  } else {
    control = `<input id="${id}" type="${f.type === 'number' ? 'number' : 'text'}" data-value="${f.path}" value="${esc(v)}"/>`;
  }
  return `<div class="form-row ${f.missing ? 'missing' : ''}" data-row="${f.path}" data-initial="${esc(v)}">
    <label class="form-label" for="${id}">${esc(f.label)}${f.help ? `<small>${esc(f.help)}</small>` : ''}</label>
    <div class="form-control">${control}</div>
  </div>`;
}

// pathRowHTML 是路径列表里的一行：输入框 + 浏览 + 删除。
function pathRowHTML(value, kind) {
  const k = esc(kind || 'dir');
  return `<div class="path-row"><input type="text" class="path-input" value="${esc(value)}" spellcheck="false"/>
    <button type="button" class="btn small path-browse" data-browse-kind="${k}" title="${esc(t('settings.browse'))}">…</button>
    <button type="button" class="btn small path-del" title="${esc(t('common.delete'))}">×</button></div>`;
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
    html += `<div class="config-group"><h4 class="config-group-title">${esc(t('settings.formPreserved'))}</h4><p class="setting-note">${esc(t('settings.formPreservedNote', { fields: form.preserved.join(t('common.listSep')) }))}</p></div>`;
  }
  $('#configForm').innerHTML = html;
  state.configInitial = {};
  $$('#configForm [data-row]').forEach((row) => { state.configInitial[row.getAttribute('data-row')] = row.getAttribute('data-initial'); });
}

// 统一取一行表单的当前值：多选拼成换行串，布尔取勾选状态，
// 路径列表把每行非空值拼成换行串，其余取 value
function configRowValue(el) {
  if (el.hasAttribute('data-multi')) {
    return [...el.querySelectorAll('input:checked')].map((x) => x.value).join('\n');
  }
  if (el.hasAttribute('data-pathlist')) {
    return [...el.querySelectorAll('.path-input')]
      .map((x) => x.value.trim())
      .filter(Boolean)
      .join('\n');
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
  setConfigStatus(n ? t('settings.statusModified', { n }) : t('settings.statusClean'), n ? 'warn' : '');
}

export async function loadConfig(force) {
  const box = $('#configForm');
  if (!box) return;
  if (state.configDirty && force) {
    const ok = await askConfirm(t('settings.discardTitle'), t('settings.discardBody'), t('settings.discardOk'));
    if (!ok) return;
  }
  setConfigStatus(t('settings.statusReading'));
  try {
    const form = await DesktopService.ReadConfigForm();
    renderConfigForm(form);
    $('#configPath').textContent = form.path;
    $('#configMeta').textContent = form.size + ' B · ' + form.modified + (form.writable ? '' : ' · ' + t('settings.readonly'));
    state.configDirty = false;
    state.configLoaded = true;
    $('#configSave').disabled = !form.writable;
    setConfigStatus(form.writable ? t('settings.statusClean') : t('settings.statusReadonly'), form.writable ? '' : 'warn');
  } catch (e) {
    $('#configPath').textContent = t('settings.readFailedTitle');
    $('#configMeta').textContent = '';
    box.innerHTML = `<p class="config-empty">${esc(t('settings.readFailedBody'))}</p>`;
    state.configInitial = {};
    state.configLoaded = false;
    $('#configSave').disabled = true;
    setConfigStatus(t('settings.statusReadFailed', { msg: msgOf(e) }), 'err');
  }
}

// ---- 配置版本：config versions / diff / rollback ----

const OP_LABEL = {
  added: 'settings.diffOpAdded',
  removed: 'settings.diffOpRemoved',
  modified: 'settings.diffOpModified'
};

function renderVersionRows(data) {
  const versions = data.versions || [];
  $('#configDriftNote').hidden = !data.drift;
  $('#configVersions').innerHTML = versions.length
    ? versions.map((v) => {
        const active = v.active ? `<span class="st-badge st-ok">${esc(t('settings.current'))}</span>` : '';
        const meta = [v.created_at, v.created_by, v.summary, v.parent ? t('settings.fromVersion', { v: v.parent }) : '']
          .filter(Boolean).join('　·　');
        return `<div class="kv-row" data-version="${esc(v.version)}">`
          + `<span class="k">${esc(v.version)}${active}</span>`
          + `<span class="v">${esc(meta)}</span>`
          + `<span class="n with-action">`
          + `<button class="btn small ver-diff" data-version="${esc(v.version)}">${esc(t('settings.diff'))}</button>`
          + (v.active ? '' : `<button class="btn small ver-rollback" data-version="${esc(v.version)}">${esc(t('settings.rollback'))}</button>`)
          + `</span></div>`;
      }).join('')
    : `<div class="kv-row"><span class="k">—</span><span class="v">${esc(t('settings.versionsEmpty'))}</span></div>`;
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
    box.innerHTML = `<div class="kv-row"><span class="k">—</span><span class="v">${esc(t('settings.statusReadFailed', { msg: msgOf(e) }))}</span></div>`;
  }
}

function renderDiff(version, data) {
  const box = $('#configDiffBox');
  box.hidden = false;
  if (data.identical || !(data.changes || []).length) {
    box.innerHTML = `<div class="diff-head"><b>${esc(version)}</b><span>${esc(t('settings.diffNone'))}</span>`
      + `<button class="btn small" id="diffClose">${esc(t('settings.diffCollapse'))}</button></div>`;
    return;
  }
  const rows = data.changes.map((c) => {
    // changed:true 是敏感键：只有变更标记，不显示新旧值
    const values = c.changed
      ? `<i>${esc(t('settings.diffSensitive'))}</i>`
      : [c.op !== 'added' && c.old !== undefined ? t('settings.diffOld', { v: JSON.stringify(c.old) }) : '',
         c.op !== 'removed' && c.new !== undefined ? t('settings.diffNew', { v: JSON.stringify(c.new) }) : '',
         c.op === 'added' && c.value !== undefined ? t('settings.diffValue', { v: JSON.stringify(c.value) }) : '']
        .filter(Boolean).join('　→　');
    return `<div class="diff-row"><span class="diff-path mono">${esc(c.path)}</span>`
      + `<span class="diff-op st-badge ${c.op === 'added' ? 'st-ok' : (c.op === 'removed' ? 'st-err' : 'st-warn')}">${esc(OP_LABEL[c.op] ? t(OP_LABEL[c.op]) : c.op)}</span>`
      + `<span class="diff-val">${values}</span></div>`;
  }).join('');
  box.innerHTML = `<div class="diff-head"><b>${esc(version)}</b><span class="muted">${esc(t('settings.diffHead'))}</span>`
    + `<button class="btn small" id="diffClose">${esc(t('settings.diffCollapse'))}</button></div>${rows}`;
}

async function showVersionDiff(version) {
  const box = $('#configDiffBox');
  box.hidden = false;
  box.innerHTML = `<div class="diff-head"><b>${esc(version)}</b><span class="muted">${esc(t('settings.diffRunning'))}</span></div>`;
  try {
    // §2.5：--candidate 可以是版本号；data.changes[{path,op,old,new,value,changed}]
    const res = await DesktopService.RunCommand(['config', 'diff', '--candidate', version]);
    renderDiff(version, res?.data || {});
  } catch (e) {
    box.innerHTML = `<div class="diff-head"><b>${esc(version)}</b><span class="dialog-error">${esc(t('settings.diffFailed', { msg: msgOf(e) }))}</span>`
      + `<button class="btn small" id="diffClose">${esc(t('settings.diffCollapse'))}</button></div>`;
  }
}

async function rollbackVersion(version) {
  const ok = await askConfirm(t('settings.rollbackTitle'),
    t('settings.rollbackBody', { version }),
    t('settings.rollbackOk'));
  if (!ok) return;
  try {
    // §2.7：config rollback --version vNNN --yes，data 为事务结果（§0.5）
    const res = await DesktopService.RunCommand(['config', 'rollback', '--version', version, '--yes']);
    const d = res?.data || {};
    toast(t('settings.rollbackDone') + (d.operation_id ? ' · ' + d.operation_id : '') + (d.version ? ' · ' + t('settings.currentVersion', { v: d.version }) : ''));
    await loadConfigVersions();
    await loadConfig();
    await refreshOverview();
    refreshOperations();
  } catch (e) {
    toast(t('settings.rollbackFailed', { msg: msgOf(e) }));
  }
}

// ---- 校验配置 ----

async function validateConfig() {
  try {
    // §2.3：data.valid 成功即通过；失败是错误封套（CONFIG_INVALID）
    const res = await DesktopService.RunCommand(['config', 'validate']);
    if (res?.data?.valid) {
      toast(t('settings.validateOk'));
      setConfigStatus(t('settings.validateOk'), 'ok');
    } else {
      toast(t('settings.validateFail'));
      setConfigStatus(t('settings.validateFail'), 'warn');
    }
  } catch (e) {
    toast(t('settings.validateError', { msg: msgOf(e) }));
    setConfigStatus(t('settings.validateError', { msg: msgOf(e) }), 'err');
  }
}

export function initConfigEditor() {
  const box = $('#configForm');
  if (!box) return;
  box.addEventListener('input', markConfigDirty);
  box.addEventListener('change', markConfigDirty);

  // 路径字段的「浏览…」与路径列表的增删：表单是动态重建的，用事件委托。
  box.addEventListener('click', async (e) => {
    const tgt = e.target;
    if (!tgt || !tgt.closest) return;

    // 路径列表：追加一行
    const add = tgt.closest('.path-add');
    if (add) {
      e.preventDefault();
      const list = $('#' + add.getAttribute('data-add-for'));
      if (list) {
        list.insertAdjacentHTML('beforeend', pathRowHTML('', list.getAttribute('data-browse-kind')));
        markConfigDirty();
      }
      return;
    }

    // 路径列表：删除一行（至少留一行，避免列表变成空的没法再加）
    const del = tgt.closest('.path-del');
    if (del) {
      e.preventDefault();
      const list = del.closest('.path-list');
      const rows = list ? list.querySelectorAll('.path-row') : [];
      if (rows.length > 1) del.closest('.path-row').remove();
      else rows[0] && (rows[0].querySelector('.path-input').value = '');
      markConfigDirty();
      return;
    }

    // 浏览按钮：单路径字段改自己的 input，路径列表改同一行的 input
    const browse = tgt.closest('.path-browse');
    if (!browse) return;
    e.preventDefault();
    const kind = browse.getAttribute('data-browse-kind') || 'dir';
    let input = null;
    const forId = browse.getAttribute('data-browse-for');
    if (forId) input = $('#' + forId);
    else {
      const row = browse.closest('.path-row');
      input = row ? row.querySelector('.path-input') : null;
    }
    if (!input) return;
    try {
      const picked = await DesktopService.PickPath(kind);
      if (picked) {
        input.value = picked;
        markConfigDirty();
      }
    } catch (err) {
      toast(t('toast.openFailed', { msg: msgOf(err) }));
    }
  });

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
    if (!n) { setConfigStatus(t('settings.statusNoChanges')); return; }
    const btn = $('#configSave');
    setConfigStatus(t('settings.statusSaving'));
    // 配置事务较慢，立即显示按钮加载态
    await withImmediateLoading({
      button: btn,
      label: t('common.save'),
      fn: async () => {
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
        setConfigStatus(t('settings.statusSaved', { n }), 'ok');
        toast(text);
        await refreshOverview();
        await loadConfigVersions();
        refreshOperations();
      }
    }).catch((e) => {
      setConfigStatus(t('settings.statusSaveFailed', { msg: msgOf(e) }), 'err');
      toast(t('settings.statusSaveFailed', { msg: msgOf(e) }));
    });
  });
}
