<script setup lang="ts">
import { onMounted } from 'vue';
import { RefreshCw } from '@lucide/vue';
import { useI18n } from '../../composables/useI18n';
import { useLogsStore, type LogKind } from '../../stores/certs';
import FhButton from '../../components/ui/FhButton.vue';

const { t } = useI18n();
const logs = useLogsStore();

const kinds: { key: LogKind; label: string }[] = [
  { key: 'access', label: 'logs.access' },
  { key: 'error', label: 'logs.error' },
  { key: 'system', label: 'logs.system' },
  { key: 'safe', label: 'logs.safe' },
];

onMounted(() => { void logs.load(); });
</script>

<template>
  <div class="main__body">
    <div class="toolbar">
      <button
        v-for="k in kinds" :key="k.key"
        class="tab" :class="{ 'tab--on': logs.kind === k.key }"
        @click="logs.load(k.key)"
      >{{ t(k.label) }}</button>
      <FhButton :loading="logs.loading" @click="logs.load()">
        <RefreshCw :size="14" /><span>{{ t('common.refresh') }}</span>
      </FhButton>
    </div>

    <p v-if="logs.error" class="notice notice--err">{{ logs.error }}</p>

    <div class="logview">
      <div v-if="!logs.entries.length && !logs.loading" class="table__empty">
        {{ t('logs.empty') }}
      </div>
      <div v-for="(e, i) in logs.entries" :key="i" class="logview__row" :class="`logview__row--${e.level || 'plain'}`">
        {{ e.raw || e.message }}
      </div>
    </div>
  </div>
</template>
