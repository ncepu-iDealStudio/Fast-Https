// 界面文案的多语言支持。
//
// 新增一种语言只要两步，业务代码不用动：
//   1. 在 src/locales/ 下复制 zh-CN.js 改名为目标语言（如 ja.js），逐条翻译；
//   2. 在下面的 LANGS 注册表里加一行 { ja: { label: '日本語', dict: ja } }。
// 语言偏好存在 localStorage，切换后整窗重载（最稳，不漏任何动态渲染）。
//
// 文案查找顺序：当前语言 → zh-CN（兜底）→ key 原文。
// 长文档（帮助）不放在字典里，见 src/help-content.js。
import { zhCN } from './locales/zh-CN.js';
import { en } from './locales/en.js';

export const LANGS = {
  'zh-CN': { label: '简体中文', dict: zhCN },
  'en': { label: 'English', dict: en },
};

const STORAGE_KEY = 'fast-https.lang';

export function getLang() {
  const saved = localStorage.getItem(STORAGE_KEY);
  return LANGS[saved] ? saved : 'zh-CN';
}

export function setLang(code) {
  if (!LANGS[code]) return;
  localStorage.setItem(STORAGE_KEY, code);
  window.location.reload();
}

// t 取一条文案；params 支持 {name} 这样的占位替换。
export function t(key, params) {
  const lang = getLang();
  const dict = LANGS[lang]?.dict || {};
  let s = dict[key] ?? zhCN[key] ?? key;
  if (params) {
    for (const [k, v] of Object.entries(params)) {
      s = s.split('{' + k + '}').join(String(v));
    }
  }
  return s;
}

// applyStatic 翻译静态 HTML 文案：
//   [data-i18n]                 → textContent
//   [data-i18n-html]            → innerHTML（含少量标记的提示语）
//   [data-i18n-placeholder]     → placeholder
//   [data-i18n-title]           → title
//   [data-i18n-aria]            → aria-label
export function applyStatic(root = document) {
  root.querySelectorAll('[data-i18n]').forEach((el) => {
    el.textContent = t(el.getAttribute('data-i18n'));
  });
  root.querySelectorAll('[data-i18n-html]').forEach((el) => {
    el.innerHTML = t(el.getAttribute('data-i18n-html'));
  });
  root.querySelectorAll('[data-i18n-placeholder]').forEach((el) => {
    el.setAttribute('placeholder', t(el.getAttribute('data-i18n-placeholder')));
  });
  root.querySelectorAll('[data-i18n-title]').forEach((el) => {
    el.setAttribute('title', t(el.getAttribute('data-i18n-title')));
  });
  root.querySelectorAll('[data-i18n-aria]').forEach((el) => {
    el.setAttribute('aria-label', t(el.getAttribute('data-i18n-aria')));
  });
}

// initI18n 在入口调用：设置 <html lang> 并翻译静态文案。
export function initI18n() {
  document.documentElement.lang = getLang();
  applyStatic();
}
