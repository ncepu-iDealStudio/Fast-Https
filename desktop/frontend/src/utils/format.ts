/** 纯工具函数：不依赖 Vue、不依赖 Go 服务。 */

export function formatUptime(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds < 0) return '—';
  const d = Math.floor(seconds / 86400);
  const h = Math.floor((seconds % 86400) / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  if (d > 0) return `${d}d ${h}h`;
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}

export function joinPorts(ports: readonly string[]): string {
  return ports.length ? ports.join(', ') : '—';
}
