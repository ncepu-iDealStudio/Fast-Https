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
export function pickPath(kind: string): Promise<string> {
  return service.PickPath(kind);
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
