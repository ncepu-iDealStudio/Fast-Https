<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { RefreshCw } from '@lucide/vue';
import { useI18n } from '../../composables/useI18n';
import { runCommand } from '../../services/api/command';
import { messageOf } from '../../services/api/base';
import { useOverviewStore } from '../../stores/overview';
import FhButton from '../../components/ui/FhButton.vue';

const { t } = useI18n();
const overview = useOverviewStore();
const loading = ref(false);
const error = ref('');
const monitor = ref<{ status?: string } | null>(null);
const events = ref<Record<string, unknown>[]>([]);

async function load(): Promise<void> {
  loading.value = true;
  error.value = '';
  try {
    await overview.refresh();
    const st = await runCommand(['monitor', 'status']);
    monitor.value = (st.data ?? null) as { status?: string } | null;
    const ev = await runCommand(['monitor', 'events', '--limit', '8']);
    const d = ev.data as { events?: Record<string, unknown>[] } | null;
    events.value = d?.events ?? [];
  } catch (e) {
    error.value = messageOf(e);
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
        <div class="card__v mono">{{ (overview.data?.ports ?? []).join(', ') || '—' }}</div>
      </div>
      <div class="card">
        <div class="card__k">{{ t('nav.certs') }}</div>
        <div class="card__v">{{ overview.data?.tlsCount ?? '—' }}</div>
      </div>
      <div class="card">
        <div class="card__k">{{ t('matrix.runtimeTitle') }}</div>
        <div class="card__v">{{ overview.data?.status ?? '—' }}</div>
      </div>
    </div>

    <div class="table" style="margin-top: 16px">
      <div class="table__head">
        <span>{{ t('matrix.accessTitle') }}</span>
        <span>{{ t('common.status') }}</span>
      </div>
      <div v-if="!events.length && !loading" class="table__empty">{{ t('certs.acmeEmpty') }}</div>
      <div v-for="(e, i) in events" :key="i" class="table__row">
        <span class="mono ellipsis">{{ e.kind ?? e.type }}</span>
        <span class="muted mono">{{ e.at ?? e.time }}</span>
      </div>
    </div>

    <p class="lede" v-if="monitor">
      monitor.status = <span class="mono">{{ monitor.status }}</span>
    </p>
  </div>
</template>
