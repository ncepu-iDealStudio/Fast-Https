<script setup lang="ts">
/** 日志页编排壳：筛选状态 / 分页 / 实时刷新，界面交给 LogToolbar + LogTable。 */
import { computed, nextTick, onMounted, onUnmounted, reactive, ref } from 'vue';
import { useI18n } from '../../composables/useI18n';
import { queryLogs } from '../../services/api/log';
import { messageOf } from '../../services/api/base';
import type { LogEntry } from '../../types/model';
import LogToolbar from './components/LogToolbar.vue';
import LogTable from './components/LogTable.vue';

const { t } = useI18n();
const KINDS = [
  { key: 'access', label: 'logs.access' }, { key: 'error', label: 'logs.error' },
  { key: 'system', label: 'logs.system' }, { key: 'safe', label: 'logs.safe' },
];
const filters = reactive({
  kind: 'access', level: '', status: '', host: '', method: '',
  pathPrefix: '', reason: '', since: '', until: '',
});
const advanced = ref(false);
const rows = ref<LogEntry[]>([]);
const hasMore = ref(false);
const cursor = ref('');
const loading = ref(false);
const error = ref('');
const live = ref(true);

let liveTimer: number | null = null;

const statusText = computed(() => {
  const n = rows.value.length;
  if (loading.value && !n) return t('logs.querying');
  return hasMore.value ? t('logs.statusLoaded', { n }) : t('logs.statusOldest');
});

async function load(append = false): Promise<void> {
  if (loading.value) return;
  loading.value = true;
  error.value = '';
  try {
    const page = await queryLogs({
      kind: filters.kind, limit: 100, level: filters.level, status: filters.status,
      host: filters.host, method: filters.method, pathPrefix: filters.pathPrefix,
      reason: filters.reason, since: filters.since, until: filters.until,
      cursor: append ? cursor.value : '',
    });
    rows.value = append ? [...rows.value, ...page.lines] : page.lines;
    hasMore.value = page.hasMore;
    cursor.value = page.nextUntil ?? '';
  } catch (e) {
    error.value = messageOf(e);
    if (!append) rows.value = [];
  } finally {
    loading.value = false;
  }
}

function onScroll(e: Event): void {
  const el = e.target as HTMLElement;
  if (!el || loading.value || !hasMore.value) return;
  if (el.scrollHeight - el.scrollTop - el.clientHeight < 120) void load(true);
}
function pickKind(k: string): void {
  filters.kind = k;
  void load();
  void nextTick(() => { const b = document.querySelector('.logtable__body'); if (b) b.scrollTop = 0; });
}
function reset(): void {
  Object.assign(filters, { level: '', status: '', host: '', method: '', pathPrefix: '', reason: '', since: '', until: '' });
  void load();
}
function setLive(on: boolean): void {
  live.value = on;
  if (liveTimer) { window.clearInterval(liveTimer); liveTimer = null; }
  if (on) liveTimer = window.setInterval(() => { void load(); }, 5000);
}
function onFilters(v: typeof filters): void { Object.assign(filters, v); }

onMounted(() => { void load(); setLive(live.value); });
onUnmounted(() => { if (liveTimer) window.clearInterval(liveTimer); });
</script>

<template>
  <div class="main__body logs-page">

    <LogToolbar
      :filters="filters" :advanced="advanced" :loading="loading" :live="live"
      :status-text="statusText" :kinds="KINDS"
      @update:filters="onFilters" @update:advanced="advanced = $event"
      @pick-kind="pickKind" @refresh="load()" @reset="reset"
      @toggle-live="setLive(!live)" @apply="load()"
    />

    <p v-if="error" class="notice notice--err">{{ error }}</p>

    <LogTable
      :rows="rows" :loading="loading" :has-more="hasMore"
      @scroll="onScroll" @more="load(true)"
    />
  </div>
</template>
