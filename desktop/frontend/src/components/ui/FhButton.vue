<script setup lang="ts">
import { computed } from 'vue';

const props = withDefaults(defineProps<{
  variant?: 'primary' | 'default' | 'ghost' | 'danger';
  size?: 'normal' | 'small';
  loading?: boolean;
  disabled?: boolean;
}>(), {
  variant: 'default', size: 'normal', loading: false, disabled: false,
});

const cls = computed(() => [
  'fh-btn', `fh-btn--${props.variant}`, `fh-btn--${props.size}`,
]);
</script>

<template>
  <button :class="cls" :disabled="disabled || loading" type="button">
    <span v-if="loading" class="fh-btn__spinner" aria-hidden="true" />
    <slot />
  </button>
</template>
