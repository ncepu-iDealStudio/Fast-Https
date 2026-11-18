// 操作命令的用户可读名称。未知命令原样返回，方便定位新命令。
import { translate } from './useI18n';

export function commandLabel(cmd?: string): string {
  if (!cmd) return '—';
  const key = `ops.command.${cmd}`;
  const label = translate(key);
  return label === key ? cmd : label;
}
