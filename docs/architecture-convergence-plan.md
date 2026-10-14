# Fast-Https 架构收敛与优化开发计划

修订日期：2026-10-13。

本文只规划代码目录与软件架构边界的渐进式收敛，不安排协议功能扩展或产品能力对标。每阶段应单独评审、提交和验证；前一阶段未达到验收条件，不进入下一阶段。

## 1. 目标

逐步使代码目录能反映真实运行架构，并明确以下边界：

- 可执行程序的启动和依赖组装
- 服务器核心的协议、连接生命周期和请求路由
- 可插拔的请求处理能力
- 配置读取、验证和运行时配置快照
- 单元测试、集成测试和实验代码

本计划不要求一次性搬迁目录，不改变现有 JSON 字段、默认部署目录或网络行为。
优化以降低包间耦合、让依赖方向可理解、让结构调整可自动验证为完成标准。

## 2. 当前结构基线

当前主要目录职责如下：

| 路径 | 当前职责 |
| --- | --- |
| 根目录 `fast-https.go` | 主服务器可执行程序入口 |
| `cmd/` | start、dev、stop、reload 等 CLI 命令及平台相关处理 |
| `config/`、`init/` | 配置加载/验证与启动初始化 |
| `modules/core/` | server、listener、events、filters、HTTP 请求响应、H2 等运行时代码 |
| `modules/static`、`proxy`、`rewrite`、`safe` 等 | 请求处理及横切功能模块 |
| `monitor/` | 独立的 Windows 托盘程序 |
| `test/` | 集中式单测、客户端 E2E、开发脚本和遗留测试入口 |
| 源码目录中的 `*_test.go` | 与部分 package 同位置的测试 |

需要优先澄清的现状：

1. 请求处理器通过 `core.GRRCHT` 注册，server 包通过 blank import 导入 `static`、`proxy`、`rewrite`、`dev_mod` 等包以触发注册。

2. `modules/module.go` 的 `RegisterModule/GetModule` 已标为遗留注册表。调用方是 logger、workchain 示例和测试，不是 HTTP 请求分发主链。代码仍保留。

3. `modules/core/filters` 直接依赖 `safe`、`proxy_tcp`、`websocket` 等功能包；功能注册、连接协议分流和请求路由职责有交叉。

4. 测试同时分布在源码目录和 `test/unit_test/`；E2E 位于 `test/client_test/`。

5. `modules/core/engine`、`modules/core/routine_pool` 已标为未启用，`modules/workchain/example` 已标为示例。它们不在默认 `start` 路径上，本阶段不删除、不搬迁。

6. 配置路径和运行资源目录是现有部署契约的一部分。即使内部配置实现重构，也要先保持配置格式和安装包目录不变。

## 3. 收敛原则

- 先确定依赖方向和职责，再移动文件；不要用目录重命名代替架构改进。
- 每次只改变一个边界，保持现有启动命令、配置格式和服务行为兼容。
- `core` 不应为了分发请求而直接依赖具体功能模块；功能模块依赖稳定的核心接口。
- 只保留一种正式的请求处理器注册/装配机制。若通用模块注册表是公开扩展 API，需先定义其契约；若只是示例机制，应明确标注并避免和运行时插件概念混淆。
- 生产代码、示例/实验代码、测试代码都应有明确归属。未确认调用方前，不删除未进入默认启动路径的包。
- 架构约束要有自动验证，不只写在文档中。

## 4. 目标依赖方向

建议逐步收敛到以下依赖关系，不要求立即采用这些精确目录名称：

```text
main / cmd
    -> app 或 runtime（启动组合层）
        -> config snapshot
        -> core（server、listener、protocol、routing contracts）
        -> feature handlers（static、proxy、rewrite、devmod）
        -> cross-cutting services（auth、safe、cache、logging）

feature handlers -> core contracts + 所需的基础服务
core             -X-> 具体 feature handlers
```

