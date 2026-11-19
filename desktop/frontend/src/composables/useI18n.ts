import { useI18n as useVueI18n } from 'vue-i18n';
import { i18n, currentLang, switchLang, LANGS } from '../i18n';

/** 组件内用 const { t } = useI18n()；切语言是响应式的，无需手动重绘。 */
export function useI18n() {
  // te 用来判断当前语言是否存在某个 key，供后端下发的展示文案做兜底。
  const { t, te } = useVueI18n();
  return { t, te, lang: currentLang, switchLang, LANGS };
}

/** 非组件上下文（utils、store）里取文案。 */
export function translate(key: string, values?: Record<string, unknown>): string {
  const fn = i18n.global.t as (k: string, v?: Record<string, unknown>) => unknown;
  const out = values ? fn(key, values) : fn(key);
  return typeof out === 'string' ? out : String(out);
}
