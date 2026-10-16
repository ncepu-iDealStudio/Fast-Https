# Fast-Https 架构收敛与优化开发计划

修订日期：2026-10-16。

本文只规划代码目录与软件架构边界的渐进式收敛，不安排协议功能扩展或产品能力对标。每一步单独评审、提交和验证；前一步未达到验收条件，不进入下一步。

2026-10-16 修订按当前代码重新核对了阶段 2 以后的内容：核心层反向依赖补全为三个包；阶段 3 拆成四步，并把依赖守卫放到最前；阶段 4 先做重载一致性，完整配置快照按需再做；阶段 2 并入测试约定，阶段 5 推迟到有发行需要时。

## 1. 目标

逐步使代码目录能反映真实运行架构，并明确以下边界：

- 可执行程序的启动和依赖组装
- 服务器核心的协议、连接生命周期和请求路由
- 可插拔的请求处理能力
- 配置读取、验证和运行时配置快照
- 单元测试、集成测试和实验代码

本计划不要求一次性搬迁目录，不改变现有 JSON 字段、默认部署目录或网络行为。
完成标准是包间耦合降低、依赖方向可理解、结构约束可自动验证。

## 2. 当前结构基线

当前主要目录职责如下：

| 路径 | 当前职责 |
| --- | --- |
| 根目录 `fast-https.go` | 主服务器可执行程序入口 |
| `cmd/` | start、dev、stop、reload、check 等 CLI 命令及平台相关处理 |
| `config/`、`init/` | 配置加载/验证与启动初始化 |
| `modules/core/` | server、listener、events、filters、HTTP 请求响应、H2 等运行时代码 |
| `modules/static`、`proxy`、`rewrite`、`safe` 等 | 请求处理及横切功能模块 |
| `monitor/` | 独立的 Windows 托盘程序 |
| `test/` | 集中式单测、客户端 E2E、开发脚本和遗留测试入口 |
| 源码目录中的 `*_test.go` | 与部分 package 同位置的测试 |

需要优先澄清的现状：

1. 请求处理器通过 `core.GRRCHT` 注册。`static`、`proxy`、`rewrite`、`dev_mod` 导出 `Register()`，由 `cmd.RegisterHandlers` 在 `start`、`dev` 的 `ServerInit` 之前调用。`ServerInit` 在监听前用 `core.MissingHandlers` 检查配置中的 location 类型。

2. `modules/module.go` 的 `RegisterModule/GetModule` 已标为遗留注册表。调用方只有 `modules/logging`、`modules/workchain/example` 和 `test/unit_test/modules` 下的两个测试，HTTP 请求分发主链不使用它。代码仍保留。

3. 核心层不再引用功能包。连接过滤、TCP 转发、WebSocket、location 计数、Basic 认证和 `safe.Init` 由 `cmd.RegisterServices` 写入 `core.Services`，`modules/core` 只调用这些钩子；钩子为 nil 时放行或不处理。依赖守卫的允许清单已空。`modules/websocket` 仍引用 `modules/proxy`，这条依赖在功能包之间。

4. 测试同时分布在源码目录和 `test/unit_test/`；E2E 位于 `test/client_test/`。2026-10 之后新增的单测已放在被测 package 旁边。

5. `modules/core/engine`、`modules/core/routine_pool` 已标为未启用，`modules/workchain/example` 已标为示例。它们不在默认 `start` 路径上。

6. 配置路径和运行资源目录是现有部署契约的一部分。根目录 `fast-https.go` 被 Dockerfile、GoReleaser 和 RPM spec 写死为构建入口。

7. 重载有以下状态窗口（按代码阅读确认，尚未复现；已在阶段 4a 修复）：
   - `config.ReloadFile` 校验通过后调用 `LoadFile`，其中先 `ClearConfig()` 把 `GConfig` 置空再逐项写回。在途请求这时读到的 `GConfig.Limit.MaxBodySize` 可能为 0，带请求体的请求会被当成超限断开。
   - `serverContentType` 先把 `GContentTypeMap` 换成空 map，再用 `json.Unmarshal` 往同一个 map 写入。静态请求同时读这张表时，Go 运行时会报 `concurrent map read and map write` 并结束进程，无法 recover。
   - `safe.countsInit` 先把 `Gcl` 置空再按新 location 追加。在途请求仍持有旧 location 的 `cfg.ID`；`Gcl` 尚未建完或新配置 location 变少时，`events` 中的 `safe.Gcl[cfg.ID]` 会越界。处理协程没有 `recover`，越界会结束整个进程。
   - 另有一处与重载无关的错位：location `ID` 按 server → location 顺序编号，`countsInit` 按端口 → location 顺序追加。多个 server 交错使用同一端口时，部分 location 会用到别的 location 的限流计数器。

