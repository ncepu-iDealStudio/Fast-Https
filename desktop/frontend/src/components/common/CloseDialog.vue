<script setup lang="ts">
/** 关闭窗口时的确认：隐藏到托盘 还是 真正退出。 */
import { useI18n } from '../../composables/useI18n';
import FhDialog from '../ui/FhDialog.vue';
import FhButton from '../ui/FhButton.vue';
import { hideWindow, quitApp } from '../../services/api/app';

defineProps<{ open: boolean }>();
const emit = defineEmits<{ 'update:open': [v: boolean] }>();
const { t } = useI18n();

function toTray(): void {
  emit('update:open', false);
  void hideWindow();
}
function quit(): void {
  emit('update:open', false);
  void quitApp();
}
</script>

<template>
  <FhDialog
    :open="open" :title="t('dialog.close.title')"
    width="380px" @update:open="(v: boolean) => emit('update:open', v)"
  >
    <p class="dialog-hint" v-html="t('dialog.close.body')" />

    <template #footer>
      <FhButton @click="emit('update:open', false)">{{ t('common.cancel') }}</FhButton>
      <FhButton @click="toTray">{{ t('dialog.close.hide') }}</FhButton>
      <FhButton variant="primary" @click="quit">{{ t('dialog.close.quit') }}</FhButton>
    </template>
  </FhDialog>
</template>
