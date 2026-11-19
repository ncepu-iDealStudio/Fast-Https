<script setup lang="ts">
/** 主配置页编排壳：读取/校验/保存，字段渲染交给 ConfigField。 */
import { computed, onMounted, ref } from 'vue';
import { RefreshCw, Save, CheckCircle2 } from '@lucide/vue';
import { useI18n } from '../../composables/useI18n';
import { readConfigForm, saveConfigForm } from '../../services/api/log';
import { runCommand } from '../../services/api/command';
import { pickAbsPath, getRuntimePaths } from '../../services/api/app';
import { messageOf } from '../../services/api/base';
import FhButton from '../../components/ui/FhButton.vue';
import ConfigField from './components/ConfigField.vue';
import RuntimePathRow from './components/RuntimePathRow.vue';
import type { ConfigField as Field, ConfigForm } from '../../types/config';

const { t } = useI18n();
const form = ref<ConfigForm | null>(null);
const initial = ref<Record<string, string>>({});
const values = ref<Record<string, string>>({});
const loading = ref(false);
const saving = ref(false);
const error = ref('');
const notice = ref('');
const runtimePid = ref('');
const runtimeLog = ref('');

const groups = computed(() => {
  if (!form.value) return [] as { name: string; fields: Field[] }[];
  const map = new Map<string, Field[]>();
  for (const f of form.value.fields) {
    if (f.path === 'pid' || f.path === 'log_root') continue;
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

function setValue(path: string, v: string): void {
  values.value = { ...values.value, [path]: v };
}

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
    const p = await getRuntimePaths();
    runtimePid.value = p.pid || '';
    runtimeLog.value = p.log || '';
  } catch (e) {
    error.value = messageOf(e);
    form.value = null;
  } finally {
    loading.value = false;
  }
}

async function browse(path: string): Promise<void> {
  const f = form.value?.fields.find((x) => x.path === path);
  try {
    // 从字段当前值打开，选中后填绝对路径；取消不当作错误。
    const p = await pickAbsPath(f?.browse === 'file' ? 'file' : 'dir', values.value[path] ?? '');
    if (p) setValue(path, p);
  } catch (e) {
    const msg = String((e as { message?: string })?.message ?? e ?? '');
    if (!/cancel/i.test(msg)) error.value = messageOf(e);
  }
}

async function save(): Promise<void> {
  saving.value = true;
  error.value = '';
  notice.value = '';
  try {
    notice.value = await saveConfigForm(changed.value);
    await load();
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

onMounted(() => { void load(); });
</script>

<template>
  <div class="main__body">
    <div class="toolbar">
      <button class="icon-btn" :title="t('settings.reloadFile')" :disabled="loading" @click="load()"><RefreshCw :size="14" /></button>
      <button class="icon-btn" :title="t('settings.validate')" @click="validate"><CheckCircle2 :size="14" /></button>
      <FhButton variant="primary" :loading="saving" :disabled="!dirty" @click="save">
        <Save :size="14" /><span>{{ t('settings.saveChanges') }}</span>
      </FhButton>
      <span v-if="dirty" class="badge badge--off">{{ Object.keys(changed).length }}</span>
    </div>

    <p v-if="error" class="notice notice--err">{{ error }}</p>
    <p v-else-if="notice" class="notice">{{ notice }}</p>

    <p v-if="form" class="dlg-source mono">
      {{ form.path }} · {{ form.size }} B · {{ form.modified }}
      <template v-if="!form.writable"> · {{ t('settings.readonly') }}</template>
    </p>

    <div v-if="loading && !form" class="notice">{{ t('settings.readingConfig') }}</div>
    <div v-if="!loading && !form" class="card">
      <div class="card-head"><h3>{{ t('settings.readFailedTitle') }}</h3></div>
      <p class="field-note">{{ t('settings.readFailedBody') }}</p>
    </div>

    <div v-for="g in groups" :key="g.name" class="card">
      <div class="card-head"><h3>{{ g.name }}</h3></div>

      <template v-if="g.name === '基础'">
        <RuntimePathRow
          :label="`pid ${t('settings.readonly')}`" :hint="t('settings.pidReadonlyHint')" :value="runtimePid"
        />
        <RuntimePathRow
          :label="t('settings.logRootLabel')" :hint="t('settings.logRootHint')" :value="runtimeLog"
        />
      </template>

      <ConfigField
        v-for="f in g.fields" :key="f.path"
        :field="f" :value="values[f.path] ?? ''"
        @change="setValue(f.path, $event)" @browse="browse(f.path)"
      />
    </div>
  </div>
</template>