8. `go.mod` 的模块名是 `fast-https`，不含域名，外部项目无法通过 `go get` 引入其中的 package。判断一个 package 能否删除或改名，只需检查仓库内调用方。

## 3. 收敛原则

- 先确定依赖方向和职责，再移动文件；目录重命名本身不算架构改进。
- 每次只改变一个边界，保持现有启动命令、配置格式和服务行为兼容。
- `core` 为了分发请求只依赖接口和已装配的处理器表；功能模块依赖稳定的核心接口。
- 只保留一种正式的请求处理器注册/装配机制。
- 生产代码、示例/实验代码、测试代码都应有明确归属。删除前先处理完仓库内全部调用方。
- 架构约束用 `go test` 可执行的检查保证，文档只记录规则和理由。
- 正确性问题（如第 2 节第 7 项）优先于结构整理，可以不按阶段编号顺序插队。

## 4. 目标依赖方向

```text
fast-https.go
    -> cmd（启动组合层：注册 handler、注入过滤和安全服务）
        -> config
        -> core（server、listener、events、filters、protocol、routing contracts）
        -> feature handlers（static、proxy、rewrite、devmod）
        -> cross-cutting services（auth、safe、proxy_tcp、websocket、cache）

feature handlers -> core contracts + 所需的基础服务
core             -X-> 具体 feature handlers 和 cross-cutting services
```

`cmd` 已经导入 `server`，`start` 和 `dev` 都从这里进入，由它承担组合层职责，改动最小。只有当 `cmd` 中的装配代码明显膨胀，或 monitor 等其他入口也需要同一套装配时，再考虑抽出独立的 `internal/app`。继续使用静态编译的 Go package，不改造成 Go `plugin` 动态插件系统。

远期目录形态仅作参考，阶段 5 之前不按它搬迁：

```text
cmd/fast-https/       # 仅在决定把主入口从仓库根迁入时采用
cmd/monitor/          # 仅在需要统一多个可执行程序时采用
internal/app/         # 仅在 cmd 中的装配代码需要复用时采用
modules/<feature>/    # 静态、代理、重写等处理能力
*_test.go             # package 内单元测试
test/client_test/     # 子进程或完整部署链路测试
```

## 5. 分阶段实施计划

### 阶段 0：建立架构地图和验证基线

**难度：易；风险：低。**

**状态：已完成（2026-10-13）。** package 职责、生产入口、注册关系、构建路径和验证基线见 [architecture-map.md](architecture-map.md)。全量测试使用匹配的 Go 1.27.1 工具链串行通过；本机默认 GOROOT 混用问题及并行测试 exe 的 Windows `Access is denied` 已记入基线，不属于本阶段代码改动。

#### 阶段 0：工作内容

- 已维护 package 职责、生产入口、主要依赖者和默认服务器启动路径清单，见 [architecture-map.md](architecture-map.md) 第 1 节。
- 已区分主服务器、Windows monitor、测试工具和示例程序的构建入口，见该文档第 4、5 节。
- 已记录 `core.GRRCHT` 与通用 Module registry 的调用方和用途，见第 2 节；去留在阶段 3 第 4 步决定。
- 已记录 package/import 结构及 Docker、GoReleaser、RPM、CI 入口；本阶段未移动目录或改变运行行为。

#### 阶段 0：测试与验证

- `go list ./...`：已记录约 62 个 package。
- `go test ./...`：默认环境因 GOROOT 混用未能编译；使用匹配的 Go 1.27.1 toolchain 执行 `go test -p 1 ./...` 已通过。
- `go test -count=1 ./test/client_test -run TestProxyHTTPIntegration -v`：代理 E2E 四个场景通过。
- 已检查构建脚本、Dockerfile、GoReleaser、RPM spec 和 `.workflow/go-test.yml` 的入口与资源路径。

#### 阶段 0：验收条件

- 每个主要 package 都能回答“由谁调用、负责什么、是否是生产路径”。
- 有已记录的默认测试命令和当前通过/跳过情况。
- 本阶段无运行时行为变化。

