<script setup lang="ts">
/** 操作记录：完整的管理操作审计，包括回滚、失败与重载；只读，不做版本管理。 */
import { computed, ref } from 'vue';
import { useI18n } from '../../composables/useI18n';
import { usePolling } from '../../composables/usePolling';
import OpsTable from '../operations/components/OpsTable.vue';
import OpsDetail from '../operations/components/OpsDetail.vue';
import DiffDialog from '../operations/components/DiffDialog.vue';
import { useOperations, type Operation, type OpChange } from '../operations/composables/useOperations';

const status = ref('');
const { rows, loading, error, load, loadMore } = useOperations(
  () => (status.value ? ['--status', status.value] : []),
);
const openId = ref('');
const diffDialog = ref<{ version: string; changes: OpChange[] } | null>(null);

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

function opVersion(op: Operation): string {
  const v = op.result?.version ?? op.version;
  return typeof v === 'string' ? v : '';
}

function toggleDetail(id: string): void {
  openId.value = openId.value === id ? '' : id;
}
function toggleDiff(op: Operation): void {
  if (!Array.isArray(op.result?.changes)) return;
  diffDialog.value = {
    version: opVersion(op) || op.command || '',
    changes: op.result.changes as OpChange[],
  };
}

// 操作记录随事务追加，10 秒刷新一次；保留当前「加载更多」的条数。
usePolling(() => { void load(false); }, 10000, computed(() => true));
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
