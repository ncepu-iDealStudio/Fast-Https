<script setup lang="ts">
/** 运行概览四张指标卡：连接 / 请求 / 响应分布 / 上游。 */

interface Metric {
  key: string;
  value: number | string;
  sub: string;
  ico: unknown;
  tone?: 'warn' | '';
  trend?: number[];
}
defineProps<{ metrics: Metric[] }>();
</script>

<template>
  <div class="metric-grid">
    <div
      v-for="m in metrics" :key="m.key"
      class="metric-card" :class="{ 'metric-card--warn': m.tone === 'warn' }"
    >
      <div class="metric-card__top">
        <span>{{ m.key }}</span>
        <component :is="m.ico" :size="16" />
      </div>
      <div class="metric-card__value mono">{{ m.value }}</div>
      <div class="metric-card__foot">
        <small>{{ m.sub }}</small>
        <!-- 极简趋势线：纯 SVG，不引图表库 -->
        <svg v-if="m.trend?.length" class="metric-card__spark" viewBox="0 0 60 20" preserveAspectRatio="none">
          <polyline
            :points="m.trend.map((v, i) => `${i * 60 / (m.trend!.length - 1)},${20 - v}`).join(' ')"
            fill="none" stroke="currentColor" stroke-width="1.5"
          />
        </svg>
      </div>
    </div>
  </div>
</template>
