import { defineStore } from 'pinia';
import { ref, computed, onUnmounted } from 'vue';
import type { Lang } from '../types/i18n';
import { readStoredLang, storeLang } from '../i18n/config';
import { switchLang } from '../i18n';
import { translate } from '../composables/useI18n';

export type PaneKey =
  | 'monitor' | 'sites' | 'certs' | 'logs'
  | 'operations' | 'versions' | 'settings';

/** 澶栬涓夋€侊細浜?/ 鏆?/ 璺熼殢绯荤粺銆傝窡闅忔ā寮忎笅 data-mode 闅忕郴缁熷疄鏃跺垏鎹€?*/
export type ThemeMode = 'light' | 'dark' | 'system';

const MODE_KEY = 'fast-https.mode';
const PANE_KEY = 'fast-https.pane';
const RAIL_KEY = 'fast-https.railCollapsed';

// 鍙粠渚ф爮杩涘叆鐨勯〉闈€俿ettings 涓嶅湪姝ゅ垪锛氬畠鐢卞簳閮ㄨ处鎴峰尯鎵撳紑锛?
// 涓嶈繘瀵艰埅锛屼篃涓嶈鎴愪负銆屼笂娆″仠鐣欑殑椤甸潰銆嶁€斺€斿惁鍒欏埛鏂板悗鍗″湪璁剧疆椤典笖
// 渚ф爮娌℃湁浠讳綍楂樹寒椤癸紝鐢ㄦ埛浠ヤ负鐣岄潰鍧忎簡銆?
const NAV_PANES: readonly PaneKey[] = [
  'monitor', 'sites', 'certs', 'logs', 'operations', 'versions',
];

function initialPane(): PaneKey {
  const saved = localStorage.getItem(PANE_KEY) as PaneKey | null;
  return saved && NAV_PANES.includes(saved) ? saved : 'monitor';
}

function initialMode(): ThemeMode {
  const saved = localStorage.getItem(MODE_KEY) as ThemeMode | null;
  return saved === 'dark' || saved === 'system' ? saved : 'light';
}

export const useAppStore = defineStore('app', () => {
  const mode = ref<ThemeMode>(initialMode());
  const lang = ref<Lang>(readStoredLang());
  const pane = ref<PaneKey>(initialPane());
  /** 渚ф爮鎶樺彔锛氭姌鍙犲悗鍙暀鍥炬爣銆傝蹇嗗埌涓嬫鍚姩銆?*/
  // 首次启动（从未保存过）默认折叠：小屏幕上一进来就腾出内容宽度。
  // 用户手动展开后会写入 '0'，之后保持他的选择。
  const railCollapsed = ref(
    localStorage.getItem(RAIL_KEY) === null || localStorage.getItem(RAIL_KEY) === '1',
  );

  function toggleRail(): void {
    railCollapsed.value = !railCollapsed.value;
    localStorage.setItem(RAIL_KEY, railCollapsed.value ? '1' : '0');
  }

  /** 绯荤粺褰撳墠鐨勪寒鏆楋紙璺熼殢绯荤粺妯″紡涓嬬敤瀹冿級銆?*/
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

  /** data-mode 鏄?CSS 鐨勫敮涓€寮€鍏筹紱system 鎶樼畻鎴愬疄闄呯殑浜垨鏆椼€?*/
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

  /** 銆屾湭閰嶇疆銆嶈鏍囪蛋 CSS 浼厓绱狅紝鏂囨瑕侀潬 CSS 鍙橀噺鍠傝繘鍘伙紙鍘熺増鏈哄埗锛夈€?*/
  function applyMissingToken(): void {
    document.documentElement.style.setProperty('--i18n-missing', JSON.stringify(translate('settings.formMissing')));
  }

  applyMode();
  document.documentElement.lang = lang.value === 'en-US' ? 'en' : lang.value;
  applyMissingToken();

  return {
    mode, lang, pane, railCollapsed,
    isDark: computed(() =>
      mode.value === 'system' ? systemDark.value : mode.value === 'dark',
    ),
    setMode, changeLang, activate, toggleRail,
  };
});
