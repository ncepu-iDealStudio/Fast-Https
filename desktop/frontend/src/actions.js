// 服务操作：data-action 按钮统一入口。启停/重载/状态直接转交主程序 CLI，
// 打开配置/日志/目录走主程序报出的运行时路径。
import { DesktopService, msgOf } from './api.js';
import { t } from './i18n.js';
import { toast } from './ui/dom.js';
import { refreshOverview, refreshDetails } from './refresh.js';
import { refreshOperations } from './ui/events.js';

async function openByPath(kind) {
  try {
    const p = await DesktopService.Paths();
    const target = kind === 'config' ? p.config : kind === 'log' ? p.log : (p.config ? p.config.replace(/[\\/][^\\/]+$/, '') : '');
    if (!target) throw new Error(t('toast.pathEmpty'));
    toast(await DesktopService.OpenPath(target));
  } catch (e) { toast(t('toast.openFailed', { msg: msgOf(e) })); }
}

export async function doAction(name) {
  const simple = {
    start: [t('action.start'), () => DesktopService.StartServer()],
    stop: [t('action.stop'), () => DesktopService.StopServer()],
    reload: [t('action.reload'), () => DesktopService.ReloadServer()],
    status: [t('action.status'), () => DesktopService.ServerStatus()]
  };
  if (name === 'open-config' || name === 'open-log' || name === 'open-config-dir') {
    await openByPath(name === 'open-config' ? 'config' : name === 'open-log' ? 'log' : 'dir');
    return;
  }
  const entry = simple[name];
  if (!entry) return;
  const [label, fn] = entry;
  try {
    const text = await fn();
    toast(text || t('toast.actionDone', { action: label }));
    await refreshOverview();
    await refreshDetails();
    refreshOperations();
  } catch (e) {
    const msg = msgOf(e);
    // 服务没跑时 reload 的失败提示翻译成运维口径
    if (name === 'reload' && /not running/i.test(msg)) {
      toast(t('toast.reloadNotRunning'));
      return;
    }
    toast(t('toast.actionFailed', { action: label, msg }));
  }
}
