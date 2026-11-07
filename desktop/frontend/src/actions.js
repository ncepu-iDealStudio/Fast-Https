// 服务操作：data-action 按钮统一入口。启停/重载/状态直接转交主程序 CLI，
// 打开配置/日志/目录走主程序报出的运行时路径。
import { DesktopService, msgOf } from './api.js';
import { toast } from './ui/dom.js';
import { refreshOverview, refreshDetails } from './refresh.js';

async function openByPath(kind) {
  try {
    const p = await DesktopService.Paths();
    const target = kind === 'config' ? p.config : kind === 'log' ? p.log : (p.config ? p.config.replace(/[\\/][^\\/]+$/, '') : '');
    if (!target) throw new Error('路径为空');
    toast(await DesktopService.OpenPath(target));
  } catch (e) { toast('打开失败：' + msgOf(e)); }
}

export async function doAction(name) {
  const simple = {
    start: ['启动', () => DesktopService.StartServer()],
    stop: ['停止', () => DesktopService.StopServer()],
    reload: ['重载配置', () => DesktopService.ReloadServer()],
    status: ['查询状态', () => DesktopService.ServerStatus()]
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
    toast(text || label + '完成');
    await refreshOverview();
    await refreshDetails();
  } catch (e) {
    toast(label + '失败：' + msgOf(e));
  }
}
