<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { RefreshCw, GitCompareArrows, Undo2, AlertTriangle } from '@lucide/vue';
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

const versions = ref<Version[]>([]);
const drift = ref(false);
const loading = ref(false);
const error = ref('');
const notice = ref('');

const diffText = ref('');
const diffVersion = ref('');
const diffOpen = ref(false);

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

async function showDiff(version: string): Promise<void> {
  diffVersion.value = version;
  diffOpen.value = true;
  diffText.value = '';
  error.value = '';
  try {
    const r = await runCommand(['config', 'diff', '--candidate', version]);
    diffText.value = r.text || JSON.stringify(r.data ?? {}, null, 2);
  } catch (e) {
    error.value = messageOf(e);
    diffOpen.value = false;
  }
}

async function rollback(version: string): Promise<void> {
  notice.value = '';
  error.value = '';
  try {
    const r = await runCommand(['config', 'rollback', '--version', version, '--yes']);
    notice.value = r.text || version;
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
      <FhButton :loading="loading" @click="load()">
        <RefreshCw :size="14" /><span>{{ t('common.refresh') }}</span>
      </FhButton>
      <span class="logs-count mono">{{ versions.length }}</span>
    </div>

    <!-- 配置漂移：磁盘上的文件与当前版本不一致 -->
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
      <div v-for="v in versions" :key="String(v.version)" class="verlist__row" :class="{ 'verlist__row--active': v.active }">
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

    <!-- diff 结果：右侧不抢位置，直接在本页底部展开 -->
    <div v-if="diffOpen" class="card">
      <div class="card-head">
        <h3>{{ t('settings.diff') }} · {{ diffVersion }}</h3>
        <FhButton size="small" @click="diffOpen = false">{{ t('common.close') }}</FhButton>
      </div>
      <pre class="cmd-result">{{ diffText || t('settings.diffNone') }}</pre>
    </div>
  </div>
</template>
