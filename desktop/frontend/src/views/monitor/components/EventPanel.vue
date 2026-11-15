<script setup lang="ts">
/** 最近动态：Monitor 事件时间线，彩色状态点 + 顶部链接。 */
import { useI18n } from '../../../composables/useI18n';

interface Ev { kind?: string; at?: string; time?: string; message?: string; level?: string }
defineProps<{ events: Ev[]; title: string; link?: string }>();
const emit = defineEmits<{ open: [] }>();
const { t } = useI18n();

function fmt(s?: string): string {
  return (s ?? '').replace('T', ' ').slice(0, 19) || '';
}
function tone(e: Ev): 'ok' | 'warn' | 'err' | 'muted' {
  const k = (e.kind || '').toLowerCase();
  if (k.includes('error') || k.includes('fail')) return 'err';
  if (k.includes('warn')) return 'warn';
  if (k.includes('start') || k.includes('recover')) return 'ok';
  return 'muted';
}
</script>

<template>
  <section class="panel">
    <div class="panel__head">
      <h3>{{ title }}</h3>
      <button v-if="link" class="panel__link" @click="emit('open')">{{ link }}</button>
    </div>
    <ul class="event-list">
      <li v-if="!events.length" class="event-list__empty">{{ t('events.empty') }}</li>
      <li v-for="(e, i) in events" :key="i" class="event-item">
        <span class="event-item__dot" :class="`is-${tone(e)}`" />
        <div class="event-item__body">
          <b>{{ e.kind || e.message || '—' }}</b>
          <small v-if="e.message && e.kind">{{ e.message }}</small>
          <small>{{ fmt(e.at || e.time) }}</small>
        </div>
      </li>
    </ul>
  </section>
</template>
