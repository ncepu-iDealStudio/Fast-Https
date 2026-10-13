# Fast-Https 当前架构地图与验证基线

更新时间：2026-10-13。本文记录阶段 0 对当前 package、生产入口、模块注册和构建/测试入口的盘点。目录和依赖关系以 `go list ./...`、源码 import 及实际测试结果为准，不表示目标架构。

相关文档：[架构收敛计划](architecture-convergence-plan.md)、[启动与运行逻辑](about.md)。

## 1. Package 与生产职责

仓库当前由一个 Go module（`fast-https`）组成，`go list ./...` 枚举约 62 个 package。没有拆分 Go 子模块；多个可执行程序和测试包共享根 `go.mod`。

| Package / 路径 | 职责和主要入口 | 默认服务器路径 |
| --- | --- | --- |
| 根 `fast-https` | 主服务器 `main`，调用 `cmd.RootCmd()` | 是 |
| `cmd` | Cobra 命令、start/dev/stop/reload/check/status、PID 和平台信号处理 | 是 |
| `config` | 配置模型、JSON/include 读取、验证、路径和全局运行配置 | 是 |
| `init` | 配置、日志、证书、缓存和安全相关初始化 | 是 |
| `output` | logo、启动提示和 listener 启动提示 | 是 |
| `utils`、`utils/{files,logger,message,security,color}` | 文件、日志消息、控制台日志、证书/密码学等基础能力 | 被生产代码使用 |
| `modules/core` | Event、请求/响应循环、处理器表 `GRRCHT` 和默认命令变量 | 是 |
| `modules/core/server` | 启停、信号、连接跟踪、listener 生命周期和 reload | 是 |
| `modules/core/listener` | 配置转 Listener/ListenCfg、虚拟主机表、TCP/TLS socket、端口 reload | 是 |
| `modules/core/events` | HTTP/1.x 连接事件、请求解析、统一事件分发及 HTTP/2 stream 事件 | 是 |
| `modules/core/filters` | 连接、监听、HTTP 解析、Host/Path 过滤 | 是；目前还直接调用具体 feature 包 |
| `modules/core/request`、`response` | HTTP 请求解析/请求体，以及响应生成 | 是 |
| `modules/core/h2/{conn,frame,hpack,util}` | HTTP/2 连接、帧、HPACK 和辅助逻辑 | H2 listener 启用时 |
| `modules/core/dynlog`、`timer` | 动态访问日志字段、计时器 | 是/按路径使用 |
| `modules/static`、`proxy`、`rewrite`、`dev_mod` | local、HTTP/HTTPS proxy、rewrite 和开发 location handler | 由 server blank import 装配 |
| `modules/proxy_tcp`、`websocket` | TCP 字节代理和 WebSocket upgrade | 通过 filters 分流使用 |
| `modules/auth`、`safe`、`appfirewall`、`cache`、`compress` | 认证、安全检查/限流、防火墙、代理缓存和静态压缩 | 配置或请求路径启用时 |
| `modules/core/engine`、`routine_pool` | 分布式 master/slave 相关代码、协程池 | 未见默认 server 启动路径调用 |
| `modules/logging`、`modules/workchain`、`modules/workchain/example` | 通用 Logger 注册和 workchain 示例 | 未见默认 server 请求处理路径调用 |
| `monitor` | Windows/amd64 托盘程序，独立 `main` | 独立可执行程序，不在服务器进程内 |
| `test`、`test/client_test`、`test/unit_test/...` | 根测试入口、子进程 E2E、集中式单测 | 测试专用 |
| 源码目录中的 `*_test.go` | 与被测 package 同路径的测试 | 测试专用 |

`modules/core/h2` 下还包含顶层 H2 stream/state/window 与子包 `conn`、`frame`、`hpack`、`util`；`hpack` 的 Huffman 和整数编码也分别作为独立 package。

## 2. 两套注册机制的调用关系

### 请求 handler 表：生产主链

- `modules/core/core.go` 定义 `GRRCHT` 和 `RRHandlerRegister`。
- `modules/static` 注册 `LOCAL`；`modules/proxy` 注册 HTTP/HTTPS proxy；`modules/rewrite` 注册 rewrite；`modules/dev_mod` 注册 devmod。
- `modules/core/server/server.go` blank import 这些包，利用包初始化函数完成注册。
- `ServerInit()` 调用 `core.MissingHandlers()`，确认配置中非 TCP 的 location type 已有 handler，再创建监听器。
- `modules/core/events.EventHandler()` 根据匹配到的 location type 从 `GRRCHT` 取得处理函数。
- TCP proxy 和 WebSocket 不使用 location handler 表的普通请求分发：`core/filters` 在监听/upgrade 阶段直接分流。

### 通用 Module 注册表：示例与测试路径

- `modules/module.go` 定义 `RegisterModule`、`GetModule[T]` 和 package 级 registry。
- 当前生产源码调用方是 `modules/logging` 的 `DefaultLogger`；`modules/workchain/example` 注册示例处理器。
- registry 的主要读取和一致性验证出现在 `test/unit_test/modules`。
- 未发现默认 server 启动代码调用 `GetModule`。它目前不等同于 HTTP request handler 插件系统，也不负责装配 `GRRCHT`。

## 3. 主要依赖边界

从源码 imports 和 `go list` 看，核心运行方向大致是：

```text
fast-https main -> cmd -> init/config/server
server -> core + listener + events + safety/logging + blank-imported handlers
handler modules -> core contracts + listener/config + required services
```

