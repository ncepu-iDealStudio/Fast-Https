// 证书相关：列表、导入本地 CA、信任指引、证书检查。
import { service } from './base';
import type { Cert } from '../../types/model';

export function listCerts(): Promise<Cert[]> {
  return service.ListCerts();
}
export function importLocalCA(): Promise<string> {
  return service.ImportLocalCA();
}
export function certTrustGuide(): Promise<Record<string, string>> {
  return service.CertTrustGuide();
}
