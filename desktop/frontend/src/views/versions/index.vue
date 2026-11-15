<script setup lang="ts">
/** 配置版本页编排壳：列表 / 对比 / 回滚 / 漂移提示。 */
import { onMounted, onUnmounted, ref } from 'vue';
import { AlertTriangle } from '@lucide/vue';
import { useI18n } from '../../composables/useI18n';
import { runCommand } from '../../services/api/command';
import { messageOf } from '../../services/api/base';
import VersionList from './components/VersionList.vue';
import DiffTable from './components/DiffTable.vue';

interface Version {
  version?: string; active?: boolean; created_at?: string;
  created_by?: string; summary?: string; parent?: string;
}
interface Change { path?: string; op?: string; value?: unknown; old?: unknown; new?: unknown }
interface DiffResult { identical?: boolean; changes?: Change[] }

const { t } = useI18n();
const versions = ref<Version[]>([]);
const drift = ref(false);
const loading = ref(false);
const error = ref('');
const notice = ref('');
const diff = ref<DiffResult | null>(null);
const diffVersion = ref('');

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

async function showDiff(version: string): Promise<void> {
  diffVersion.value = version;
  diff.value = null;
  error.value = '';
  try {
    const r = await runCommand(['config', 'diff', '--candidate', version]);
    diff.value = (r.data ?? null) as DiffResult | null;
  } catch (e) {
    error.value = messageOf(e);
  }
}

async function rollback(version: string): Promise<void> {
  notice.value = '';
  error.value = '';
  try {
    const r = await runCommand(['config', 'rollback', '--version', version, '--yes']);
    notice.value = r.text || version;
    diffVersion.value = '';
    await load();
  } catch (e) {
    error.value = messageOf(e);
  }
}

onMounted(() => {
  void load();
  // 自动刷新：配置版本随事务新增，20 秒一次。
  const timer = window.setInterval(() => { void load(); }, 20000);
  onUnmounted(() => window.clearInterval(timer));
});
</script>

<template>
  <div class="main__body versions-page">
    <div class="toolbar">
      <span class="logs-count mono">{{ versions.length }}</span>
    </div>

    <div v-if="drift" class="alert-strip">
      <span class="alert-strip__title"><AlertTriangle :size="12" /> {{ t('settings.driftNote') }}</span>
    </div>
    <p v-if="error" class="notice notice--err">{{ error }}</p>
    <p v-else-if="notice" class="notice">{{ notice }}</p>

    <VersionList :versions="versions" :loading="loading" @diff="showDiff" @rollback="rollback" />

    <DiffTable
      v-if="diffVersion && diff"
      :version="diffVersion" :changes="diff.changes ?? []" :identical="!!diff.identical"
      @close="diffVersion = ''"
    />
  </div>
</template>
