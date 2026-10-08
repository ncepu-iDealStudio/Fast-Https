# Fast-https 开发迭代加固计划（技术债分级）

现行排期见 [iteration-plan.md](iteration-plan.md)。本文保留到 2026-10-08 为止的问题复核和已关闭记录。

修订日期：2026-10-08。2026-10-08 已落地 P1-2、P2-1、P2-3，以及无效配置重载时保留旧配置。

## 1. 文档目标

本计划用于指导后续迭代，优先处理会影响稳定性、可运维性、可测试性的技术债。

每条记录包含：

- 当前代码里的实际表现
- 不继续处理时的风险
- 治理动作与验收标准

## 2. 分级标准

- P0（阻断级）: 影响核心稳定性、发布可靠性或线上可恢复能力
- P1（高优先级）: 当前可运行，但有明显事故隐患或持续拖慢开发效率
- P2（中优先级）: 短期不阻断运行，会放大后续维护成本
- P3（优化级）: 体验、性能或工程一致性改进，可放在专项窗口

难易度：

- 易: 改动点集中，0.5–2 天可完成并验证
- 中: 跨 2–4 个模块，通常 2–5 天
- 难: 涉及协议或跨平台行为，通常 1–3 周

状态：

- 已关闭: 原问题在当前代码中已不成立
- 部分完成: 主路径已有实现或测试，验收标准仍有缺口
- 仍开放: 当前代码仍是问题描述中的状态

## 3. 当前状态一览

| 编号 | 问题 | 优先级 | 状态 | 难易度 | 建议顺序 |
| --- | --- | --- | --- | --- | --- |
| P0-1 | 启动前配置校验 | P0 | 已关闭 | — | — |
| P0-2 | 测试基线与 CI | P0 | 部分完成 | 中 | 2 |
| P0-3 | Reload 跨平台与失败恢复 | P0 | 部分完成 | 难 | 1 |
| P1-1 | 监听与连接生命周期 | P1 | 仍开放 | 难 | 3 |
| P1-2 | 模块注册与启动期检测 | P1 | 已关闭 | — | — |
| P1-3 | 配置真源与说明文档 | P1 | 已关闭 | — | — |
| P2-1 | 两套日志管道 | P2 | 已关闭 | — | — |
| P2-2 | HTTP/2 与 HPACK 边界 | P2 | 仍开放 | 难 | 4 |
| P2-3 | 安全与限流验证 | P2 | 已关闭 | — | — |
| P3-1 | 配置文件自动监听 | P3 | 仍开放 | 中 | 5 |
| P3-2 | 代理 upstream | P3 | 仍开放 | 中 | 6 |

建议顺序只针对未关闭项。下一批是 reload 失败恢复，以及把 `go test ./...` 接到 CI。

## 4. 已关闭

### P0-1 启动前配置校验

`config.CheckConfig` 调用 `ValidateConfigFile`。启动预检会检查 `http`、监听端口、`server_name`、带 `ssl` 时的证书文件、`location.type`（`local` / `proxy` / `rewrite` / `devmod`）、`proxy_pass`，以及 `http.include` 指向的 JSON。`test/unit_test/config/check_config_test.go` 覆盖坏端口、缺证书、缺 `proxy_pass` 和非法 include。

### P1-3 配置真源与说明文档

运行时只读取 `config/fast-https.json`，再按 `http.include` 加载其它 JSON。`README.md`、`README.en.md` 和 `config/dev.yaml` 都写明 `dev.yaml` 不会被加载。

### P1-2 模块注册与启动期检测

`server.ServerInit` 在监听前调用 `core.MissingHandlers`。已配置的 `location` 类型没有处理函数时，进程退出，不会 `Accept`。TCP 代理由监听过滤器处理，不要求请求处理函数。`test/unit_test/modules/core/handler_ready_test.go` 只调用 `RRHandlerRegister`，不导入 `modules/core/server`。

处理函数仍由 `server.go` 的空白导入完成注册。缺的是某一种 `location.type` 的处理函数，启动会被拦住。

### P2-1 两套日志管道

