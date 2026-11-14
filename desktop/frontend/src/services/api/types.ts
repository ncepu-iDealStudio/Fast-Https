// Wails bindings 是生成代码（JS + JSDoc），这里给出精确签名，
// 业务层只依赖这个接口，不接触生成物，也不使用 any。
import type { Overview, Site, Cert, LogEntry, RuntimePaths } from '../../types/model';

export interface ConfigForm {
  [key: string]: string;
}
export interface SiteInput {
  id: string;
  name: string;
  listen: string;
  type: string;
  root: string;
  proxyPass: string;
  zip: string[];
  limitRate: number;
  limitBurst: number;
  limitMem: number;
  limitNodelay: boolean;
  authOn: boolean;
  authUser: string;
  authPass: string;
  authRealm: string;
  cachePath: string;
  cacheValid: string[];
  cacheMaxSize: number;
}
export interface SiteDetail extends SiteInput {
  source: string;
  cert: string;
  certKey: string;
  index: string;
  path: string;
  raw: string;
}
export interface CertTrustGuide {
  [key: string]: string;
}
export interface LogQuery {
  keyword: string;
  level: string;
  page: number;
  pageSize: number;
}
export interface LogPage {
  entries: LogEntry[];
  total: number;
}

/** DesktopService 契约（与 REFACTOR-BASELINE.md 的 28 个方法一致）。 */
export interface DesktopServiceContract {
  MinimiseWindow(): Promise<void>;
  ToggleMaximiseWindow(): Promise<void>;
  HideWindow(): Promise<void>;
  QuitApp(): Promise<void>;
  OpenHelp(): Promise<string>;
  ReadConfigForm(): Promise<ConfigForm>;
  SaveConfigForm(values: Record<string, string>): Promise<string>;
  ReadSiteDetail(id: string): Promise<SiteDetail>;
  AddSite(input: SiteInput): Promise<string>;
  SaveSiteDetail(input: SiteInput): Promise<string>;
  DeleteSite(id: string): Promise<string>;
  SetSiteEnabled(id: string, enabled: boolean): Promise<string>;
  GetOverview(): Promise<Overview>;
  ListSites(): Promise<Site[]>;
  ListCerts(): Promise<Cert[]>;
  TailLog(name: string, lines: number): Promise<LogEntry[]>;
  Paths(): Promise<RuntimePaths>;
  StartServer(): Promise<string>;
  StopServer(): Promise<string>;
  ReloadServer(): Promise<string>;
  ServerStatus(): Promise<string>;
  OpenPath(path: string): Promise<string>;
  ImportLocalCA(): Promise<string>;
  QueryLogs(query: LogQuery): Promise<LogPage>;
  CertTrustGuide(): Promise<CertTrustGuide>;
  PickPath(kind: string): Promise<string>;
  RunCommand(args: string[]): Promise<unknown>;
}
