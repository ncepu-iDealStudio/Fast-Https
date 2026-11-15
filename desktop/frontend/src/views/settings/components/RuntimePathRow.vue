<script setup lang="ts">
/** 只读运行期路径行：pid / 日志根，派生自安装位置，不可编辑。 */
defineProps<{ label: string; hint: string; value: string }>();
</script>

<template>
  <div class="cfg-row">
    <label class="cfg-row__label">
      <span>{{ label }}</span>
      <small>{{ hint }}</small>
    </label>
    <div class="cfg-row__control">
      <div class="readonly-path mono" :title="value">{{ value || '—' }}</div>
    </div>
  </div>
</template>
