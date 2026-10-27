# Fast-Https 开发迭代计划

版本：2026-10-13。原先的加固计划已并入本文，不再单独维护。旧编号 P0–P3 与本文 R1–R8 的对应关系见第 3 节。

## 1. 目标

后面几次迭代把服务变成可以重复发布的状态：配置重载失败时旧服务还在，测试能在持续集成里挡住回归，连接关闭和 HTTP/2 错误有明确行为。

## 2. 已经具备的基线

这些能力已经在代码和测试里，本计划不再排期：

- 启动前校验 `config/fast-https.json` 和 `http.include`
- 缺处理函数的 `location.type` 不会进入监听。`start` 和 `dev` 由 `cmd.RegisterHandlers` 登记 static、proxy、rewrite、devmod（2026-10-16，架构收敛阶段 3 第 2 步）
- 日志默认写到 `logs/`，控制台 DEBUG/TRACE 走 stderr
- 黑名单、限流、`xss` 有拒绝和放行测试；`sql` 对 query 与 body 做保守注入特征检测（2026-10-25）
- 无效配置重载不会替换内存中的 `GConfig`，也不会继续切换监听
- HTTP/2 连接可以发 GOAWAY 并关闭，common 端口可以热更新配置
- `status` 读取 `fast-https.pid`，区分没有 pid 文件、pid 已过期、进程仍在运行
- 配置重载成功后按当前 `log_root` 重新打开四份日志；新目录打不开时继续写原来的文件
- Gitee Go 流水线文件是 `.workflow/go-test.yml`，执行 `go test ./...`。开通 Gitee Go 后才会在推送和合并请求上运行
- `SIGINT` 在守护进程模式下重载配置；`dev` 前台模式优雅停机（2026-10-25）。停止走 `stop`、`SIGTERM` 或 `SIGQUIT`
- 未知 HTTP/2 帧会被丢掉并继续读；非法 HPACK 索引关闭该流；对端窗口不够时停止写出 DATA 并关闭连接
- `xss` 会清洗 JSON 对象和 JSON 数组里的 script 标签
- 运行时只读 `config/fast-https.json` 和 `http.include`。`config/dev.yaml` 不加载
- 空日志目录、`.`、`./` 落到 `./logs`。DEBUG、TRACE 走 stderr，其余级别走 stdout
- `ServerInit` 在监听前调用 `core.MissingHandlers`。已配置的 location 类型没有处理函数时直接退出，不会 `Accept`
- `check` 只校验配置，不监听、不写 pid。`HEAD` 不写正文。`Transfer-Encoding: chunked` 按块读取。静态 gzip/brotli 按客户端 Accept-Encoding 协商压缩正文，两种都配置时优先 br，配置了压缩的响应带 Vary: Accept-Encoding。`error_page.not_found` 和 `error_page.proxy` 可选，文件不可读时仍用默认响应
- `SIGTERM`、`SIGQUIT` 停止接受并最多等待 5 秒。`stop` 仍发送 `Kill`
- 重载时新配置和 MIME 表先在局部建好，再一次性替换 `GConfig`、`GContentTypeMap`，处理中的请求不会读到清空的配置或写入中的 MIME 表。限流计数表整体替换，按 location `ID` 定位；`ID` 不在新表中时这次请求不计数（2026-10-16，架构收敛阶段 4a）
- 配置、MIME 表、监听器路由表、访问日志字段表、四路文件日志器全部改为 atomic 快照：reload 校验通过后整体替换，读取端要么看到完整的旧状态、要么看到完整的新状态，与 reload 协程之间没有数据竞争。读取端经 `config.Current()`/`config.MIME()`/`Listener.Hosts()`/`Listener.Cfgs()` 访问，直接读全局字段的做法已移除（2026-10-24，架构收敛阶段 4b 快照化部分；`-race` 下验证）

## 3. 分级

- P0：影响发布或线上恢复
- P1：能运行，但故障时难查、难收口
- P2：协议和维护成本
- P3：体验和后续能力

难易度：易为 0.5–2 天，中为 2–5 天，难为 1–3 周。

旧编号并入关系：

