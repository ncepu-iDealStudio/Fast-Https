import { defineStore } from 'pinia';
import { ref, computed, onUnmounted } from 'vue';
import type { Lang } from '../types/i18n';
import { readStoredLang, storeLang } from '../i18n/config';
import { switchLang } from '../i18n';
import { translate } from '../composables/useI18n';

export type PaneKey =
  | 'overview' | 'matrix' | 'sites' | 'certs' | 'logs'
  | 'runtime' | 'operations' | 'versions' | 'settings';

/** 外观三态：亮 / 暗 / 跟随系统。跟随模式下 data-mode 随系统实时切换。 */
export type ThemeMode = 'light' | 'dark' | 'system';

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

function initialMode(): ThemeMode {
  const saved = localStorage.getItem(MODE_KEY) as ThemeMode | null;
  return saved === 'dark' || saved === 'system' ? saved : 'light';
}

export const useAppStore = defineStore('app', () => {
  const mode = ref<ThemeMode>(initialMode());
  const lang = ref<Lang>(readStoredLang());
  const pane = ref<PaneKey>(initialPane());

  /** 系统当前的亮暗（跟随系统模式下用它）。 */
  const systemDark = ref(
    typeof window.matchMedia === 'function'
      ? window.matchMedia('(prefers-color-scheme: dark)').matches
      : false,
  );
  const mql: MediaQueryList | null =
    typeof window.matchMedia === 'function'
      ? window.matchMedia('(prefers-color-scheme: dark)')
      : null;
  function onSystemChange(e: MediaQueryListEvent): void {
    systemDark.value = e.matches;
    if (mode.value === 'system') applyMode();
  }
  mql?.addEventListener('change', onSystemChange);
  onUnmounted(() => mql?.removeEventListener('change', onSystemChange));

  /** data-mode 是 CSS 的唯一开关；system 折算成实际的亮或暗。 */
  function applyMode(): void {
    const effective = mode.value === 'system' ? (systemDark.value ? 'dark' : 'light') : mode.value;
    document.documentElement.setAttribute('data-mode', effective);
  }
  function setMode(next: ThemeMode): void {
    mode.value = next;
    localStorage.setItem(MODE_KEY, next);
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
    isDark: computed(() =>
      mode.value === 'system' ? systemDark.value : mode.value === 'dark',
    ),
    setMode, changeLang, activate,
  };
});
