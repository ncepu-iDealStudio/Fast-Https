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
const rows = ref<Record<string, unknown>[]>([]);

async function load(): Promise<void> {
  loading.value = true;
  error.value = '';
  try {
    const r = await runCommand(['ops', 'list', '--limit', '50', '--json']);
    const data = r.data as { operations?: Record<string, unknown>[] } | null;
    rows.value = data?.operations ?? [];
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

    <div class="table">
      <div class="table__head">
        <span>op</span>
        <span>{{ t('common.status') }}</span>
        <span>time</span>
      </div>
      <div v-if="!rows.length && !loading" class="table__empty">{{ t('ops.empty') }}</div>
      <div v-for="(r, i) in rows" :key="i" class="table__row">
        <span class="mono ellipsis">{{ r.id }}</span>
        <span>
          <span class="badge" :class="String(r.status) === 'succeeded' ? 'badge--ok' : 'badge--off'">
            {{ r.status }}
          </span>
        </span>
        <span class="muted mono">{{ r.created_at ?? r.time }}</span>
      </div>
    </div>
  </div>
</template>
