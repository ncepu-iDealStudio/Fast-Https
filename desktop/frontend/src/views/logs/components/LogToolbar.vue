<script setup lang="ts">
/** 日志页工具栏：四类切换 / 重置 / 筛选框 / 高级筛选。 */
import { RotateCcw, ChevronDown, Search } from '@lucide/vue';
import { useI18n } from '../../../composables/useI18n';

interface Filters {
  kind: string; level: string; status: string; host: string;
  method: string; pathPrefix: string; reason: string; rejectReason: string;
  since: string; until: string;
}
const props = defineProps<{
  filters: Filters;
  advanced: boolean;
  statusText: string;
  kinds: { key: string; label: string }[];
}>();
const emit = defineEmits<{
  'update:filters': [v: Filters];
  'update:advanced': [v: boolean];
  'pick-kind': [k: string];
  reset: [];
  apply: [];
}>();
const { t } = useI18n();

// 与 R-10 的 reject_reason 枚举保持一致；空值表示不过滤。
const REJECT_REASONS = [
  'path_traversal', 'invalid_path', 'hidden_path', 'symlink_rejected',
  'header_too_large', 'body_too_large', 'unknown_sni', 'blacklist',
  'rate_limit', 'location_limit', 'waf', 'host_check', 'compression_skipped',
];
// 安全日志 reason 字段的固定取值（safe 模块 + Host 校验）。
const SAFE_REASONS = ['blacklist', 'rate_limit', 'location_limit', 'host_check', 'control', 'waf'];
const LEVELS = ['debug', 'info', 'warn', 'error'];
// 状态码支持单值、类别与区间；这里只列固定项，选完立即查询。
const STATUSES = ['2xx', '3xx', '4xx', '5xx', '200', '204', '301', '302', '304',
  '400', '401', '403', '404', '405', '413', '429', '431', '500', '502', '503', '504'];
const METHODS = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS', 'TRACE', 'CONNECT'];

function set(key: keyof Filters, value: string): void {
  emit('update:filters', { ...props.filters, [key]: value });
}

// 下拉都是固定枚举，选完即应用，不用再点搜索。
function pick(key: keyof Filters, e: Event): void {
  set(key, (e.target as HTMLSelectElement).value);
  emit('apply');
}

// 原因代码转中文；字典缺项时回退原代码，避免出现空选项。
function reasonLabel(code: string): string {
  const key = `logs.reasonCode.${code}`;
  const text = t(key);
  return text === key ? code : text;
}

function levelLabel(level: string): string {
  const key = `logs.levelOption.${level}`;
  const text = t(key);
  return text === key ? level : `${level} · ${text}`;
}

// 2xx / 4xx 这类类别加中文说明；具体状态码保持数字。
function statusLabel(code: string): string {
  if (code.length === 3 && code[1] === 'x') {
    const key = `logs.statusClass.${code}`;
    const text = t(key);
    return text === key ? code : `${code} · ${text}`;
  }
  return code;
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
    <span class="logs-count mono">{{ statusText }}</span>
  </div>

  <div class="filter-bar">
    <select :value="filters.level" :title="t('logs.level')" @change="pick('level', $event)">
      <option value="">{{ t('logs.levelAll') }}</option>
      <option v-for="l in LEVELS" :key="l" :value="l">{{ levelLabel(l) }}</option>
    </select>
    <select :value="filters.status" :title="t('logs.status')" @change="pick('status', $event)">
      <option value="">{{ t('logs.statusAll') }}</option>
      <option v-for="s in STATUSES" :key="s" :value="s">{{ statusLabel(s) }}</option>
    </select>
    <input :value="filters.host" type="text" :placeholder="t('logs.hostPh')"
           @input="set('host', ($event.target as HTMLInputElement).value)" @keyup.enter="emit('apply')" />
    <select :value="filters.method" :title="t('logs.method')" @change="pick('method', $event)">
      <option value="">{{ t('logs.methodAll') }}</option>
      <option v-for="m in METHODS" :key="m" :value="m">{{ m }}</option>
    </select>
    <input :value="filters.pathPrefix" type="text" :placeholder="t('logs.pathPh')"
           @input="set('pathPrefix', ($event.target as HTMLInputElement).value)" @keyup.enter="emit('apply')" />
    <button class="icon-btn" :title="t('logs.apply')" @click="emit('apply')"><Search :size="14" /></button>
  </div>

  <button class="filter-toggle" @click="emit('update:advanced', !advanced)">
    <ChevronDown :size="12" :style="{ transform: advanced ? 'rotate(180deg)' : 'none' }" />
    <span>{{ advanced ? t('logs.fewerFilters') : t('logs.moreFilters') }}</span>
  </button>

  <div v-if="advanced" class="filter-bar">
    <select :value="filters.reason" :title="t('logs.reason')" @change="pick('reason', $event)">
      <option value="">{{ t('logs.reasonAll') }}</option>
      <option v-for="r in SAFE_REASONS" :key="r" :value="r">{{ reasonLabel(r) }}</option>
    </select>
    <select :value="filters.rejectReason" :title="t('logs.rejectReason')" @change="pick('rejectReason', $event)">
      <option value="">{{ t('logs.rejectReasonAll') }}</option>
      <option v-for="r in REJECT_REASONS" :key="r" :value="r">{{ reasonLabel(r) }}</option>
    </select>
    <input :value="filters.since" type="text" :placeholder="t('logs.sincePh')"
           @input="set('since', ($event.target as HTMLInputElement).value)" @keyup.enter="emit('apply')" />
    <input :value="filters.until" type="text" :placeholder="t('logs.untilPh')"
           @input="set('until', ($event.target as HTMLInputElement).value)" @keyup.enter="emit('apply')" />
  </div>
</template>
