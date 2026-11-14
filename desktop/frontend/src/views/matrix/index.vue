<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { RefreshCw, Activity, Eye } from '@lucide/vue';
import { useI18n } from '../../composables/useI18n';
import { runCommand } from '../../services/api/command';
import { messageOf } from '../../services/api/base';
import { useOverviewStore } from '../../stores/overview';
import FhButton from '../../components/ui/FhButton.vue';

const { t } = useI18n();
const overview = useOverviewStore();

const loading = ref(false);
const error = ref('');
const runRows = ref<{ k: string; v: string }[]>([]);
const monRows = ref<{ k: string; v: string }[]>([]);
const events = ref<Record<string, unknown>[]>([]);

function durationText(seconds: number): string {
  const s = Number(seconds) || 0;
  if (s <= 0) return '—';
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  return h > 0 ? t('runtime.durationHM', { h, m }) : t('runtime.durationM', { m });
}

async function load(): Promise<void> {
  loading.value = true;
  error.value = '';
  try {
    await overview.refresh();

    // 运行快照：runtime status
    const r = await runCommand(['runtime', 'status']);
    const d = (r.data ?? {}) as {
      state?: string; pid?: number; uptime_seconds?: number;
      connections?: Record<string, number>; requests?: Record<string, number>;
    };
    const c = d.connections ?? {};
    const q = d.requests ?? {};
    runRows.value = [
      { k: t('common.status'), v: d.state ?? '—' },
      { k: 'pid', v: d.pid != null ? String(d.pid) : '—' },
      { k: t('runtime.uptime'), v: durationText(d.uptime_seconds ?? 0) },
      { k: t('runtime.connActive'), v: String(c.active ?? '—') },
      { k: t('runtime.connAccepted'), v: String(c.accepted_total ?? '—') },
      { k: t('runtime.connClosed'), v: String(c.closed_total ?? '—') },
      { k: t('runtime.connRejected'), v: String(c.rejected_total ?? '—') },
      { k: t('runtime.reqActive'), v: String(q.active ?? '—') },
      { k: t('runtime.reqTotal'), v: String(q.total ?? '—') },
    ];
  } catch (e) {
    error.value = messageOf(e);
    runRows.value = [];
  }

  // Monitor 面板：未安装也要给出明确结论，不当作错误
  try {
    const st = await runCommand(['monitor', 'status']);
    const m = (st.data ?? {}) as {
      status?: string; healthy?: boolean; last_collect_at?: string;
      last_event_at?: string; last_error?: string; version?: string;
    };
    monRows.value = [
      { k: t('common.status'), v: m.status ?? '—' },
      { k: t('monitor.healthy'), v: m.healthy ? t('common.on') : t('common.off') },
      { k: t('monitor.version'), v: m.version ?? '—' },
      { k: t('monitor.lastCollect'), v: m.last_collect_at ?? '—' },
      { k: t('monitor.lastEvent'), v: m.last_event_at ?? '—' },
      { k: t('monitor.lastError'), v: m.last_error || '—' },
    ];
    const ev = await runCommand(['monitor', 'events', '--limit', '8']);
    const ed = (ev.data ?? {}) as { events?: Record<string, unknown>[] };
    events.value = ed.events ?? [];
  } catch {
    monRows.value = [{ k: t('monitor.notInstalled'), v: t('monitor.notInstalledBody') }];
    events.value = [];
  }

  loading.value = false;
}

onMounted(() => { void load(); });
</script>

<template>
  <div class="main__body">
    <div class="toolbar">
      <FhButton :loading="loading" @click="load()">
        <RefreshCw :size="14" /><span>{{ t('matrix.msRefresh') }}</span>
      </FhButton>
    </div>

    <p v-if="error" class="notice notice--err">{{ error }}</p>

    <div class="cards">
      <div class="card">
        <div class="card__k">{{ t('nav.sites') }}</div>
        <div class="card__v">{{ overview.data?.siteCount ?? '—' }}</div>
      </div>
      <div class="card">
        <div class="card__k">{{ t('matrix.msPorts') }}</div>
        <div class="card__v mono small">({{ (overview.data?.ports ?? []).join(', ') || '—' }})</div>
      </div>
      <div class="card">
        <div class="card__k">{{ t('nav.certs') }}</div>
        <div class="card__v">{{ overview.data?.tlsCount ?? '—' }}</div>
      </div>
    </div>

    <div class="matrix-grid">
      <!-- 运行快照 -->
      <div class="card">
        <div class="card-head">
          <h3><Activity :size="14" /> {{ t('matrix.runtimeTitle') }}</h3>
        </div>
        <div class="kv-table">
          <div v-for="r in runRows" :key="r.k" class="kv-row">
            <span class="k">{{ r.k }}</span><span class="v mono">{{ r.v }}</span>
          </div>
          <div v-if="!runRows.length && !loading" class="kv-row">
            <span class="k">—</span><span class="v">{{ t('matrix.reading') }}</span>
          </div>
        </div>
      </div>

      <!-- Monitor -->
      <div class="card">
        <div class="card-head">
          <h3><Eye :size="14" /> Monitor</h3>
        </div>
        <div class="kv-table">
          <div v-for="r in monRows" :key="r.k" class="kv-row">
            <span class="k">{{ r.k }}</span><span class="v mono">{{ r.v }}</span>
          </div>
        </div>
        <div v-if="events.length" class="kv-table">
          <div v-for="(e, i) in events" :key="i" class="kv-row">
            <span class="k mono">{{ e.kind ?? e.type }}</span>
            <span class="v mono muted">{{ e.at ?? e.time }}</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
