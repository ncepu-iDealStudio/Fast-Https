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
const stalecache = process.argv.includes('stalecache');
const query = stopped ? '?stopped=1' : (stalecache ? '?stalecache=1' : '');

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

  // 加载态：慢操作时按钮要出现 spinner 并禁用，结束后必须还原。
  // 用 page.evaluate 直接调 withLoading 模块，配合一个 300ms 的慢 fn，
  // 观察按钮在延迟窗口后的状态与结束后的还原。
  {
    const result = await page.evaluate(async () => {
      const mod = await import('/src/ui/loading.js');
      const btn = document.createElement('button');
      btn.className = 'btn';
      btn.textContent = '启动服务';
      document.body.appendChild(btn);
      const original = btn.textContent;
      let during = null;
      const p = mod.withLoading({
        button: btn,
        label: '处理中…',
        fn: () => new Promise((r) => setTimeout(r, 300))
      });
      // 等超过 SHOW_DELAY(140ms) 但还没结束
      await new Promise((r) => setTimeout(r, 190));
      during = {
        loading: btn.classList.contains('is-loading'),
        disabled: btn.disabled,
        hasSpinner: !!btn.querySelector('.spinner'),
        bar: document.querySelector('.global-progress')?.classList.contains('is-active') || false
      };
      await p;
      const after = {
        loading: btn.classList.contains('is-loading'),
        disabled: btn.disabled,
        text: btn.textContent,
        restored: btn.textContent === original
      };
      btn.remove();
      return { during, after };
    });
    check('慢操作显示加载态',
      result.during.loading && result.during.disabled && result.during.hasSpinner && result.during.bar,
      JSON.stringify(result.during));
    check('加载结束后按钮还原',
      !result.after.loading && !result.after.disabled && result.after.restored,
      JSON.stringify(result.after));

    // 快操作（< 140ms）不该出现加载态——否则每点一下都抖一下，反而更僵
    const fast = await page.evaluate(async () => {
      const mod = await import('/src/ui/loading.js');
      const btn = document.createElement('button');
      btn.className = 'btn';
      btn.textContent = '快操作';
      document.body.appendChild(btn);
      let seenLoading = false;
      const p = mod.withLoading({
        button: btn,
        fn: () => new Promise((r) => setTimeout(r, 30))
      });
      // 在 fn 执行期间轮询，看是否出现过 is-loading
      const poll = setInterval(() => { if (btn.classList.contains('is-loading')) seenLoading = true; }, 10);
      await p;
      clearInterval(poll);
      btn.remove();
      return seenLoading;
    });
    check('快操作不闪加载态', fast === false, `seenLoading=${fast}`);
  }

  // 全局体检：DOM 里不允许出现重复 id。$('#x') 只拿第一个，重复 id 会让
  // 后出现的那个元素永远拿不到数据（运行快照页就踩过：旧面板与新页面同 id，
  // 新页面一直是 loading 占位）。这类 bug 界面看起来"没报错"，极难发现。
  const dupIds = await page.evaluate(() => {
    const seen = new Map();
    document.querySelectorAll('[id]').forEach((el) => {
      seen.set(el.id, (seen.get(el.id) || 0) + 1);
    });
    return [...seen.entries()].filter(([, n]) => n > 1).map(([id, n]) => id + '×' + n);
  });
  check('DOM 无重复 id', dupIds.length === 0, dupIds.join(', '));

  // 服务未启动场景：TailLog 失败不应连累站点/证书/路径（磁盘数据不依赖服务）
  if (stopped) {
    await page.click('[data-nav="sites"]');
    await sleep(500);
    const rows = await page.locator('#sitesTable .site-row').count();
    const siteLabel = await page.locator('#siteCountLabel').textContent();
    check('服务停止时站点仍刷出', rows > 0 && Number(siteLabel) > 0,
      `rows=${rows} countLabel=${siteLabel}`);
    await shot('02-stopped-sites');
  }

  // 左侧导航逐个打开
  for (const [key, label] of [['matrix', '矩阵'], ['sites', '站点'], ['certs', '证书'], ['logs', '日志']]) {
    await page.click(`[data-nav="${key}"]`);
    await sleep(500);
    await shot('02-nav-' + label);
  }

  // 站点页核心契约：一个站点文件只显示一行。
  // 一个站点可同时监听 80 与 443（listen "80, 443 ssl h2"），那是同一个
  // 配置文件、同一个启停开关。按监听拆行会让用户以为有两个站点，
  // 且点其中一个另一个也跟着变——曾经就是这么错的。
  await page.click('[data-nav="sites"]');
  await sleep(400);
  {
    const rows = await page.locator('#sitesTable .site-row').count();
    const label = Number(await page.locator('#siteCountLabel').textContent());
    check('一个站点文件一行', rows === label, `rows=${rows} countLabel=${label}`);
    // 多端口站点的监听标签都要展示出来（80 与 443 各一个 chip）
    const chips = await page.locator('#sitesTable .site-row').first().locator('.listen-chip').count();
    check('多端口站点监听完整展示', chips >= 2, `chips=${chips}`);

    // 即时反馈：点击开关后颜色必须立刻变，不切页面、不等事务。
    // mock 的 SetSiteEnabled 故意延迟 600ms，所以这里只等 80ms 就检查——
    // 若实现是「等后端回来再刷新」，此刻必然还是旧颜色。
    const firstRow = page.locator('#sitesTable .site-row').first();
    const pill = firstRow.locator('.site-state');
    const before = (await pill.getAttribute('class')) || '';
    const beforeText = (await pill.textContent()) || '';
    await firstRow.locator('.site-toggle').click();
    await sleep(80); // 远小于 mock 的 600ms 事务延迟
    const during = (await page.locator('#sitesTable .site-row').first().locator('.site-state').getAttribute('class')) || '';
    const duringText = (await page.locator('#sitesTable .site-row').first().locator('.site-state').textContent()) || '';
    check('点击后状态立即变化', during !== before && duringText !== beforeText,
      `before="${before}" during="${during}"`);
    // 再等事务完成 + 强制刷新，颜色应保持（不能被 loading 的旧 HTML 还原覆盖）
    await sleep(900);
    const after = (await page.locator('#sitesTable .site-row').first().locator('.site-state').getAttribute('class')) || '';
    check('事务完成后状态不被还原覆盖', after === during,
      `during="${during}" after="${after}"`);
    // 还原现场：再点回去，避免影响后续断言
    await page.locator('#sitesTable .site-row').first().locator('.site-toggle').click();
    await sleep(900);
  }

  // 新增页面：运行快照、操作记录、日志实时、ACME 表单
  await page.click('[data-nav="runtime"]');
  // 等卡片真正渲染（loading 占位消失），否则截图只有"正在读取…"
  await page.waitForSelector('#rtPageBody .rt-card, #rtPageBody .runtime-empty', { timeout: 5000 }).catch(() => {});
  check('运行快照页渲染', await page.locator('#rtPageBody .rt-card').count() > 0,
    'cards=' + await page.locator('#rtPageBody .rt-card').count());
  await shot('07-runtime');
  await page.click('[data-nav="operations"]');
  await page.waitForSelector('#opsList .ops-row, #opsList .runtime-empty', { timeout: 5000 }).catch(() => {});
  check('操作记录页渲染', await page.locator('#opsList .ops-row').count() > 0);
  await shot('08-operations');
  await page.click('[data-nav="logs"]');
  check('日志实时开关存在', await page.locator('#logLive').count() > 0);
  check('日志高级筛选默认收起', await page.locator('#logAdvanced').isHidden());
  await page.click('#logMoreFilters');
  check('日志高级筛选可展开', await page.locator('#logAdvanced').isVisible());
  await page.click('#logMoreFilters');
  await page.click('[data-nav="certs"]');
  await sleep(300);
  // ACME 属于主配置的 acme 段，只在「设置」页的表单里改；
  // 证书页不该再放一份表单（两处都能改会让人不知道以哪份为准）。
  check('证书页不再有 ACME 表单', await page.locator('#acmeForm').count() === 0);
  await shot('09-certs-no-acme');

  // 站点页：添加站点弹窗 + 协议/端口联动
  await page.click('[data-nav="sites"]');
  await page.click('#addSiteBtn');
  await sleep(300);
  await shot('03-site-add');
  // 高级配置只用于编辑已有站点：site add 不接受这些 flag
  const addAdvancedHidden = await page.evaluate(() => {
    const box = document.getElementById('siteAdvanced');
    return !box || box.hidden;
  });
  check('新增模式不显示高级配置', addAdvancedHidden);
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
  // 高级配置：编辑模式显示且默认折叠；auth 未启用时三个输入框禁用
  const adv = await page.evaluate(() => {
    const box = document.getElementById('siteAdvanced');
    return {
      present: !!box,
      hidden: box ? box.hidden : true,
      open: box ? box.open : false,
      authUserDisabled: document.getElementById('siteAuthUser').disabled
    };
  });
  check('高级配置在编辑模式可见且默认折叠', adv.present && !adv.hidden && !adv.open);
  check('认证未启用时用户名禁用', adv.authUserDisabled);
  await page.evaluate(() => { document.getElementById('siteAdvanced').open = true; });
  await sleep(200);
  await shot('05b-site-advanced');
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
  // 设置页表单要按字段语义给控件，而不是一律文本框：
  // ACME 分组在设置页（证书页不再有）、路径字段带「浏览…」按钮、
  // include 目录是路径列表（可增删）。
  {
    const acmeGroup = await page.locator('.config-group-title', { hasText: 'ACME' }).count();
    const browseBtns = await page.locator('#configForm .path-browse').count();
    const pathList = await page.locator('#configForm .path-list').count();
    const selects = await page.locator('#configForm select').count();
    check('设置页含 ACME 分组', acmeGroup > 0, `groups=${acmeGroup}`);
    check('路径字段带浏览按钮', browseBtns > 0, `browse=${browseBtns}`);
    check('include 是路径列表', pathList > 0, `pathList=${pathList}`);
    check('枚举字段用下拉', selects > 0, `selects=${selects}`);
  }
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
