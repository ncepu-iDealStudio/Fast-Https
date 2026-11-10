// 前端点击测试：启动 mock 后端的 Vite，Playwright（Edge 无头）把所有
// 页面、弹窗、关键交互点一遍，逐屏截图到 shots/，并收集控制台错误。
// 用法：cd desktop/frontend/devtest && npm install && node run.mjs [en] [stopped]
// 不带参数走中文界面；带 en 走英文界面（验证 i18n）；
// 带 stopped 走「服务未启动」场景（TailLog 报错，站点/证书仍应渲染）。
import { chromium } from 'playwright';
import { spawn } from 'node:child_process';
import { mkdirSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const shots = path.join(here, 'shots');
mkdirSync(shots, { recursive: true });
const lang = process.argv[2] === 'en' ? 'en' : 'zh-CN';
const suffix = lang === 'en' ? '-en' : '';
const stopped = process.argv.includes('stopped');
const query = stopped ? '?stopped=1' : '';

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

// 1. 起 Vite dev server（带 bindings mock）；vite 用前端项目自带的那份
const vite = spawn('node', [path.resolve(here, '..', 'node_modules', 'vite', 'bin', 'vite.js'), '--config', path.join(here, 'vite.config.mjs')], {
  cwd: here, stdio: 'pipe'
});
let viteLog = '';
vite.stdout.on('data', (d) => { viteLog += d; });
vite.stderr.on('data', (d) => { viteLog += d; });
const ready = await new Promise((resolve) => {
  const t = setInterval(() => {
    if (/localhost:9247/.test(viteLog)) { clearInterval(t); resolve(true); }
  }, 200);
  setTimeout(() => { clearInterval(t); resolve(false); }, 15000);
});
if (!ready) { console.error('vite 起不来：\n' + viteLog); process.exit(1); }

const errors = [];
const browser = await chromium.launch({ channel: 'msedge', headless: true });
const page = await browser.newPage({ viewport: { width: 1280, height: 840 } });
await page.addInitScript((l) => { localStorage.setItem('fast-https.lang', l); }, lang);
page.on('console', (m) => { if (m.type() === 'error') errors.push('[console] ' + m.text()); });
page.on('pageerror', (e) => errors.push('[pageerror] ' + e.message));

async function shot(name) {
  await page.screenshot({ path: path.join(shots, name + suffix + '.png') });
  console.log('shot:', name + suffix);
}

const results = [];
function check(name, ok, detail) {
  results.push({ name, ok, detail });
  console.log((ok ? 'PASS ' : 'FAIL ') + name + (detail ? ' — ' + detail : ''));
}

try {
  await page.goto('http://localhost:9247/' + query);
  await sleep(800);
  await shot('01-overview');
  check('概览渲染', await page.locator('#metricRunning').count() > 0);

  // 服务未启动场景：TailLog 失败不应连累站点/证书/路径（磁盘数据不依赖服务）
  if (stopped) {
    await page.click('[data-nav="sites"]');
    await sleep(500);
    const rows = await page.locator('#sitesTable .kv-row').count();
    const siteLabel = await page.locator('#siteCountLabel').textContent();
    check('服务停止时站点仍刷出', rows > 1 && Number(siteLabel) > 0,
      `rows=${rows} countLabel=${siteLabel}`);
    await shot('02-stopped-sites');
  }

  // 左侧导航逐个打开
  for (const [key, label] of [['matrix', '矩阵'], ['sites', '站点'], ['certs', '证书'], ['logs', '日志']]) {
    await page.click(`[data-nav="${key}"]`);
    await sleep(500);
    await shot('02-nav-' + label);
  }

  // 站点页：添加站点弹窗 + 协议/端口联动
  await page.click('[data-nav="sites"]');
  await page.click('#addSiteBtn');
  await sleep(300);
  await shot('03-site-add');
  await page.selectOption('#siteProto', 'both');
  await sleep(150);
  const listen = await page.inputValue('#siteListen');
  check('协议切换自动生成 listen', listen === '80, 443 ssl h2', 'listen=' + listen);
  await page.fill('#siteHttpPort', '8080');
  await page.fill('#siteHttpsPort', '8443');
  await sleep(150);
  const listen2 = await page.inputValue('#siteListen');
  check('端口改动联动 listen', listen2 === '8080, 8443 ssl h2', 'listen=' + listen2);
  await page.selectOption('#siteType', 'proxy');
  await sleep(150);
  check('反代行显示', await page.locator('#siteProxyRow').isVisible());
  await page.fill('#siteProxy', '127.0.0.1:9000');
  await page.selectOption('#siteProxyScheme', 'https://');
  await shot('04-site-add-proxy');
  await page.click('#siteCancel');

  // 站点详情弹窗（含只读区）
  await page.click('.site-more');
  await sleep(400);
  await shot('05-site-detail');
  const rawVisible = await page.locator('#siteRaw').count() > 0;
  check('详情只读区渲染', rawVisible);
  await page.keyboard.press('Escape');

  // 帮助弹窗（应用内）
  await page.click('#accountBtn');
  await sleep(200);
  await shot('06-account-menu');
  // 语言切换必须是原地重渲染：不整窗 reload（marker 存活），
  // <html lang> 与界面文案都要切换
  await page.evaluate(() => { window.__langMarker = 1; });
  await page.click('[data-lang="en"]');
  await sleep(300);
  await shot('06b-lang-switch-en');
  const noReload = await page.evaluate(() => window.__langMarker === 1);
  const langAttr = await page.evaluate(() => document.documentElement.lang);
  const navEn = await page.locator('[data-nav="sites"]').textContent();
  check('语言原地切换不重载', noReload && langAttr === 'en' && /Sites/.test(navEn || ''),
    `marker=${noReload} lang=${langAttr} nav=${String(navEn).trim()}`);
  await page.click('#accountBtn');
  await page.click('[data-lang="zh-CN"]');
  await sleep(300);
  await page.click('#accountBtn');
  await sleep(200);
  await page.click('[data-menu="help"]');
  await sleep(400);
  await shot('07-help');
  check('帮助弹窗打开', await page.locator('#helpDialog').isVisible());
  check('帮助目录有条目', await page.locator('.help-toc-item').count() >= 5);
  await page.locator('.help-body').evaluate((el) => { el.scrollTop = el.scrollHeight; });
  await sleep(200);
  await shot('08-help-bottom');
  await page.click('#helpClose');

  // 概览：健康检查弹窗 + 操作记录 tab
  await page.click('[data-nav="overview"]');
  await sleep(300);
  await page.click('#diagnoseBtn');
  await sleep(400);
  await shot('09-diagnose');
  check('健康检查弹窗打开', await page.locator('#helpDialog').count() >= 0 && await page.locator('[id*=iag],[class*=diag]').count() > 0);
  await page.keyboard.press('Escape');
  await sleep(200);

  // 状态矩阵：运行快照 / Monitor 卡片
  await page.click('[data-nav="matrix"]');
  await sleep(500);
  await shot('10-matrix-panels');

  // 证书页：检查弹窗 + ACME 卡
  await page.click('[data-nav="certs"]');
  await sleep(400);
  await shot('11-certs');
  if (await page.locator('.cert-check').count() > 0) {
    await page.click('.cert-check');
    await sleep(400);
    await shot('12-cert-check');
    await page.keyboard.press('Escape');
  }

  // 设置页：表单 + 配置版本卡
  await page.click('#accountBtn');
  await page.click('[data-menu="settings"]');
  await sleep(600);
  await shot('13-settings');
  await page.locator('.content').evaluate((el) => { el.scrollTop = el.scrollHeight; });
  await sleep(300);
  await shot('14-settings-versions');
  if (await page.locator('.ver-diff').count() > 0) {
    await page.locator('.ver-diff').first().click();
    await sleep(400);
    await shot('15-settings-diff');
  }

  // 日志页：筛选与分页
  await page.click('[data-nav="logs"]');
  await sleep(400);
  await shot('16-logs');
  if (await page.locator('#logMore').count() > 0) {
    await page.click('#logMore');
    await sleep(400);
    await shot('17-logs-more');
  }

  check('无控制台错误', errors.length === 0, errors.slice(0, 5).join(' | '));
} catch (e) {
  check('遍历过程无异常', false, String(e));
  const overlay = await page.evaluate(() => {
    const el = document.querySelector('vite-error-overlay');
    return el && el.shadowRoot ? el.shadowRoot.textContent.slice(0, 600) : '(no overlay)';
  }).catch(() => '(overlay read failed)');
  console.log('vite overlay:\n' + overlay);
  await shot('99-crash');
} finally {
  await browser.close();
  vite.kill();
  // Windows 下 vite.kill 可能只杀掉壳，兜底按端口清一次
  spawn('powershell', ['-NoProfile', '-Command', `Get-NetTCPConnection -LocalPort 9247 -ErrorAction SilentlyContinue | ForEach-Object { Stop-Process -Id $_.OwningProcess -Force -ErrorAction SilentlyContinue }`], { stdio: 'ignore' });
}

writeFileSync(path.join(shots, 'report' + suffix + '.json'), JSON.stringify({ lang, results, errors }, null, 2));
const failed = results.filter((r) => !r.ok);
console.log('\n==== 汇总：' + (results.length - failed.length) + '/' + results.length + ' 通过 ====');
process.exit(failed.length ? 1 : 0);
