// i18n 自查脚本：构建前跑一遍，node scripts/i18n-check.mjs（或 npm run i18n:check）。
//
// 检查四件事：
//   A. zh-CN.js 与 en.js 的 key 集合完全一致，且都有非空值；
//   B. 代码里用到的 key（t('…') / data-i18n* / 字典式常量表）两份字典都存在；
//   C. index.html 里没有「裸中文」：带中文的行必须挂 data-i18n 系列属性
//      （初始中文只是回退文本，启动时 applyStatic 会整窗替换）；
//   D. src/ 的 JS 里没有漏翻的中文提示（渲染函数里的文案都包进 t() 或
//      data-i18n 模板）。例外：locales/（字典本身）、help-content.js 与
//      dialogs/help.js（帮助长文自带双语）、theme.js 的 ACCENTS.n（UI 不显示）。
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join, relative } from 'node:path';
import { fileURLToPath } from 'node:url';
import { zhCN } from '../src/locales/zh-CN.js';
import { en } from '../src/locales/en.js';

const ROOT = join(fileURLToPath(new URL('.', import.meta.url)), '..');
const CJK = /[\u4e00-\u9fff]/;
// "像 key 的字符串"：小写开头、点号分组（用于捕获常量表里的 key）
const KEY_LIKE = /^[a-z][a-z0-9]*(?:\.[a-zA-Z0-9_]+)+$/;
const problems = [];
const notes = [];

// ---- A. 两份字典 key 一一对应 ----
const zhKeys = Object.keys(zhCN);
const enKeys = new Set(Object.keys(en));
const zhSet = new Set(zhKeys);
for (const k of zhKeys) {
  if (!enKeys.has(k)) problems.push(`[A] en.js 缺 key：${k}`);
  if (!String(zhCN[k]).trim()) problems.push(`[A] zh-CN.js 值为空：${k}`);
}
for (const k of enKeys) {
  if (!zhSet.has(k)) problems.push(`[A] zh-CN.js 缺 key：${k}`);
  else if (!String(en[k]).trim()) problems.push(`[A] en.js 值为空：${k}`);
}

// ---- 收集源码 ----
function walk(dir, out = []) {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name);
    if (statSync(p).isDirectory()) walk(p, out);
    else out.push(p);
  }
  return out;
}
const srcFiles = walk(join(ROOT, 'src')).filter((p) => p.endsWith('.js'));
const html = readFileSync(join(ROOT, 'index.html'), 'utf8');

const EXEMPT_JS = new Set([
  'src/locales/zh-CN.js', 'src/locales/en.js',   // 字典本身
  'src/i18n.js',                                 // LANGS 的 label 按约定用各语言自己的写法
  'src/help-content.js', 'src/dialogs/help.js',  // 帮助长文自带双语
]);
// 常量表里的 key 候选必须以已知分组开头（nav. / status. / dialog. …），
// 避免把 blog.example 这类示例值误判成 key
const GROUPS = new Set(zhKeys.map((k) => k.split('.')[0]));