目前需要在后续阶段进一步解耦的反向/横向引用：

- `modules/core/filters` import `modules/proxy_tcp`、`modules/safe`、`modules/websocket`，core 子层认识具体功能实现。
- `modules/websocket` import `modules/proxy`，WebSocket 与 HTTP proxy 实现存在较强耦合。
- `modules/core/request` import `modules/core/listener`，请求 Host 解析接收 listener 对象；可作为后续接口边界评估项。
- `modules/static` 和 `modules/proxy` 都直接依赖 `appfirewall`；安全处理横切功能当前由 handler 显式调用。

这些是依赖结构观察，不等价于要求立即拆包。后续重构应结合调用方向和测试替换需求，优先消除 core 对具体 feature 的直接依赖。

## 4. 可执行程序和构建入口

| 产物/环境 | 当前入口与构建方式 |
| --- | --- |
| 主服务器本地构建 | 仓库根 package：`go build .`；主入口文件为 `fast-https.go` |
| Docker | 根 `Dockerfile` 使用 Go 1.27 Alpine，在 `/app` 执行 `go build -o fast-https .`；运行镜像复制配置、MIME 和默认静态文件 |
| GoReleaser | `.goreleaser.yaml` 的主程序 `main` 为 `./fast-https.go`，构建 Linux/Darwin amd64/arm64 和 Windows amd64 |
| Windows monitor | 独立 package `./monitor`，源码 build tag 为 `windows && amd64`；GoReleaser before hook 编译后打包 |
| RPM | 根目录 `spec` 文件执行 `go build -tags=rpm .`，安装程序及 `config/`、`httpdoc/root/` 等运行资源 |
| Engine 脚本 | `engine.sh` 临时改写 `config/consts.go` 标识并多次构建；属于专用脚本，不是标准主程序构建入口 |
| Go CI | `.workflow/go-test.yml` 使用 Go 1.27.1，执行 `go test ./...` |
| Windows 测试脚本 | `_run_tests.bat` 选取部分 package，使用 `go test -p 1` 并将输出追加到 `_gotest_run.log` |

运行配置和默认资源仍按安装包布局提供；配置路径由工作目录和 `config` 常量共同决定。架构迁移不能只移动 Go package，还必须同步检查 Docker、GoReleaser、RPM 和工作目录相关契约。

## 5. 测试结构基线

- 源码邻近测试：目前存在于 `config/`、`init/`、`modules/core/`、H2、listener、request、response、server、proxy、static、safe 等目录。
- 集中单测：`test/unit_test/` 覆盖 cmd、config、compress、modules/core、safe、logger、message、security 等。
- 集成测试：`test/client_test/` 包含隔离子进程 proxy E2E 和可选 reload E2E；同时保留手工客户端样例。
- 根测试：`test/fast-https_test.go` 覆盖初始化集成路径。
- CI 默认命令是全仓 `go test ./...`；环境变量门控的 E2E 需单独记录执行条件。

## 6. 阶段 0 验证结果

验证日期：2026-10-13。仓库声明 `go 1.27.0`、`toolchain go1.27.1`；有效工具链为 `go1.27.1 windows/amd64`。

| 检查 | 结果 |
| --- | --- |
| `go list ./...` | 成功；枚举约 62 个 package，用于本地图和入口盘点 |
| `go test ./...`（当前默认环境） | 首次失败在编译阶段：`go` 命令为 1.27.1，但 `GOROOT=C:\Users\xjg\.g\go` 的标准库/工具带有 1.26.3 产物标记；未进入项目断言 |
| 匹配工具链的 `go test ./...` | 并行运行时仅 `modules/core/h2/hpack` 测试进程启动返回 Windows `Access is denied`，其他包通过；未显示 Go 断言失败 |
| 匹配工具链的 `go test -p 1 ./...` | 通过。串行验证避免了上述 Windows 测试可执行文件启动错误；输出中 `[no test files]` 是无测试 package 的正常结果 |
| `go test -count=1 ./test/client_test -run TestProxyHTTPIntegration -v` | 通过；动态 HTML、JSON API 请求体/头、静态资源和上游 404 四个场景均通过 |

匹配工具链通过当前终端临时设置 `GOROOT` 为缓存内 Go 1.27.1 toolchain 目录执行，没有修改系统安装或仓库文件。若在本机复现全量基线，先确保 `GOROOT`、`go`、`compile`、`asm` 来自同一个 Go 版本；Windows 环境若偶发测试 exe 启动拒绝，可使用 `go test -p 1 ./...`。

## 7. 阶段 0 结论

- 主服务器是根 package，`cmd/` 是 CLI 实现包；monitor 是独立 Windows/amd64 package。尚无必要为了目录惯例立即迁移可执行入口。
- 请求 handler registry 与通用 Module registry 当前是两套不同机制，只有前者进入默认 server 请求处理链。阶段 3 需要先决定正式扩展契约，再统一组合入口。
- 核心耦合的主要落点在 filters 直接调用 feature、WebSocket 依赖 proxy，以及配置/请求对象跨层传递；应按小步注入接口，不做批量目录搬迁。
- 测试已经形成源码邻近单测、集中单测、E2E 三种形式。后续阶段 2 应明确规范，而不是先机械移动现有测试。
- 基线测试可通过匹配的 Go 1.27.1 工具链以串行方式运行；本机 Go 环境本身存在 GOROOT 混用问题，需独立修复，不属于项目架构改造。
