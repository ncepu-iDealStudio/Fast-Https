// 数据拉取编排：概览每 2 秒一次，明细（站点/证书/路径/矩阵日志）15 秒一次。
// 新增卡片（运行快照/Monitor/配置版本）的刷新各自管理，不进这里的轮询。
//
// 重要：每项独立取，任何一项失败都不连累其它项——site list / cert list /
// config path 是纯磁盘读取，不依赖服务进程；服务没启动时 TailLog 可能失败，
// 绝不能因为一条日志读不到就把站点、证书、路径整片留白。
//
// 关于「操作后立即刷新」：写操作（启停站点、增删改）完成后要调
// refreshDetails({ force: true })。定时器可能正好在跑，默认的防重入会让
// 手动刷新被直接丢掉——那正是「点启用/停用后界面没变」的根因。
// force 时把这次请求挂到正在跑的那次之后，保证一定重新拉一遍。
import { DesktopService, msgOf } from './api.js';
import { t } from './i18n.js';
import { $, bind } from './ui/dom.js';
import { state } from './state.js';
import { renderOverview } from './pages/overview.js';
import { renderSites, renderCerts, renderPaths, renderLogEntries } from './pages/details.js';

// one 独立执行一项拉取，失败时返回 fallback 并记日志（不抛出、不中断别的项）。
async function one(label, fn, fallback) {
  try {
    return await fn();
  } catch (e) {
    console.warn(label + ':', msgOf(e));
    return fallback;
  }
}

export async function refreshOverview() {
  if (state.overviewBusy) return;
  state.overviewBusy = true;
  try {
    renderOverview(await DesktopService.GetOverview());
  } catch (e) {
    bind('running', t('status.notConnected'));
    const alert = $('#overviewError');
    alert.hidden = false;
    alert.className = 'alert-strip';
    $('#alertTitle').textContent = t('overview.alertBackendTitle');
    $('#alertBody').textContent = t('overview.alertBackendBody', { msg: msgOf(e) });
  } finally {
    state.overviewBusy = false;
  }
}

async function pullDetails() {
  // 四项并行但互相隔离：服务没启动时 TailLog('access') 会失败，
  // 站点/证书/路径照样渲染（它们来自 site list / cert list / config path）。
  const [sites, certs, paths, entries] = await Promise.all([
    one('ListSites', () => DesktopService.ListSites(), []),
    one('ListCerts', () => DesktopService.ListCerts(), []),
    one('Paths', () => DesktopService.Paths(), null),
    one('TailLog', () => DesktopService.TailLog('access', 8), [])
  ]);
  renderSites(sites || [], certs || []);
  renderCerts(certs || []);
  if (paths) renderPaths(paths);
  // 状态矩阵固定展示 access 尾部（TailLog 现在返回 model.LogEntry[]）；
  // 日志页有自己的筛选与游标分页，不跟着这里的定时刷新重建列表。
  renderLogEntries(entries || [], '#matrixLogs');
}

// refreshDetails 拉取并重绘明细。opts.force=true 时即使定时刷新正在跑，
// 也会在它结束后再拉一遍——写操作后必须用这个，否则界面可能停在旧数据。
export async function refreshDetails(opts) {
  const force = !!(opts && opts.force);
  if (state.detailsBusy) {
    if (!force) return;
    // 挂到当前这一轮之后：等它释放，再自己跑一遍
    await new Promise((resolve) => {
      const tick = setInterval(() => {
        if (!state.detailsBusy) {
          clearInterval(tick);
          resolve();
        }
      }, 40);
    });
  }
  state.detailsBusy = true;
  try {
    await pullDetails();
  } finally {
    state.detailsBusy = false;
  }
}
