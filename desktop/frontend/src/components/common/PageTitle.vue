<script setup lang="ts">
/** 页面标题栏：只显示当前页面名称。运行状态由底部状态栏统一承担，不在这里重复。 */
defineProps<{ title: string }>();
</script>

<template>
  <div class="page-title">
    <h1>{{ title }}</h1>
    <slot />
  </div>
</template>