### 阶段 1：先收敛目录语义，不移动生产代码

**难度：易；风险：低。**

**状态：已完成（2026-10-14）。** 只改了说明。`engine` 和 `routine_pool` 标为未启用，`workchain/example` 标为示例，`modules/module.go` 标为遗留注册表。代码未删，导入未改。记录见 [architecture-map.md](architecture-map.md) 第 1、2、5 节，以及 README 的目录树。

#### 阶段 1：工作内容

- 已标注 `modules/core/engine`、`routine_pool` 为未启用，`workchain/example` 为示例。
- 已标明 `modules/module.go` 是遗留注册表，请求分发不经过它。代码仍保留。
- README 与架构地图已按当前 package 区分生产路径和未启用路径。
- 已写明 `test/scripts`、`test/single/workloop.go` 不进入 `go test`，`engine.sh` 不是标准构建入口。

#### 阶段 1：测试与验证

- `go list ./...` 结果不变，或每个 package 变化都有明确说明。
- `go test ./...` 通过。
- 人工按文档从入口追到 server、handler，确认调用关系与文档一致。

#### 阶段 1：验收条件

- 目录中的“生产/示例/实验”身份明确。
- 没有因为整理说明而改变构建、导入或运行行为。

### 阶段 2：固定测试位置约定

**难度：易；风险：低。并入约定，不单独做目录迁移。**

**状态：已完成（2026-10-16）。** 约定写在 [test/readme.md](../test/readme.md) 第 1.0 节，1.1 节目录树已按实际文件更新（去掉不存在的 `safe/safe_test.go`，补上 `handler_ready_test.go`、`utils/message`、H2 连接测试和 `client_test` 的进程辅助文件）。核对时发现两个存量文件需要说明：`test/fast-https_test.go` 有写 pid、生成证书的副作用；`test/client_test/http_https_change_test.go` 是只有 `func main` 的手工客户端。两者均已在 1.0 节写明。后者已于同日移到 `test/single/httpclient/main.go`，`client_test` 的进程辅助文件改为 `_test.go` 后缀，`go build ./...` 整体通过。生产代码没有导入 `test/` 下的 package。

现有测试已经形成三种稳定形式：源码旁的单测、`test/unit_test/` 下的集中单测、`test/client_test/` 下的子进程 E2E。批量迁移或把 `client_test` 改名为 `e2e` 会牵动 CI、文档和命令，对后续阶段没有帮助。

#### 阶段 2：工作内容

- 在 [test/readme.md](../test/readme.md) 写明约定：新增单测放在被测 package 旁；`test/unit_test/` 只保留已有测试，不再新增；跨进程、真实 listener/upstream 的测试放在 `test/client_test/`。
- `test/unit_test/` 中的测试只在所测代码被阶段 3、4 改动时顺带迁到对应 package 旁，不专门搬迁。
- 生产代码不能依赖 `test/` 下的任何 package。

#### 阶段 2：验收条件

- `test/readme.md` 描述的位置约定与仓库现状一致。
- `go test ./...` 覆盖范围不变；环境变量控制的 E2E（`FASTHTTPS_E2E_RELOAD`）运行方式有记录。

### 阶段 3：统一请求处理器注册和启动装配

**难度：中；风险：中。这是最重要的运行时边界改造，按以下四步依次进行，每步单独提交。**

#### 第 1 步：依赖守卫测试（约 0.5 天）

**状态：已完成（2026-10-16）。** 测试位于 `modules/core/deps_test.go`。

- 用标准库 `go/parser` 解析 `modules/core/...` 下所有非测试 `.go` 文件的 import（包括带构建标签的平台文件），不引入第三方依赖。
- 规则按前缀判断：`modules/core/...` 不得导入 `fast-https/modules` 及其下除 `fast-https/modules/core/...` 以外的任何 package。新增的功能包自动受约束，不需要维护禁用名单。
- 第 2 节第 3 项原先的 10 条违规写入 `coreImportAllowlist`。第 2 步去掉 4 条，第 3 步去掉其余 6 条，允许清单现已为空。
- 出现新违规时测试失败，输出 `违规 package -> import 路径`；允许清单中的某条 import 已不存在时测试同样失败，要求删掉该条，保证清单只减不增。

验收结果：`go test ./modules/core/` 通过；临时放入引用 `modules/compress` 的探针文件时，测试失败并指出 `fast-https/modules/core/filters -> fast-https/modules/compress`。

