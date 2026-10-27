# 功能特性索引

按编号组织本项目已实现与规划中的功能。每篇文档包含：功能说明、使用方式、实现方案、测试位置与已知限制。

状态图例：✅ 已实现 ｜ 🚧 部分实现 ｜ 📋 计划占位（未实现）

## 已实现

| 编号 | 文档 | 功能 | 一句话说明 |
| --- | --- | --- | --- |
| 0 | [0-architecture.md](0-architecture.md) | 架构总览 | 分层结构与一次请求的完整生命周期 |
| 1 | [1-http1.md](1-http1.md) | HTTP/1.1 服务 | 自研解析管线、keep-alive、优雅停机 |
| 2 | [2-http2.md](2-http2.md) | HTTP/2 | 委托 x/net/http2，ALPN 协商，Event 适配 |
| 3 | [3-tls-certs.md](3-tls-certs.md) | TLS 与证书 | SNI 多证书、自签 CA 自动生成、热替换 |
| 4 | [4-reload.md](4-reload.md) | 配置热重载 | 配置快照原子切换、监听器增删改、零停机 |
| 5 | [5-routing.md](5-routing.md) | 路由与虚拟主机 | server_name + path 正则匹配 |
| 6 | [6-static.md](6-static.md) | 静态文件 | try_files、目录索引、304/Range、穿越防护 |
| 7 | [7-proxy.md](7-proxy.md) | 反向代理 | 上游连接池、header 改写、chunked 解码 |
| 8 | [8-proxy-tcp.md](8-proxy-tcp.md) | TCP 代理 | 四层转发 |
| 9 | [9-websocket.md](9-websocket.md) | WebSocket 代理 | 升级劫持与双向管道 |
| 10 | [10-rewrite.md](10-rewrite.md) | URL 重写与开发模式 | 301 重定向、devmod |
| 11 | [11-cache.md](11-cache.md) | 代理响应缓存 | 磁盘持久化 + 内存热层 LRU |
| 12 | [12-compress.md](12-compress.md) | 响应压缩 | gzip/brotli 内容协商 |
| 13 | [13-safe.md](13-safe.md) | 黑名单与限流 | per-IP 令牌桶、location 计数限流 |
| 14 | [14-auth.md](14-auth.md) | Basic 认证 | location 级 HTTP Basic |
| 15 | [15-appfirewall.md](15-appfirewall.md) | 应用防火墙 | GuardianWAF 引擎，sql/xss 规则 |
| 16 | [16-logging.md](16-logging.md) | 日志系统 | 五路 JSON 日志、log_format 定制 |
| 17 | [17-config.md](17-config.md) | 配置系统 | JSON schema 解析、conf.d include、校验 |
| 18 | [18-cli-service.md](18-cli-service.md) | 服务管理与 CLI | start/stop/reload/install/status |
| 19 | [19-monitor.md](19-monitor.md) | 托盘监控 | Windows 系统托盘控制服务 |

## 规划中（占位）

| 编号 | 文档 | 功能 | 状态 |
| --- | --- | --- | --- |
| 20 | [20-planned-guardianwaf.md](20-planned-guardianwaf.md) | GuardianWAF 集成 | 已落地（R9），待灰度运营决策 |
| 21 | [21-planned-upstream-failover.md](21-planned-upstream-failover.md) | 代理上游故障转移 | 未排期（R8） |
| 22 | [22-planned-config-autoreload.md](22-planned-config-autoreload.md) | 配置文件自动重载 | 暂缓（R7） |

## 约定

- 每篇文档描述**当前主干**的实现，历史演进见 `docs/develop/plans/`（架构收敛计划、迭代计划）。
- 涉及并发语义的功能均以 `go test -race ./...` 全绿为底线（29 个包）。
- 依赖清单与选型理由见根目录 `README.md` 依赖表。
