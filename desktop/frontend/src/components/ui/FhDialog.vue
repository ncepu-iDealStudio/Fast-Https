<script setup lang="ts">
/**
 * 用 Reka UI 的 Dialog 原语（headless：焦点陷阱、Esc 关闭、aria 全由它负责），
 * 外观完全由本文件的 scoped CSS 决定 —— 这正是「不上成品组件库」的意义。
 */
import {
  DialogRoot, DialogPortal, DialogOverlay, DialogContent,
  DialogTitle, DialogDescription, DialogClose,
} from 'reka-ui';
import { X } from '@lucide/vue';
import { useI18n } from '../../composables/useI18n';

withDefaults(defineProps<{
  open: boolean;
  title: string;
  description?: string;
  width?: string;
}>(), { description: '', width: '430px' });

const emit = defineEmits<{ 'update:open': [value: boolean] }>();
const { t } = useI18n();
</script>

<template>
  <DialogRoot :open="open" @update:open="(v: boolean) => emit('update:open', v)">
    <DialogPortal>
      <DialogOverlay class="fh-dialog__overlay" />
      <DialogContent class="fh-dialog" :style="{ width }">
        <div class="fh-dialog__head">
          <DialogTitle class="fh-dialog__title">{{ title }}</DialogTitle>
          <DialogClose class="fh-dialog__close" :aria-label="t('common.close')"><X :size="16" /></DialogClose>
        </div>
        <DialogDescription v-if="description" class="fh-dialog__desc">{{ description }}</DialogDescription>
        <div class="fh-dialog__body"><slot /></div>
        <div v-if="$slots.footer" class="fh-dialog__footer"><slot name="footer" /></div>
      </DialogContent>
    </DialogPortal>
  </DialogRoot>
</template>
