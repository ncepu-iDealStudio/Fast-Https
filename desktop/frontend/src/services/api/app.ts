// 应用外壳相关：窗口控制、路径、服务启停、配置目录。
import { service } from './base';
import type { RuntimePaths } from '../../types/model';

export function minimiseWindow(): Promise<void> {
  return service.MinimiseWindow();
}
export function toggleMaximiseWindow(): Promise<void> {
  return service.ToggleMaximiseWindow();
}
export function hideWindow(): Promise<void> {
  return service.HideWindow();
}
export function quitApp(): Promise<void> {
  return service.QuitApp();
}
export function openConfigDir(path: string): Promise<string> {
  return service.OpenPath(path);
}
/** 打开随发行包分发的中文完整命令参考（docs/usage.html）。 */
export function openHelp(): Promise<string> {
  return service.OpenHelp();
}
export function pickPath(kind: string): Promise<string> {
  return service.PickPath(kind);
}

/**
 * 选目录/文件并返回**绝对路径**。
 *
 * PickPath 原本返回「相对 config 目录」的写法（配置字段需要相对路径，整目录
 * 搬迁才不会失效），站点根目录这类场景却必须用绝对路径，否则界面里会出现
 * "..\..\..\front\xxx" 这种看不出基准的字符串。这里用同一个基准还原成绝对路径。
 */
export async function pickAbsPath(kind: string): Promise<string> {
  const picked = await pickPath(kind);
  if (!picked || isAbsPath(picked)) return picked;
  const paths = await getRuntimePaths();
  return joinPath(dirOf(paths.config), picked);
}

/** 绝对路径判定：盘符开头（D:\ / D:/）或 UNC（\\server\share）。 */
function isAbsPath(p: string): boolean {
  return /^[A-Za-z]:[\\/]/.test(p) || p.startsWith('\\\\');
}

/** 取文件所在目录（不依赖 Node 的 path 模块）。 */
function dirOf(file: string): string {
  const s = file.replace(/[\\/]+$/, '');
  const i = Math.max(s.lastIndexOf('\\'), s.lastIndexOf('/'));
  return i > 0 ? s.slice(0, i) : s;
}

/** 以 base 目录为基准拼接相对路径，处理 . 与 ..（Windows 分隔符）。 */
function joinPath(base: string, rel: string): string {
  const unc = base.startsWith('\\\\');
  const baseParts = base
    .replace(/\//g, '\\')
    .replace(/^\\\\/, '')
    .split('\\')
    .filter(Boolean);
  const parts = baseParts.slice();
  for (const seg of rel.replace(/\//g, '\\').split('\\')) {
    if (!seg || seg === '.') continue;
    if (seg === '..') {
      if (parts.length > 1) parts.pop();
      continue;
    }
    parts.push(seg);
  }
  return (unc ? '\\\\' : '') + parts.join('\\');
}
export function getRuntimePaths(): Promise<RuntimePaths> {
  return service.Paths();
}
export function startServer(): Promise<string> {
  return service.StartServer();
}
export function stopServer(): Promise<string> {
  return service.StopServer();
}
export function reloadServer(): Promise<string> {
  return service.ReloadServer();
}
export function serverStatus(): Promise<string> {
  return service.ServerStatus();
}