关键点是由组合层了解有哪些功能模块并注册它们，而 `core` 只认识接口和已装配的处理器表。这样既可以继续使用静态编译的 Go package，也可以单测替换 handler；不要求改造成 Go `plugin` 动态插件系统。

可供后续讨论的目录形态示例：

```text
cmd/fast-https/       # 仅在决定把主入口从仓库根迁入时采用
cmd/monitor/          # 仅在需要统一多个可执行程序时采用
internal/app/         # 程序组合与启动编排
internal/config/      # 配置模型、加载和验证
internal/core/        # server、listener、events、routing contracts
modules/<feature>/    # 静态、代理、重写等处理能力
modules/experimental/ # 已确认非生产功能的实验实现
*_test.go             # package 内单元测试
 test/e2e/             # 子进程或完整部署链路测试
```

这是目标参考，不是当前迭代的批量移动清单。Go `internal/` 也会改变外部 package 可见性，迁入前必须确认模块对外使用情况。

## 5. 分阶段实施计划

### 阶段 0：建立架构地图和验证基线

**难度：易；风险：低；建议作为本计划第一项。**

**状态：已完成（2026-10-13）。** package 职责、生产入口、注册关系、构建路径和验证基线见 [architecture-map.md](architecture-map.md)。全量测试使用匹配的 Go 1.27.1 工具链串行通过；本机默认 GOROOT 混用问题及并行测试 exe 的 Windows `Access is denied` 已记入基线，不属于本阶段代码改动。

#### 阶段 0：工作内容

- 已维护 package 职责、生产入口、主要依赖者和默认服务器启动路径清单，见 [architecture-map.md](architecture-map.md) 第 1 节。
- 已区分主服务器、Windows monitor、测试工具和示例程序的构建入口，见该文档第 4、5 节。
- 已记录 `core.GRRCHT` 与通用 Module registry 的调用方和用途；通用 registry 是否作为正式扩展 API 留待阶段 3 决策，见第 2 节。
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
- 已明确 `modules/module.go` 是遗留注册表，不是请求分发 API。代码仍保留。
- README 与架构地图已按当前 package 区分生产路径和未启用路径。
- 已写明 `test/scripts`、`test/single/workloop.go` 不进入 `go test`，`engine.sh` 不是标准构建入口。

#### 阶段 1：测试与验证

- `go list ./...` 结果不变，或每个 package 变化都有明确说明。
- `go test ./...` 通过。
- 人工按文档从入口追到 server、handler，确认调用关系与文档一致。

#### 阶段 1：验收条件

- 目录中的“生产/示例/实验”身份明确。
- 没有因为整理说明而改变构建、导入或运行行为。

### 阶段 2：整理测试布局和测试职责

**难度：易到中；风险：低到中。**

#### 阶段 2：工作内容

- 新增单元测试优先放在被测 package 旁，特别是需要验证未导出实现细节的测试。
- `test/` 保留跨 package、跨进程、真实 listener/upstream 和部署流程测试；可将 `client_test` 逐步更名为 `e2e` 或 `integration`，一次只迁移一个 package。
- 将共用 fixture 放到 `test/testdata` 或有明确边界的测试辅助 package；生产代码不能反向依赖 `test/`。
- 将手工运行样例和 `go test` 自动测试分开命名和说明。

#### 阶段 2：测试与验证

- 迁移前后分别运行受影响 package 的 `go test`，再运行 `go test ./...`。
- 确认 E2E 默认使用本地临时配置和临时目录，不改写用户工作区配置、不要求外网或真实应用运行时。
- 验证 `.workflow/go-test.yml` 仍覆盖所有默认测试；环境变量控制的 E2E 要单独记录运行方式。

#### 阶段 2：验收条件

- 测试位置能区分单元测试和端到端测试。
- 所有迁移后的测试仍进入标准 CI 命令，E2E 仍可独立运行。
- 测试辅助代码不污染生产 package。

### 阶段 3：统一请求处理器注册和启动装配

**难度：中到难；风险：中到高；这是最重要的运行时边界改造。**

#### 阶段 3：工作内容

