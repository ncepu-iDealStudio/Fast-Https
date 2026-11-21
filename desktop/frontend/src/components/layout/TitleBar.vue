<script setup lang="ts">
/**
 * 顶栏：左品牌、中间「当前位置」胶囊（页面图标 + 名称 + 实时时钟）、右窗口按钮。
 * 整条可拖拽，按钮区域 no-drag。
 */
import { computed, onMounted, onUnmounted, ref } from 'vue';
import { useI18n } from '../../composables/useI18n';
import { useAppStore } from '../../stores/app';
import * as appApi from '../../services/api/app';
import CloseDialog from '../common/CloseDialog.vue';
import { navItemOf } from './navItems';

const { t } = useI18n();
const app = useAppStore();
const closeOpen = ref(false);

const current = computed(() => navItemOf(app.pane));

const now = ref(new Date());
let timer: number | undefined;
onMounted(() => { timer = window.setInterval(() => { now.value = new Date(); }, 1000); });
onUnmounted(() => { if (timer) window.clearInterval(timer); });
const clock = computed(() => {
  const d = now.value;
  const p = (n: number) => String(n).padStart(2, '0');
  return `${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
});

async function minimise(): Promise<void> { await appApi.minimiseWindow(); }
async function maximise(): Promise<void> { await appApi.toggleMaximiseWindow(); }
function requestHide(): void { closeOpen.value = true; }
</script>

<template>
  <header class="topbar">
    <div class="brand">
      <span class="mark-wrap"><img class="mark" src="/appicon.png" alt="fast-https" /></span>
      <b class="grad-text">fast-https</b><small>DESKTOP</small>
    </div>

    <div class="crumb" aria-live="polite">
      <span class="crumb__ico"><component :is="current.icon" :size="13" /></span>
      <Transition name="crumb" mode="out-in">
        <span :key="current.key" class="crumb__txt">{{ t(current.label) }}</span>
      </Transition>
      <span class="crumb__sep" />
      <span class="crumb__clock mono">{{ clock }}</span>
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
