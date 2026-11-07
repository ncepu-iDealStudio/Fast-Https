// 数据拉取编排：概览每 2 秒一次，明细（站点/证书/路径/矩阵日志）15 秒一次。
import { DesktopService, msgOf } from './api.js';
import { $, bind } from './ui/dom.js';
import { state } from './state.js';
import { renderOverview } from './pages/overview.js';
import { renderSites, renderCerts, renderPaths, renderLogs } from './pages/details.js';

export async function refreshOverview() {
  if (state.overviewBusy) return;
  state.overviewBusy = true;
  try {
    renderOverview(await DesktopService.GetOverview());
  } catch (e) {
    bind('running', '未连接后端');
    const alert = $('#overviewError');
    alert.hidden = false;
    alert.className = 'alert-strip';
    $('#alertTitle').textContent = '无法连接 Go 后端';
    $('#alertBody').textContent = msgOf(e) + '。请用 wails3 dev 或构建后的程序打开。';
  } finally {
    state.overviewBusy = false;
  }
}

export async function refreshDetails() {
  if (state.detailsBusy) return;
  state.detailsBusy = true;
  try {
    const [sites, certs, paths, lines] = await Promise.all([
      DesktopService.ListSites(),
      DesktopService.ListCerts(),
      DesktopService.Paths(),
      DesktopService.TailLog('access', 12)
    ]);
    renderSites(sites || [], certs || []);
    renderCerts(certs || []);
    renderPaths(paths);
    // 状态矩阵固定展示 access 尾部；日志页有自己的筛选与滚动分页，
    // 不跟着这里的定时刷新重建列表。
    renderLogs(lines, '#matrixLogs', 'rail');
  } catch (e) {
    console.warn('refreshDetails:', msgOf(e));
  } finally {
    state.detailsBusy = false;
  }
}
