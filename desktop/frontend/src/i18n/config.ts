import type { Lang } from '../types/i18n';

/** 支持的语言。label 用于语言切换菜单的展示。 */
export const LANGS: Record<Lang, { label: string }> = {
  'zh-CN': { label: '简体中文' },
  'en-US': { label: 'English' },
};

export const DEFAULT_LANG: Lang = 'zh-CN';
export const FALLBACK_LANG: Lang = 'zh-CN';

/** 旧版前端用 'en' 作为 key；迁移时把历史值映射到新的 en-US。 */
const STORAGE_KEY = 'fast-https.lang';

export function readStoredLang(): Lang {
  const saved = localStorage.getItem(STORAGE_KEY);
  if (saved === 'zh-CN' || saved === 'en-US') return saved;
  if (saved === 'en') return 'en-US';
  return DEFAULT_LANG;
}

export function storeLang(lang: Lang): void {
  localStorage.setItem(STORAGE_KEY, lang);
  document.documentElement.lang = lang === 'en-US' ? 'en' : lang;
}
