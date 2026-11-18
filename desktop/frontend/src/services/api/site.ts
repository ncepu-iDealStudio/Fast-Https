// 站点相关：列表、详情、增删改、启停。
import { service } from './base';
import type { Site } from '../../types/model';
import type { SiteDetail, SiteInput } from './types';

export function listSites(): Promise<Site[]> {
  return service.ListSites();
}
export function readSiteDetail(id: string): Promise<SiteDetail> {
  return service.ReadSiteDetail(id);
}
export function addSite(input: SiteInput): Promise<string> {
  return service.AddSite(input);
}
export function saveSiteDetail(input: SiteInput): Promise<string> {
  return service.SaveSiteDetail(input);
}
export function deleteSite(id: string): Promise<string> {
  return service.DeleteSite(id);
}
export function setSiteEnabled(id: string, enabled: boolean): Promise<string> {
  return service.SetSiteEnabled(id, enabled);
}
