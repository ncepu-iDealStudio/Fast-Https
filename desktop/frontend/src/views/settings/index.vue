<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { RefreshCw, Save, CheckCircle2, Plus, X } from '@lucide/vue';
import { useI18n } from '../../composables/useI18n';
import { readConfigForm, saveConfigForm } from '../../services/api/log';
import { runCommand } from '../../services/api/command';
import { pickPath } from '../../services/api/app';
import { messageOf } from '../../services/api/base';
import FhButton from '../../components/ui/FhButton.vue';
import type { ConfigField, ConfigForm } from '../../types/config';

const { t } = useI18n();

const form = ref<ConfigForm | null>(null);
const initial = ref<Record<string, string>>({});
const values = ref<Record<string, string>>({});
const loading = ref(false);
const saving = ref(false);
const error = ref('');
const notice = ref('');

const versions = ref<Record<string, unknown>[]>([]);
const drift = ref(false);
const diffText = ref('');
const diffOpen = ref(false);

  /** 字段按 group 分组，与后端 fieldSpec 的 group 一致。 */
const groups = computed(() => {
  if (!form.value) return [] as { name: string; fields: ConfigField[] }[];
  const map = new Map<string, ConfigField[]>();
  for (const f of form.value.fields) {
    const list = map.get(f.group) ?? [];
    list.push(f);
    map.set(f.group, list);
  }
  return [...map.entries()].map(([name, fields]) => ({ name, fields }));
});

const changed = computed(() => {
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(values.value)) {
    if (v !== initial.value[k]) out[k] = v;
  }
  return out;
});
const dirty = computed(() => Object.keys(changed.value).length > 0);

async function load(): Promise<void> {
  loading.value = true;
  error.value = '';
  try {
    const f = await readConfigForm();
    form.value = f;
    const init: Record<string, string> = {};
    for (const fld of f.fields) init[fld.path] = fld.value ?? '';
    initial.value = init;
    values.value = { ...init };
  } catch (e) {
    error.value = messageOf(e);
    form.value = null;
  } finally {
    loading.value = false;
  }
}

function setValue(path: string, v: string): void {
  values.value = { ...values.value, [path]: v };
}

function multiChecked(f: ConfigField, opt: string): boolean {
  return String(values.value[f.path] ?? '').split('\n').map((s) => s.trim()).includes(opt);
}
function toggleMulti(f: ConfigField, opt: string): void {
  const cur = String(values.value[f.path] ?? '').split('\n').map((s) => s.trim()).filter(Boolean);
  const next = cur.includes(opt) ? cur.filter((x) => x !== opt) : [...cur, opt];
  setValue(f.path, next.join('\n'));
}
function pathRows(f: ConfigField): string[] {
  const rows = String(values.value[f.path] ?? '').split('\n').map((s) => s.trim()).filter(Boolean);
  return rows.length ? rows : [''];
}
function setPathRow(f: ConfigField, i: number, v: string): void {
  const rows = pathRows(f);
  rows[i] = v;
  setValue(f.path, rows.filter(Boolean).join('\n'));
}
function addPathRow(f: ConfigField): void {
  const rows = pathRows(f);
  rows.push('');
  setValue(f.path, rows.filter(Boolean).join('\n'));
}
function delPathRow(f: ConfigField, i: number): void {
  const rows = pathRows(f).filter((_, idx) => idx !== i);
  setValue(f.path, rows.filter(Boolean).join('\n'));
}

async function browse(f: ConfigField): Promise<void> {
  try {
    const p = await pickPath(f.browse === 'file' ? 'file' : 'dir');
    if (p) setValue(f.path, p);
  } catch (e) {
    error.value = messageOf(e);
  }
}

async function save(): Promise<void> {
  saving.value = true;
  error.value = '';
  notice.value = '';
  try {
    notice.value = await saveConfigForm(changed.value);
    await load();
    await loadVersions();
  } catch (e) {
    error.value = messageOf(e);
  } finally {
    saving.value = false;
  }
}

async function validate(): Promise<void> {
  try {
    const r = await runCommand(['config', 'validate']);
    notice.value = r.text || t('settings.validateOk');
  } catch (e) {
    error.value = messageOf(e);
  }
}

async function loadVersions(): Promise<void> {
  try {
    const r = await runCommand(['config', 'versions']);
    const d = r.data as { versions?: Record<string, unknown>[]; drift?: boolean } | null;
    versions.value = d?.versions ?? [];
    drift.value = !!d?.drift;
  } catch {
    versions.value = [];
  }
}

async function showDiff(version: string): Promise<void> {
  try {
    const r = await runCommand(['config', 'diff', '--candidate', String(version)]);
    diffText.value = r.text || JSON.stringify(r.data, null, 2);
    diffOpen.value = true;
  } catch (e) {
    error.value = messageOf(e);
  }
}

async function rollback(version: string): Promise<void> {
  try {
    const r = await runCommand(['config', 'rollback', '--version', String(version), '--yes']);
    notice.value = r.text || String(version);
    await load();
    await loadVersions();
  } catch (e) {
    error.value = messageOf(e);
  }
}

onMounted(() => { void load(); void loadVersions(); });
</script>

