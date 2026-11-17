<script setup lang="ts">
/** 日志页工具栏：四类切换 / 刷新 / 重置 / 实时 / 筛选框 / 高级筛选。 */
import { RotateCcw, ChevronDown, Pause, Play, Search } from '@lucide/vue';
import { useI18n } from '../../../composables/useI18n';

interface Filters {
  kind: string; level: string; status: string; host: string;
  method: string; pathPrefix: string; reason: string; rejectReason: string;
  since: string; until: string;
}
const props = defineProps<{
  filters: Filters;
  advanced: boolean;
  loading: boolean;
  live: boolean;
  statusText: string;
  kinds: { key: string; label: string }[];
}>();
const emit = defineEmits<{
  'update:filters': [v: Filters];
  'update:advanced': [v: boolean];
  'pick-kind': [k: string];
  refresh: [];
  reset: [];
  'toggle-live': [];
  apply: [];
}>();
const { t } = useI18n();

// 与 R-10 的 reject_reason 枚举保持一致；空值表示不过滤。
const REJECT_REASONS = [
  'path_traversal', 'hidden_path', 'symlink_rejected', 'header_too_large',
  'body_too_large', 'unknown_sni', 'blacklist', 'rate_limit',
  'location_limit', 'waf', 'host_check', 'compression_skipped',
];

function set(key: keyof Filters, value: string): void {
  emit('update:filters', { ...props.filters, [key]: value });
}
</script>

<template>
  <div class="toolbar">
    <div class="seg">
      <button
        v-for="k in kinds" :key="k.key"
        class="seg__item" :class="{ 'seg__item--on': filters.kind === k.key }"
        @click="emit('pick-kind', k.key)"
      >{{ t(k.label) }}</button>
    </div>
    <button class="icon-btn" :title="t('logs.reset')" @click="emit('reset')">
      <RotateCcw :size="14" />
    </button>
    <button
      class="icon-btn live-btn" :class="{ 'live-btn--on': live }"
      :title="live ? t('logs.autoRefreshOn') : t('logs.autoRefreshOff')"
      @click="emit('toggle-live')"
    >
      <component :is="live ? Pause : Play" :size="14" />
    </button>
    <span class="logs-count mono">{{ statusText }}</span>
  </div>

  <div class="filter-bar">
    <input :value="filters.level" type="text" :placeholder="t('logs.levelPh')"
           @input="set('level', ($event.target as HTMLInputElement).value)" @keyup.enter="emit('apply')" />
    <input :value="filters.status" type="text" :placeholder="t('logs.statusAria')"
           @input="set('status', ($event.target as HTMLInputElement).value)" @keyup.enter="emit('apply')" />
    <input :value="filters.host" type="text" :placeholder="t('logs.hostPh')"
           @input="set('host', ($event.target as HTMLInputElement).value)" @keyup.enter="emit('apply')" />
    <input :value="filters.method" type="text" :placeholder="t('logs.methodPh')"
           @input="set('method', ($event.target as HTMLInputElement).value)" @keyup.enter="emit('apply')" />
    <input :value="filters.pathPrefix" type="text" :placeholder="t('logs.pathPh')"
           @input="set('pathPrefix', ($event.target as HTMLInputElement).value)" @keyup.enter="emit('apply')" />
    <button class="icon-btn" :title="t('logs.apply')" @click="emit('apply')"><Search :size="14" /></button>
  </div>

  <button class="filter-toggle" @click="emit('update:advanced', !advanced)">
    <ChevronDown :size="12" :style="{ transform: advanced ? 'rotate(180deg)' : 'none' }" />
    <span>{{ advanced ? t('logs.fewerFilters') : t('logs.moreFilters') }}</span>
  </button>

  <div v-if="advanced" class="filter-bar">
    <input :value="filters.reason" type="text" :placeholder="t('logs.reasonPh')"
           @input="set('reason', ($event.target as HTMLInputElement).value)" @keyup.enter="emit('apply')" />
    <select :value="filters.rejectReason"
            @change="set('rejectReason', ($event.target as HTMLSelectElement).value)">
      <option value="">{{ t('logs.rejectReasonAll') }}</option>
      <option v-for="r in REJECT_REASONS" :key="r" :value="r">{{ r }}</option>
    </select>
    <input :value="filters.since" type="text" :placeholder="t('logs.sincePh')"
           @input="set('since', ($event.target as HTMLInputElement).value)" @keyup.enter="emit('apply')" />
    <input :value="filters.until" type="text" :placeholder="t('logs.untilPh')"
           @input="set('until', ($event.target as HTMLInputElement).value)" @keyup.enter="emit('apply')" />
  </div>
</template>
