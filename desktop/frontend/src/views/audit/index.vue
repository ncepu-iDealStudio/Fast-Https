<script setup lang="ts">
/** 操作记录：完整的管理操作审计，包括回滚、失败与重载；只读，不做版本管理。 */
import { onMounted, onUnmounted, ref } from 'vue';
import { useI18n } from '../../composables/useI18n';
import { runCommand } from '../../services/api/command';
import { messageOf } from '../../services/api/base';
import OpsTable from '../operations/components/OpsTable.vue';
import OpsDetail from '../operations/components/OpsDetail.vue';
import DiffDialog from '../operations/components/DiffDialog.vue';

interface Op {
  operation_id?: string; command?: string; status?: string;
  created_at?: string; started_at?: string; finished_at?: string;
  version?: string; result?: Record<string, unknown>;
  error?: { code?: string; message?: string; details?: { code?: string; message?: string }[] } | null;
}
interface Change { path?: string; op?: string; value?: unknown; old?: unknown; new?: unknown; changed?: boolean }

const MAX_LIMIT = 1000;
const rows = ref<Op[]>([]);
const loading = ref(false);
const error = ref('');
const openId = ref('');
const limit = ref(50);
const allLoaded = ref(false);
const diffDialog = ref<{ version: string; changes: Change[] } | null>(null);
const status = ref('');

// 只列实际会出现的状态：queued 目前没有写入路径，cancelled 保留但极少见。
const STATUSES = ['succeeded', 'failed', 'running', 'recovery_required', 'unknown'] as const;
const STATUS_LABEL: Record<string, string> = {
  succeeded: 'ops.succeeded', failed: 'ops.failed', running: 'ops.running',
  recovery_required: 'ops.recoveryRequired', unknown: 'status.unknown',
};

const { t } = useI18n();

function statusLabel(s: string): string {
  return t(STATUS_LABEL[s] ?? s);
}
function onStatus(e: Event): void {
  status.value = (e.target as HTMLSelectElement).value;
  void load();
}

function opVersion(op: Op): string {
  const v = op.result?.version ?? op.version;
  return typeof v === 'string' ? v : '';
}

async function load(reset = true): Promise<void> {
  if (loading.value) return;
  if (reset) { limit.value = 50; allLoaded.value = false; }
  loading.value = true;
  error.value = '';
  try {
    const args = ['operation', 'list', '--limit', String(limit.value)];
    if (status.value) args.push('--status', status.value);
    const r = await runCommand(args);
    const d = r.data as { operations?: Op[] } | null;
    rows.value = d?.operations ?? [];
    if (rows.value.length < limit.value) allLoaded.value = true;
  } catch (e) {
    error.value = messageOf(e);
    if (reset) rows.value = [];
  } finally {
    loading.value = false;
  }
}

async function loadMore(): Promise<void> {
  if (loading.value || allLoaded.value || limit.value >= MAX_LIMIT) return;
  limit.value = Math.min(MAX_LIMIT, limit.value * 2);
  await load(false);
}

function toggleDetail(id: string): void {
  openId.value = openId.value === id ? '' : id;
}
function toggleDiff(op: Op): void {
  if (!Array.isArray(op.result?.changes)) return;
  diffDialog.value = {
    version: opVersion(op) || op.command || '',
    changes: op.result.changes as Change[],
  };
}

onMounted(() => {
  void load();
  // 操作记录随事务追加，10 秒刷新一次。
  const timer = window.setInterval(() => { void load(true); }, 10000);
  onUnmounted(() => window.clearInterval(timer));
});
</script>

<template>
  <div class="main__body ops-page">
    <p v-if="error" class="notice notice--err">{{ error }}</p>

    <OpsTable
      mode="audit" :rows="rows" :loading="loading" :open-id="openId" :filtered="!!status"
      @toggle="toggleDetail" @more="loadMore" @diff="toggleDiff"
    >
      <template #toolbar>
        <select class="ops-filter" :value="status" :title="t('common.status')" @change="onStatus">
          <option value="">{{ t('ops.filterAll') }}</option>
          <option v-for="s in STATUSES" :key="s" :value="s">{{ statusLabel(s) }}</option>
        </select>
      </template>
      <template #detail="{ op }">
        <OpsDetail audit v-if="openId === op.operation_id" :op="op" />
      </template>
    </OpsTable>

    <DiffDialog
      :open="!!diffDialog"
      :version="diffDialog?.version ?? ''"
      :changes="diffDialog?.changes ?? []"
      @update:open="(v: boolean) => { if (!v) diffDialog = null; }"
    />
  </div>
</template>
