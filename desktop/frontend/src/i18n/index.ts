import { createI18n } from 'vue-i18n';
import { zhCN } from './locales/zh-CN';
import { enUS } from './locales/en-US';
import { DEFAULT_LANG, FALLBACK_LANG, LANGS, readStoredLang, storeLang } from './config';
import type { Lang } from '../types/i18n';

export const i18n = createI18n({
  legacy: false,
  locale: readStoredLang(),
  fallbackLocale: FALLBACK_LANG,
  messages: { 'zh-CN': zhCN, 'en-US': enUS },
  missingWarn: false,
  fallbackWarn: false,
});

export function currentLang(): Lang {
  return i18n.global.locale.value as Lang;
}

export function switchLang(lang: Lang): void {
  i18n.global.locale.value = lang;
  storeLang(lang);
}

export { LANGS, DEFAULT_LANG };
