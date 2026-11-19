// Go 侧 model 包的 TS 镜像。字段与 desktop/internal/model/model.go 一致。
// 供 services/api 与 stores 使用；新增字段时同步 Go 与这里。

export interface RuntimePaths {
  config: string;
  cert: string;
  log: string;
  pid: string;
  /** 沿用 CLI `config path --json` 的键名，不是 camelCase。 */
  root_crt: string;
  state: string;
}

export interface Overview {
  status: string;
  running: boolean;
  pid: number;
  version: string;
  uptimeSeconds: number;
  configVersion: string;
  ports: string[];
  siteCount: number;
  tlsCount: number;
  cacheZoneCount: number;
  nearestCertDays: number;
  configDirty: boolean;
  paths: RuntimePaths;
  checkedAt: string;
  error: string;
}

export interface Site {
  id: string;
  name: string;
  port: string;
  listen: string;
  listens: string[];
  tls: boolean;
  source: string;
  enabled: boolean;
  certStatus: string;
  error: string;
}

export interface Cert {
  domain: string;
  subject: string;
  names: string;
  issuer: string;
  notAfter: string;
  daysLeft: number;
  selfSigned: boolean;
  local: boolean;
  status: string;
  acmeManaged: boolean;
  file: string;
  note: string;
}

export interface LogEntry {
  raw: string;
  time: string;
  level: string;
  kind: string;
  message: string;
  method: string;
  path: string;
  status: string;
  size: string;
  host: string;
  remoteAddr: string;
  rejectReason: string;
  rule: string;
  remoteIp: string;
  reason: string;
}