#### 第 2 步：显式注册 handler（约 1 天）

**状态：已完成（2026-10-16）。**

- `static.Register`、`proxy.Register`、`rewrite.Register`、`devmod.Register` 取代各自的 `init()`，内部仍调用 `core.RRHandlerRegister`。
- `cmd.RegisterHandlers` 在 `start` 和 `dev` 路径上、`ServerInit` 之前调用这四个函数。代理 E2E 的子进程同样调用它，再进入 `ServerInit`。
- 已删除 `modules/core/server` 中的四个空白导入。
- `core.MissingHandlers` 检查保持原样，缺失时在监听前返回可诊断错误。
- location JSON 名称、handler 行为和功能模块目录未改。

验收结果：`cmd/register_test.go` 先清空五种 location 类型，确认 `MissingHandlers` 报出 `local`、`proxy-http`、`proxy-https`、`rewrite`、`devmod`，再调用 `RegisterHandlers` 后全部就绪。代理 E2E 通过。允许清单已去掉 server 对这四个包的导入，依赖守卫测试通过。

#### 第 3 步：过滤、计数、认证和安全初始化改为注入（约 1–2 天）

**状态：已完成（2026-10-16）。**

- `core.Services` 集中保存连接拒绝、监听分流、location 限流、认证和监听后初始化五个钩子。未设置的钩子放行或不处理。
- `cmd.RegisterServices` 在 `start`、`dev` 和代理 E2E 子进程里、`ServerInit` 之前填写这些钩子：黑名单和令牌桶、TCP 转发、WebSocket、`safe.CountLimitFor`、Basic 认证，以及 `safe.Init`。重载仍通过 `initModules` 调用 `AfterListen`。
- `filters`、`events`、`server` 不再导入功能包。允许清单已清空，依赖守卫测试通过。
- TCP 字节转发由 `TestProxyEventTcpEcho` 覆盖：本地上游回显 `ping`。WebSocket 字节转发由 `TestWebSocketHandlerEcho` 覆盖，不包含 HTTP Upgrade 握手。
- 监听分流仍沿用原来的比较：`LisType == config.PROXY_TCP`（值为 3）。监听器把带 `tcp` 的 listen 记为 `LisType` 2，因此这条分支在现有配置下不会进入 `ProxyEventTcp`。本步没有改这个判断。

验收结果：`go test ./modules/... ./cmd/...` 与代理 E2E 通过。静态、HTTP/HTTPS 代理、rewrite、devmod 仍由第 2 步的注册测试和代理 E2E 覆盖。

#### 第 4 步：处理遗留注册表（约 0.5 天）

- 依据第 2 节第 8 项，`modules/module.go` 没有仓库外调用方。`modules/logging` 和整个 `modules/workchain`（含 `example`）只被彼此和下列两个测试引用。
- 先在 `module.go` 加 `Deprecated` 注释，再在单独一次提交中删除 `modules/module.go`、`modules/logging`、`modules/workchain`，以及 `test/unit_test/modules/module_test.go`、`module_registration_consistency_test.go`。
- 同步更新 README、README.en.md 目录树和 [architecture-map.md](architecture-map.md)。

验收：`go build ./...`、`go test ./...` 通过；文档中不再出现已删除 package 的生产路径描述。

#### 阶段 3：整体验收条件

- 请求 handler 只有一种正式装配路径，由 `cmd` 完成。
- `modules/core/...` 不依赖具体功能包，并由依赖守卫测试持续保证。
- 所有当前配置的 `location.type` 仍在监听前完成 handler 检查。
- 主服务器和 monitor 均能构建。

### 阶段 4：配置与重载状态

**难度：4a 中、4b 难；风险：4a 中、4b 高。**

#### 4a：重载一致性（约 1–2 天，可在阶段 3 之前做）

**状态：已完成（2026-10-16）。** 验证结果：

