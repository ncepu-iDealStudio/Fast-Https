// 数据拉取编排：概览每 2 秒一次，明细（站点/证书/路径/矩阵日志）15 秒一次。
// 新增卡片（运行快照/Monitor/ACME/配置版本）的刷新各自管理，不进这里的轮询。
import { DesktopService, msgOf } from './api.js';
import { t } from './i18n.js';
import { $, bind } from './ui/dom.js';
import { state } from './state.js';
import { renderOverview } from './pages/overview.js';
import { renderSites, renderCerts, renderPaths, renderLogEntries } from './pages/details.js';

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

export async function refreshDetails() {
  if (state.detailsBusy) return;
  state.detailsBusy = true;
  try {
    const [sites, certs, paths, entries] = await Promise.all([
      DesktopService.ListSites(),
      DesktopService.ListCerts(),
      DesktopService.Paths(),
      DesktopService.TailLog('access', 8)
    ]);
    renderSites(sites || [], certs || []);
    renderCerts(certs || []);
    renderPaths(paths);
    // 状态矩阵固定展示 access 尾部（TailLog 现在返回 model.LogEntry[]）；
    // 日志页有自己的筛选与游标分页，不跟着这里的定时刷新重建列表。
    renderLogEntries(entries || [], '#matrixLogs');
  } catch (e) {
    console.warn('refreshDetails:', msgOf(e));
  } finally {
    state.detailsBusy = false;
  }
}