| 旧编号 | 本文 | 状态 |
| --- | --- | --- |
| P0-1 启动前配置校验 | 基线 | 已关闭 |
| P0-2 测试基线与 CI | R2 | 流水线文件已落地，Gitee 上还没跑过 |
| P0-3 Reload 跨平台与失败恢复 | R1 | 已关闭 |
| P1-1 监听与连接生命周期 | R4 | 关闭路径已落地。连接数仍未做 |
| P1-2 模块注册与启动期检测 | 基线 | 已关闭 |
| P1-3 配置真源与说明文档 | 基线 | 已关闭 |
| P2-1 两套日志管道 | 基线 | 已关闭 |
| P2-2 HTTP/2 与 HPACK 边界 | R5 | 已关闭 |
| P2-3 安全与限流验证 | 基线 | 已关闭（sql 检测于 2026-10-25 落地） |
| P3-1 配置文件自动监听 | R7 | 暂缓 |
| P3-2 代理 upstream | R8 | 未做 |

2026-06-23 的任务卡已在 2026-10-08 按代码复核，结果就是上表。不再单列过程记录。

## 4. 本版要做的事

| 编号 | 问题 | 优先级 | 难易度 | 顺序 | 状态 |
| --- | --- | --- | --- | --- | --- |
| R1 | 重载信号语义与端到端测试 | P0 | 中 | 1 | 已落地 |
| R2 | 持续集成门禁 | P0 | 易 | 2 | 已落地：`.workflow/go-test.yml`，需在 Gitee 开通 Gitee Go 后才会执行 |
| R3 | 重载后日志文件重新打开 | P1 | 易 | 3 | 已落地 |
| R4 | 连接关闭与请求失败路径 | P1 | 难 | 4 | 已落地 |
| R5 | HTTP/2 与 HPACK 错误分支 | P2 | 难 | 5 | 已落地 |
| R6 | `status` 命令给出进程状态 | P3 | 易 | 6 | 已落地 |
| R7 | 配置文件变更后自动重载 | P3 | 中 | 7 | 暂缓：继续手工重载 |
| R8 | 代理上游故障转移 | P3 | 中 | 8 | 未做 |
| R9 | GuardianWAF 接入应用防火墙 | P1 | 中 | 9 | 已落地（2026-10-25，v1.0.0；灰度见 guardianwaf-adoption.md） |

## 5. 工作项

### R1 重载信号语义与端到端测试

已落地。`SIGINT` 在各平台都调用 `Reload()`。`SIGTERM` 和 `SIGQUIT` 停止接受新连接，最多等待 5 秒后退出。`stop` 仍然发送 `Kill`。Windows 的 `reload` 先发 Ctrl+Break，再试 Ctrl+C，不再把进程停掉。前台 Ctrl+C 会重载，不会退出。2026-10-25 起例外：`dev` 前台模式 SIGINT 触发优雅停机。

`runtime/server/signal_test.go` 锁定这三种信号。`FASTHTTPS_E2E_RELOAD=1` 时，`TestReloadPortSwitchE2E` 在当前系统上切换端口；默认 `go test` 不跑它。同一 SSL 端口重载时不重新绑定套接字，能解析的新证书在下一次握手生效；证书文件缺失时监听保持打开。

### R2 持续集成门禁

已落地文件 `.workflow/go-test.yml`。推送到 `master`、`main` 以及对应合并请求时执行 `go test ./...`。需要环境变量才运行的端到端测试不会进入这条命令。

仓库远程在 Gitee。开通 Gitee Go 之后这份文件才会执行。故意失败是否标红，以及主干是否必须通过后才能合并，还没有在 Gitee 上验证。

### R3 重载后日志文件重新打开

已落地。`Server.Reload` 在 `config.Reload` 成功之后调用 `logging.Open`，关闭旧文件并按当前 `log_root` 打开 `system.log`、`access.log`、`error.log`、`safe.log`。新目录打开失败时保留原来的四个文件，监听仍按新配置切换。

`tests/unit_test/infra/logging/logdir_test.go` 覆盖切换到新目录，以及新目录无法创建时旧文件仍然可写。

### R4 连接关闭与请求失败路径

已落地。

- `Event.Close` 只关闭一次。第二次调用不再关闭连接，也不再写错误或警告日志。
- HTTP/1.1 处理函数退出时都会关闭连接。解析失败、读请求体超时、请求体过大，以及监听上下文取消，都走这一条关闭。取消时会先关掉正在读的连接，处理 goroutine 随之返回。
- 写响应失败时不再复用连接，处理函数返回后关闭。
- 监听上下文取消时关闭该端口的 listener，`Accept` 返回后接受循环退出。重载删掉端口时仍先取消上下文再关监听。
- `safe.Init` 由 `cmd.RegisterServices` 装入 `core.Services.AfterListen`，只在监听器创建之后调用，重载时再调用一次。限流计数每次重建为新表后整体替换，不会在旧切片后面继续追加。未设置该钩子时不做安全初始化。

