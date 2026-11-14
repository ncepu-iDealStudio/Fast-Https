<script setup lang="ts">
/**
 * 左下角「本机管理」账户菜单：外观切换、语言切换、设置入口、隐藏到托盘、帮助文档。
 * 语言列表从 i18n 的 LANGS 注册表生成，label 用各语言自己的写法；
 * 新增语言只要在注册表加一行，这里不用改。
 *
 * 组件自带触发按钮（.side-foot）与面板（.account-menu）：面板按旧版规格
 * 绝对定位在窗口左下角，放进侧栏底部即可，不需要父组件再传状态。
 * 帮助弹窗也由本组件持有，点击「帮助文档」直接打开。
 */
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { ChevronDown, Settings2 } from '@lucide/vue';
import { useI18n } from '../../composables/useI18n';
import { useAppStore } from '../../stores/app';
import { LANGS } from '../../i18n/config';
import { hideWindow } from '../../services/api/app';
import { messageOf } from '../../services/api/base';
import HelpDialog from './HelpDialog.vue';
import type { Lang } from '../../types/i18n';

const { t } = useI18n();
const app = useAppStore();

const open = ref(false);
const helpOpen = ref(false);
const error = ref('');
const root = ref<HTMLElement | null>(null);

/** 语言项按注册表顺序展开，label 用各语言自称。 */
const langItems = computed(() =>
  (Object.entries(LANGS) as [Lang, { label: string }][]).map(([code, l]) => ({ code, label: l.label })));

const modes = computed(() => [
  { key: 'light' as const, label: 'menu.light' },
  { key: 'dark' as const, label: 'menu.dark' },
]);

function close(): void {
  open.value = false;
}

function toggle(): void {
  open.value = !open.value;
  error.value = '';
}

function setMode(mode: 'light' | 'dark'): void {
  if (app.mode !== mode) app.toggleMode();
}

function pickLang(code: Lang): void {
  if (app.lang !== code) app.changeLang(code);
  close();
}

function gotoSettings(): void {
  close();
  app.activate('settings');
}

async function toTray(): Promise<void> {
  close();
  error.value = '';
  try {
    await hideWindow();
  } catch (e) {
    error.value = messageOf(e);
  }
}

function openHelpDialog(): void {
  close();
  helpOpen.value = true;
}

function onDocClick(e: MouseEvent): void {
  if (!open.value) return;
  const el = root.value;
  if (el && !el.contains(e.target as Node | null)) close();
}

function onKeydown(e: KeyboardEvent): void {
  if (e.key === 'Escape' && open.value) {
    e.preventDefault();
    close();
  }
}

onMounted(() => {
  document.addEventListener('click', onDocClick);
  document.addEventListener('keydown', onKeydown);
});
onBeforeUnmount(() => {
  document.removeEventListener('click', onDocClick);
  document.removeEventListener('keydown', onKeydown);
});
</script>

<template>
  <div ref="root" class="side-foot-wrap">
    <button
      type="button" class="side-foot" aria-haspopup="menu" :aria-expanded="open"
      @click="toggle"
    >
      <span class="avatar"><Settings2 :size="12" /></span>
      <span class="acct-name">{{ t('nav.account') }}</span>
      <ChevronDown :size="12" class="acct-caret" />
    </button>

    <div v-if="open" class="account-menu" role="menu">
      <div class="menu-head">
        <img class="menu-logo" src="/appicon.png" alt="" />
        <div class="menu-head-text">
          <b>Fast-Https Desktop</b>
          <small>{{ t('app.console') }}</small>
        </div>
      </div>

      <div class="menu-title">{{ t('menu.appearance') }}</div>
      <button
        v-for="m in modes" :key="m.key" type="button" class="menu-item"
        role="menuitemradio" :aria-checked="app.mode === m.key"
        @click="setMode(m.key)"
      >
        <span>{{ t(m.label) }}</span>
        <span class="menu-check">{{ app.mode === m.key ? '✓' : '' }}</span>
      </button>

      <div class="menu-title">{{ t('common.language') }}</div>
      <button
        v-for="l in langItems" :key="l.code" type="button" class="menu-item"
        role="menuitemradio" :aria-checked="app.lang === l.code"
        @click="pickLang(l.code)"
      >
        <span>{{ l.label }}</span>
        <span class="menu-check">{{ app.lang === l.code ? '✓' : '' }}</span>
      </button>

      <div class="menu-sep" />
      <button type="button" class="menu-item" @click="gotoSettings">
        <span>{{ t('menu.settings') }}</span>
        <span class="menu-hint">{{ t('menu.settingsHint') }}</span>
      </button>
      <button type="button" class="menu-item" @click="toTray">
        <span>{{ t('menu.tray') }}</span>
        <span class="menu-hint">{{ t('menu.trayHint') }}</span>
      </button>

      <div class="menu-sep" />
      <button type="button" class="menu-item" @click="openHelpDialog">
        <span>{{ t('help.open') }}</span>
        <span class="menu-hint">{{ t('menu.helpHint') }}</span>
      </button>

      <p v-if="error" class="notice notice--err">{{ error }}</p>
    </div>

    <HelpDialog v-model:open="helpOpen" />
  </div>
</template>
