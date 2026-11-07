# Fast-Https 当前架构地图与验证基线

更新时间：2026-10-24。本文记录当前 package 盘点与依赖方向。目录和依赖关系以源码 import 及 `tests/arch` 的约束测试为准。

2026-10-24 数据面重构要点：`utils/*` 上移为 `infra/*`；`modules/core` 拆分为契约层 `core` 与数据面运行时 `runtime`；处理器注册表由定长数组改为 map；`core.Services` 补齐停机钩子；日志统一为 zerolog 单出口 JSON。

相关文档：[项目架构图](architecture.html)（[PNG](images/architecture.png) / [深色 PNG](images/architecture-dark.png)）、[架构收敛计划](develop/archive/architecture-convergence-plan.md)、[启动与运行逻辑](about.md)、[当前产品设计](product-design-latest.md)。

## 1. 分层结构与依赖方向

`go list ./...` 当前枚举 49 个 package。分层与允许的依赖方向如下，任何反向边都会被 `tests/arch/layering_test.go` 拦下：

| 层 | 包 | 允许依赖 | 说明 |
| --- | --- | --- | --- |
| 基础设施 | `infra/{logging,files,color,timeutil,jsonc}` | 仅 `infra/*` 与标准库、zerolog | 不依赖 `config`，日志常量由本层定义、配置层引用；`jsonc` 是 JSONC 编解码工具、`files` 是原子写文件工具，都不认任何配置字段 |
| 配置 | `config` | `infra/*` | 配置 schema、校验、快照与站点文件；JSONC 解析交给 `infra/jsonc`。`TestNoViperLeak` 保证配置库不外泄 |
| 契约 | `core` | `infra/*`、`config`、`runtime/{listener,request,response}` | 只放 `Event`、处理器注册表与生命周期钩子；后三个是 `Event` 内嵌的数据模型，属登记例外 |
| 数据面运行时 | `runtime/*` | `infra/*`、`config`、`core`、`runtime/*` | 不允许依赖 `modules` |
| 可插拔功能 | `modules/*` | 下层 + `runtime/{listener,request,response,dynlog}` | 不允许依赖 `runtime/{server,events,filters}` |
| 组合根 | `cmd`、`init`、根 `main` | 任意 | 唯一允许装配全部层的地方 |

## 2. Package 与生产职责

| Package / 路径 | 职责和主要入口 | 默认服务器路径 |
| --- | --- | --- |
| 根 `fast-https` | 主服务器 `main`，调用 `cmd.RootCmd()` | 是 |
| `cmd` | Cobra 命令、start/dev/stop/reload/check/status、`acme` 配置向导（`huh` 表单 + 参数模式）、PID 和平台信号处理；`RegisterHandlers` / `RegisterServices` 是唯一的装配点 | 是 |
| `config` | 配置模型、JSON/include 读取、验证、路径和全局运行配置 | 是 |
| `init` | 配置、日志、证书、缓存初始化 | 是 |
| `infra/banner` | 彩色横幅、版本号与初始化阶段提示（版本由组合根注入） | 由组合根调用 |
| `infra/logging` | 进程唯一日志出口：控制台一路 + 文件四路，全部 JSON | 是 |
| `infra/color`、`timeutil` | 终端着色（唯一入口，后端为 `fatih/color`）、nginx 风格时长解析 | 是 |
| `infra/jsonc` | JSONC 编解码与原文级改写：`StripComments`/`Unmarshal`（等长掩码）+ `ScanObject`/`PatchMember`（保留注释与字段顺序地替换或追加成员），`acme` 段写回靠它 | 是 |
| `infra/files` | 原子写文件：先写 `.tmp` 再 rename，配置回写与 ACME PEM 落盘共用 | 是 |
| `core` | `Event`、处理器注册表、默认命令变量、`Services` 生命周期钩子 | 是 |
| `runtime/server` | 启停、信号、连接跟踪、listener 生命周期和 reload | 是 |
| `runtime/listener` | 配置转 Listener/ListenCfg、虚拟主机表、TCP/TLS socket、端口 reload | 是 |
| `runtime/events` | HTTP/1.x 连接事件、请求解析、统一事件分发；HTTP/2 走 `golang.org/x/net/http2`，这里只做 `http.Request`/`http.ResponseWriter` 与 `core.Event` 的适配 | 是 |
| `runtime/filters` | 连接、监听、HTTP 解析、Host/Path 过滤 | 是。连接拒绝和监听分流调用 `core.Services` 钩子 |
| `runtime/request`、`response` | HTTP 请求解析/请求体，以及响应生成 | 是 |
| `runtime/dynlog` | 按 `http.log_format` 组装访问日志结构化字段 | 是 |
| `golang.org/x/net/http2` | HTTP/2 协议栈：分帧、HPACK、流状态机、流量控制 | H2 listener 启用时 |
| `modules/static`、`proxy`、`rewrite`、`dev_mod` | local、HTTP/HTTPS proxy、rewrite 和开发 location handler | 由 `cmd.RegisterHandlers` 在启动时装配 |
| `modules/proxy_tcp`、`websocket` | TCP 字节代理和 WebSocket upgrade | 通过监听钩子分流 |
| `modules/auth`、`safe`、`appfirewall`、`cache`、`compress` | 认证、安全检查/限流、防火墙、代理缓存和静态压缩 | 配置或请求路径启用时 |
| `modules/certmanager` | ACME 证书编排（certmagic + acmez）：账户、订单、续期调度、PEM 落盘与热替换；挑战应答通过 `core.Services.HandleEarlyRequest` 与 `listener.RegisterTLSALPNProvider` 注入数据面 | `acme.enabled` 为真时 |
| `modules/site` | 站点文件（`conf.d/<name>.json`）的命名校验、原子落盘与目录扫描；合并与校验复用 `config`，不碰运行期对象 | `site` 命令组 |
| `desktop/` | Wails v3 桌面管理端，独立 Go module | 独立可执行程序 |
| `tests`、`tests/client_test`、`tests/unit_test/...`、`tests/arch` | 根测试入口、子进程 E2E、集中式单测、分层约束 | 测试专用 |

