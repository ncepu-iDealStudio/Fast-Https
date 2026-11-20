<script setup lang="ts">
/** 版本更新：唯一历史时间线；行内可看变更、回滚、删除版本。 */
import { computed, ref } from 'vue';
import { useI18n } from '../../composables/useI18n';
import { usePolling } from '../../composables/usePolling';
import { runCommand } from '../../services/api/command';
import { messageOf } from '../../services/api/base';
import OpsTable from './components/OpsTable.vue';
import OpsDetail from './components/OpsDetail.vue';
import DiffDialog from './components/DiffDialog.vue';
import ConfirmDialog from '../../components/common/ConfirmDialog.vue';
import { useOperations, type Operation, type OpChange } from './composables/useOperations';

interface Version {
  version?: string; active?: boolean;
}

const { rows, loading, error, load, loadMore } = useOperations();
const notice = ref('');
const openId = ref('');
const activeVersion = ref('');
const versions = ref<Version[]>([]);
const drift = ref(false);
const diffDialog = ref<{ base?: string; version: string; changes: OpChange[]; missing?: boolean } | null>(null);
const rollbackTarget = ref<{ id?: string; version: string } | null>(null);
const rollbackBusy = ref(false);
const deleteTarget = ref('');
const deleteBusy = ref(false);

const { t } = useI18n();

function opVersion(op: Operation): string {
  const v = op.result?.version ?? op.version;
  return typeof v === 'string' ? v : '';
}
function changesOf(op: Operation): OpChange[] {
  const c = op.result?.changes;
  return Array.isArray(c) ? (c as OpChange[]) : [];
}
function hasRecordedChanges(op: Operation): boolean {
  return Array.isArray(op.result?.changes);
}
function isActive(op: Operation): boolean {
  return !!activeVersion.value && opVersion(op) === activeVersion.value;
}

// 回滚不产生新版本，只是把当前版本切回旧快照；版本列表里不把它当成一次版本更新。
const visibleRows = computed(() => rows.value.filter((op) => op.command !== 'config.rollback'));

// 同一个版本可能被多条操作引用；「当前」只标在最新一条产生该版本的操作上。
const currentOpId = computed(() => {
  const hit = visibleRows.value.find((op) => opVersion(op) === activeVersion.value);
  return hit?.operation_id ?? '';
});

async function refresh(reset = true): Promise<void> {
  await load(reset);
  await loadCurrentVersion();
}

async function loadCurrentVersion(): Promise<void> {
  try {
    const r = await runCommand(['config', 'versions']);
    const d = r.data as { versions?: Version[]; drift?: boolean } | null;
    versions.value = d?.versions ?? [];
    activeVersion.value = versions.value.find((v) => v.active)?.version ?? '';
    drift.value = !!d?.drift;
  } catch {
    versions.value = [];
    activeVersion.value = '';
    drift.value = false;
  }
}

function toggleDetail(id: string): void {
  openId.value = openId.value === id ? '' : id;
}
function toggleDiff(op: Operation): void {
  if (!opVersion(op)) return;
  diffDialog.value = {
    version: opVersion(op),
    changes: changesOf(op),
    missing: !hasRecordedChanges(op),
  };
}
function askRollback(op: Operation): void {
  if (!opVersion(op) || isActive(op)) return;
  notice.value = '';
  rollbackTarget.value = { id: op.operation_id, version: opVersion(op) };
}
async function doRollback(): Promise<void> {
  const target = rollbackTarget.value;
  if (!target) return;
  rollbackBusy.value = true;
  error.value = '';
  try {
    const args = target.id
      ? ['config', 'rollback', '--operation', target.id, '--yes']
      : ['config', 'rollback', '--version', target.version, '--yes'];
    await runCommand(args);
    notice.value = t('settings.rollbackDone');
    rollbackTarget.value = null;
    diffDialog.value = null;
    await refresh();
  } catch (e) {
    error.value = messageOf(e);
  } finally {
    rollbackBusy.value = false;
  }
}

function askDeleteVersion(op: Operation): void {
  const v = opVersion(op);
  if (!v || v === activeVersion.value) return;
  notice.value = '';
  deleteTarget.value = v;
}
async function doDeleteVersion(): Promise<void> {
  const v = deleteTarget.value;
  if (!v) return;
  deleteBusy.value = true;
  error.value = '';
  try {
    await runCommand(['config', 'versions', 'delete', v, '--yes']);
    notice.value = t('ops.deleted', { v });
    deleteTarget.value = '';
    diffDialog.value = null;
    await refresh();
  } catch (e) {
    error.value = messageOf(e);
  } finally {
    deleteBusy.value = false;
  }
}

// 操作记录随事务追加，10 秒刷新一次；保留当前「加载更多」的条数。
usePolling(() => { void refresh(false); }, 10000, computed(() => true));
</script>

<template>
  <div class="main__body ops-page">
    <p v-if="error" class="notice notice--err">{{ error }}</p>
    <p v-else-if="notice" class="notice">{{ notice }}</p>
    <p v-if="drift" class="notice">{{ t('settings.driftNote') }}</p>

    <OpsTable
      :rows="visibleRows" :loading="loading" :open-id="openId"
      :active-version="activeVersion" :current-op-id="currentOpId" :versions="versions"
      @toggle="toggleDetail" @more="loadMore"
      @diff="toggleDiff" @rollback="askRollback" @delete="askDeleteVersion"
    >
      <template #detail="{ op }">
        <OpsDetail v-if="openId === op.operation_id" :op="op" />
      </template>
    </OpsTable>

    <DiffDialog
      :open="!!diffDialog"
      :base="diffDialog?.base"
      :version="diffDialog?.version ?? ''"
      :changes="diffDialog?.changes ?? []"
      :missing="diffDialog?.missing"
      @update:open="(v: boolean) => { if (!v) diffDialog = null; }"
    />
    <ConfirmDialog
      :open="!!rollbackTarget"
      :title="t('settings.rollbackTitle')"
      :message="t('settings.rollbackBody', { version: rollbackTarget?.version ?? '' })"
      :loading="rollbackBusy"
      @update:open="(v: boolean) => { if (!v) rollbackTarget = null; }"
      @confirm="doRollback"
    />

    <ConfirmDialog
      :open="!!deleteTarget"
      :title="t('ops.deleteVersionTitle')"
      :message="t('ops.deleteVersionBody', { v: deleteTarget })"
      :loading="deleteBusy"
      @update:open="(v: boolean) => { if (!v) deleteTarget = ''; }"
      @confirm="doDeleteVersion"
    />
  </div>
</template>
