<script setup lang="ts">
/** 组件健康四卡：图标 + 标题 + 描述 + 徽标 + 查看详情链接。 */
import { ChevronRight } from '@lucide/vue';

export interface HealthItem {
  title: string;
  desc: string;
  status: string;
  ok: boolean;
  ico: unknown;
  action?: string;
}
defineProps<{ items: HealthItem[]; title: string; note: string }>();
const emit = defineEmits<{ detail: [title: string] }>();
</script>

<template>
  <div class="section-head"><span>{{ title }}</span><small>{{ note }}</small></div>
  <div class="health-grid">
    <div v-for="h in items" :key="h.title" class="health-card">
      <div class="health-card__head">
        <div class="health-card__ico"><component :is="h.ico" :size="15" /></div>
        <b>{{ h.title }}</b>
        <span class="badge" :class="h.ok ? 'badge--ok' : 'badge--off'">{{ h.status }}</span>
      </div>
      <div class="health-card__desc">{{ h.desc }}</div>
      <button v-if="h.action" class="health-card__link" @click="emit('detail', h.title)">
        {{ h.action }} <ChevronRight :size="12" />
      </button>
    </div>
  </div>
</template>