- 生产 package 编译和改动包的 `go vet` 通过。
- `go test -p 1 ./...` 全部通过。其间 `TestRootCmdUnknownCommand` 曾失败：`878e3df` 把命令注册为 cobra 子命令后，未知命令返回错误并以退出码 1 结束，测试仍期望返回 nil。已按现行为改为断言返回 `unknown command` 错误。
- 代理 E2E 和 reload E2E（`FASTHTTPS_E2E_RELOAD=1`）通过，`config/fast-https.json` 未被改写。
- 本机有 gcc，`-race` 下 `config`、`modules/safe`、`modules/core/events`、`server`、`listener` 及集中单测中的 listener 重载测试通过。
- 反向验证：把 `config/config.go` 临时换回改动前版本，`TestReloadNeverExposesClearedConfig` 在读取 MIME 表处触发 Go 运行时并发 map 读写致命错误，证实第 2 节第 7 项中的 MIME 表问题存在。

修复第 2 节第 7 项的窗口，不做全局依赖注入：

- `config.LoadFile` 由 `processRoot` 在局部 `Fast_Https` 中构建配置，`loadContentTypes` 在局部 map 中解析 MIME 表，两者都成功后一次性赋给 `GConfig`、`GContentTypeMap`；失败时旧值保持不动。`ReloadFile` 不再需要先清空再恢复，`ClearConfig` 已删除。`SetDefault`、`ResolveRuntimePaths` 改为接收 `*Fast_Https`（仓库内无外部调用方）。
- `safe` 的计数表改为 `atomic.Pointer` 持有，`countsInit` 建好新表后整体替换，并按 location `ID` 放到对应下标，顺带修正编号顺序错位。导出的 `Gcl` 由 `safe.CountLimitFor(id)` 取代；`ID` 不在当前表中时返回 nil，`events` 对这次请求不计数。
- `config.Reload`、`config.ReloadFile`、`safe.Init` 签名不变。

测试：

- `config/reload_window_test.go`：一个协程持续读取 `MaxBodySize`、`Servers` 和 MIME 表，同时反复 `ReloadFile`，断言读不到零值或空表。`GConfig` 仍是无锁的结构体整体赋值，竞态检测器会把它标为数据竞争，所以该测试带 `!race` 构建标签；彻底消除属于 4b。
- `modules/safe/policy_test.go`：`TestCountLimitForUsesLocationID`（同一端口持有 ID 0 和 2 时各自拿到正确的计数器）、`TestCountLimitForOutOfRange`、`TestCountLimitForDuringRebuild`（反复在 3 个和 1 个 location 之间重建时并发查找不越界）。后者在 `-race` 下通过；Windows 上 `-race` 需要 cgo 和 gcc。
- 回归：`config/reload_test.go`、`config/paths_test.go`、`test/unit_test/modules/core/listener/reload_common_test.go`、reload E2E（`FASTHTTPS_E2E_RELOAD=1`）。
- 已同步 [iteration-plan.md](iteration-plan.md) 第 2 节和 R4。

#### 4b：完整配置快照（按需，暂不排期）

只在出现以下需求之一时启动：listener 需要整体替换而不是原地修改 `Cfg`/`HostMap`；需要多份配置并存（如测试并行）；全局配置读取导致新的正确性问题。

- 定义运行时配置快照，监听器和 handler 显式持有所需配置，逐个迁移对 `config.GConfig`、`GContentTypeMap` 的直接读取。
- 拆分配置 schema、文件加载/校验、路径解析和默认值职责；按责任拆包，不为每个结构体创建小 package。
- 保留安装包布局、`-tags=rpm` 路径、JSON 格式和现有 CLI 行为。
- reload 构造完整的新快照并验证，再切换配置和 listener。

验收：listener 和 handler 的运行行为不再依赖进程级全局配置；reload 成功是整份配置切换，失败保留整份旧状态；Windows 与 Linux 分别验证工作目录和资源定位。

### 阶段 5：按证据收拢目录和可执行入口

**难度：中；风险：中。推迟，只在出现发行或多入口需求时启动。**

根目录 `fast-https.go` 被 Dockerfile、GoReleaser、RPM spec 写死，搬迁入口的成本集中在发行链路，对代码边界没有直接收益。

#### 阶段 5：启动条件

- 需要统一主程序与 monitor 的构建方式，或发行工具要求 `cmd/<name>` 布局；
- 或阶段 3 第 4 步之后，`engine`、`routine_pool` 仍需保留并需要与生产代码物理隔离。

#### 阶段 5：工作内容

- 将主入口迁至 `cmd/fast-https`，按需要将 monitor 入口放至 `cmd/monitor`。
- 未启用的 `engine`、`routine_pool`：仓库内无调用方时直接删除；仍要保留时迁入 `experimental/`。
- 同步更新 Dockerfile、GoReleaser、RPM spec、Windows 资源、CI 工作目录和文档中的路径。

