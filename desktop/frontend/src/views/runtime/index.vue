<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { RefreshCw } from '@lucide/vue';
import { useI18n } from '../../composables/useI18n';
import { runCommand } from '../../services/api/command';
import { messageOf } from '../../services/api/base';
import FhButton from '../../components/ui/FhButton.vue';

const { t } = useI18n();
const loading = ref(false);
const error = ref('');
const rows = ref<{ k: string; v: string }[]>([]);

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
    // 与原版一致：runtime status 拿连接/请求，runtime stats 拿响应分布与上游。
    const r = await runCommand(['runtime', 'status']);
    const d = (r.data ?? {}) as {
      state?: string; pid?: number; uptime_seconds?: number;
      connections?: Record<string, number>; requests?: Record<string, number>;
    };
    const c = d.connections ?? {};
    const q = d.requests ?? {};
    const out: { k: string; v: string }[] = [
      { k: t('runtime.state'), v: d.state ?? '—' },
      { k: 'pid', v: d.pid != null ? String(d.pid) : '—' },
      { k: t('runtime.uptime'), v: durationText(d.uptime_seconds ?? 0) },
      { k: t('runtime.connActive'), v: String(c.active ?? '—') },
      { k: t('runtime.connAccepted'), v: String(c.accepted_total ?? '—') },
      { k: t('runtime.connClosed'), v: String(c.closed_total ?? '—') },
      { k: t('runtime.connRejected'), v: String(c.rejected_total ?? '—') },
      { k: t('runtime.reqActive'), v: String(q.active ?? '—') },
      { k: t('runtime.reqTotal'), v: String(q.total ?? '—') },
    ];

    // stats：响应分布与上游小结
    try {
      const st = await runCommand(['runtime', 'stats']);
      const sd = (st.data ?? {}) as {
        responses?: Record<string, number>; upstream?: Record<string, number>;
      };
      const resp = sd.responses ?? {};
      for (const code of ['2xx', '3xx', '4xx', '5xx']) {
        if (resp[code] != null) out.push({ k: `${t('runtime.responses')} ${code}`, v: String(resp[code]) });
      }
      const up = sd.upstream ?? {};
      for (const [k, v] of Object.entries(up)) {
        out.push({ k: `${t('runtime.upstream')} ${k}`, v: String(v) });
      }
    } catch {
      // stats 不可用不影响 status 的展示
    }

    rows.value = out;
  } catch (e) {
    error.value = messageOf(e);
    rows.value = [];
  } finally {
    loading.value = false;
  }
}

onMounted(() => { void load(); });
</script>

<template>
  <div class="main__body">
    <div class="toolbar">
      <FhButton :loading="loading" @click="load()">
        <RefreshCw :size="14" /><span>{{ t('common.refresh') }}</span>
      </FhButton>
    </div>

    <p v-if="error" class="notice notice--err">{{ error }}</p>

    <div v-if="rows.length" class="card">
      <div class="card-head">
        <h3>{{ t('runtime.pageTitle') }}</h3>
        <span class="card-note">{{ t('runtime.pageNote') }}</span>
      </div>
      <div class="kv-table">
        <div v-for="r in rows" :key="r.k" class="kv-row">
          <span class="k">{{ r.k }}</span><span class="v mono">{{ r.v }}</span>
        </div>
      </div>
    </div>
    <div v-else-if="!loading && !error" class="table__empty">{{ t('runtime.loading') }}</div>
  </div>
</template>