`TestCloseIsSilentWhenRepeated`、`TestHandleEventExitsWhenContextCanceled`、`TestServeListenerExitsWhenContextCanceled`、`TestCountsInitReplacesSlice` 覆盖上述行为。HTTP/2 连接关闭仍由已有的 `closeOnce` 和 `GracefulClose` 负责。连接数统计还没有做。

### R5 HTTP/2 与 HPACK 错误分支

已落地。

- 类型大于 `0x9` 的帧丢掉载荷，读循环继续读下一帧。
- HPACK 索引为 0，或动态表下标越界时，`Context.Decode` 返回错误。对应流发出 `RST_STREAM`（`COMPRESSION_ERROR`）并关闭，不再进入请求回调。编码索引 0 不产生字节。
- 写出 DATA 时，载荷长度大于对端窗口则停止写循环并关闭连接；写成功后扣减对端窗口。
- Huffman `TestQuickCheckEncodeDecode` 已通过，去掉了 `fixme` 标记。
- `xss` 会去掉 JSON 数组（对象或字符串）里的 `<script`。数组元素不是对象或字符串时返回错误，不再因类型断言中断进程。

### R6 `status` 命令

已落地。`status` 读取 `fast-https.pid`。进程在运行时打印 `fast-https is running, pid <pid>` 并以 0 退出。没有 pid 文件时提示 `no pid file`，pid 已不对应进程时提示 `stale pid`，这两种情况以非 0 退出。

`cmd/status_test.go` 覆盖这三种情况，不启动服务器。

### R7 配置文件变更后自动重载

2026-10-09 决定暂缓，不改代码。配置变更继续靠 `reload` 子命令或 `SIGINT`。`watchConfigChanges` 仍是没有调用方的休眠循环。

自动重载只解决“改完文件忘记执行重载”。Web 服务器的常规路径是手工重载：文件写完后，由管理员或发布脚本显式触发。编辑器连续保存、半截 JSON 会把端口、证书和黑名单立刻换掉，删掉的端口还会关掉正在处理的连接。

以后若要做，单独加开关，默认 `start` 不监视文件。无效配置仍保留旧配置和旧监听。

### R8 代理上游故障转移

现状：反向代理每个 location 只有一个 `ProxyAddr`。注释里预留了 upstream，还没有健康检查、轮询或失败后换地址。

风险：上游进程退出后，这个 location 的请求会一直失败。

做法：

- 保持单个 `proxy_pass` 的现有 JSON 不变
- 增加可选的上游地址列表。拨号失败时按顺序试下一个地址
- 健康检查和权重放到列表可用之后再做

验收：

- 只有 `proxy_pass` 时，请求仍发到原来的地址
- 配置两个上游且第一个拒绝连接时，请求到达第二个

## 6. 建议节奏

2026-10-09 已落地：R1 重载信号、R2 流水线文件、R3 重开日志、R4 连接关闭、R5 HTTP/2 与 HPACK 错误分支、R6 `status`。R2 还要在 Gitee 开通 Gitee Go 后确认流水线会执行。

R7 暂缓，默认保持手工重载。计划内还没做的是 R8 多上游。和 Nginx 核心能力的差距见 [nginx-core-gap.md](../../nginx-core-gap.md)。其中的配置检查、优雅停止、`HEAD`、分块请求体、gzip 正文和可选错误页已在 2026-10-13 落地，不改变本表的状态。

## 7. 完成标准

已经满足：

- 本地测试覆盖 `status` 的三种进程状态，重载后日志切目录，重复关闭、取消后连接和接受循环退出，未知帧、非法 HPACK 索引、对端窗口耗尽、JSON 数组 XSS，以及各平台 `SIGINT` 都是重载
- 无效配置重载后，内存中的旧配置保留，监听不切换

仍待：

- Gitee 上默认 `go test ./...` 实际跑通，失败时能看到包名和输出

## 8. 执行约定

- 每项合并时带上能重复运行的测试
- 现有 `config/fast-https.json` 不改字段名也能继续启动
- 行为变化同时改 `README.md` 和本文件的状态表
