<script setup lang="ts">
/** 操作表：版本模式看版本与回滚，审计模式看状态与完整操作记录。 */
import { AlertTriangle, GitCompareArrows, Trash2, Undo2 } from '@lucide/vue';
import { useI18n } from '../../../composables/useI18n';
import { commandLabel } from '../../../composables/commandLabel';

interface Op {
  operation_id?: string; command?: string; status?: string;
  created_at?: string; version?: string;
  result?: Record<string, unknown>;
}
const props = withDefaults(defineProps<{
  rows: Op[]; loading: boolean; openId: string;
  mode?: 'versions' | 'audit';
  activeVersion?: string; currentOpId?: string; versions?: { version?: string }[];
  filtered?: boolean;
}>(), { mode: 'versions', activeVersion: '', currentOpId: '', versions: () => [], filtered: false });
const emit = defineEmits<{
  toggle: [id: string]; more: [];
  diff: [op: Op]; rollback: [op: Op]; delete: [op: Op];
}>();
const { t } = useI18n();

const DASH = '\u2014';

const STATUS: Record<string, string> = {
  succeeded: 'ops.succeeded', failed: 'ops.failed',
  recovery_required: 'ops.recoveryRequired', running: 'ops.running',
  queued: 'ops.queued', cancelled: 'ops.cancelled', unknown: 'status.unknown',
};

function fmtTime(s?: string): string { return (s || DASH).replace('T', ' ').slice(0, 19); }
function opVersion(op: Op): string {
  const v = op.result?.version ?? op.version;
  return typeof v === 'string' ? v : '';
}
// 版本号本身是时间戳；行首已有完整日期，这里只显示时分秒，避免挤占操作列。
function versionTime(v: string): string {
  const m = /^(\d{4})(\d{2})(\d{2})-(\d{2})(\d{2})(\d{2})/.exec(v);
  return m ? `${m[4]}:${m[5]}:${m[6]}` : v;
}
function versionExists(op: Op): boolean {
  const v = opVersion(op);
  return !!v && props.versions.some((x) => x.version === v);
}
function hasChanges(op: Op): boolean {
  return Array.isArray(op.result?.changes);
}
function isFailed(op: Op): boolean {
  return !!op.status && !['succeeded', 'running', 'queued'].includes(op.status);
}
function statusOf(op: Op): string {
  const key = STATUS[op.status ?? ''];
  return key ? t(key) : (op.status || DASH);
}
function statusClass(op: Op): string {
  if (op.status === 'succeeded') return 'badge--ok';
  if (op.status === 'failed' || op.status === 'cancelled') return 'badge--err';
  if (op.status === 'recovery_required') return 'badge--warn';
  return 'badge--off';
}
</script>

<template>
  <div class="toolbar">
    <slot name="toolbar" />
    <span class="logs-count mono">{{ rows.length }}</span>
  </div>

  <div class="opstable" :class="{ 'opstable--audit': mode === 'audit' }">
    <div class="opstable__head">
      <span>{{ t('ops.colTime') }}</span><span>{{ t('ops.colCommand') }}</span>
      <span v-if="mode === 'audit'">{{ t('common.status') }}</span>
      <span>{{ t('ops.colVersion') }}</span><span>{{ t('ops.colActions') }}</span>
    </div>
    <div class="opstable__body" @scroll.passive="emit('more')">
      <div v-if="!rows.length && !loading" class="table__empty">
        {{ mode === 'audit' ? (filtered ? t('audit.emptyFiltered') : t('audit.empty')) : t('ops.empty') }}
      </div>
      <template v-for="op in rows" :key="op.operation_id">
        <div class="opsrow" :class="{ 'opsrow--open': openId === op.operation_id }"
             @click="emit('toggle', op.operation_id ?? '')">
          <span class="opsrow__time mono">{{ fmtTime(op.created_at) }}</span>
          <span class="opsrow__cmd" :class="{ 'opsrow__cmd--err': isFailed(op) }" :title="op.command || ''">
            <AlertTriangle v-if="isFailed(op)" :size="13" />
            {{ commandLabel(op.command) }}
          </span>
          <span v-if="mode === 'audit'">
            <span class="badge" :class="statusClass(op)">{{ statusOf(op) }}</span>
          </span>
          <span class="opsrow__ver mono" :title="opVersion(op)">
            {{ opVersion(op) ? versionTime(opVersion(op)) : DASH }}
            <span v-if="mode === 'versions' && op.operation_id === currentOpId" class="badge badge--ok">{{ t('settings.current') }}</span>
          </span>
          <span class="opsrow__actions">
            <button
              v-if="mode === 'audit' ? hasChanges(op) : (!!opVersion(op) && opVersion(op) !== activeVersion)"
              class="icon-btn" :title="t('settings.diff')" :aria-label="t('settings.diff')" @click.stop="emit('diff', op)"
            >
              <GitCompareArrows :size="14" />
            </button>
            <template v-if="mode === 'versions'">
              <button
                v-if="versionExists(op) && opVersion(op) !== activeVersion"
                class="icon-btn" :title="t('settings.rollback')" :aria-label="t('settings.rollback')" @click.stop="emit('rollback', op)"
              >
                <Undo2 :size="14" />
              </button>
              <button
                v-if="versionExists(op) && opVersion(op) !== activeVersion"
                class="icon-btn" :title="t('ops.deleteVersion')" :aria-label="t('ops.deleteVersion')" @click.stop="emit('delete', op)"
              >
                <Trash2 :size="14" />
              </button>
            </template>
          </span>
        </div>
        <slot name="detail" :op="op" />
      </template>
      <div v-if="rows.length" class="opstable__end">{{ t('logs.statusOldest') }}</div>
    </div>
  </div>
</template>
