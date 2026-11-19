// 缓存区：主配置 cache 段的增删改查。站点弹窗与设置页共用。
import { service } from './base';
import type { CacheZone } from './types';

export function listCacheZones(): Promise<CacheZone[]> {
  return service.ListCacheZones();
}
export function addCacheZone(zone: CacheZone): Promise<string> {
  return service.AddCacheZone(zone);
}
export function saveCacheZone(name: string, zone: CacheZone): Promise<string> {
  return service.SaveCacheZone(name, zone);
}
export function deleteCacheZone(name: string, purge: boolean): Promise<string> {
  return service.DeleteCacheZone(name, purge);
}
