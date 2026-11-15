// 统一的加载态：所有可能有延迟的点击都走 withLoading。
//
// 设计要点：
// 1. 延迟出现（默认 140ms）——快操作不闪加载图标，界面不会「一惊一乍」；
//    只有真的慢下来才显示 spinner，避免「点一下就抖一下」的僵硬感。
// 2. 按钮级：给触发按钮换成 spinner + 禁用防重复点击，结束自动还原
//    （包括文案、图标、disabled 状态），不依赖调用方记得清理。
// 3. 全局进度条：慢操作时顶部一条细进度线，让用户知道「程序在动」。
// 4. 最短展示时间：spinner 一旦出现至少留 320ms，否则会一闪而过更难看。
// 5. 所有状态都用 CSS 过渡，不生硬地瞬间切换。
import { t } from '../i18n.js';

const SHOW_DELAY = 140;   // 超过这个时长才显示加载态
const MIN_VISIBLE = 320;  // 一旦显示，至少停留这么久

let activeCount = 0;
let barEl = null;
let barTimer = null;

function progressBar() {
  if (barEl) return barEl;
  barEl = document.createElement('div');
  barEl.className = 'global-progress';
  barEl.setAttribute('aria-hidden', 'true');
  document.body.appendChild(barEl);
  return barEl;
}

function startGlobal() {
  activeCount++;
  const bar = progressBar();
  clearTimeout(barTimer);
  // 同样延迟出现：快操作不闪进度条
  barTimer = setTimeout(() => {
    if (activeCount > 0) bar.classList.add('is-active');
  }, SHOW_DELAY);
}

function stopGlobal() {
  activeCount = Math.max(0, activeCount - 1);
  if (activeCount > 0) return;
  clearTimeout(barTimer);
  const bar = progressBar();
  bar.classList.remove('is-active');
  bar.classList.add('is-done');
  setTimeout(() => { if (barEl) barEl.classList.remove('is-done'); }, 420);
}

// spinnerSVG 是按钮里替换出来的旋转图标。
function spinnerSVG() {
  return '<svg class="spinner" viewBox="0 0 16 16" aria-hidden="true"><circle cx="8" cy="8" r="6" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-dasharray="28 10"/></svg>';
}

// setButtonBusy 把按钮切到加载态，返回还原函数。
// 保存原始 HTML 与 disabled，结束时原样恢复——不破坏图标和文案。
function setButtonBusy(btn, label) {
  if (!btn) return () => {};
  const originalHTML = btn.innerHTML;
  const originalDisabled = btn.disabled;
  const isIcon = btn.classList.contains('icon-btn') || btn.classList.contains('btn-icon');
  const text = label || t('common.working');
  btn.classList.add('is-loading');
  btn.disabled = true;
  if (isIcon) {
    btn.innerHTML = spinnerSVG();
    btn.setAttribute('aria-busy', 'true');
  } else {
    btn.innerHTML = spinnerSVG() + '<span>' + escapeHTML(text) + '</span>';
    btn.setAttribute('aria-busy', 'true');
  }
  return () => {
    // 若按钮已被重渲染替换（不在文档里了），绝不能把保存的旧 HTML
    // 写回去——那会覆盖刚渲染出的新状态。站点启停就踩过：事务成功后
    // refreshDetails 重建了表格，这里的还原又把旧的按钮内容塞回去，
    // 用户得切走再切回来才看到新颜色。
    if (!btn.isConnected) return;
    btn.innerHTML = originalHTML;
    btn.disabled = originalDisabled;
    btn.classList.remove('is-loading');
    btn.removeAttribute('aria-busy');
  };
}

function escapeHTML(s) {
  return String(s ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
}

/**
 * withLoading 包装一个异步操作，给它加上加载态。
 *
 * @param {object}   opts
 * @param {Element}  opts.button  触发操作的按钮（会被禁用并显示 spinner）
 * @param {string}   opts.label   加载时按钮上的文字（默认「处理中…」）
 * @param {number}   opts.delay   显示加载态的延迟（默认 140ms）
 * @param {Function} opts.fn      要执行的异步函数
 * @returns {Promise<any>} 原样返回 fn 的结果
 *
 * 用法：
 *   await withLoading({ button: btn, label: t('...'), fn: () => Service.X() });
 * 不需要 button 时（例如页面切换触发的刷新）可以只传 fn，此时只走全局进度条。
 */
export async function withLoading(opts) {
  const { button = null, label = '', delay = SHOW_DELAY, fn } = opts || {};
  if (typeof fn !== 'function') throw new Error('withLoading requires fn');

  let restore = null;
  let shown = false;
  let shownAt = 0;
  let timer = null;

  startGlobal();
  timer = setTimeout(() => {
    shown = true;
    shownAt = Date.now();
    restore = setButtonBusy(button, label);
  }, delay);

  const finish = async () => {
    clearTimeout(timer);
    stopGlobal();
    if (shown && restore) {
      const left = MIN_VISIBLE - (Date.now() - shownAt);
      if (left > 0) await new Promise((r) => setTimeout(r, left));
      restore();
    }
  };

  try {
    return await fn();
  } finally {
    await finish();
  }
}

// showBusyImmediately 给「必然很慢」的操作（配置保存、诊断）用：
// 立刻显示加载态，不走延迟判断。
export async function withImmediateLoading(opts) {
  return withLoading({ ...opts, delay: 0 });
}
