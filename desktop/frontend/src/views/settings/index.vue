<script setup lang="ts">
/**
 * 主配置页：顶部固定操作条 + 左侧分组锚点目录 + 右侧独立滚动配置卡片。
 * 无论滚到哪个分组，校验与保存始终触手可及；修改过的字段与分组实时标示。
 */
import { computed, onMounted, ref, type Component } from 'vue';
import {
  RefreshCw, Save, CheckCircle2, FileCode2,
  SlidersHorizontal, Shield, Flame, Sparkles, Lock, Network, Globe, Settings2,
} from '@lucide/vue';
import { useI18n } from '../../composables/useI18n';
import { readConfigForm, saveConfigForm } from '../../services/api/log';
import { runCommand } from '../../services/api/command';
import { pickAbsPath, getRuntimePaths } from '../../services/api/app';
import { messageOf } from '../../services/api/base';
import FhButton from '../../components/ui/FhButton.vue';
import ConfigField from './components/ConfigField.vue';
import RuntimePathRow from './components/RuntimePathRow.vue';
import type { ConfigField as Field, ConfigForm } from '../../types/config';

const { t, te } = useI18n();
const form = ref<ConfigForm | null>(null);
const initial = ref<Record<string, string>>({});
const values = ref<Record<string, string>>({});
const loading = ref(false);
const saving = ref(false);
const error = ref('');
const notice = ref('');
const runtimePid = ref('');
const runtimeLog = ref('');
const activeGroup = ref('');
const contentEl = ref<HTMLElement | null>(null);

const GROUP_ICONS: Record<string, Component> = {
  basic: SlidersHorizontal,
  security: Shield,
  waf: Flame,
  acme: Sparkles,
  tls: Lock,
  proxy: Network,
  http: Globe,
};

function iconOf(name: string): Component {
  return GROUP_ICONS[name] ?? Settings2;
}

function msg(key: string, fallback: string): string {
  return te(key) ? t(key) : fallback;
}

function localizeField(f: Field): Field {
  return {
    ...f,
    label: msg(`settings.field.${f.path}.label`, f.label),
    help: msg(`settings.field.${f.path}.help`, f.help ?? ''),
  };
}

const groups = computed(() => {
  if (!form.value) return [] as { name: string; title: string; fields: Field[] }[];
  const map = new Map<string, Field[]>();
  for (const f of form.value.fields) {
    if (f.path === 'pid' || f.path === 'log_root') continue;
    const list = map.get(f.group) ?? [];
    list.push(f);
    map.set(f.group, list);
  }
  return [...map.entries()].map(([name, fields]) => ({
    name,
    title: msg(`settings.group.${name}`, name),
    fields: fields.map(localizeField),
  }));
});

const changed = computed(() => {
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(values.value)) {
    if (v !== initial.value[k]) out[k] = v;
  }
  return out;
});
const changedCount = computed(() => Object.keys(changed.value).length);
const dirty = computed(() => changedCount.value > 0);

function groupDirtyCount(fields: Field[]): number {
  let n = 0;
  for (const f of fields) {
    if (changed.value[f.path] !== undefined) n += 1;
  }
  return n;
}

function setValue(path: string, v: string): void {
  values.value = { ...values.value, [path]: v };
}

function jumpTo(name: string): void {
  activeGroup.value = name;
  const el = contentEl.value?.querySelector<HTMLElement>(`[data-group="${name}"]`);
  el?.scrollIntoView({ behavior: 'smooth', block: 'start' });
}

function onScroll(): void {
  const box = contentEl.value;
  if (!box) return;
  const top = box.getBoundingClientRect().top + 36;
  let current = groups.value[0]?.name ?? '';
  for (const g of groups.value) {
    const el = box.querySelector<HTMLElement>(`[data-group="${g.name}"]`);
    if (el && el.getBoundingClientRect().top <= top) current = g.name;
  }
  if (current) activeGroup.value = current;
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
    if (!activeGroup.value && groups.value.length) {
      activeGroup.value = groups.value[0].name;
    }
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
    const p = await pickAbsPath(f?.browse === 'file' ? 'file' : 'dir', values.value[path] ?? '');
    if (p) setValue(path, p);
  } catch (e) {
    const m = String((e as { message?: string })?.message ?? e ?? '');
    if (!/cancel/i.test(m)) error.value = messageOf(e);
  }
}

async function save(): Promise<void> {
  saving.value = true;
  error.value = '';
  notice.value = '';
  try {
    const n = changedCount.value;
    await saveConfigForm(changed.value);
    notice.value = t('settings.statusSaved', { n });
    await load();
  } catch (e) {
    error.value = messageOf(e);
  } finally {
    saving.value = false;
  }
}

