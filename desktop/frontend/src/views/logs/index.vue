<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, reactive, ref, watch } from 'vue';
import { RefreshCw, RotateCcw, ChevronDown, Pause, Play, Search } from '@lucide/vue';
import { useI18n } from '../../composables/useI18n';
import { queryLogs } from '../../services/api/log';
import { messageOf } from '../../services/api/base';
import type { LogEntry } from '../../types/model';

const { t } = useI18n();

const KINDS = [
  { key: 'access', label: 'logs.access' },
  { key: 'error', label: 'logs.error' },
  { key: 'system', label: 'logs.system' },
  { key: 'safe', label: 'logs.safe' },
];

const filters = reactive({
  kind: 'access',
  level: '',
  status: '',
  host: '',
  method: '',
  pathPrefix: '',
  reason: '',
  since: '',
  until: '',
});
const advanced = ref(false);
const rows = ref<LogEntry[]>([]);
const hasMore = ref(false);
const cursor = ref('');
const loading = ref(false);
const error = ref('');
const live = ref(true);

const viewport = ref<HTMLElement | null>(null);
let liveTimer: number | null = null;

/** 日志时间只取 HH:MM:SS.mmm，日期在 title 里给全。 */
function timeText(s?: string): string {
  const m = String(s || '').match(/T(\d{2}:\d{2}:\d{2})(?:\.(\d{1,3}))?/);
  return m ? m[1] + (m[2] ? '.' + m[2] : '') : (s || '—');
}

/** 状态码决定行的严重程度：5xx 报错、4xx 警告、其余正常。 */
function rowTone(e: LogEntry): 'ok' | 'warn' | 'err' | '' {
  const code = Number(String(e.status || '').replace(/\D.*$/, ''));
  if (!code) return e.level === 'error' ? 'err' : '';
  if (code >= 500) return 'err';
  if (code >= 400) return 'warn';
  return 'ok';
}

