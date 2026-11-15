<script setup lang="ts">
/** 最近动态：Monitor 事件时间线。 */
import { Clock } from '@lucide/vue';
import { useI18n } from '../../../composables/useI18n';

interface Ev { kind?: string; at?: string; time?: string; message?: string }
defineProps<{ events: Ev[]; title: string }>();
const { t } = useI18n();

function fmt(s?: string): string {
  return (s ?? '').replace('T', ' ').slice(0, 19) || '—';
}
function text(e: Ev): string {
  return e.kind || e.message || '—';
}
</script>

<template>
  <section class="panel">
    <div class="panel__head"><h3>{{ title }}</h3><Clock :size="14" /></div>
    <ul class="event-list">
      <li v-if="!events.length" class="event-list__empty">{{ t('events.empty') }}</li>
      <li v-for="(e, i) in events" :key="i" class="event-list__item">
        <span class="ev-dot" />
        <div><b>{{ text(e) }}</b><small>{{ fmt(e.at || e.time) }}</small></div>
      </li>
    </ul>
  </section>
</template>
