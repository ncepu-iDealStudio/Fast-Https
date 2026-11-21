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

const W = 100;
const H = 36;

/** 把数值序列映射到 100×36 的坐标点。全等值时画一条水平中线。 */
function coords(series: number[]): Array<[number, number]> {
  if (!series || series.length < 2) return [];
  const min = Math.min(...series);
  const max = Math.max(...series);
  const span = max - min;
  return series.map((v, i) => {
    const x = (i / (series.length - 1)) * W;
    const y = span === 0 ? H / 2 : H - ((v - min) / span) * (H - 8) - 4;
    return [x, y];
  });
}

/** 平滑折线（Catmull-Rom → 三次贝塞尔），比直角折线更像「心率」。 */
function linePath(series: number[]): string {
  const p = coords(series);
  if (!p.length) return '';
  let d = `M${p[0][0].toFixed(1)},${p[0][1].toFixed(1)}`;
  for (let i = 0; i < p.length - 1; i++) {
    const p0 = p[i - 1] ?? p[i];
    const p1 = p[i];
    const p2 = p[i + 1];
    const p3 = p[i + 2] ?? p2;
    const c1x = p1[0] + (p2[0] - p0[0]) / 6;
    const c1y = p1[1] + (p2[1] - p0[1]) / 6;
    const c2x = p2[0] - (p3[0] - p1[0]) / 6;
    const c2y = p2[1] - (p3[1] - p1[1]) / 6;
    d += ` C${c1x.toFixed(1)},${c1y.toFixed(1)} ${c2x.toFixed(1)},${c2y.toFixed(1)} ${p2[0].toFixed(1)},${p2[1].toFixed(1)}`;
  }
  return d;
}

/** 折线下方的面积填充。 */
function areaPath(series: number[]): string {
  const line = linePath(series);
  return line ? `${line} L${W},${H} L0,${H} Z` : '';
}

function hasTrend(c: MetricCard): boolean {
  return !!c.trend && c.trend.length >= 2;
}
</script>

<template>
  <div class="metric-grid">
    <div
      v-for="(c, i) in cards" :key="c.label"
      class="mcard" :class="[`mcard--${c.tone}`, { 'mcard--warn': c.warn }]"
      :style="{ '--i': i }"
    >
      <div class="mcard__top">
        <span class="mcard__ico" :class="`ico-${c.tone}`">
          <component :is="c.ico" :size="14" />
        </span>
        <span class="mcard__label">{{ c.label }}</span>
        <span class="mcard__sub" :class="`is-${c.subTone}`">
          <span v-if="c.subTone !== 'muted'" class="mcard__dot" />
          {{ c.sub }}
        </span>
      </div>
      <div class="mcard__val">{{ c.value }}</div>
      <svg
        class="mcard__spark" :class="`spark-${c.tone}`"
        :viewBox="`0 0 ${W} ${H}`" preserveAspectRatio="none" aria-hidden="true"
      >
        <defs>
          <linearGradient :id="`mfill-${i}`" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stop-color="currentColor" stop-opacity=".18" />
            <stop offset="100%" stop-color="currentColor" stop-opacity="0" />
          </linearGradient>
        </defs>
        <template v-if="hasTrend(c)">
          <path :d="areaPath(c.trend!)" :fill="`url(#mfill-${i})`" />
          <path :d="linePath(c.trend!)" class="mcard__line" />
        </template>
        <!-- 没有历史采样时只画一条虚线基线，不编造趋势 -->
        <line v-else x1="0" :y1="H - 6" :x2="W" :y2="H - 6" class="mcard__base" />
      </svg>
    </div>
  </div>
</template>