1. 为 location handler、连接/监听过滤器定义稳定的小接口，说明注册时机和生命周期。
2. 增加组合层（可先放在现有 server 附近，稳定后再决定是否使用 `internal/app`），由它导入并注册 static、proxy、rewrite、devmod 等生产 handler。
3. 调整 `core/filters`：将 TCP proxy、WebSocket 和安全服务通过接口/依赖参数传入，避免 core import 具体 feature package。
4. 保留启动期缺 handler 检查；检查对象改为最终装配结果，缺失时在监听前返回可诊断错误。
5. 对通用 `modules.RegisterModule/GetModule` 作出明确决策：
   - 若是正式 API，写清注册契约、重复注册和类型校验规则，并让它服务于真实功能；
   - 若不是正式 API，先迁移或标注 logger/workchain 示例消费者，再弃用或删除孤立注册表。
6. 不在该阶段变更 location JSON 名称、请求 handler 行为或功能模块目录。

#### 阶段 3：测试与验证

- 单测覆盖 handler 注册、重复/缺失 handler、TCP listener 特殊分流、默认 handler 集合。
- 通过依赖图检查 core 不再 import 具体 feature package；允许组合层依赖 core 和 feature。
- `go test ./modules/core/... ./modules/...`，再运行 `go test ./...`。
- 运行 `go test ./test/client_test -run TestProxyHTTPIntegration -v`，验证代理、静态和错误响应不变。
- 对主服务器和 monitor 运行构建，确认 blank import 移除后没有遗漏功能模块。

#### 阶段 3：验收条件

- 请求 handler 只有一种正式装配路径。
- core 包不依赖具体请求功能包。
- 所有当前配置的 `location.type` 仍在启动时完成 handler 检查。
- 默认启动、静态站点、HTTP/HTTPS 代理、rewrite、devmod、TCP proxy 和 WebSocket 的既有入口有对应测试或明确的暂缺说明。

### 阶段 4：让配置依赖从全局状态转为显式快照

**难度：难；风险：高；应在阶段 3 稳定后开始。**

#### 阶段 4：工作内容

- 定义不可变的运行时配置快照，加载和验证产生新快照，监听器和 handler 显式持有需要的配置。
- 逐个迁移对 `config.GConfig`、`GContentTypeMap` 和包级全局状态的直接读取，不要求一次性消除所有全局量。
- 拆分配置 schema、文件加载/校验、路径解析和默认值职责；按责任拆包，不为每个结构体创建小 package。
- 明确相对路径基准和工作目录策略，保留安装包布局、`-tags=rpm` 路径、JSON 格式和现有 CLI 行为。
- 让 reload 构造完整的新快照并验证，再原子地切换配置和 listener，避免半更新状态。

#### 阶段 4：测试与验证

- 配置解析单测覆盖主配置、include、默认值、相对/绝对路径和无效字段。
- reload 测试确认无效快照不替换现有配置；有效快照更新 common listener 并正确新增/移除端口。
- 在 Windows 与 Linux 构建/测试；对容器前台与 Linux daemon 路径分别验证工作目录和资源定位。
- 运行 `go test ./...`、proxy E2E 和 reload E2E（`FASTHTTPS_E2E_RELOAD=1`）。

#### 阶段 4：验收条件

- listener 和 handler 的运行行为不再依赖“当前进程全局配置恰好已加载”。
- reload 成功是整份配置切换，失败保留整份旧状态。
- 现有发行目录、配置文件字段和命令兼容。

### 阶段 5：按证据收拢目录和可执行入口

**难度：中；风险：中。**

#### 阶段 5：工作内容

- 只有在 package 边界稳定后，才考虑将主入口迁至 `cmd/fast-https`，并按需要将 monitor 入口放至 `cmd/monitor`。
- 只有确认 engine/routine_pool/workchain 是实验或示例后，才迁入 `experimental/` 或 `examples/`；生产能力则保留在功能模块目录并补齐启动装配说明。
- 根据阶段 2 决策整理测试目录；避免生产源码和测试文件被机械地整体搬迁。
- 同步更新 Dockerfile、GoReleaser、RPM spec、Windows 资源、CI 工作目录和文档中的路径。

