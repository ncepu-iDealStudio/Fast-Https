<script setup lang="ts">
/**
 * 组件健康卡：图标 + 名称 + 状态徽标 + 描述 + 右下角查看链接。
 * 每张卡是一个可跳转的入口，action 为空则不显示链接。
 */
import { ChevronRight } from '@lucide/vue';

export interface HealthCard {
  title: string;
  ico: unknown;
  ok: boolean;
  status: string;
  desc: string;
  meta?: string;
  action?: string;
  pane?: string;
}
defineProps<{ cards: HealthCard[] }>();
const emit = defineEmits<{ go: [pane: string] }>();
</script>

<template>
  <div class="comp-grid">
    <div v-for="c in cards" :key="c.title" class="comp">
      <div class="comp__top">
        <span class="comp__ico"><component :is="c.ico" :size="12" /></span>
        <span class="comp__name">{{ c.title }}</span>
        <span class="badge" :class="c.ok ? 'badge--ok' : 'badge--off'">
          <span class="badge__dot" />{{ c.status }}
        </span>
      </div>
      <div class="comp__desc">{{ c.desc }}</div>
      <div class="comp__bottom">
        <span class="comp__meta">{{ c.meta }}</span>
        <button v-if="c.action && c.pane" class="comp__link" @click="emit('go', c.pane)">
          {{ c.action }}<ChevronRight :size="11" />
        </button>
      </div>
    </div>
  </div>
</template>
