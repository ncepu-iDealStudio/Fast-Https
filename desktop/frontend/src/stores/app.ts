import { defineStore } from 'pinia';
import { ref, computed } from 'vue';
import type { Lang } from '../types/i18n';
import { readStoredLang, storeLang } from '../i18n/config';
import { switchLang } from '../i18n';

export type PaneKey =
  | 'overview' | 'matrix' | 'sites' | 'certs' | 'logs'
  | 'runtime' | 'operations' | 'settings';

const MODE_KEY = 'fast-https.mode';
const PANE_KEY = 'fast-https.pane';

export const useAppStore = defineStore('app', () => {
  const savedMode = localStorage.getItem(MODE_KEY);
  const mode = ref<'light' | 'dark'>(savedMode === 'dark' ? 'dark' : 'light');
  const lang = ref<Lang>(readStoredLang());
  const pane = ref<PaneKey>((localStorage.getItem(PANE_KEY) as PaneKey | null) ?? 'overview');

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
  }
  function activate(next: PaneKey): void {
    pane.value = next;
    localStorage.setItem(PANE_KEY, next);
  }

  applyMode();
  document.documentElement.lang = lang.value === 'en-US' ? 'en' : lang.value;

  return {
    mode, lang, pane,
    isDark: computed(() => mode.value === 'dark'),
    toggleMode, changeLang, activate,
  };
});
