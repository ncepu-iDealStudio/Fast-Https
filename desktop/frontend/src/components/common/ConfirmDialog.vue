<script setup lang="ts">
import FhDialog from '../ui/FhDialog.vue';
import FhButton from '../ui/FhButton.vue';
import { useI18n } from '../../composables/useI18n';

withDefaults(defineProps<{
  open: boolean;
  title: string;
  message: string;
  loading?: boolean;
}>(), { loading: false });

const emit = defineEmits<{
  'update:open': [value: boolean];
  confirm: [];
}>();
const { t } = useI18n();

function close(): void { emit('update:open', false); }
function confirm(): void { emit('confirm'); }
</script>

<template>
  <FhDialog :open="open" :title="title" :description="message" @update:open="(v: boolean) => emit('update:open', v)">
    <slot />
    <template #footer>
      <FhButton @click="close">{{ t('common.cancel') }}</FhButton>
      <FhButton variant="primary" :loading="loading" @click="confirm">{{ t('common.ok') }}</FhButton>
    </template>
  </FhDialog>
</template>
