<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { RefreshCw } from '@lucide/vue';
import { useI18n } from '../../composables/useI18n';
import { runCommand } from '../../services/api/command';
import { messageOf } from '../../services/api/base';
import FhButton from '../../components/ui/FhButton.vue';

const { t } = useI18n();

interface OpErrorDetail { code?: string; message?: string }
interface OpError { code?: string; message?: string; details?: OpErrorDetail[] }

interface Op {
  operation_id?: string;
  command?: string;
  status?: string;
  created_at?: string;
  started_at?: string;
  finished_at?: string;
  phase?: string;
  owner_pid?: number;
  version?: string;
  restore_point?: string;
  result?: Record<string, unknown>;
  error?: OpError | null;
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

const MAX_LIMIT = 1000;   // 与 CLI 的 MaxLimit 一致

const rows = ref<Op[]>([]);
const loading = ref(false);
const error = ref('');
const filter = ref('');
const openId = ref('');
// operation list 只支持 limit / status / since，没有 cursor：
// 滚动加载用「递增 limit 重新拉」实现，直到拉到上限或条数不再增长。
const limit = ref(50);
const allLoaded = ref(false);

const shown = computed(() => (filter.value ? rows.value.filter((o) => o.status === filter.value) : rows.value));

function statusOf(op: Op): { cls: string; label: string } {
  const [cls, key] = STATUS[op.status ?? ''] ?? ['off', ''];
  return { cls, label: key ? t(key) : (op.status ?? '—') };
}

function fmtTime(s?: string): string {
  return (s ?? '—').replace('T', ' ').slice(0, 19);
}

/** result 是 map[string]any：逐项转成可读行，对象/数组压成一行。 */
function resultRows(op: Op): { k: string; v: string }[] {
  const r = op.result;
  if (!r || typeof r !== 'object') return [];
  return Object.entries(r)
    .filter(([, v]) => v !== undefined && v !== null && v !== '')
    .map(([k, v]) => ({
      k,
      v: typeof v === 'string' ? v : typeof v === 'number' || typeof v === 'boolean' ? String(v) : JSON.stringify(v),
    }));
}

/** 结果里的字段可能是对象，取值时统一转字符串。 */
function resultField(op: Op, key: string): string {
  const v = op.result?.[key];
  if (v === undefined || v === null || v === '') return '';
  return typeof v === 'string' ? v : String(v);
}

async function fetchOps(nextLimit: number, status: string): Promise<Op[]> {
  const args = ['operation', 'list', '--limit', String(nextLimit)];
  if (status) args.push('--status', status);
  const r = await runCommand(args);
  const d = r.data as { operations?: Op[]; ops?: Op[] } | null;
  return d?.operations ?? d?.ops ?? [];
}

/** reset=true 时从 50 条重新开始，否则按当前 limit 继续。 */
async function load(reset = true): Promise<void> {
  if (loading.value) return;
  if (reset) { limit.value = 50; allLoaded.value = false; }
  loading.value = true;
  error.value = '';
  try {
    const list = await fetchOps(limit.value, filter.value);
    rows.value = list;
    // 拉到的条数少于上限，说明后面没有了
    if (list.length < limit.value) allLoaded.value = true;
  } catch (e) {
    error.value = messageOf(e);
    if (reset) rows.value = [];
  } finally {
    loading.value = false;
  }
}

/** 滚到底部附近时把 limit 翻倍再拉一次。 */
async function loadMore(): Promise<void> {
  if (loading.value || allLoaded.value || limit.value >= MAX_LIMIT) return;
  limit.value = Math.min(MAX_LIMIT, limit.value * 2);
  await load(false);
}

function onScroll(e: Event): void {
  const el = e.target as HTMLElement;
  if (el.scrollHeight - el.scrollTop - el.clientHeight < 120) void loadMore();
}

function toggleDetail(id: string): void {
  openId.value = openId.value === id ? '' : id;
}

function pickStatus(): void {
  void load(true);
}

onMounted(() => { void load(); });
</script>

<template>
  <div class="main__body ops-page">
    <div class="toolbar">
      <FhButton :loading="loading" @click="load()">
        <RefreshCw :size="14" /><span>{{ t('common.refresh') }}</span>
      </FhButton>
      <select v-model="filter" class="ops-filter" @change="pickStatus">
        <option value="">{{ t('ops.filterAll') }}</option>
        <option v-for="(v, k) in STATUS" :key="k" :value="k">{{ t(v[1]) }}</option>
      </select>
      <span class="logs-count mono">{{ shown.length }}</span>
    </div>

