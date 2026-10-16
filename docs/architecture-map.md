# Fast-Https 当前架构地图与验证基线

更新时间：2026-10-14。本文记录阶段 0 的 package 盘点，以及阶段 1 对生产、未启用、示例和遗留注册表的标注。目录和依赖关系以源码 import 及当时的测试结果为准，不表示目标架构。本轮只改说明，不改 Go 代码。

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
| `modules/core/filters` | 连接、监听、HTTP 解析、Host/Path 过滤 | 是。连接拒绝和监听分流调用 `core.Services` 钩子 |
| `modules/core/request`、`response` | HTTP 请求解析/请求体，以及响应生成 | 是 |
| `modules/core/h2/{conn,frame,hpack,util}` | HTTP/2 连接、帧、HPACK 和辅助逻辑 | H2 listener 启用时 |
| `modules/core/dynlog`、`timer` | 动态访问日志字段、计时器 | 是/按路径使用 |
| `modules/static`、`proxy`、`rewrite`、`dev_mod` | local、HTTP/HTTPS proxy、rewrite 和开发 location handler | 由 `cmd.RegisterHandlers` 在启动时装配 |
| `modules/proxy_tcp`、`websocket` | TCP 字节代理和 WebSocket upgrade | 通过 filters 分流使用 |
| `modules/auth`、`safe`、`appfirewall`、`cache`、`compress` | 认证、安全检查/限流、防火墙、代理缓存和静态压缩 | 配置或请求路径启用时 |
| `modules/core/engine` | master/slave 注册与心跳。`engine.sh` 会改标识后单独编译 | 未启用。默认 `start` 不调用 `EngineInit` |
| `modules/core/routine_pool` | 协程池 | 未启用。`server.go` 里的导入保持注释 |
| `monitor` | Windows/amd64 托盘程序，独立 `main` | 独立可执行程序，不在服务器进程内 |
| `test`、`test/client_test`、`test/unit_test/...` | 根测试入口、子进程 E2E、集中式单测 | 测试专用 |
| 源码目录中的 `*_test.go` | 与被测 package 同路径的测试 | 测试专用 |

`modules/core/h2` 下还包含顶层 H2 stream/state/window 与子包 `conn`、`frame`、`hpack`、`util`；`hpack` 的 Huffman 和整数编码也分别作为独立 package。

## 2. 两套注册机制的调用关系

### 请求 handler 表：生产主链

- `modules/core/core.go` 定义 `GRRCHT` 和 `RRHandlerRegister`。
- `modules/static` 的 `Register` 登记 `LOCAL`；`modules/proxy` 登记 HTTP/HTTPS proxy；`modules/rewrite` 登记 rewrite；`modules/dev_mod` 登记 devmod。
- `cmd.RegisterHandlers` 在 `start`、`dev` 的 `ServerInit` 之前调用这四个函数。导入这些包本身不再注册。
- `ServerInit()` 调用 `core.MissingHandlers()`，确认配置中非 TCP 的 location type 已有 handler，再创建监听器。
- `modules/core/events.EventHandler()` 根据匹配到的 location type 从 `GRRCHT` 取得处理函数。
- TCP proxy 和 WebSocket 不使用 location handler 表的普通请求分发。`cmd.RegisterServices` 把它们放进监听钩子，`filters` 只调用这个钩子。

阶段 3 第 4 步（2026-10-16）已删除 `modules/module.go` 的 `RegisterModule` / `GetModule`，以及只被它使用的 `modules/logging`、`modules/workchain` 和对应测试。请求日志仍走 `utils/message` 和 `dynlog`。

## 3. 主要依赖边界

从源码 imports 和 `go list` 看，核心运行方向大致是：

```text
fast-https main -> cmd -> init/config/server + handler Register()
server -> core + listener + events + safe hook + dynlog
handler modules -> core contracts + listener/config + required services
```

目前功能包之间仍有的横向引用：

- `modules/websocket` import `modules/proxy`，WebSocket 与 HTTP proxy 实现存在较强耦合。
- `modules/core/request` import `modules/core/listener`，请求 Host 解析接收 listener 对象；可作为后续接口边界评估项。
- `modules/static` 和 `modules/proxy` 都直接依赖 `appfirewall`；安全处理横切功能当前由 handler 显式调用。

这些是依赖结构观察，不等价于要求立即拆包。core 对功能包的直接引用已在阶段 3 第 3 步去掉。

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

## 5. 测试、脚本和示例

自动测试：

- 源码邻近的 `*_test.go`：`config/`、`init/`、`modules/core/`、H2、listener、request、response、server、proxy、static、safe 等。
- 集中单测：`test/unit_test/`。
- 子进程集成测试：`test/client_test/` 里的 `*_test.go`。`TestProxyHTTPIntegration` 默认会跑。`TestReloadPortSwitchE2E` 需要 `FASTHTTPS_E2E_RELOAD=1`。
- 根目录 `test/fast-https_test.go` 覆盖初始化路径。
- CI 命令是 `go test ./...`。

不进入 `go test` 的文件：

- `test/scripts/dev-test.sh`、`dev-test.ps1`：手工拉起 `dev` 的脚本。
- `test/single/workloop.go`：独立 `main`，只演示等待 `SIGINT`，不是服务器测试。
- `engine.sh`：改写引擎标识后另编 master/slave，不是 `go build .` 的入口。

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
- 请求处理只有 `GRRCHT` 这一条生产路径，由 `cmd.RegisterHandlers` 装配。遗留注册表已在阶段 3 第 4 步删除。
- 核心层通过 `core.Services` 调用功能包，由 `cmd.RegisterServices` 填写。WebSocket 仍依赖 proxy，配置和请求对象仍跨层传递。
- 测试已经形成源码邻近单测、集中单测、E2E 三种形式。阶段 2（2026-10-16）已把位置约定写进 [test/readme.md](../test/readme.md) 第 1.0 节，现有测试未移动。
- 基线测试可通过匹配的 Go 1.27.1 工具链以串行方式运行；本机 Go 环境本身存在 GOROOT 混用问题，需独立修复，不属于项目架构改造。
