<script setup lang="ts">
/** 运行概览四张指标卡：彩色圆底图标 + 大数字 + 状态小字 + 趋势线。 */
interface Metric {
  label: string;
  value: number | string;
  sub: string;
  subTone: 'ok' | 'warn' | 'muted';
  ico: unknown;
  icoTone: 'blue' | 'green' | 'orange' | 'purple';
  trend?: number[];
}
defineProps<{ metrics: Metric[] }>();
</script>

<template>
  <div class="metric-grid">
    <div v-for="m in metrics" :key="m.label" class="metric-card">
      <div class="metric-card__top">
        <span>{{ m.label }}</span>
        <span class="metric-card__ico" :class="`metric-card__ico--${m.icoTone}`">
          <component :is="m.ico" :size="15" />
        </span>
      </div>
      <div class="metric-card__value mono">{{ m.value }}</div>
      <div class="metric-card__foot">
        <span class="metric-card__sub" :class="`is-${m.subTone}`">
          <span v-if="m.subTone !== 'muted'" class="metric-card__dot" />
          {{ m.sub }}
        </span>
        <svg v-if="m.trend?.length" class="metric-card__spark" viewBox="0 0 60 20" preserveAspectRatio="none">
          <polyline
            :points="m.trend.map((v, i) => `${(i * 60) / (m.trend!.length - 1)},${20 - v}`).join(' ')"
            fill="none" stroke="currentColor" stroke-width="1.5"
          />
        </svg>
      </div>
    </div>
  </div>
</template>
