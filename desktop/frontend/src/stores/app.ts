import { defineStore } from 'pinia';
import { ref, computed } from 'vue';
import type { Lang } from '../types/i18n';
import { readStoredLang, storeLang } from '../i18n/config';
import { switchLang } from '../i18n';
import { translate } from '../composables/useI18n';

export type PaneKey =
  | 'overview' | 'matrix' | 'sites' | 'certs' | 'logs'
  | 'runtime' | 'operations' | 'versions' | 'settings';

const MODE_KEY = 'fast-https.mode';
const PANE_KEY = 'fast-https.pane';

// 可从侧栏进入的页面。settings 不在此列：它由底部账户区打开，
// 不进导航，也不该成为「上次停留的页面」——否则刷新后卡在设置页且
// 侧栏没有任何高亮项，用户以为界面坏了。
const NAV_PANES: readonly PaneKey[] = [
  'overview', 'matrix', 'sites', 'certs', 'logs', 'runtime', 'operations', 'versions',
];

function initialPane(): PaneKey {
  const saved = localStorage.getItem(PANE_KEY) as PaneKey | null;
  return saved && NAV_PANES.includes(saved) ? saved : 'overview';
}

export const useAppStore = defineStore('app', () => {
  const savedMode = localStorage.getItem(MODE_KEY);
  const mode = ref<'light' | 'dark'>(savedMode === 'dark' ? 'dark' : 'light');
  const lang = ref<Lang>(readStoredLang());
  const pane = ref<PaneKey>(initialPane());

  function applyMode(): void {
    document.documentElement.setAttribute('data-mode', mode.value);
  }
  function toggleMode(): void {
    mode.value = mode.value === 'dark' ? 'light' : 'dark';
    localStorage.setItem(MODE_KEY, mode.value);
    applyMode();
  }
  function changeLang(next: Lang): void {
    lang.value = next;
    switchLang(next);
    storeLang(next);
    applyMissingToken();
  }
  function activate(next: PaneKey): void {
    pane.value = next;
    localStorage.setItem(PANE_KEY, next);
  }

  /** 「未配置」角标走 CSS 伪元素，文案要靠 CSS 变量喂进去（原版机制）。 */
  function applyMissingToken(): void {
    document.documentElement.style.setProperty('--i18n-missing', JSON.stringify(translate('settings.formMissing')));
  }

  applyMode();
  document.documentElement.lang = lang.value === 'en-US' ? 'en' : lang.value;
  applyMissingToken();

  return {
    mode, lang, pane,
    isDark: computed(() => mode.value === 'dark'),
    toggleMode, changeLang, activate,
  };
});
