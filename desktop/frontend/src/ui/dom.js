// 基础 DOM 工具与全局提示：所有模块只从这里拿 $ / $$ / esc / toast。
let toastTimer = null;

export const $ = (s) => document.querySelector(s);
export const $$ = (s) => Array.from(document.querySelectorAll(s));

export function bind(key, value) {
  $$('[data-bind="' + key + '"]').forEach((el) => { el.textContent = String(value); });
}

export function esc(s) {
  return String(s ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
}

export function toast(msg) {
  const el = $('#toast');
  el.textContent = msg;
  el.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => { el.hidden = true; }, 3200);
}

export function store(k, v) { try { localStorage.setItem(k, v); } catch (e) { /* 忽略 */ } }
export function load(k) { try { return localStorage.getItem(k); } catch (e) { return null; } }
