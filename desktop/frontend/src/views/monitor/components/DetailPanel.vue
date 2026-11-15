<script setup lang="ts">
/** 运行数据明细：连接 / 请求 / 响应 / 上游 四组键值。 */
defineProps<{
  groups: { title: string; rows: { k: string; v: string | number }[] }[];
  title: string;
}>();
</script>

<template>
  <section class="panel">
    <div class="panel__head"><h3>{{ title }}</h3></div>
    <div class="detail-grid">
      <div v-for="g in groups" :key="g.title" class="detail-group">
        <div class="detail-group__title">{{ g.title }}</div>
        <div v-for="r in g.rows" :key="r.k" class="detail-row">
          <span class="mono">{{ r.k }}</span><b class="mono">{{ r.v }}</b>
        </div>
        <div v-if="!g.rows.length" class="detail-row muted">—</div>
      </div>
    </div>
  </section>
</template>
