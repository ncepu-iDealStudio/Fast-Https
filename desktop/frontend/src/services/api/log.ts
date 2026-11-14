// 日志与配置：日志尾读 / 检索，配置表单读取与保存。
import { service } from './base';
import type { LogEntry } from '../../types/model';
import type { ConfigForm, LogQuery, LogPage } from './types';

export function tailLog(name: string, lines: number): Promise<LogEntry[]> {
  return service.TailLog(name, lines);
}
export function queryLogs(query: LogQuery): Promise<LogPage> {
  return service.QueryLogs(query);
}
export function readConfigForm(): Promise<ConfigForm> {
  return service.ReadConfigForm();
}
export function saveConfigForm(values: Record<string, string>): Promise<string> {
  return service.SaveConfigForm(values);
}
