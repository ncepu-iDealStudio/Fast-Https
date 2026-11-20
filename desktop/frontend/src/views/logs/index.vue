<script setup lang="ts">
/** 日志页编排壳：筛选状态 / 分页 / 自动刷新，界面交给 LogToolbar + LogTable。 */
import { computed, nextTick, reactive, ref } from 'vue';
import { useI18n } from '../../composables/useI18n';
import { usePolling } from '../../composables/usePolling';
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
// 时间范围默认是「最近 24 小时」：当前时间往前推 1 天。
// 窗口跟着当前时间滚动，否则页面每 5 秒自动刷新时，固定在打开时刻的截止会挡住新日志。
// 用户手动改过起止后就交给用户，不再自动滚动。
let rollingRange = true;
function lastDayRange(): { since: string; until: string } {
  const now = Date.now();
  return {
    since: new Date(now - 24 * 60 * 60 * 1000).toISOString(),
    until: new Date(now).toISOString(),
  };
}
const defaultRange = lastDayRange();
const filters = reactive({
  kind: 'access', level: '', status: '', host: '', method: '',
  pathPrefix: '', reason: '', rejectReason: '',
  since: defaultRange.since, until: defaultRange.until,
});
const advanced = ref(false);
const rows = ref<LogEntry[]>([]);
const hasMore = ref(false);
const cursor = ref('');
const loading = ref(false);
const error = ref('');
// 用户滚出顶部后暂停自动刷新，避免 5 秒一次把正在看的旧日志替换掉。
const scrolled = ref(false);
// 请求序号：筛选快速切换时，旧请求的结果不再覆盖新结果。
let seq = 0;

const statusText = computed(() => {
  const n = rows.value.length;
  if (loading.value && !n) return t('logs.querying');
  return hasMore.value ? t('logs.statusLoaded', { n }) : t('logs.statusOldest');
});

async function load(append = false): Promise<void> {
  // 追加更多时仍串行，避免同一页被重复追加；重新查询允许并发，旧结果由 seq 丢弃。
  if (append && loading.value) return;
  const my = ++seq;
  // 翻页沿用同一窗口，只有重新查询时才把默认窗口推到当前时间。
  if (!append && rollingRange) Object.assign(filters, lastDayRange());
  loading.value = true;
  error.value = '';
  try {
    const page = await queryLogs({
      kind: filters.kind, limit: 100, level: filters.level, status: filters.status,
      host: filters.host, method: filters.method, pathPrefix: filters.pathPrefix,
      reason: filters.reason, rejectReason: filters.rejectReason,
      since: filters.since, until: filters.until,
      cursor: append ? cursor.value : '',
    });
    if (my !== seq) return;
    rows.value = append ? [...rows.value, ...page.lines] : page.lines;
    hasMore.value = page.hasMore;
    cursor.value = page.nextUntil ?? '';
  } catch (e) {
    if (my !== seq) return;
    error.value = messageOf(e);
    if (!append) rows.value = [];
  } finally {
    if (my === seq) loading.value = false;
  }
}

function onScroll(e: Event): void {
  const el = e.target as HTMLElement;
  if (!el) return;
  scrolled.value = el.scrollTop > 0;
  if (loading.value || !hasMore.value) return;
  if (el.scrollHeight - el.scrollTop - el.clientHeight < 120) void load(true);
}
function pickKind(k: string): void {
  filters.kind = k;
  scrolled.value = false;
  void load();
  void nextTick(() => { const b = document.querySelector('.logtable__body'); if (b) b.scrollTop = 0; });
}
function reset(): void {
  rollingRange = true;
  scrolled.value = false;
  Object.assign(filters, {
    level: '', status: '', host: '', method: '', pathPrefix: '', reason: '', rejectReason: '',
    ...lastDayRange(),
  });
  void load();
}
function onFilters(v: typeof filters): void {
  // 用户改过起止就停掉滚动，避免下次刷新把他们选的时刻覆盖掉。
  if (v.since !== filters.since || v.until !== filters.until) rollingRange = false;
  Object.assign(filters, v);
}

// 日志本身持续写入：5 秒刷新一次最新内容；用户滚出顶部时暂停，避免打断阅读。
usePolling(() => {
  if (!scrolled.value) void load();
}, 5000, computed(() => true));
</script>

<template>
  <div class="main__body logs-page">

    <LogToolbar
      :filters="filters" :advanced="advanced"
      :status-text="statusText" :kinds="KINDS"
      @update:filters="onFilters" @update:advanced="advanced = $event"
      @pick-kind="pickKind" @reset="reset" @apply="load()"
    />

    <p v-if="error" class="notice notice--err">{{ error }}</p>

    <LogTable
      :rows="rows" :loading="loading" :has-more="hasMore"
      @scroll="onScroll" @more="load(true)"
    />
  </div>
</template>
