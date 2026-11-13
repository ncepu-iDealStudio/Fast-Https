// i18n 自查脚本：node scripts/i18n-check.mjs（或 npm run i18n:check）。
//
// Vue 3 重构后字典在 src/i18n/locales/*.ts（TS，554 键）。检查两件事：
//   A. zh-CN.ts 与 en-US.ts 的 key 集合完全一致，且都有非空值；
//   B. src/ 的 .vue / .ts 里用到的 key（t('…') / translate('…') / 常量表）
//      两份字典都存在。
// 旧版针对 data-i18n 与裸中文的 C/D 检查随 vanilla 前端一起退役（在
// src-legacy/ 里仍可查），Vue 版靠 vue-i18n 的 fallback 与 TypeScript 保证。
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

const ROOT = join(fileURLToPath(new URL('.', import.meta.url)), '..');

// TS 字典用正则抽 key（值里可能有转义单引号，故只校验 key 行 + 非空）。
function loadDict(file) {
  const body = readFileSync(join(ROOT, file), 'utf8');
  const dict = {};
  for (const m of body.matchAll(/^\s*'([^']+)':\s*(.*)$/gm)) {
    dict[m[1]] = m[2];
  }
  return dict;
}

const zh = loadDict('src/i18n/locales/zh-CN.ts');
const en = loadDict('src/i18n/locales/en-US.ts');
const problems = [];

// ---- A. 两份字典 key 一一对应 ----
const zhKeys = Object.keys(zh);
const enSet = new Set(Object.keys(en));
const zhSet = new Set(zhKeys);
for (const k of zhKeys) {
  if (!enSet.has(k)) problems.push(`[A] en-US.ts 缺 key：${k}`);
  if (!String(zh[k]).trim()) problems.push(`[A] zh-CN.ts 值为空：${k}`);
}
for (const k of enSet) {
  if (!zhSet.has(k)) problems.push(`[A] zh-CN.ts 缺 key：${k}`);
  else if (!String(en[k]).trim()) problems.push(`[A] en-US.ts 值为空：${k}`);
}

// ---- B. 代码引用的 key 必须存在 ----
function walk(dir, out = []) {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name);
    if (statSync(p).isDirectory()) walk(p, out);
    else if (/\.(vue|ts)$/.test(name)) out.push(p);
  }
  return out;
}

const srcDir = join(ROOT, 'src');
const referenced = new Set();
for (const file of walk(srcDir)) {
  const body = readFileSync(file, 'utf8');
  // t('key') / t("key") / translate('key') / label: 'nav.xxx'
  for (const m of body.matchAll(/(?:\bt|translate)\(\s*['"]([a-zA-Z][\w.]+)['"]/g)) {
    referenced.add(m[1]);
  }
  // 常量表里的 key 字面量：必须带点号分组（pane 名 / 类型声明不算）。
  // 覆盖两种写法：label: 'nav.overview' 与 overview: 'nav.overview'。
  for (const m of body.matchAll(/\b(?:label|[a-z][A-Za-z]*):\s*['"]([a-z][\w]*\.[\w.]+)['"]/g)) {
    referenced.add(m[1]);
  }
}
for (const k of referenced) {
  if (!zhSet.has(k)) problems.push(`[B] 代码引用了字典里没有的 key：${k}`);
}

if (problems.length) {
  for (const p of problems) console.error(p);
  console.error(`i18n 自查失败：${problems.length} 个问题`);
  process.exit(1);
}
console.log(`i18n 自查通过：两份字典各 ${zhKeys.length} 键，代码引用 ${referenced.size} 键，全部覆盖。`);