## 3. 请求 handler 注册机制

- `core/registry.go` 定义 `RegisterHandler`、`Handler`、`HandlerReady` 和 `RegisteredTypes`。注册表是 `map[int]RRcircleHandler`，类型码由 `config` 定义，不再受定长数组长度限制。
- `modules/static` 的 `Register` 登记 `LOCAL`；`modules/proxy` 登记 HTTP/HTTPS proxy；`modules/rewrite` 登记 rewrite；`modules/dev_mod` 登记 devmod。导入这些包本身不注册。
- `cmd.RegisterHandlers` 在 `start` 的 `ServerInit` 之前调用这四个函数。
- `ServerInit()` 调用 `core.MissingHandlers()`，确认配置中非 TCP 的 location type 已有 handler，再创建监听器。
- `runtime/events.EventHandler()` 根据匹配到的 location type 从 `core.Handler` 取得处理函数；缺失时返回 500，不再空指针崩溃。
- TCP proxy 和 WebSocket 不走 location 表。`cmd.RegisterServices` 把它们放进 `core.Services.HandleListen`，`filters` 只调用这个钩子。
- `core.Services` 的钩子覆盖完整生命周期：`HandleEarlyRequest`（location 匹配之前处理保留路径，如 ACME 挑战）、`RejectConn`、`HandleListen`、`RejectLocation`、`RejectAuth`、`AfterListen`（reload 会再次触发，实现需幂等）、`BeforeStop`、`AfterStop`。`config.Reload` 成功后 `dynlog.LogRegister` 会先清空再重建字段表。

## 4. 日志子系统

`infra/logging` 是唯一出口，基于 zerolog，所有输出都是单行 JSON：

| 输出 | 变量 | `kind` | 目的地 | 级别 |
| --- | --- | --- | --- | --- |
| 控制台 | `ConsoleLog` | `console` | stdout | 默认 `info` |
| 系统 | `SystemLog` | `system` | `logs/system.log` + stdout | `debug` 及以上 |
| 访问 | `AccessLog` | `access` | `logs/access.log` + stdout | `debug` 及以上 |
| 错误 | `ErrorLog` | `error` | `logs/error.log` + stdout | `debug` 及以上 |
| 安全 | `SafeLog` | `safe` | `logs/safe.log` + stdout | `debug` 及以上 |

字段约定集中在 `infra/logging/consts.go`，是对外契约：

- 时间格式为 RFC3339 带毫秒与时区，字典序等于时间序，可直接做范围比较。
- `kind` 区分日志来源，合并读取多个文件后无需依赖文件名判断。
- 访问日志字段：`remote_addr`、`type`、`method`、`path`、`host`、`status`、`size`、`user_agent`，由 `http.log_format` 选择子集。
- `status` 与 `size` 输出为 JSON number，查询侧可直接做区间比较。
- 安全日志带 `reason`：`rate_limit`、`location_limit`、`blacklist`。

写入路径没有中间缓冲：`logging.AccessEvent()` 返回原始 zerolog 事件，`runtime/dynlog` 直接追加字段后 `Send()`，每个文件由一个 `syncWriter` 串行化以避免多连接协程写出的行交错。

## 5. 仍存在的横向依赖

