<script setup lang="ts">
/** 操作记录表格：五列 + 点行展开详情 slot。 */
import { RefreshCw } from '@lucide/vue';
import { useI18n } from '../../../composables/useI18n';

interface Op {
  operation_id?: string; command?: string; status?: string;
  created_at?: string; version?: string; restore_point?: string;
  result?: Record<string, unknown>;
}
defineProps<{ rows: Op[]; loading: boolean; filter: string; openId: string }>();
const emit = defineEmits<{ refresh: []; 'update:filter': [v: string]; toggle: [id: string]; more: [] }>();
const { t } = useI18n();

const STATUS: Record<string, [string, string]> = {
  succeeded: ['ok', 'ops.succeeded'], failed: ['err', 'ops.failed'],
  recovery_required: ['err', 'ops.recoveryRequired'], running: ['info', 'ops.running'],
  queued: ['off', 'ops.queued'], cancelled: ['off', 'ops.cancelled'], unknown: ['off', 'status.unknown'],
};
function statusOf(op: Op): string {
  const key = STATUS[op.status ?? '']?.[1];
  return key ? t(key) : (op.status ?? '—');
}
function fmtTime(s?: string): string { return (s ?? '—').replace('T', ' ').slice(0, 19); }
function field(op: Op, key: string): string {
  const v = op.result?.[key];
  if (v === undefined || v === null || v === '') return '';
  return typeof v === 'string' ? v : String(v);
}
</script>

<template>
  <div class="toolbar">
    <button class="icon-btn" :title="t('common.refresh')" :disabled="loading" @click="emit('refresh')"><RefreshCw :size="14" /></button>
    <select class="ops-filter" :value="filter" @change="emit('update:filter', ($event.target as HTMLSelectElement).value)">
      <option value="">{{ t('ops.filterAll') }}</option>
      <option v-for="(v, k) in STATUS" :key="k" :value="k">{{ t(v[1]) }}</option>
    </select>
    <span class="logs-count mono">{{ rows.length }}</span>
  </div>

  <div class="opstable">
    <div class="opstable__head">
      <span>time</span><span>command</span><span>{{ t('common.status') }}</span>
      <span>version</span><span>restore</span>
    </div>
    <div class="opstable__body" @scroll.passive="emit('more')">
      <div v-if="!rows.length && !loading" class="table__empty">{{ t('ops.empty') }}</div>
      <template v-for="op in rows" :key="op.operation_id">
        <div class="opsrow" :class="{ 'opsrow--open': openId === op.operation_id }"
             @click="emit('toggle', op.operation_id ?? '')">
          <span class="opsrow__time mono">{{ fmtTime(op.created_at) }}</span>
          <span class="opsrow__cmd mono">{{ op.command || '—' }}</span>
          <span><span class="badge" :class="STATUS[op.status ?? '']?.[0] === 'ok' ? 'badge--ok' : 'badge--off'">{{ statusOf(op) }}</span></span>
          <span class="opsrow__ver mono">{{ field(op, 'version') || op.version || '—' }}</span>
          <span class="opsrow__restore mono">{{ field(op, 'restore_point') || op.restore_point || '—' }}</span>
        </div>
        <slot name="detail" :op="op" />
      </template>
      <div v-if="rows.length" class="opstable__end">{{ t('logs.statusOldest') }}</div>
    </div>
  </div>
</template>