async function load(append = false): Promise<void> {
  if (loading.value) return;
  loading.value = true;
  error.value = '';
  try {
    const page = await queryLogs({
      kind: filters.kind,
      limit: 100,
      level: filters.level,
      status: filters.status,
      host: filters.host,
      method: filters.method,
      pathPrefix: filters.pathPrefix,
      reason: filters.reason,
      since: filters.since,
      until: filters.until,
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

/** 滚到底部附近自动续拉（与原版一致：距底 < 120px 触发）。 */
function onScroll(): void {
  const el = viewport.value;
  if (!el || loading.value || !hasMore.value) return;
  if (el.scrollHeight - el.scrollTop - el.clientHeight < 120) void load(true);
}

function pickKind(k: string): void {
  filters.kind = k;
  void load();
}

function reset(): void {
  Object.assign(filters, {
    level: '', status: '', host: '', method: '', pathPrefix: '',
    reason: '', since: '', until: '',
  });
  void load();
}

function setLive(on: boolean): void {
  live.value = on;
  if (liveTimer) { window.clearInterval(liveTimer); liveTimer = null; }
  if (on) liveTimer = window.setInterval(() => { void load(); }, 5000);
}

const statusText = computed(() => {
  const n = rows.value.length;
  if (loading.value && !n) return t('logs.querying');
  return hasMore.value ? t('logs.statusLoaded', { n }) : t('logs.statusOldest');
});

watch(live, (on) => setLive(on));

onMounted(() => {
  void load();
  setLive(live.value);
});
onUnmounted(() => { if (liveTimer) window.clearInterval(liveTimer); });

// 换筛选后把视口滚回顶部，避免停在上一批的末尾
watch(() => filters.kind, () => nextTick(() => { if (viewport.value) viewport.value.scrollTop = 0; }));
</script>

<template>
  <div class="main__body logs-page">
    <p class="lede" v-html="t('logs.lede')" />

    <div class="toolbar">
      <div class="seg">
        <button
          v-for="k in KINDS" :key="k.key"
          class="seg__item" :class="{ 'seg__item--on': filters.kind === k.key }"
          @click="pickKind(k.key)"
        >{{ t(k.label) }}</button>
      </div>
      <button class="icon-btn" :title="t('common.refresh')" :disabled="loading" @click="load()">
        <RefreshCw :size="14" />
      </button>
      <button class="icon-btn" :title="t('logs.reset')" @click="reset">
        <RotateCcw :size="14" />
      </button>
      <button
        class="icon-btn live-btn" :class="{ 'live-btn--on': live }"
        :title="live ? t('logs.autoRefreshOn') : t('logs.autoRefreshOff')"
        @click="setLive(!live)"
      >
        <component :is="live ? Pause : Play" :size="14" />
      </button>
      <span class="logs-count mono">{{ statusText }}</span>
    </div>

    <div class="filter-bar">
      <input v-model="filters.level" type="text" :placeholder="t('logs.levelPh')" @keyup.enter="load()" />
      <input v-model="filters.status" type="text" :placeholder="t('logs.statusAria')" @keyup.enter="load()" />
      <input v-model="filters.host" type="text" :placeholder="t('logs.hostPh')" @keyup.enter="load()" />
      <input v-model="filters.method" type="text" :placeholder="t('logs.methodPh')" @keyup.enter="load()" />
      <input v-model="filters.pathPrefix" type="text" :placeholder="t('logs.pathPh')" @keyup.enter="load()" />
      <button class="icon-btn" :title="t('logs.apply')" @click="load()"><Search :size="14" /></button>
    </div>

    <button class="filter-toggle" @click="advanced = !advanced">
      <ChevronDown :size="12" :style="{ transform: advanced ? 'rotate(180deg)' : 'none' }" />
      <span>{{ advanced ? t('logs.fewerFilters') : t('logs.moreFilters') }}</span>
    </button>

    <div v-if="advanced" class="filter-bar">
      <input v-model="filters.reason" type="text" :placeholder="t('logs.reasonPh')" @keyup.enter="load()" />
      <input v-model="filters.since" type="text" :placeholder="t('logs.sincePh')" @keyup.enter="load()" />
      <input v-model="filters.until" type="text" :placeholder="t('logs.untilPh')" @keyup.enter="load()" />
    </div>

    <p v-if="error" class="notice notice--err">{{ error }}</p>

    <!-- 结构化日志表：一行一条记录，按列对齐，状态色编码严重程度。
         表头与行放在同一个滚动容器内，宽度计算完全一致，不会错列。 -->
    <div class="logtable">
      <div ref="viewport" class="logtable__body" @scroll.passive="onScroll">
        <div class="logtable__head">
          <span>time</span>
          <span>lvl</span>
          <span>method</span>
          <span>path</span>
          <span>status</span>
          <span>size</span>
          <span>host / peer</span>
        </div>
        <div v-if="!rows.length && !loading" class="table__empty">{{ t('logs.empty') }}</div>
        <div
          v-for="(e, i) in rows" :key="`${e.time}-${i}`"
          class="logrow" :class="`logrow--${rowTone(e)}`"
          :title="e.raw || e.message"
        >
          <span class="logrow__time mono">{{ timeText(e.time) }}</span>
          <span class="logrow__lvl mono">{{ e.level || '—' }}</span>
          <span class="logrow__method mono">{{ e.method || '—' }}</span>
          <span class="logrow__path mono">{{ e.path || e.message || '—' }}</span>
          <span class="logrow__status mono">{{ e.status || '—' }}</span>
          <span class="logrow__size mono">{{ e.size || '—' }}</span>
          <span class="logrow__host mono">{{ e.host || '—' }}<template v-if="e.remoteAddr"> · {{ e.remoteAddr }}</template></span>
        </div>
        <div v-if="hasMore" class="logtable__more">
          <button class="icon-btn" :title="t('logs.more')" :disabled="loading" @click="load(true)"><ChevronDown :size="14" /></button>
        </div>
      </div>
    </div>
  </div>
</template>