- `modules/websocket` import `modules/proxy`，WebSocket 与 HTTP proxy 实现存在较强耦合。
- `runtime/request` import `runtime/listener`，请求 Host 解析接收 listener 对象。
- `modules/static` 与 `modules/proxy` 都直接依赖 `modules/appfirewall`；安全处理由 handler 显式调用。

这些是依赖结构观察，不等价于要求立即拆包。

## 6. 可执行程序和构建入口

| 产物/环境 | 当前入口与构建方式 |
| --- | --- |
| 主服务器本地构建 | 仓库根 package：`go build .`；主入口文件为 `fast-https.go` |
| Docker | `build/Dockerfile` 使用 Go 1.27 Alpine，在 `/app` 执行 `go build -o fast-https .`；运行镜像复制配置、MIME 和默认静态文件 |
| GoReleaser | `build/goreleaser.yaml` 的主程序 `main` 为 `./fast-https.go`，构建 Linux/Darwin amd64/arm64 和 Windows amd64 |
| 桌面端 | 独立 module `./desktop`，由 Wails v3 构建 |
| RPM | `build/spec` 文件执行 `go build -tags=rpm .`，按 FHS 安装：配置到 `%{_sysconfdir}/fast-https`、站点到 `%{_datadir}/fast-https`、日志目录 `%{_localstatedir}/log/fast-https` |
| Linux 包（deb/rpm/apk） | `build/goreleaser.yaml` 的 `nfpms` 段，安装单元 `build/packaging/systemd/`、安装脚本 `build/packaging/linux/` |
| Go CI | `.workflow/go-test.yml` 使用 Go 1.27.1，执行 `go test ./...` |
| Windows 本地测试 | 直接 `go test -p 1 ./...`（并行测试 exe 在 Windows 上会偶发 `Access is denied`） |

运行配置和默认资源仍按安装包布局提供；配置路径由工作目录和 `config` 常量共同决定。架构迁移不能只移动 Go package，还必须同步检查 Docker、GoReleaser、RPM 和工作目录相关契约。`httpdoc/`、`config/*.json`、`config/cert/` 是随包发布的运行资源（FHS 计划见 [fhs-layout-plan.md](develop/plans/fhs-layout-plan.md)），不放进 `build/`；`build/` 只放构建脚本、打包配方与 Dockerfile。

## 7. 测试组织

- 源码邻近的 `*_test.go`：`cmd/`、`config/`、`infra/*`、`init/`、`core/`、`runtime/*`、`modules/*`。
- 集中单测：`tests/unit_test/`，只保留存量，新单测写在被测包旁。
- 分层约束：`tests/arch/layering_test.go` 校验 `infra`↔`config`、`core`、`runtime`、`modules` 的依赖方向，并禁止 viper 泄漏与生产代码依赖 `tests/`。
- 子进程集成测试：`tests/client_test/`。`TestProxyHTTPIntegration` 默认会跑；`TestReloadPortSwitchE2E` 需要 `FASTHTTPS_E2E_RELOAD=1`。
- 根目录 `tests/fast-https_test.go` 覆盖初始化路径。

不进入 `go test` 的文件：`tests/scripts/dev-test.sh`、`dev-test.ps1`（手工前台拉起服务），`tests/single/workloop.go`（独立 `main` 演示）。

## 8. 历史验证基线（2026-10-13）

仓库声明 `go 1.27.0`、`toolchain go1.27.1`。

| 检查 | 结果 |
| --- | --- |
| `go list ./...` | 成功 |
| `go test ./...`（当前默认环境） | 首次失败在编译阶段：`go` 为 1.27.1，但 `GOROOT` 标准库带有 1.26.3 产物标记 |
| 匹配工具链的 `go test ./...` | 并行运行时偶发测试进程启动返回 Windows `Access is denied` |
| 匹配工具链的 `go test -p 1 ./...` | 通过 |
| `go test -count=1 ./tests/client_test -run TestProxyHTTPIntegration -v` | 通过 |

Windows 环境若偶发测试 exe 启动拒绝，可使用 `go test -p 1 ./...`。

## 9. 结论

- 主服务器是根 package，`cmd/` 是 CLI 实现包；`desktop/` 是独立 Go module 的 Wails 桌面端。
- 请求处理只有 `core.RegisterHandler` 这一条生产路径，由 `cmd.RegisterHandlers` 装配。
- 核心层通过 `core.Services` 调用功能模块，由 `cmd.RegisterServices` 填写，并覆盖启动、重载与停机三个阶段。
- 日志只有 `infra/logging` 一个出口，格式统一为 JSON，字段名在 `consts.go` 集中定义。
- 测试形成源码邻近单测、集中单测、E2E、分层约束四种形式。
