// 与 Go 后端的所有交互都从这里走：bindings 由 wails3 生成，
// 错误统一提取成可读文本，界面模块不再各自解析异常对象。
export { DesktopService } from '../bindings/fast-https-desktop';

export function msgOf(e) {
  if (!e) return '未知错误';
  return typeof e === 'string' ? e : (e.message || String(e));
}