    <p v-if="error" class="notice notice--err">{{ error }}</p>

    <div class="opstable">
      <div class="opstable__head">
        <span>time</span>
        <span>command</span>
        <span>{{ t('common.status') }}</span>
        <span>version</span>
        <span>restore</span>
      </div>
      <div class="opstable__body" @scroll.passive="onScroll">
        <div v-if="!shown.length && !loading" class="table__empty">{{ t('ops.empty') }}</div>
        <template v-for="op in shown" :key="op.operation_id">
          <div class="opsrow" :class="{ 'opsrow--open': openId === op.operation_id }"
               @click="toggleDetail(op.operation_id ?? '')">
            <span class="opsrow__time mono">{{ fmtTime(op.created_at) }}</span>
            <span class="opsrow__cmd mono">{{ op.command || '—' }}</span>
            <span>
              <span class="badge" :class="statusOf(op).cls === 'ok' ? 'badge--ok' : 'badge--off'">
                {{ statusOf(op).label }}
              </span>
            </span>
            <span class="opsrow__ver mono">{{ resultField(op, 'version') || op.version || '—' }}</span>
            <span class="opsrow__restore mono">{{ resultField(op, 'restore_point') || op.restore_point || '—' }}</span>
          </div>
          <div v-if="openId === op.operation_id" class="ops-detail">
            <div class="kv-table">
              <div class="kv-row"><span class="k">operation_id</span><span class="v mono">{{ op.operation_id }}</span></div>
              <div class="kv-row"><span class="k">command</span><span class="v mono">{{ op.command }}</span></div>
              <div class="kv-row"><span class="k">{{ t('common.status') }}</span><span class="v">{{ statusOf(op).label }}</span></div>
              <div class="kv-row"><span class="k">{{ t('ops.detailStart') }}</span><span class="v mono">{{ fmtTime(op.started_at) }}</span></div>
              <div class="kv-row"><span class="k">{{ t('ops.detailFinish') }}</span><span class="v mono">{{ fmtTime(op.finished_at) }}</span></div>
              <div v-if="op.phase" class="kv-row"><span class="k">phase</span><span class="v mono">{{ op.phase }}</span></div>
              <div v-if="op.owner_pid" class="kv-row"><span class="k">pid</span><span class="v mono">{{ op.owner_pid }}</span></div>
            </div>

            <!-- 错误：结构化展示 code + message，不再是一坨 JSON -->
            <div v-if="op.error" class="ops-err">
              <div class="ops-err__code mono">{{ op.error.code || t('common.unknownError') }}</div>
              <div class="ops-err__msg">{{ op.error.message }}</div>
              <div v-if="op.error.details?.length" class="ops-err__details">
                <div v-for="(d, di) in op.error.details" :key="di" class="ops-err__detail">
                  <span class="mono">{{ d.code }}</span> {{ d.message }}
                </div>
              </div>
            </div>

            <!-- 结果：按 key-value 逐行展示，值可能是对象则压成一行 -->
            <div v-if="resultRows(op).length" class="kv-table" style="margin-top: 8px">
              <div v-for="r in resultRows(op)" :key="r.k" class="kv-row">
                <span class="k mono">{{ r.k }}</span><span class="v mono">{{ r.v }}</span>
              </div>
            </div>
          </div>
        </template>
        <div v-if="!allLoaded && shown.length" class="opstable__more">
          <FhButton :loading="loading" size="small" @click="loadMore">{{ t('logs.more') }}</FhButton>
        </div>
        <div v-else-if="allLoaded && shown.length" class="opstable__end">{{ t('logs.statusOldest') }}</div>
      </div>
    </div>
  </div>
</template>
