# 功能特性索引

按编号组织本项目已实现的功能。每篇文档包含：功能说明、使用方式、实现方案、测试位置与已知限制。

编号文档之外，本目录还收 CLI 与 Monitor 两个改造已落地的**扫描报告、开发规范与分步计划**（见文末「设计与实施记录」）。未实现的方案不在本目录，统一写在 [docs/develop/plans/](../develop/README.md)。

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
| 18 | [18-cli-service.md](18-cli-service.md) | 服务管理与 CLI | start/stop/reload/status、runtime 与 monitor 只读查询、受控 apply |
| 19 | [19-desktop-console.md](19-desktop-console.md) | 桌面管理端 | Wails v3 单窗口 + 托盘：状态、站点与配置表单、证书日志，转交 CLI 启停 |
| 20 | [20-runtime.md](20-runtime.md) | 本机运行快照 | `GET 127.0.0.1:9088/runtime`：生命周期、连接、请求、耗时与上游计数 |
| 21 | [21-monitor.md](21-monitor.md) | 独立 Monitor 进程 | 周期采集 `/runtime`、写快照与事件文件；规则：不可达/恢复/重启/5xx 比例/端口探测/疑似卡死 |
| 22 | [22-ai-agent.md](22-ai-agent.md) | 独立 AI Agent | 尾读 Monitor 事件，只读诊断并记录待审批；不自动改配置 |

## 设计与实施记录

CLI 与 Monitor 两轮改造开工前各有一份扫描报告与一份分步计划，CLI 另有命令与接口规范。代码落地后这些文档随功能文档一起留在本目录：正文保留开工时的判断，只在开头补一段「执行状态」指出现行行为看哪一篇。

| 文档 | 内容 |
| --- | --- |
| [fast-https-CLI代码扫描报告.md](fast-https-CLI代码扫描报告.md) | 开工前扫描：现有代码是否具备开发管理 CLI 的条件 |
| [fast-https-CLI开发规范.md](fast-https-CLI开发规范.md) | CLI 命令与接口规范（面向 AI 代码智能体的实现基线） |
| [fast-https-CLI改造开发计划.md](fast-https-CLI改造开发计划.md) | S0–S14 的改动范围、验收测试与回退方式 |
| [fast-https-Monitor代码扫描报告.md](fast-https-Monitor代码扫描报告.md) | 开工前扫描：是否具备开发独立 Monitor 进程的条件 |
| [fast-https-Monitor开发前置条件与代码扫描清单.md](fast-https-Monitor开发前置条件与代码扫描清单.md) | 开工前的逐项核对清单，已于 2026-11-08 扫完 |
| [fast-https-Monitor改造开发计划.md](fast-https-Monitor改造开发计划.md) | 第 0–8 步的改动范围、验收测试与回退方式 |

命令级的字段、错误码与实测输出见 [docs/cli/](../cli/command-spec.md)；尚未闭环的验收与可选能力见 [cli-monitor-followup.md](../develop/plans/cli-monitor-followup.md)。

## 约定

- 每篇文档描述**当前主干**的实现，历史演进见 `docs/develop/archive/`（架构收敛计划、迭代计划）。
- 尚未实现的方案不进本目录：上游故障转移见 [upstream-failover-plan.md](../develop/plans/upstream-failover-plan.md)，配置自动重载见 [config-autoreload-plan.md](../develop/plans/config-autoreload-plan.md)。CLI 与 Monitor 改造计划的代码主路径已落地，尚未闭环的验证与可选能力见 [cli-monitor-followup.md](../develop/plans/cli-monitor-followup.md)。
- 涉及并发语义的功能均以 `go test -race ./...` 全绿为底线（29 个包）。
- 依赖清单与选型理由见根目录 `README.md` 依赖表。