#### 阶段 5：测试与验收

- `go list ./...` 无旧路径残留；`go test ./...` 和所有目标平台的 `go build` 通过。
- 实际构建 Linux/RPM、Windows 主程序和 monitor 发布物，检查包内配置、静态资源和启动工作目录。
- 在临时安装目录运行 `check`、`start`、`status`、`reload`、`stop` 冒烟测试。
- 用户可见命令、配置和安装包结构没有未说明的破坏性变化。

## 6. 架构约束的自动验证

- 依赖守卫在阶段 3 第 1 步落地，以普通 Go 测试形式存在，随 `go test ./...` 和 `.workflow/go-test.yml` 一起执行，不需要额外 CI 步骤。
- 规则和允许清单集中在 `modules/core/deps_test.go`，允许项必须注明计划去掉的步骤。
- 规则扩展到多个边界时，仍优先在同一测试中增加规则；不为单条规则引入第三方架构检查工具。
- 失败信息必须指出违规 package 和 import 路径。

## 7. 阶段门禁与回退策略

每一步合并前都必须满足：

1. 先运行受影响 package 的测试，再运行 `go test ./...`（Windows 上使用 `-p 1`）。
2. 结构变化同步更新 README、README.en.md、[architecture-map.md](architecture-map.md) 和构建路径；行为变化同步更新 [iteration-plan.md](iteration-plan.md)。
3. 协议与配置不变时，运行代理 E2E；涉及重载时运行 reload E2E。
4. Windows 与 Linux 相关文件有各自的编译或 CI 验证，不以单平台本地通过代替。
5. 测试不得改写仓库内的 `config/fast-https.json`。

回退时按步 revert。第 2、3 步若影响既有配置或发行包，先恢复旧的注册方式，同时保留可独立测试的新函数；任何时刻只能有一套 handler 注册机制生效。

## 8. 推荐执行顺序和优先级

| 顺序 | 步骤 | 主要收益 | 工作量 | 优先级 |
| --- | --- | --- | --- | --- |
| — | 阶段 0：地图与验证基线 | 后续决策有证据 | — | 已完成（2026-10-13） |
| — | 阶段 1：标注目录语义 | 降低误读成本 | — | 已完成（2026-10-14） |
| — | 阶段 3 第 1 步：依赖守卫测试 | 冻结现有违规，后续只减不增 | — | 已完成（2026-10-16） |
| — | 阶段 4a：重载一致性 | 消除重载窗口内的误断连接和进程崩溃风险 | — | 已完成（2026-10-16） |
| — | 阶段 3 第 2 步：显式注册 handler | 去掉空白导入，装配入口唯一 | — | 已完成（2026-10-16） |
| — | 阶段 3 第 3 步：过滤与安全注入 | 核心层不再依赖功能包 | — | 已完成（2026-10-16） |
| 5 | 阶段 3 第 4 步：删除遗留注册表 | 只剩一套注册机制 | 0.5 天 | P2 |
| — | 阶段 2：测试位置约定 | 新测试有固定归属 | — | 已完成（2026-10-16） |
| — | 阶段 4b：完整配置快照 | 去除全局配置依赖 | 较大 | 按需 |
| — | 阶段 5：目录与入口搬迁 | 统一发行布局 | 中 | 按需 |

阶段 4a 与阶段 3 互不依赖，修的是正确性问题，因此排在阶段 3 第 2 步之前。

## 9. 本计划不包含

- 增加 upstream 负载均衡、HTTP/3、FastCGI 等新协议或产品功能。
- 重写 HTTP/1.1、HTTP/2、HPACK 或连接池实现。
- 因目录偏好而一次性将所有包移入 `internal/`。
- 配置文件变更后的自动重载（R7，已暂缓）。
- 改变现有 JSON 配置、安装包运行目录或 daemon 行为作为“顺便整理”。

## 10. 完成定义

架构收敛完成以以下结果衡量，目录数量和重命名数量不计入：

- package 职责和生产启动路径可被文档准确描述。
- core 与具体功能模块的依赖方向清晰，模块装配只有 `cmd` 一个入口，并由测试持续检查。
- 重载期间配置和计数表的切换没有中间状态。
- 单元测试、集成测试和实验代码各有稳定归属。
- CI 能验证行为回归与关键依赖规则；发布构建和现有部署方式保持兼容。
