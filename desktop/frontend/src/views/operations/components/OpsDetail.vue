<script setup lang="ts">
/** 操作详情：基础信息 kv + 结构化错误块 + result 键值行。 */
import { useI18n } from '../../../composables/useI18n';

interface OpErrorDetail { code?: string; message?: string }
interface OpError { code?: string; message?: string; details?: OpErrorDetail[] }
interface Op {
  operation_id?: string; command?: string; status?: string;
  created_at?: string; started_at?: string; finished_at?: string;
  phase?: string; owner_pid?: number; result?: Record<string, unknown>;
  error?: OpError | null;
}
defineProps<{ op: Op }>();
const { t } = useI18n();

function fmtTime(s?: string): string { return (s ?? '—').replace('T', ' ').slice(0, 19); }
function resultRows(op: Op): { k: string; v: string }[] {
  const r = op.result;
  if (!r || typeof r !== 'object') return [];
  return Object.entries(r)
    .filter(([, v]) => v !== undefined && v !== null && v !== '')
    .map(([k, v]) => ({ k, v: typeof v === 'string' ? v : typeof v === 'number' || typeof v === 'boolean' ? String(v) : JSON.stringify(v) }));
}
function statusLabel(s?: string): string {
  const map: Record<string, string> = {
    succeeded: 'ops.succeeded', failed: 'ops.failed', recovery_required: 'ops.recoveryRequired',
    running: 'ops.running', queued: 'ops.queued', cancelled: 'ops.cancelled',
  };
  return map[s ?? ''] ? t(map[s ?? '']) : (s ?? '—');
}
</script>

<template>
  <div class="ops-detail">
    <div class="kv-table">
      <div class="kv-row"><span class="k">operation_id</span><span class="v mono">{{ op.operation_id }}</span></div>
      <div class="kv-row"><span class="k">command</span><span class="v mono">{{ op.command }}</span></div>
      <div class="kv-row"><span class="k">{{ t('common.status') }}</span><span class="v">{{ statusLabel(op.status) }}</span></div>
      <div class="kv-row"><span class="k">{{ t('ops.detailStart') }}</span><span class="v mono">{{ fmtTime(op.started_at) }}</span></div>
      <div class="kv-row"><span class="k">{{ t('ops.detailFinish') }}</span><span class="v mono">{{ fmtTime(op.finished_at) }}</span></div>
      <div v-if="op.phase" class="kv-row"><span class="k">phase</span><span class="v mono">{{ op.phase }}</span></div>
      <div v-if="op.owner_pid" class="kv-row"><span class="k">pid</span><span class="v mono">{{ op.owner_pid }}</span></div>
    </div>

    <div v-if="op.error" class="ops-err">
      <div class="ops-err__code mono">{{ op.error.code || t('common.unknownError') }}</div>
      <div class="ops-err__msg">{{ op.error.message }}</div>
      <div v-if="op.error.details?.length" class="ops-err__details">
        <div v-for="(d, di) in op.error.details" :key="di" class="ops-err__detail">
          <span class="mono">{{ d.code }}</span> {{ d.message }}
        </div>
      </div>
    </div>

    <div v-if="resultRows(op).length" class="kv-table" style="margin-top: 8px">
      <div v-for="r in resultRows(op)" :key="r.k" class="kv-row">
        <span class="k mono">{{ r.k }}</span><span class="v mono">{{ r.v }}</span>
      </div>
    </div>
  </div>
</template>
