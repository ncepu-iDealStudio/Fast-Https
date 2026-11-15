<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { RefreshCw, GitCompareArrows, Undo2, AlertTriangle, X } from '@lucide/vue';
import { useI18n } from '../../composables/useI18n';
import { runCommand } from '../../services/api/command';
import { messageOf } from '../../services/api/base';
import FhButton from '../../components/ui/FhButton.vue';

const { t } = useI18n();

interface Version {
  version?: string;
  active?: boolean;
  created_at?: string;
  created_by?: string;
  summary?: string;
  parent?: string;
}

/** cfgdiff.Result 的镜像：Changes 每条是一个字段路径的增 / 删 / 改。 */
interface Change {
  path?: string;
  op?: string;
  value?: unknown;
  old?: unknown;
  new?: unknown;
}
interface DiffResult {
  identical?: boolean;
  changes?: Change[];
}

const versions = ref<Version[]>([]);
const drift = ref(false);
const loading = ref(false);
const error = ref('');
const notice = ref('');

const diff = ref<DiffResult | null>(null);
const diffVersion = ref('');
const diffLoading = ref(false);

async function load(): Promise<void> {
  loading.value = true;
  error.value = '';
  try {
    const r = await runCommand(['config', 'versions']);
    const d = r.data as { versions?: Version[]; drift?: boolean } | null;
    versions.value = d?.versions ?? [];
    drift.value = !!d?.drift;
  } catch (e) {
    error.value = messageOf(e);
    versions.value = [];
  } finally {
    loading.value = false;
  }
}

function metaText(v: Version): string {
  return [v.created_at, v.created_by, v.summary].filter(Boolean).join(' · ');
}

/** 值转成一行的可读文本：字符串原样，对象/数组压成 JSON。 */
function showVal(v: unknown): string {
  if (v === undefined || v === null) return '—';
  if (typeof v === 'string') return v;
  if (typeof v === 'number' || typeof v === 'boolean') return String(v);
  return JSON.stringify(v);
}

function opLabel(op?: string): string {
  if (op === 'added') return t('settings.diffOpAdded');
  if (op === 'removed') return t('settings.diffOpRemoved');
  return t('settings.diffOpModified');
}

function opClass(op?: string): string {
  if (op === 'added') return 'diffrow--add';
  if (op === 'removed') return 'diffrow--del';
  return 'diffrow--mod';
}

async function showDiff(version: string): Promise<void> {
  diffVersion.value = version;
  diff.value = null;
  diffLoading.value = true;
  error.value = '';
  try {
    const r = await runCommand(['config', 'diff', '--candidate', version]);
    diff.value = (r.data ?? null) as DiffResult | null;
  } catch (e) {
    error.value = messageOf(e);
  } finally {
    diffLoading.value = false;
  }
}

async function rollback(version: string): Promise<void> {
  notice.value = '';
  error.value = '';
  try {
    const r = await runCommand(['config', 'rollback', '--version', version, '--yes']);
    notice.value = r.text || version;
    diff.value = null;
    await load();
  } catch (e) {
    error.value = messageOf(e);
  }
}

onMounted(() => { void load(); });
</script>

<template>
  <div class="main__body versions-page">
    <div class="toolbar">
      <button class="icon-btn" :title="t('common.refresh')" :disabled="loading" @click="load()"><RefreshCw :size="14" /></button>
      <span class="logs-count mono">{{ versions.length }}</span>
    </div>

    <div v-if="drift" class="alert-strip">
      <span class="alert-strip__title">
        <AlertTriangle :size="12" /> {{ t('settings.driftNote') }}
      </span>
    </div>

    <p v-if="error" class="notice notice--err">{{ error }}</p>
    <p v-else-if="notice" class="notice">{{ notice }}</p>

    <p class="field-note">{{ t('settings.versionsNote') }}</p>

    <div class="verlist">
      <div v-if="!versions.length && !loading" class="table__empty">
        {{ t('settings.versionsEmpty') }}
      </div>
      <div
        v-for="v in versions" :key="String(v.version)"
        class="verlist__row" :class="{ 'verlist__row--active': v.active }"
      >
        <span class="verlist__ver mono">
          {{ v.version }}
          <span v-if="v.active" class="badge badge--ok">{{ t('settings.current') }}</span>
        </span>
        <span class="verlist__meta muted">{{ metaText(v) || '—' }}</span>
        <span class="verlist__ops">
          <FhButton size="small" @click="showDiff(String(v.version))">
            <GitCompareArrows :size="12" /><span>{{ t('settings.diff') }}</span>
          </FhButton>
          <FhButton v-if="!v.active" size="small" @click="rollback(String(v.version))">
            <Undo2 :size="12" /><span>{{ t('settings.rollback') }}</span>
          </FhButton>
        </span>
      </div>
    </div>

    <!-- diff 结果：结构化表格，不再丢原始 JSON 给用户 -->
    <div v-if="diffVersion" class="card">
      <div class="card-head">
        <h3>
          {{ t('settings.diff') }} · <span class="mono">{{ diffVersion }}</span>
        </h3>
        <button class="icon-btn" :title="t('common.close')" @click="diffVersion = ''; diff = null"><X :size="14" /></button>
      </div>

      <div v-if="diffLoading" class="notice">{{ t('settings.diffRunning') }}</div>

      <template v-else-if="diff">
        <div v-if="diff.identical" class="notice">{{ t('settings.diffNone') }}</div>
        <div v-else class="difftable">
          <div class="difftable__head">
            <span>{{ t('settings.diffPath') }}</span>
            <span>{{ t('settings.diffChange') }}</span>
            <span>{{ t('settings.diffOld') }}</span>
            <span>{{ t('settings.diffNew') }}</span>
          </div>
          <div
            v-for="(c, i) in diff.changes" :key="i"
            class="diffrow" :class="opClass(c.op)"
          >
            <span class="diffrow__path mono">{{ c.path || '—' }}</span>
            <span><span class="diffrow__op">{{ opLabel(c.op) }}</span></span>
            <span class="diffrow__val mono">{{ showVal(c.old ?? (c.op === 'added' ? undefined : c.value)) }}</span>
            <span class="diffrow__val mono">{{ showVal(c.new ?? (c.op === 'added' ? c.value : undefined)) }}</span>
          </div>
        </div>
      </template>
    </div>
  </div>
</template>
