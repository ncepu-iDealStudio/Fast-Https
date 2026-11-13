<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from '../../composables/useI18n';
import { useOverviewStore } from '../../stores/overview';
import { formatUptime } from '../../utils/format';

// 页面只依赖 store 与纯工具，不 import services / bindings。
const { t } = useI18n();
const overview = useOverviewStore();

interface Card { key: string; value: string | number; mono?: boolean }
const cards = computed<Card[]>(() => [
  { key: 'nav.sites', value: overview.data?.siteCount ?? '—' },
  { key: 'nav.certs', value: overview.data?.tlsCount ?? '—' },
  { key: 'overview.configState', value: overview.data?.configVersion ?? '—', mono: true },
  { key: 'common.status', value: overview.data ? formatUptime(overview.data.uptimeSeconds) : '—' },
]);
</script>

<template>
  <div class="main__body">
    <section class="cards">
      <div v-for="card in cards" :key="card.key" class="card">
        <div class="card__k">{{ t(card.key) }}</div>
        <div class="card__v" :class="{ mono: card.mono }">{{ card.value }}</div>
      </div>
      <div v-if="overview.error" class="card card--error">{{ overview.error }}</div>
    </section>
  </div>
</template>
