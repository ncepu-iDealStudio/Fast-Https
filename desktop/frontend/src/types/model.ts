// Go 侧 model 包的 TS 镜像。字段与 desktop/internal/model/model.go 一致。
// 供 services/api 与 stores 使用；新增字段时同步 Go 与这里。

export interface RuntimePaths {
  config: string;
  cert: string;
  log: string;
  pid: string;
  rootCrt: string;
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
  nearestCertDays: number;
  configDirty: boolean;
  paths: RuntimePaths;
  checkedAt: string;
  error: string;
}

export interface Site {
  id: string;
  name: string;
  listen: string;
  listens: string[];
  port: string;
  type: string;
  root: string;
  proxyPass: string;
  enabled: boolean;
  tls: boolean;
}

export interface Cert {
  id: string;
  name: string;
  daysLeft: number;
  path: string;
}

export interface LogEntry {
  line: string;
  kind: string;
  time: string;
}