async function validate(): Promise<void> {
  error.value = '';
  try {
    await runCommand(['config', 'validate']);
    notice.value = t('settings.validateOk');
  } catch (e) {
    error.value = messageOf(e);
  }
}

onMounted(() => { void load(); });
</script>

<template>
  <div class="settings-page">
    <!-- 顶部固定操作条：文件信息 + 校验/重新读取/保存 -->
    <div class="settings-bar">
      <div class="settings-bar__meta">
        <span class="settings-bar__ico"><FileCode2 :size="14" /></span>
        <div class="settings-bar__info">
          <span class="settings-bar__path mono" :title="form?.path">
            {{ form ? form.path : t('settings.configNote') }}
          </span>
          <span v-if="form" class="settings-bar__sub mono">
            {{ form.size }} B · {{ form.modified }}
            <template v-if="!form.writable"> · {{ t('settings.readonly') }}</template>
          </span>
        </div>
      </div>

      <div class="settings-bar__actions">
        <span v-if="dirty" class="settings-bar__dirty">
          {{ t('settings.statusModified', { n: changedCount }) }}
        </span>
        <button
          class="icon-btn"
          :title="t('settings.reloadFile')"
          :aria-label="t('settings.reloadFile')"
          :disabled="loading"
          @click="load()"
        >
          <RefreshCw :size="14" />
        </button>
        <button
          class="icon-btn"
          :title="t('settings.validate')"
          :aria-label="t('settings.validate')"
          @click="validate"
        >
          <CheckCircle2 :size="14" />
        </button>
        <FhButton variant="primary" :loading="saving" :disabled="!dirty" @click="save">
          <Save :size="14" /><span>{{ t('settings.saveChanges') }}</span>
        </FhButton>
      </div>
    </div>

    <p v-if="error" class="notice notice--err">{{ error }}</p>
    <p v-else-if="notice" class="notice">{{ notice }}</p>

    <div v-if="loading && !form" class="notice">{{ t('settings.readingConfig') }}</div>
    <div v-else-if="!loading && !form" class="card">
      <div class="card-head"><h3>{{ t('settings.readFailedTitle') }}</h3></div>
      <p class="field-note">{{ t('settings.readFailedBody') }}</p>
    </div>

    <!-- 双栏：左分组目录，右配置卡片 -->
    <div v-else-if="groups.length" class="settings-body">
      <nav class="settings-toc" :aria-label="t('settings.configTitle')">
        <button
          v-for="g in groups"
          :key="g.name"
          type="button"
          class="settings-toc__item"
          :class="{ 'is-active': activeGroup === g.name }"
          @click="jumpTo(g.name)"
        >
          <component :is="iconOf(g.name)" :size="14" class="settings-toc__ico" />
          <span class="settings-toc__txt">{{ g.title }}</span>
          <span v-if="groupDirtyCount(g.fields) > 0" class="settings-toc__dot" />
          <span v-else class="settings-toc__count">{{ g.fields.length }}</span>
        </button>
      </nav>

      <div ref="contentEl" class="settings-content" @scroll.passive="onScroll">
        <section
          v-for="g in groups"
          :key="g.name"
          :data-group="g.name"
          class="settings-card"
        >
          <div class="settings-card__head">
            <span class="settings-card__ico"><component :is="iconOf(g.name)" :size="14" /></span>
            <h3>{{ g.title }}</h3>
            <span
              class="settings-card__badge"
              :class="{ 'settings-card__badge--dirty': groupDirtyCount(g.fields) > 0 }"
            >
              <template v-if="groupDirtyCount(g.fields) > 0">+{{ groupDirtyCount(g.fields) }}</template>
              <template v-else>{{ g.fields.length }}</template>
            </span>
          </div>

          <template v-if="g.name === 'basic'">
            <RuntimePathRow
              :label="`pid ${t('settings.readonly')}`"
              :hint="t('settings.pidReadonlyHint')"
              :value="runtimePid"
            />
            <RuntimePathRow
              :label="t('settings.logRootLabel')"
              :hint="t('settings.logRootHint')"
              :value="runtimeLog"
            />
          </template>

          <ConfigField
            v-for="f in g.fields"
            :key="f.path"
            :field="f"
            :value="values[f.path] ?? ''"
            :dirty="changed[f.path] !== undefined"
            @change="setValue(f.path, $event)"
            @browse="browse(f.path)"
          />
        </section>
      </div>
    </div>
  </div>
</template>