`utils/message.ResolveLogDir` 在路径为空、`.` 或 `./` 时使用 `./logs`，并创建目录。文件日志是 `logs/system.log`、`logs/access.log`、`logs/error.log`、`logs/safe.log`。`utils/logger` 的 DEBUG、TRACE 走 stderr，其余级别走 stdout，不写入这四个文件。对照说明写在 `utils/message/logger.go`。

`test/fast-https_test.go` 先 `config.Init()`，再按 `LogRoot` 打开日志。`test/unit_test/utils/message/logdir_test.go` 和 `test/unit_test/utils/logger/logger_test.go` 锁定目录和输出流。

### P2-3 安全与限流验证

`modules/safe/policy_test.go` 断言黑名单命中、范围外地址放行，以及限流突发用尽后拒绝。`xss` 在 JSON 正文里的 `<script` 被去掉后返回 false，静态和代理路径据此回 403；普通 JSON 放行。`sql` 保持放行，启动时打出「does not intercept」。`README.md` 与 `README.en.md` 按这个行为描述，不再把 `sql` 写成已经生效的拦截规则。

## 5. 部分完成

### P0-2 测试基线与 CI

已落地：

- `modules/core/h2/hpack` 不再在库初始化里调用 `flag.Parse`
- 请求解析、响应、配置校验、HTTP/2 关闭、common 端口热更新已有单测

仍缺：

- 仓库中没有把 `go test ./...` 固化下来的 CI 配置

验收：

- 标准开发环境执行 `go test ./...` 可作为合并门禁
- CI 保存失败包名和日志

### P0-3 Reload 跨平台与失败恢复

已落地：

- `reload` 命令在 Windows 上发送控制台 Ctrl+C，在其它平台发送 `SIGINT`（`cmd/reload_windows.go`、`cmd/reload_other.go`）
- 移除端口时取消 context 并关闭旧监听
- HTTP/2 `GracefulClose`：发送 GOAWAY，等待在途 stream，再关闭连接
- common 端口复用原 context，并热更新 `Cfg` / `HostMap`
- 单测覆盖连接关闭幂等、GracefulClose、common 端口配置更新
- `config.Reload` 先校验配置。无效文件不替换 `GConfig`，`server.Reload` 因此不会改动已有监听。`config/reload_test.go` 覆盖拒绝无效文件和接受下一份有效文件
- `ReloadHandler` 里过期的 Windows 注释已去掉，信号仍由 `signalReloadProcess` 发送

仍缺：

- 进程内 `SIGINT` 的含义按操作系统分开。Windows 上停止服务，其它平台上重载配置
- `test/client_test/reload_e2e_test.go` 默认跳过，且只在 `FASTHTTPS_E2E_RELOAD=1` 的 Windows 上运行
- 证书更新没有单独的回归测试

验收：

- 明确并测试「Windows 停止 / 其它平台重载」这条信号语义，或把两边的 `SIGINT` 收成同一种行为
- 非 Windows 上也有可启用的 reload 端到端测试

## 6. 仍开放

### P1-1 监听与连接生命周期

当前表现：

- `modules/core/events/events.go`、`modules/core/server/server.go`、`modules/core/h2/conn/conn.go` 仍有关闭、窗口和解析失败分支的 TODO
- 连接超时、重载切换、重复关闭没有统一的状态记录

风险：

- 长连接或频繁重载时可能出现悬挂连接、重复关闭或 goroutine 堆积
- 故障时缺少连接数和文件描述符依据

动作：

- 写清 listener、event、HTTP/2 conn 在接受、处理、重载、退出时的状态
- 为超时、关闭和重载使用同一条退出路径
- 在监听循环记录当前连接数

验收：

- 重载和关停后，goroutine 与连接数回到基线
- 重复关闭不会产生新的错误日志

### P2-2 HTTP/2 与 HPACK 边界

当前表现：

- 编解码、帧处理和连接窗口仍有 Decoding Error、忽略帧、窗口检查等 TODO
- `modules/core/h2/hpack/huffman/huffman_test.go` 留有未修复标记
- reload 和 GracefulClose 已有单测，协议错误分支覆盖仍少

