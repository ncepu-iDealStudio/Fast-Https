<script setup lang="ts">
/**
 * 最近动态：Monitor 采集到的事件（来自 logs/monitor/events.jsonl）。
 * 注意：这是探针发现的运行异常，与「操作记录」（用户执行过的命令）是两个不同的数据源，
 * 因此这里不提供跳转到操作记录的入口。
 * 面板内部独立滚动。
 */
import { computed } from 'vue';
import { Check, AlertTriangle, Info } from '@lucide/vue';
import { useI18n } from '../../../composables/useI18n';
import { humanizeEvent, type RawEvent } from '../composables/humanizeEvent';

const props = defineProps<{ events: RawEvent[]; title: string; limit: number }>();
const { t } = useI18n();

const rows = computed(() => props.events.map((e) => humanizeEvent(e, t)));

function iconOf(tone: 'ok' | 'warn' | 'info') {
  return tone === 'ok' ? Check : tone === 'warn' ? AlertTriangle : Info;
}
</script>

<template>
  <section class="pane">
    <div class="pane__head">
      <h3>{{ title }}</h3>
      <span class="pane__note">{{ t('monitor.eventLimit', { n: limit }) }}</span>
    </div>

    <div class="evs">
      <div v-if="!rows.length" class="evs__empty">{{ t('events.empty') }}</div>
      <div v-for="(r, i) in rows" :key="i" class="ev">
        <span class="ev__ico" :class="`ev__ico--${r.tone}`">
          <component :is="iconOf(r.tone)" :size="9" />
        </span>
        <div class="ev__body">
          <div class="ev__h">{{ r.title }}</div>
          <div v-if="r.detail" class="ev__s">{{ r.detail }}</div>
        </div>
        <span class="ev__time">{{ r.time }}</span>
      </div>
    </div>
  </section>
</template>