// 去掉 JS 注释（对本仓库的源码足够；字面量里没有 // 或 /*）
function stripComments(code) {
  return code.replace(/\/\*[\s\S]*?\*\//g, (m) => m.replace(/[^\n]/g, ' '))
    .replace(/(^|[^:])\/\/[^\n]*/g, (m, p1) => p1 + ' '.repeat(m.length - p1.length));
}

// ---- B. 用到的 key 都在字典里 ----
const used = new Set();
const addKey = (k, where) => {
  used.add(k);
  if (!zhSet.has(k)) problems.push(`[B] 字典缺 key：${k}（${where}）`);
  else if (!enKeys.has(k)) problems.push(`[B] en.js 缺 key：${k}（${where}）`);
};
for (const f of srcFiles) {
  const rel = relative(ROOT, f).replace(/\\/g, '/');
  // 字典文件是「定义」不是「引用」，不计入 used（否则未使用的 key 会被自己命中）
  if (rel.startsWith('src/locales/')) continue;
  const code = stripComments(readFileSync(f, 'utf8'));
  for (const m of code.matchAll(/\bt\(\s*'([^']+)'/g)) addKey(m[1], rel);
  for (const m of code.matchAll(/\bt\(\s*"([^"]+)"/g)) addKey(m[1], rel);
  for (const m of code.matchAll(/'([a-z][a-z0-9]*(?:\.[a-zA-Z0-9_]+)+)'/g)) {
    if (KEY_LIKE.test(m[1]) && GROUPS.has(m[1].split('.')[0])) addKey(m[1], rel + '（常量表）');
  }
}
for (const m of html.matchAll(/data-i18n(?:-html|-placeholder|-title|-aria)?="([^"]+)"/g)) {
  addKey(m[1], 'index.html');
}
for (const f of srcFiles) {
  const rel = relative(ROOT, f).replace(/\\/g, '/');
  const code = readFileSync(f, 'utf8');
  for (const m of code.matchAll(/data-i18n(?:-html|-placeholder|-title|-aria)?="([^"]+)"/g)) {
    addKey(m[1], rel + '（模板）');
  }
}

// ---- C. index.html 裸中文检查（带中文的行必须挂 data-i18n） ----
html.split('\n').forEach((line, i) => {
  const noComment = line.replace(/<!--.*?-->/g, '');
  if (CJK.test(noComment) && !/data-i18n/.test(noComment)) {
    problems.push(`[C] index.html:${i + 1} 有未挂 data-i18n 的中文：${line.trim().slice(0, 60)}…`);
  }
});

// ---- D. JS 里的中文提示检查 ----
for (const f of srcFiles) {
  const rel = relative(ROOT, f).replace(/\\/g, '/');
  if (EXEMPT_JS.has(rel)) continue;
  const code = stripComments(readFileSync(f, 'utf8'));
  code.split('\n').forEach((line, i) => {
    if (!CJK.test(line)) return;
    // 模板里挂在 data-i18n* 上的回退文本可以是中文
    if (/data-i18n/.test(line)) return;
    // theme.js 的 ACCENTS.n 是 UI 不显示的内部名字，界面文案走 menu.accent* 字典
    if (rel === 'src/theme.js' && /\bn: '/.test(line)) return;
    problems.push(`[D] ${rel}:${i + 1} 有未走 t() 的中文：${line.trim().slice(0, 60)}…`);
  });
}

// ---- 未使用的 key（提示，不算失败） ----
const unused = zhKeys.filter((k) => !used.has(k));
if (unused.length) notes.push(`未被代码引用的 key（通用词汇，留作备用）：${unused.join(', ')}`);

// ---- E. 语言切换机制冒烟：setLang('en') 原地切换后 t() 全量返回英文 ----
{
  const storeMap = new Map();
  globalThis.localStorage = {
    getItem: (k) => (storeMap.has(k) ? storeMap.get(k) : null),
    setItem: (k, v) => storeMap.set(k, String(v)),
    removeItem: (k) => storeMap.delete(k),
  };
  // setLang 是原地切换：更新 <html lang>、重刷静态文案、通知重绘回调
  globalThis.document = {
    documentElement: { lang: '' },
    querySelectorAll: () => [],
  };
  const i18n = await import('../src/i18n.js');
  let changed = 0;
  i18n.onLangChange(() => { changed++; });
  i18n.setLang('en');
  if (changed !== 1) problems.push('[E] setLang 没有触发 onLangChange 回调（原地重渲染依赖它）');
  if (globalThis.document.documentElement.lang !== 'en') problems.push('[E] setLang 后 <html lang> 未更新为 en');
  if (i18n.getLang() !== 'en') problems.push('[E] setLang 后 getLang() 不是 en');
  if (i18n.t('overview.start') !== en['overview.start']) problems.push('[E] en 下 t(overview.start) 未返回英文');
  // 全量 key 覆盖率：en 下不允许任何 key 回落到中文基准
  const fellBack = zhKeys.filter((k) => i18n.t(k) === zhCN[k] && en[k] !== zhCN[k]);
  if (fellBack.length) problems.push('[E] en 下仍回落中文的 key：' + fellBack.join(', '));
  i18n.setLang('zh-CN');
  if (changed !== 2) problems.push('[E] 切回 zh-CN 时 onLangChange 应再触发一次');
  if (i18n.getLang() !== 'zh-CN' || i18n.t('overview.start') !== zhCN['overview.start']) {
    problems.push('[E] 切回 zh-CN 后 t() 未返回中文');
  }
}

// ---- 报告 ----
console.log(`zh-CN keys: ${zhKeys.length} / en keys: ${enKeys.size}`);
console.log(`代码引用的 key：${used.size}`);
notes.forEach((n) => console.log('note: ' + n));
if (problems.length) {
  console.error(`\n${problems.length} 个问题：`);
  problems.forEach((p) => console.error('  ' + p));
  process.exit(1);
}
console.log('i18n 自查通过：两份字典一致，界面文案全覆盖。');