风险：

- 畸形帧或窗口耗尽时的行为不可预期
- 后续协议修改变更成本高

动作：

- 为解码错误、非法帧、窗口耗尽补上期望行为
- 每条行为对应一个不依赖外网的测试

验收：

- 上述分支失败时返回明确错误或关闭连接，并有测试锁定该行为

### P3-1 配置文件自动监听

当前表现：

- `cmd/commands.go` 的 `watchConfigChanges` 只有休眠循环，注释写明以后再做
- 配置变更依赖 `reload` 子命令；非 Windows 上也可以向进程发送 `SIGINT`

风险：

- 改完文件若忘记重载，进程继续使用旧配置

动作：

- 若要做自动重载，监听 `config/fast-https.json` 和 include 目录，防抖后走现有 reload 管线
- 坏配置沿用 P0-3 的失败保留旧配置

验收：

- 保存有效配置后进程切到新监听
- 保存无效配置后旧监听保持可用，并留下错误日志

### P3-2 代理 upstream

当前表现：

- `modules/proxy/event_proxy.go` 把上游地址放在单个 `ProxyAddr` 上，并注明增加 upstream 时要改解析

风险：

- 单个上游故障时，该 location 没有备用地址

动作：

- 在现有 `proxy_pass` 之外增加可选 upstream 列表
- 先支持轮询和失败后尝试下一个地址，再考虑健康检查

验收：

- 只配置 `proxy_pass` 的现有 JSON 行为不变
- 配置多个上游时，第一个失败后请求落到下一个地址

## 7. 后续迭代建议

第一批：

- P0-3：非 Windows 的 reload 端到端测试，以及 Windows 与其它平台的 `SIGINT` 语义
- P0-2：把 `go test ./...` 接到 CI

第二批：

- P1-1：连接关闭和重载的退出路径
- P2-2：HTTP/2 与 HPACK 的错误分支测试

P3-1、P3-2 放在上述事项之后，单独开设计说明再实现。

## 8. 里程碑

- 错配在 `start` / `dev` 预检阶段失败，错误信息能定位到字段或 include 文件
- 坏配置重载不会拆掉正在服务的监听（配置层已保留旧 `GConfig`，见第 5 节）
- `go test ./...` 在 CI 中通过
- 黑名单、限流和 `xss` 测试包含拒绝与放行（已有单测，见第 4 节）

## 9. 执行原则

- 未关闭项先做能用测试证明的部分
- 每项改动带自动化验证
- 配置和协议行为保持现有 JSON 可继续加载
- 代码及 `README.md` 的行为变化，同一轮写回本计划的状态表

## 10. 2026-06-23 已记录的交付

以下内容来自当时的任务卡，状态按 2026-10-08 的代码复核后写入第 3 节。

- 配置校验器：`CheckConfig`、include 校验和坏配置单测
- 测试修复：去掉 hpack 库初始化里的 `flag.Parse`，关键包单测可编译运行
- Reload 第一至四阶段：信号抽象、移除端口、HTTP/2 GracefulClose、common 端口热更新，以及对应单测
- 模块注册单测：重复注册拒绝、workchain 示例可解析
- 说明文档：JSON 为配置真源，`dev.yaml` 仅作示例
- 控制台日志单测：DEBUG 及以上走 stderr

当时任务卡里尚未达到、并且仍开放的目标：

- Linux 与 Windows 的 reload 端到端结果一致，且坏配置可以回滚
- CI 把全量测试设为门禁

## 11. 2026-10-08 已落地

- P1-2：`core.MissingHandlers` 在 `ServerInit` 监听前检查 location 类型；单测不导入 server 包
- P2-1：空日志目录落到 `./logs`；`test/fast-https_test.go` 先加载配置再打开日志
- P2-3：黑名单与限流有命中/放行断言；`xss` 去掉 `<script` 后返回 403；`sql` 明确为不拦截
- P0-3 中可单测的一部分：无效配置重载保留上一份 `GConfig`，服务器不继续切换监听
