<script setup lang="ts">
/**
 * 左下角「本机管理」账户菜单：外观（三张图标卡片）、语言（下拉框）、
 * 设置入口、开机自启动、隐藏到托盘、帮助文档。
 * 语言列表从 i18n 的 LANGS 注册表生成，label 用各语言自己的写法；
 * 新增语言只要在注册表加一行，这里不用改。
 */
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { Settings2, Sun, Moon, Monitor } from '@lucide/vue';
import { useI18n } from '../../composables/useI18n';
import { useAppStore, type ThemeMode } from '../../stores/app';
import { LANGS } from '../../i18n/config';
import { hideWindow, getAutoStart, setAutoStart } from '../../services/api/app';
import { messageOf } from '../../services/api/base';
import HelpDialog from './HelpDialog.vue';
import type { Lang } from '../../types/i18n';

const { t } = useI18n();
const app = useAppStore();

const open = ref(false);
const helpOpen = ref(false);
const error = ref('');
const root = ref<HTMLElement | null>(null);
const autoStart = ref(false);
const autoStartBusy = ref(false);

/** 语言项按注册表顺序展开，label 用各语言自称。 */
const langItems = computed(() =>
  (Object.entries(LANGS) as [Lang, { label: string }][]).map(([code, l]) => ({ code, label: l.label })));

const modes = computed(() => [
  { key: 'light' as ThemeMode, label: 'menu.light', icon: Sun },
  { key: 'dark' as ThemeMode, label: 'menu.dark', icon: Moon },
  { key: 'system' as ThemeMode, label: 'menu.system', icon: Monitor },
]);

function close(): void {
  open.value = false;
}

function toggle(): void {
  open.value = !open.value;
  error.value = '';
  // 自启动以注册表为准：每次打开菜单重新读一次，避免显示过期状态
  if (open.value) void loadAutoStart();
}

async function loadAutoStart(): Promise<void> {
  try {
    autoStart.value = await getAutoStart();
  } catch {
    // 读失败不影响菜单其它项，保留上次状态
  }
}

/** 点菜单项即切换；勾选态以服务端返回的注册表结果为准，菜单保持展开。 */
async function toggleAutoStart(): Promise<void> {
  autoStartBusy.value = true;
  error.value = '';
  try {
    autoStart.value = await setAutoStart(!autoStart.value);
  } catch (e) {
    error.value = messageOf(e);
  } finally {
    autoStartBusy.value = false;
  }
}

function setMode(mode: ThemeMode): void {
  app.setMode(mode);
}

function pickLang(code: Lang): void {
  app.changeLang(code);
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
  // WebView2 偶发把点击 target 给成脱离 DOM 的节点（Vue 重渲染瞬间），
  // contains 会误判成「点在菜单外」，菜单刚点开就被关掉——表现为点不动。
  // 防御：target 为空视为菜单内，不关闭。
  const target = e.target as Node | null;
  if (!target) return;
  const el = root.value;
  if (el && !el.contains(target)) close();
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
  <div ref="root" class="account-anchor">
    <!-- 无下拉箭头：点击整行弹出菜单 -->
    <button
      type="button" class="side-foot" aria-haspopup="menu" :aria-expanded="open"
      @click="toggle"
    >
      <span class="avatar"><Settings2 :size="12" /></span>
      <span class="acct-name">{{ t('nav.account') }}</span>
    </button>

    <!-- 菜单内点击不冒泡到 document：外点关闭只处理菜单外的点击 -->
    <div v-if="open" class="account-menu" role="menu" @click.stop>
      <div class="menu-head">
        <img class="menu-logo" src="/appicon.png" alt="" />
        <div class="menu-head-text">
          <b>Fast-Https Desktop</b>
          <small>{{ t('app.console') }}</small>
        </div>
      </div>

      <!-- 语言：下拉框，一眼看到当前语言与全部选项 -->
      <div class="menu-row">
        <span class="menu-row__label">{{ t('common.language') }}</span>
        <div class="menu-select">
          <select
            :value="app.lang" aria-label="Language"
            @change="pickLang(($event.target as HTMLSelectElement).value as Lang)"
          >
            <option v-for="l in langItems" :key="l.code" :value="l.code">{{ l.label }}</option>
          </select>
        </div>
      </div>

      <div class="menu-sep" />

      <!-- 外观：三张图标卡片，当前项高亮描边 -->
      <div class="menu-row">
        <span class="menu-row__label">{{ t('menu.appearance') }}</span>
        <div class="mode-cards" role="radiogroup" aria-label="Appearance">
          <button
            v-for="m in modes" :key="m.key" type="button"
            class="mode-card" :class="{ 'mode-card--on': app.mode === m.key }"
            role="radio" :aria-checked="app.mode === m.key"
            :title="t(m.label)"
            @click="setMode(m.key)"
          >
            <component :is="m.icon" :size="15" />
          </button>
        </div>
      </div>

      <div class="menu-sep" />
      <button type="button" class="menu-item" @click="gotoSettings">
        <span>{{ t('menu.settings') }}</span>
      </button>
      <button
        type="button" class="menu-item" role="menuitemcheckbox"
        :aria-checked="autoStart" :disabled="autoStartBusy" @click="toggleAutoStart"
      >
        <span>{{ t('menu.autostart') }}</span>
        <span v-if="autoStart" class="menu-check" aria-hidden="true">✓</span>
      </button>
      <button type="button" class="menu-item" @click="toTray">
        <span>{{ t('menu.tray') }}</span>
      </button>

      <div class="menu-sep" />
      <button type="button" class="menu-item" @click="openHelpDialog">
        <span>{{ t('help.open') }}</span>
      </button>

      <p v-if="error" class="notice notice--err">{{ error }}</p>
    </div>

    <HelpDialog v-model:open="helpOpen" />
  </div>
</template>
