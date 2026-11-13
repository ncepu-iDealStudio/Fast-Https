<script setup lang="ts">
import { useI18n } from '../../composables/useI18n';

defineProps<{ title: string; running: boolean }>();
const { t } = useI18n();
</script>

<template>
  <div class="main__head">
    <h1>{{ title }}</h1>
    <span class="badge" :class="running ? 'badge--ok' : 'badge--off'">
      {{ running ? t('status.running') : t('status.stopped') }}
    </span>
  </div>
</template>
