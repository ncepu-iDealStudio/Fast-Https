<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';
import { RefreshCw, RotateCcw, ChevronDown } from '@lucide/vue';
import { useI18n } from '../../composables/useI18n';
import { queryLogs } from '../../services/api/log';
import { messageOf } from '../../services/api/base';
import FhButton from '../../components/ui/FhButton.vue';
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
const statusText = ref('');

async function load(append = false): Promise<void> {
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
      until: append ? (filters.until || '') : filters.until,
      cursor: append ? cursor.value : '',
    });
    rows.value = append ? [...rows.value, ...page.lines] : page.lines;
    hasMore.value = page.hasMore;
    cursor.value = page.nextUntil ?? '';
    statusText.value = `${rows.value.length}${page.hasMore ? ' +' : ''}`;
  } catch (e) {
    error.value = messageOf(e);
    rows.value = append ? rows.value : [];
  } finally {
    loading.value = false;
  }
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

onMounted(() => { void load(); });
</script>

<template>
  <div class="main__body">
    <p class="lede" v-html="t('logs.lede')" />

    <div class="toolbar">
      <button
        v-for="k in KINDS" :key="k.key"
        class="tab" :class="{ 'tab--on': filters.kind === k.key }"
        @click="pickKind(k.key)"
      >{{ t(k.label) }}</button>
      <FhButton :loading="loading" @click="load()">
        <RefreshCw :size="14" /><span>{{ t('common.refresh') }}</span>
      </FhButton>
      <FhButton @click="reset">
        <RotateCcw :size="14" /><span>{{ t('logs.reset') }}</span>
      </FhButton>
      <span v-if="statusText" class="muted mono">{{ statusText }}</span>
    </div>

    <div class="filter-bar">
      <input v-model="filters.level" type="text" :placeholder="t('logs.levelPh')" @keyup.enter="load()" />
      <input v-model="filters.status" type="text" :placeholder="t('logs.statusAria')" @keyup.enter="load()" />
      <input v-model="filters.host" type="text" :placeholder="t('logs.hostPh')" @keyup.enter="load()" />
      <input v-model="filters.method" type="text" :placeholder="t('logs.methodPh')" @keyup.enter="load()" />
      <input v-model="filters.pathPrefix" type="text" :placeholder="t('logs.pathPh')" @keyup.enter="load()" />
      <FhButton size="small" @click="load()">{{ t('logs.apply') }}</FhButton>
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

    <div class="logview">
      <div v-if="!rows.length && !loading" class="table__empty">{{ t('logs.empty') }}</div>
      <div
        v-for="(e, i) in rows" :key="i"
        class="logview__row" :class="`logview__row--${e.level || 'plain'}`"
        :title="e.raw || e.message"
      >{{ e.message || e.raw }}</div>
    </div>

    <div v-if="hasMore" class="toolbar" style="justify-content: center; margin-top: 12px">
      <FhButton :loading="loading" @click="load(true)">{{ t('logs.more') }}</FhButton>
    </div>
  </div>
</template>
