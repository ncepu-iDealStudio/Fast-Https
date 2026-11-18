<script setup lang="ts">
import { ref, watch } from 'vue';
import { useI18n } from '../../composables/useI18n';
import FhDialog from '../ui/FhDialog.vue';
import FhButton from '../ui/FhButton.vue';
import { certTrustGuide, importLocalCA } from '../../services/api/cert';
import { openConfigDir } from '../../services/api/app';
import { messageOf } from '../../services/api/base';

const props = defineProps<{ open: boolean }>();
const emit = defineEmits<{ 'update:open': [v: boolean] }>();
const { t } = useI18n();

interface Guide {
  platform?: string; platformLabel?: string; risk?: string;
  certPath?: string; keyPath?: string; exists?: boolean;
  steps?: string[]; command?: string;
}

const guide = ref<Guide>({});
const busy = ref(false);
const error = ref('');
const result = ref('');

watch(() => props.open, async (open) => {
  if (!open) return;
  error.value = '';
  result.value = '';
  try {
    guide.value = (await certTrustGuide()) as Guide;
  } catch (e) {
    error.value = messageOf(e);
  }
}, { immediate: true });

async function doImport(): Promise<void> {
  busy.value = true;
  error.value = '';
  try {
    result.value = await importLocalCA();
  } catch (e) {
    error.value = messageOf(e);
  } finally {
    busy.value = false;
  }
}

async function openDir(): Promise<void> {
  if (guide.value.certPath) {
    try { await openConfigDir(guide.value.certPath); } catch (e) { error.value = messageOf(e); }
  }
}

async function copyCmd(): Promise<void> {
  try {
    await navigator.clipboard.writeText(guide.value.command ?? '');
    result.value = t('dialog.trust.copied');
  } catch {
    result.value = guide.value.command ?? '';
  }
}
</script>

<template>
  <FhDialog :open="open" :title="t('dialog.trust.title')" width="680px"
            @update:open="(v: boolean) => emit('update:open', v)">
    <p v-if="guide.risk" class="notice">{{ guide.risk }}</p>

    <div class="trust-path">
      <span class="mono">{{ guide.certPath || '—' }}</span>
      <span class="badge" :class="guide.exists ? 'badge--ok' : 'badge--off'">
        {{ guide.exists ? t('dialog.trust.exists') : t('dialog.trust.missing') }}
      </span>
    </div>

    <p class="field-note" v-html="t('dialog.trust.hint')" />

    <ol class="trust-steps">
      <li v-for="(s, i) in (guide.steps ?? [])" :key="i">{{ s }}</li>
    </ol>

    <pre v-if="guide.command" class="cmd-result">{{ guide.command }}</pre>

    <p v-if="result" class="notice">{{ result }}</p>
    <p v-if="error" class="dialog-error">{{ error }}</p>

    <template #footer>
      <FhButton @click="emit('update:open', false)">{{ t('common.cancel') }}</FhButton>
      <FhButton @click="openDir">{{ t('dialog.trust.openDir') }}</FhButton>
      <FhButton @click="copyCmd">{{ t('dialog.trust.copy') }}</FhButton>
      <FhButton variant="primary" :loading="busy" :disabled="!guide.exists" @click="doImport">
        {{ t('dialog.trust.import') }}
      </FhButton>
    </template>
  </FhDialog>
</template>
