// 通用 CLI 透传：状态矩阵、运行快照、操作记录等面板走这里。
import { service } from './base';

export interface CommandResult {
  command: string;
  data: unknown;
  warnings: string[];
  text?: string;
}

export function runCommand(args: string[]): Promise<CommandResult> {
  return service.RunCommand(args) as Promise<CommandResult>;
}
