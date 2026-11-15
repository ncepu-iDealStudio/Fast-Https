<script setup lang="ts">
/**
 * 运行概览四卡：彩色圆底图标 + 大数字 + 状态小字 + 迷你趋势线。
 * 趋势线只在有真实历史采样时绘制（至少 2 个点）；没有数据就留空，不编造。
 */
export interface MetricCard {
  label: string;
  value: number | string;
  sub: string;
  subTone: 'ok' | 'warn' | 'muted';
  ico: unknown;
  tone: 'blue' | 'green' | 'orange' | 'purple';
  trend?: number[];
  warn?: boolean;
}
defineProps<{ cards: MetricCard[] }>();

/** 把数值序列映射到 54×20 的折线坐标。全等值时画一条水平中线。 */
function points(series: number[]): string {
  if (!series || series.length < 2) return '';
  const min = Math.min(...series);
  const max = Math.max(...series);
  const span = max - min;
  const w = 54;
  const h = 20;
  return series
    .map((v, i) => {
      const x = (i / (series.length - 1)) * w;
      const y = span === 0 ? h / 2 : h - ((v - min) / span) * (h - 4) - 2;
      return `${x.toFixed(1)},${y.toFixed(1)}`;
    })
    .join(' ');
}
</script>

<template>
  <div class="metric-grid">
    <div v-for="c in cards" :key="c.label" class="mcard" :class="{ 'mcard--warn': c.warn }">
      <div class="mcard__top">
        <span class="mcard__label">{{ c.label }}</span>
        <span class="mcard__ico" :class="`ico-${c.tone}`">
          <component :is="c.ico" :size="13" />
        </span>
      </div>
      <div class="mcard__val">{{ c.value }}</div>
      <div class="mcard__foot">
        <span class="mcard__sub" :class="`is-${c.subTone}`">
          <span v-if="c.subTone !== 'muted'" class="mcard__dot" />
          {{ c.sub }}
        </span>
        <svg
          v-if="c.trend && c.trend.length >= 2"
          class="mcard__spark" :class="`spark-${c.tone}`"
          viewBox="0 0 54 20" preserveAspectRatio="none"
        >
          <polyline :points="points(c.trend)" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linejoin="round" />
        </svg>
      </div>
    </div>
  </div>
</template>
