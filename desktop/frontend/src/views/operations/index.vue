<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { RefreshCw } from '@lucide/vue';
import { useI18n } from '../../composables/useI18n';
import { runCommand } from '../../services/api/command';
import { messageOf } from '../../services/api/base';
import FhButton from '../../components/ui/FhButton.vue';

const { t } = useI18n();

interface Op {
  operation_id?: string;
  command?: string;
  status?: string;
  created_at?: string;
  version?: string;
  restore_point?: string;
  result?: { version?: string; restore_point?: string };
  error?: string;
}

const STATUS: Record<string, [string, string]> = {
  succeeded: ['ok', 'ops.succeeded'],
  failed: ['err', 'ops.failed'],
  recovery_required: ['err', 'ops.recoveryRequired'],
  running: ['info', 'ops.running'],
  queued: ['off', 'ops.queued'],
  cancelled: ['off', 'ops.cancelled'],
  unknown: ['off', 'status.unknown'],
};

const rows = ref<Op[]>([]);
const loading = ref(false);
const error = ref('');
const filter = ref('');
const openId = ref('');

const shown = computed(() => (filter.value ? rows.value.filter((o) => o.status === filter.value) : rows.value));

function statusOf(op: Op): { cls: string; label: string } {
  const [cls, key] = STATUS[op.status ?? ''] ?? ['off', ''];
  return { cls, label: key ? t(key) : (op.status ?? '—') };
}

function fmtTime(s?: string): string {
  return (s ?? '—').replace('T', ' ').slice(0, 19);
}

async function load(): Promise<void> {
  loading.value = true;
  error.value = '';
  try {
    const r = await runCommand(['ops', 'list', '--limit', '100']);
    const d = r.data as { operations?: Op[]; ops?: Op[] } | null;
    rows.value = d?.operations ?? d?.ops ?? [];
  } catch (e) {
    error.value = messageOf(e);
    rows.value = [];
  } finally {
    loading.value = false;
  }
}

function toggleDetail(id: string): void {
  openId.value = openId.value === id ? '' : id;
}

onMounted(() => { void load(); });
</script>

<template>
  <div class="main__body">
    <div class="toolbar">
      <FhButton :loading="loading" @click="load()">
        <RefreshCw :size="14" /><span>{{ t('common.refresh') }}</span>
      </FhButton>
      <select v-model="filter" class="ops-filter">
        <option value="">{{ t('ops.filterAll') }}</option>
        <option v-for="(v, k) in STATUS" :key="k" :value="k">{{ t(v[1]) }}</option>
      </select>
    </div>

    <p v-if="error" class="notice notice--err">{{ error }}</p>

    <div class="table">
      <div class="table__head table__head--ops">
        <span>time</span>
        <span>command</span>
        <span>{{ t('common.status') }}</span>
        <span>version</span>
        <span>restore</span>
      </div>
      <div v-if="!shown.length && !loading" class="table__empty">{{ t('ops.empty') }}</div>
      <template v-for="op in shown" :key="op.operation_id">
        <div class="table__row table__row--ops" @click="toggleDetail(op.operation_id ?? '')">
          <span class="mono muted">{{ fmtTime(op.created_at) }}</span>
          <span class="mono ellipsis">{{ op.command || '—' }}</span>
          <span>
            <span class="badge" :class="statusOf(op).cls === 'ok' ? 'badge--ok' : 'badge--off'">
              {{ statusOf(op).label }}
            </span>
          </span>
          <span class="mono">{{ op.result?.version || op.version || '—' }}</span>
          <span class="mono muted">{{ op.result?.restore_point || op.restore_point || '—' }}</span>
        </div>
        <div v-if="openId === op.operation_id" class="ops-detail">
          <div class="kv-table">
            <div class="kv-row"><span class="k">operation_id</span><span class="v mono">{{ op.operation_id }}</span></div>
            <div class="kv-row"><span class="k">command</span><span class="v mono">{{ op.command }}</span></div>
            <div class="kv-row"><span class="k">{{ t('common.status') }}</span><span class="v">{{ statusOf(op).label }}</span></div>
            <div v-if="op.error" class="kv-row"><span class="k">error</span><span class="v mono">{{ op.error }}</span></div>
          </div>
          <pre class="cmd-result">{{ JSON.stringify(op.result ?? {}, null, 2) }}</pre>
        </div>
      </template>
    </div>
  </div>
</template>