#### 阶段 5：测试与验证

- `go list ./...`：确认 package 路径变化完整且无旧路径残留。
- `go test ./...` 和所有目标平台的 `go build`。
- 实际构建 Linux/RPM、Windows 主程序和 monitor 发布物，检查包内配置、静态资源和启动工作目录。
- 在临时安装目录运行 `check`、`start`、`status`、`reload`、`stop` 冒烟测试。

#### 阶段 5：验收条件

- 新目录在职责上比原目录更清晰，而不只是路径不同。
- CI 和所有发行方式使用新路径并通过。
- 用户可见命令、配置和安装包结构没有未说明的破坏性变化。

## 6. 架构约束的自动验证

当阶段 3 开始时，新增至少一项可重复运行的依赖边界验证：

- 短期可使用 `go list -f` 导出 import graph，并在 CI 检查 `modules/core` 不依赖列入禁用清单的 feature package。
- 依赖名单应集中维护，允许项必须有说明，避免在脚本里散落大量字符串。
- 若规则扩展到多个边界，再考虑小型 Go 架构测试或静态分析工具；不为单条规则引入大型依赖。
- 保持 `go test ./...` 为全仓基础门禁；架构规则失败应指出违规 package 和 import 路径。

## 7. 阶段门禁与回退策略

每个阶段合并前都必须满足：

1. 先运行受影响 package 的测试，再运行 `go test ./...`。
2. 结构变化同步更新 README/架构说明和构建路径。
3. 协议与配置不变时，运行代理 E2E；涉及重载时运行 reload E2E。
4. Windows 与 Linux 相关文件有各自的编译或 CI 验证，不以单平台本地通过代替。
5. 阶段 3 之后，架构依赖检查必须在 CI 执行。

回退时按阶段 revert，而不是跨多个阶段一次性恢复。每阶段尽量通过 adapter 保持旧接口可用；若新装配路径影响既有配置或发行包，应先恢复旧组合入口，再保留可独立测试的新接口，不要留下两套同时生效的 handler 注册机制。

## 8. 推荐执行顺序和优先级

| 顺序 | 阶段 | 主要收益 | 优先级 |
| --- | --- | --- | --- |
| 1 | 阶段 0：地图与验证基线 | 后续决策有证据，低风险 | P0（前置） |
| 2 | 阶段 1：标注目录语义 | 降低误读成本，不改行为 | 已完成（2026-10-14） |
| 3 | 阶段 2：整理测试布局 | 为重构提供稳定回归网 | P1 |
| 4 | 阶段 3：统一 handler 装配 | 修正最明显的依赖倒置和注册分裂 | P1 |
| 5 | 阶段 4：配置快照与依赖注入 | 降低全局状态耦合，重构风险较高 | P2 |
| 6 | 阶段 5：按稳定边界搬迁目录 | 统一入口和发布布局，收益依赖前序 | P3 |

## 9. 本计划不包含

- 增加 upstream 负载均衡、HTTP/3、FastCGI 等新协议或产品功能。
- 重写 HTTP/1.1、HTTP/2、HPACK 或连接池实现。
- 因目录偏好而一次性将所有包移入 `internal/`。
- 未确认外部使用和产品状态就删除通用模块注册表、engine 或 workchain。
- 改变现有 JSON 配置、安装包运行目录或 daemon 行为作为“顺便整理”。

## 10. 完成定义

架构收敛完成不以目录数量或重命名数量衡量，而以以下结果衡量：

- package 职责和生产启动路径可被文档准确描述。
- core 与具体功能模块的依赖方向清晰，模块装配只有一个权威入口。
- 配置加载、校验、运行时使用和 reload 的边界清楚，状态可测试。
- 单元测试、集成测试和实验代码各有稳定归属。
- CI 能验证行为回归与关键依赖规则；发布构建和现有部署方式保持兼容。
