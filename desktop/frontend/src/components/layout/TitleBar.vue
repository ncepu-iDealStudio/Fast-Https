<script setup lang="ts">
import { ref } from 'vue';
import { useI18n } from '../../composables/useI18n';
import * as appApi from '../../services/api/app';
import CloseDialog from '../common/CloseDialog.vue';

const { t } = useI18n();
const closeOpen = ref(false);

async function minimise(): Promise<void> { await appApi.minimiseWindow(); }
async function maximise(): Promise<void> { await appApi.toggleMaximiseWindow(); }
function requestHide(): void { closeOpen.value = true; }
</script>

<template>
  <header class="topbar">
    <div class="brand">
      <img class="mark" src="/appicon.png" alt="fast-https" />
      <b>fast-https</b><small>DESKTOP</small>
    </div>
    <div class="win-controls">
      <button class="win-btn" :aria-label="t('win.minimise')" :title="t('win.minimise')" @click="minimise">
        <svg viewBox="0 0 10 10"><path d="M1 5h8" /></svg>
      </button>
      <button class="win-btn" :aria-label="t('win.maximise')" :title="t('win.maximise')" @click="maximise">
        <svg viewBox="0 0 10 10"><rect x="1.7" y="1.7" width="6.6" height="6.6" /></svg>
      </button>
      <button class="win-btn close" :aria-label="t('common.close')" :title="t('common.close')" @click="requestHide">
        <svg viewBox="0 0 10 10"><path d="M2.2 2.2l5.6 5.6M7.8 2.2l-5.6 5.6" /></svg>
      </button>
    </div>

    <CloseDialog v-model:open="closeOpen" />
  </header>
</template>
