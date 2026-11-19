// Wails bindings 是生成代码（JS + JSDoc），这里给出精确签名，
// 业务层只依赖这个接口，不接触生成物，也不使用 any。
import type { Overview, Site, Cert, LogEntry, RuntimePaths } from '../../types/model';
import type { ConfigForm } from '../../types/config';

export type { ConfigForm };

export interface SiteInput {
  id: string;
  name: string;
  listen: string;
  type: string;
  root: string;
  proxyPass: string;
  /** 静态根符号链接策略：'' 默认 / check-at-load / reject / allow；只对 local 生效。 */
  symlinkPolicy: string;
  /** 证书 / 私钥路径；留空表示走默认约定（./cert/<域名>.pem，首次启动生成）。 */
  cert: string;
  certKey: string;
  zip: string[];
  /** 最小压缩阈值（字节）；0 表示不限制。 */
  zipMinLength: number;
  /** 跳过压缩的 MIME 类型；空数组表示用内置已压缩类型清单。 */
  zipSkipTypes: string[];
  limitRate: number;
  limitBurst: number;
  limitMem: number;
  limitNodelay: boolean;
  authOn: boolean;
  authUser: string;
  authPass: string;
  authRealm: string;
  /** 主配置 cache 段里的缓存区名；空串表示不启用缓存。 */
  cacheZone: string;
}

/** 主配置 cache 段里的一个缓存区。 */
export interface CacheZone {
  name: string;
  path: string;
  valid: string[];
  max_size: number;
}

export interface SiteDetail extends SiteInput {
  source: string;
  path: string;
  editable: boolean;
  note: string;
  cert: string;
  certKey: string;
  index: string;
  raw: string;
}

/** logs query 的筛选条件，字段与 Go 的 model.LogQuery 一一对应。 */
export interface LogQuery {
  kind: string;
  limit: number;
  level: string;
  status: string;
  host: string;
  method: string;
  pathPrefix: string;
  reason: string;
  rejectReason: string;
  since: string;
  until: string;
  cursor: string;
}

export interface LogPage {
  lines: LogEntry[];
  hasMore: boolean;
  nextUntil: string;
}

export interface CertTrustGuide {
  [key: string]: string;
}

/** DesktopService 契约（与 REFACTOR-BASELINE.md 的 Service 清单一致）。 */
export interface DesktopServiceContract {
  MinimiseWindow(): Promise<void>;
  ToggleMaximiseWindow(): Promise<void>;
  HideWindow(): Promise<void>;
  QuitApp(): Promise<void>;
  SetLanguage(lang: string): Promise<void>;
  OpenHelp(): Promise<string>;
  OpenRepo(): Promise<void>;
  ReadConfigForm(): Promise<ConfigForm>;
  SaveConfigForm(values: Record<string, string>): Promise<string>;
  ReadSiteDetail(id: string): Promise<SiteDetail>;
  AddSite(input: SiteInput): Promise<string>;
  SaveSiteDetail(input: SiteInput): Promise<string>;
  DeleteSite(id: string): Promise<string>;
  SetSiteEnabled(id: string, enabled: boolean): Promise<string>;
  ListCacheZones(): Promise<CacheZone[]>;
  AddCacheZone(zone: CacheZone): Promise<string>;
  SaveCacheZone(name: string, zone: CacheZone): Promise<string>;
  DeleteCacheZone(name: string, purge: boolean): Promise<string>;
  GetOverview(): Promise<Overview>;
  ListSites(): Promise<Site[]>;
  ListCerts(): Promise<Cert[]>;
  TailLog(name: string, lines: number): Promise<LogEntry[]>;
  Paths(): Promise<RuntimePaths>;
  /** 开机自启动：Windows 读写 HKCU\...\Run，其它平台恒为 false。 */
  GetAutoStart(): Promise<boolean>;
  SetAutoStart(enabled: boolean): Promise<boolean>;
  StartServer(): Promise<string>;
  StopServer(): Promise<string>;
  ReloadServer(): Promise<string>;
  ServerStatus(): Promise<string>;
  OpenPath(path: string): Promise<string>;
  ImportLocalCA(): Promise<string>;
  QueryLogs(query: LogQuery): Promise<LogPage>;
  CertTrustGuide(): Promise<CertTrustGuide>;
  PickPath(kind: string, initial: string): Promise<string>;
  /** 程序数据目录的绝对路径，用于把配置里的相对目录拼成可读路径。 */
  DataDir(): Promise<string>;
  RunCommand(args: string[]): Promise<unknown>;
}
