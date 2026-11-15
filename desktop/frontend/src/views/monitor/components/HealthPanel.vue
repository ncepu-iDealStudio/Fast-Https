<script setup lang="ts">
/** 组件健康：HTTP / Monitor / 站点 / 证书 四张卡。 */
defineProps<{
  items: { title: string; ico: unknown; status: string; ok: boolean }[];
  title: string;
}>();
</script>

<template>
  <section class="panel">
    <div class="panel__head"><h3>{{ title }}</h3></div>
    <div class="health-grid">
      <div v-for="h in items" :key="h.title" class="health-item">
        <div class="health-item__ico"><component :is="h.ico" :size="16" /></div>
        <div class="health-item__body">
          <b>{{ h.title }}</b>
          <small>{{ h.status }}</small>
        </div>
        <span class="dot" :class="h.ok ? 'state-ok' : 'state-off'" />
      </div>
    </div>
  </section>
</template>