<template>
  <div class="main__body">
    <div class="toolbar">
      <FhButton :loading="loading" @click="load()">
        <RefreshCw :size="14" /><span>{{ t('settings.reloadFile') }}</span>
      </FhButton>
      <FhButton @click="validate">
        <CheckCircle2 :size="14" /><span>{{ t('settings.validate') }}</span>
      </FhButton>
      <FhButton variant="primary" :loading="saving" :disabled="!dirty" @click="save">
        <Save :size="14" /><span>{{ t('settings.saveChanges') }}</span>
      </FhButton>
      <span v-if="dirty" class="badge badge--off">{{ Object.keys(changed).length }}</span>
    </div>

    <p v-if="error" class="notice notice--err">{{ error }}</p>
    <p v-else-if="notice" class="notice">{{ notice }}</p>

    <p v-if="form" class="dlg-source mono">
      {{ form.path }} 路 {{ form.size }} B 路 {{ form.modified }}
      <template v-if="!form.writable"> 路 {{ t('settings.readonly') }}</template>
    </p>

    <div v-if="loading && !form" class="notice">{{ t('settings.readingConfig') }}</div>

    <!-- 读取失败或拿不到字段时给明确结论，避免只剩一张空白页 -->
    <div v-if="!loading && !form" class="card">
      <div class="card-head"><h3>{{ t('settings.readFailedTitle') }}</h3></div>
      <p class="field-note">{{ t('settings.readFailedBody') }}</p>
    </div>

    <div v-for="g in groups" :key="g.name" class="card">
      <div class="card-head">
        <h3>{{ g.name }}</h3>
      </div>
      <div v-for="f in g.fields" :key="f.path" class="cfg-row" :class="{ 'cfg-row--missing': f.missing }">
        <label class="cfg-row__label">
          <span>{{ f.label }}</span>
          <small v-if="f.help">{{ f.help }}</small>
        </label>
        <div class="cfg-row__control">
          <!-- 多选 -->
          <div v-if="f.type === 'multiselect'" class="choice-group">
            <label v-for="o in (f.options ?? [])" :key="o" class="mini-field">
              <input type="checkbox" :checked="multiChecked(f, o)" @change="toggleMulti(f, o)" />
              <span>{{ o }}</span>
            </label>
          </div>
          <!-- 寮€鍏?-->
          <label v-else-if="f.type === 'bool'" class="mini-field">
            <input
              type="checkbox" :checked="values[f.path] === 'true'"
              @change="setValue(f.path, ($event.target as HTMLInputElement).checked ? 'true' : 'false')"
            />
            <span>{{ t('settings.formOn') }}</span>
          </label>
          <!-- 下拉 -->
          <select
            v-else-if="f.type === 'select'"
            :value="values[f.path]"
            @change="setValue(f.path, ($event.target as HTMLSelectElement).value)"
          >
            <option v-for="o in (f.options ?? [])" :key="o" :value="o">
              {{ o === '' ? t('settings.formUnset') : o }}
            </option>
          </select>
          <!-- 路径列表 -->
          <div v-else-if="f.type === 'pathlist'" class="path-list">
            <div v-for="(row, i) in pathRows(f)" :key="i" class="path-row">
              <input type="text" :value="row" spellcheck="false"
                     @input="setPathRow(f, i, ($event.target as HTMLInputElement).value)" />
              <FhButton size="small" @click="browse(f)">…</FhButton>
              <FhButton size="small" @click="delPathRow(f, i)"><X :size="12" /></FhButton>
            </div>
            <FhButton size="small" @click="addPathRow(f)">
              <Plus :size="12" /><span>{{ t('settings.pathAdd') }}</span>
            </FhButton>
          </div>
          <!-- 多行 -->
          <textarea
            v-else-if="f.type === 'list' || f.type === 'textarea'"
            rows="3" spellcheck="false" :value="values[f.path]"
            @input="setValue(f.path, ($event.target as HTMLTextAreaElement).value)"
          />
          <!-- 鍗曡矾寰?-->
          <div v-else-if="f.browse" class="inline-fields">
            <input type="text" spellcheck="false" :value="values[f.path]"
                   @input="setValue(f.path, ($event.target as HTMLInputElement).value)" />
            <FhButton size="small" @click="browse(f)">{{ t('settings.browse') }}</FhButton>
          </div>
          <!-- 鏅€氭枃鏈?/ 鏁板瓧 / 閭 -->
          <input
            v-else
            :type="f.type === 'number' ? 'number' : (f.type === 'email' ? 'email' : 'text')"
            :value="values[f.path]"
            @input="setValue(f.path, ($event.target as HTMLInputElement).value)"
          />
        </div>
      </div>
    </div>

    <!-- 配置版本 -->
    <div class="card">
      <div class="card-head">
        <h3>{{ t('settings.versionsTitle') }}</h3>
        <FhButton size="small" @click="loadVersions">{{ t('common.refresh') }}</FhButton>
      </div>
      <p v-if="drift" class="notice">{{ t('settings.driftNote') }}</p>
      <div class="kv-table">
        <div v-if="!versions.length" class="ver-row">
          <span class="k">—</span><span class="v">{{ t('settings.versionsEmpty') }}</span>
        </div>
        <div v-for="v in versions" :key="String(v.version)" class="ver-row">
          <span class="k mono">
            {{ v.version }}
            <span v-if="v.active" class="badge badge--ok">{{ t('settings.current') }}</span>
          </span>
          <span class="v muted">
            {{ [v.created_at, v.created_by, v.summary].filter(Boolean).join(' · ') }}
          </span>
          <span class="n">
            <FhButton size="small" @click="showDiff(String(v.version))">{{ t('settings.diff') }}</FhButton>
            <FhButton v-if="!v.active" size="small" @click="rollback(String(v.version))">
              {{ t('settings.rollback') }}
            </FhButton>
          </span>
        </div>
      </div>
      <details v-if="diffOpen" class="raw-details" open>
        <summary>{{ t('settings.diff') }}</summary>
        <pre class="cmd-result">{{ diffText }}</pre>
      </details>
    </div>
  </div>
</template>
