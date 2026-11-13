// 主题：亮暗 + 五档中性色。选中／激活态一律用中性色（亮色淡灰、暗色略亮的灰），
// 语义色（ok / warn / danger）写在 CSS 里，不随主题色变化。
import { $, $$, store, load } from './ui/dom.js';

const ACCENTS = [
  { n: '石墨', v: '#5F6670', vd: '#C2C8D0', s: '#ECEDF0', d: 'rgba(255,255,255,.09)' },
  { n: '暖灰', v: '#6F655C', vd: '#D6C7B7', s: '#F0EDE9', d: 'rgba(214,199,183,.13)' },
  { n: '松绿', v: '#4F6B62', vd: '#8FBCAE', s: '#E8EFEC', d: 'rgba(143,188,174,.13)' },
  { n: '赭石', v: '#8A5A3B', vd: '#D1A183', s: '#F3EBE4', d: 'rgba(209,161,131,.13)' },
  { n: '靛灰', v: '#55607A', vd: '#A7B2C9', s: '#EAECF2', d: 'rgba(167,178,201,.13)' }
];

const root = document.documentElement;
let accentIndex = 0;
let mode = 'light';

function paintTheme() {
  const a = ACCENTS[accentIndex];
  root.style.setProperty('--accent', mode === 'dark' ? a.vd : a.v);
  root.style.setProperty('--accent-soft', mode === 'dark' ? a.d : a.s);
  root.style.setProperty('--accent-ink', mode === 'dark' ? '#14171C' : '#FFFFFF');
  root.setAttribute('data-mode', mode);
  $$('[data-accent]').forEach((b, i) => b.setAttribute('aria-checked', String(i === accentIndex)));
  $$('[data-mode-check]').forEach((b) => {
    const on = b.getAttribute('data-mode-check') === mode;
    b.textContent = on ? '✓' : '';
    if (b.parentElement) b.parentElement.setAttribute('aria-checked', String(on));
  });
}

export function setAccent(i) { accentIndex = i; store('fh-accent', String(i)); paintTheme(); }
export function setMode(m) { mode = m; store('fh-mode', m); paintTheme(); }

export function initTheme() {
  const a = load('fh-accent');
  const m = load('fh-mode');
  if (a !== null && Number(a) >= 0 && Number(a) < ACCENTS.length) accentIndex = Number(a);
  if (m === 'light' || m === 'dark') mode = m;
  else if (window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches) mode = 'dark';
  paintTheme();
  $$('[data-accent]').forEach((b, i) => b.addEventListener('click', () => setAccent(i)));
}
