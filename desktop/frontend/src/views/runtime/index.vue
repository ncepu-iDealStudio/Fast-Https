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
const snapshot = ref<Record<string, unknown> | null>(null);

async function load(): Promise<void> {
  loading.value = true;
  error.value = '';
  try {
    const r = await runCommand(['runtime', 'show', '--json']);
    snapshot.value = (r.data ?? null) as Record<string, unknown> | null;
  } catch (e) {
    error.value = messageOf(e);
    snapshot.value = null;
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

    <div v-if="snapshot" class="cards">
      <div v-for="(v, k) in snapshot" :key="k" class="card">
        <div class="card__k mono">{{ k }}</div>
        <div class="card__v mono">{{ typeof v === 'object' ? JSON.stringify(v) : v }}</div>
      </div>
    </div>
    <div v-else-if="!loading && !error" class="table__empty">{{ t('runtime.loading') }}</div>
  </div>
</template>
