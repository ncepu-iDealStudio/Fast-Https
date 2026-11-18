<script setup lang="ts">
/**
 * 统一的表单行：左侧 label（含说明）+ 右侧控件。
 * 原版每个弹窗各写一套 .form-row 并自己调列宽，导致控件对不齐；
 * 这里列宽/间距全部走设计令牌，所有弹窗共用一份。
 */
withDefaults(defineProps<{
  label: string;
  hint?: string;
  labelWidth?: string;
}>(), {
  hint: '',
  labelWidth: 'var(--field-label-w)',
});
</script>

<template>
  <div class="fh-field" :style="{ '--field-w': labelWidth }">
    <label class="fh-field__label">
      <span class="fh-field__title">{{ label }}</span>
      <small v-if="hint" class="fh-field__hint">{{ hint }}</small>
    </label>
    <div class="fh-field__control"><slot /></div>
  </div>
</template>
