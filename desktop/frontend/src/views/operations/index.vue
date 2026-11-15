<script setup lang="ts">
/** 操作记录编排壳：分页（递增 limit）/ 状态筛选 / 详情展开。 */
import { computed, onMounted, onUnmounted, ref } from 'vue';
import { runCommand } from '../../services/api/command';
import { messageOf } from '../../services/api/base';
import OpsTable from './components/OpsTable.vue';
import OpsDetail from './components/OpsDetail.vue';

interface Op {
  operation_id?: string; command?: string; status?: string;
  created_at?: string; started_at?: string; finished_at?: string;
  phase?: string; owner_pid?: number; version?: string; restore_point?: string;
  result?: Record<string, unknown>;
  error?: { code?: string; message?: string; details?: { code?: string; message?: string }[] } | null;
}
const MAX_LIMIT = 1000;
const rows = ref<Op[]>([]);
const loading = ref(false);
const error = ref('');
const filter = ref('');
const openId = ref('');
const limit = ref(50);
const allLoaded = ref(false);

const shown = computed(() => (filter.value ? rows.value.filter((o) => o.status === filter.value) : rows.value));

async function load(reset = true): Promise<void> {
  if (loading.value) return;
  if (reset) { limit.value = 50; allLoaded.value = false; }
  loading.value = true;
  error.value = '';
  try {
    const args = ['operation', 'list', '--limit', String(limit.value)];
    if (filter.value) args.push('--status', filter.value);
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
function pickStatus(): void { void load(true); }
onMounted(() => {
  void load();
  // 自动刷新：操作记录随事务追加，10 秒一次。
  const timer = window.setInterval(() => { void load(true); }, 10000);
  onUnmounted(() => window.clearInterval(timer));
});
</script>

<template>
  <div class="main__body ops-page">
    <p v-if="error" class="notice notice--err">{{ error }}</p>
    <OpsTable
      :rows="shown" :loading="loading" :filter="filter" :open-id="openId"
      @refresh="load()" @update:filter="filter = $event; pickStatus()"
      @toggle="toggleDetail" @more="loadMore"
    >
      <template #detail="{ op }">
        <OpsDetail v-if="openId === op.operation_id" :op="op" />
      </template>
    </OpsTable>
  </div>
</template>
