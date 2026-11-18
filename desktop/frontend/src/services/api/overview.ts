// 概览 / 列表类数据查询。
import { service } from './base';
import type { Overview } from '../../types/model';

export function getOverview(): Promise<Overview> {
  return service.GetOverview();
}
