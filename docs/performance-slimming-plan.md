# Fast-Https 精简与性能优化计划（Web 服务器方向）

修订日期：2026-10-24。

本文是 [architecture-convergence-plan.md](architecture-convergence-plan.md) 的**补充计划**，不重复其已完成的结构收敛工作，聚焦三件事：**技术栈/死代码精简**、**请求热路径性能**、**稳定性缺陷**。本文只做规划，不含代码改动；每项单独评审、提交和验证，前一项未达验收不进入下一项。

不做 2.0 智能体（见 [agent-refactor-plan.md](agent-refactor-plan.md)），不新增协议或产品能力，不重写 HTTP/1.1、HTTP/2、HPACK 与连接池实现。

## 1. 现状基线

**构建/静态检查（2026-10-24 实测）**

- `go build ./...` 通过。
- `go vet ./...` 通过。

**结构收敛现状**（见 architecture-convergence-plan.md）

- 阶段 0–3、4a 已完成：请求 handler 只有 `core.RegisterHandler` + `cmd.RegisterHandlers` 一条装配路径；`core` 经 `core.Services` 调用功能包、不再直接依赖；遗留 `Module` 注册表及其 `logging`/`workchain` 已删除；重载一致性已修。
- 仅剩：**4b 完整配置快照**（按需）、**阶段 5 目录/入口搬迁**（按需），以及 3 处功能包间横向耦合（见第 5 节）。
- 结论：本计划**不重做结构收敛**，只在其上补精简与性能。

**当前请求热路径**（供优化定位）

`server.serveListener → serveConn（goroutine-per-conn）→ events.HandleEvent / H2HandleEvent → EventHandler → core.Handler(type) → static / proxy / rewrite / dev_mod`；访问日志经 `dynlog.Log` → `infra/logging`。

## 2. 目标与范围

**目标**

- 删除无用依赖与死代码，降低构建体积与维护面。
- 降低请求热路径的分配、锁竞争与无效计算。
- 修掉会打死进程的稳定性缺陷。
- 保持配置 JSON、CLI 行为、部署目录不变。

**范围（只做 Web 服务器）**：静态站点、反向代理、TLS/H2、gzip、代理缓存、限流/黑名单/防火墙、访问日志。

**不在范围**：2.0 agent；HTTP/3、FastCGI、upstream 负载均衡等新功能；重写协议实现；为目录偏好一次性搬迁全部包。

## 3. 技术栈精简

### 3.1 依赖处置

| 依赖 | 处置 | 依据 |
| --- | --- | --- |
| `github.com/Jxck/assertion` | 保留 | 经 `go mod why` 核实为 `runtime/h2/hpack` 的传递依赖，非死依赖 |
| `github.com/panjf2000/ants/v2` | 删除（连 `core/routine_pool`） | 仅死包，`server.go` 导入为注释 |
| `github.com/andybalholm/brotli` | 删除（连 `modules/compress` 的 br 函数） | 生产响应只用 gzip，br 仅测试引用 |
| `github.com/chenhg5/collection` | 替换为 stdlib | 热路径逐请求线性 `Contains`，改用 `slices.Contains` / `map` |
| `github.com/fufuok/chanx` | 评估替换为固定 worker + buffered chan | 现每条日志起一个 goroutine |
| `github.com/Jxck/logger` | 统一到 `infra/logging` | 与项目日志两套级别，无法统一关闭 |
| `github.com/spf13/viper` | 最后评估换 `encoding/json` | 拖入 yaml/toml/properties/afero 等大量间接依赖，风险高 |
| `cobra`、`kardianos/service`、`logrus`、`fatih/color`、`bluemonday`、`gods`、`x/sys`、`x/time`、`Jxck/hpack` | 保留 | 功能需要 |
| `getlantern/systray` | 保留但隔离 | 只在独立 `monitor` 二进制，不进服务器 |

> 预计可移除 3 个直接依赖（ants、brotli、collection）与若干间接依赖；`assertion` 经核实为 hpack 传递依赖，保留。

### 3.2 死代码/死包

生产零引用，删除前先跑测试确认：

- 包：`core/engine`、`core/routine_pool`、`core/timer`（仅 `Event.Timer` 字段类型引用）、`hpack/huffman`、`hpack/integer_representation`、`utils/security`、`modules/proxy/upstream.go`（空文件）。
- 函数：`infra/timeutil/timeutil.go` 的 blog 函数族（仅 `GetWaitGroup` 在用）；未使用导出函数 `EventWriteEarly`、`RequestHeaderValid`、`TryFixHeader`（空实现）、`response.Test`、`dynlog.NewLogger`、`static.GetGID`、`appfirewall.getReqInfo`、`cmd.watchConfigChanges`。

> 注意：`modules/compress` **不是**死代码——`modules/static` 的 gzip 路径在用；只删其 br/gzip 解压中未被生产调用的函数与 `brotli` 依赖。
>
> 执行状态（2026-10-24）：上述死包、死函数及仅测试使用的 RSA 包已删除；`ants`、`brotli` 依赖已从依赖图移除（`assertion` 保留）。`go build ./...`、`go vet ./...`、受影响包与 `tests/unit_test` 测试均通过。

## 4. 性能与稳定性优化

按「收益高、风险低」排序。每项注明证据位置。

### P0 稳定性（零/低风险，先做）

| 问题 | 位置 | 处置 |
| --- | --- | --- |
| 非法 `Content-Length` 直接 `panic`，可被单请求打死进程 | `runtime/request/parse.go:329` | 改为返回错误 |
| `Authorization` 解析无长度检查 | `modules/auth/auth.go` | 加越界校验 |
| 请求协程内 `logger.Fatal`（`os.Exit`） | `modules/proxy/proxy_read.go`、`event_proxy.go` | 改为返回错误并记录 |
| `LogRegister` 只 append 不重置，reload 后访问日志字段成倍重复 | `runtime/dynlog/dynlog.go:20` | 每次重建切片 |
| `LisType == PROXY_TCP(3)` 与实际写入的 `2` 不一致，tcp 分支永不进入 | `cmd/handlers.go`、`runtime/listener/listener.go` | 统一取值 |
| `ssl`（非 h2）也把 `h2` 放进 `NextProtos` | `runtime/listener/listener.go` | 只在 `LisType==10` 启用 |
| `TryFixHeader` 为空实现，>4096 的 header 无法拼装 | `runtime/request/parse.go` | 实现或显式拒绝并返回明确错误 |

### P1 日志降本（全在热路径）

- 已完成：`infra/console` 连同 `runtime.Caller` 取路径一起删除，控制台改由 zerolog 直写标准输出。
- `runtime/h2/conn/conn.go`：每帧构造 `frame.String()`/`Indent` 调试串 → 用日志级别短路。
- `infra/logging`：每条日志一个 goroutine → 固定 worker 池；`dynlog` 的字符串累加改 `strings.Builder`。
- `runtime/events/events.go`：热路径 `color.RedString(...)` 参数先求值 → 按级别短路。

### P2 HTTP/1 热路径

- 响应生成 `response.go`：`+= fmt.Sprintf` → `bytes.Buffer` + `strconv.AppendInt`；去掉仅为测试的 `sortedHeaderKeys`。
- 请求解析 `parse.go`：整块 `string()` + `strings.Split` → `bytes.IndexByte`；`ByteRow` 预分配并稳定 header 顺序。
- 读缓冲每次 `make([]byte,4096)` → `sync.Pool`。
- `static.getContentType` 每请求 `strings.Split` → 预建扩展名表。
- location 匹配：精确前缀优先，正则仅兜底。

### P3 HTTP/2 写路径

- 窗口为 0 时 `continue` 忙等自旋 → 改为等待/让出。
- 每帧 `make+copy` 与 `copy(data, data[frameSize:])` 的 O(n²) → 索引切片。
- stream 清理用 `time.After` goroutine → 延迟队列/复用。
- `StreamsLock` 每帧两次加解锁 → 合并或读写锁优化。

### P4 代理 I/O

- 每请求 `net.Dial` → 引入上游 keep-alive 连接池（代理最大收益）；超时可配置。
- `ChangeHeader`/`ReadBytes` 多次全量 `string()` 拷贝 → 减少拷贝。
- 去掉热路径 `fmt.Println`；`InsecureSkipVerify` 改为可配置。

### P5 并发与全局配置

- 每 accept 加 `s.mu` → 原子 `stopping` 标志。
- `safe` 计数每请求写锁 → 分片/原子计数。
- 配置读取改为快照发布（收敛计划 4b 的 `atomic.Pointer`），消除 reload 竞态。

## 5. 剩余结构收敛（并入本计划，按需）

- `modules/websocket → modules/proxy`：抽出共享的上游拨号/转发接口。
- `runtime/request → runtime/listener`：只传 Host/Port 最小接口，不传整个 `*listener.Listener`。
- `modules/static`、`modules/proxy → modules/appfirewall`：改为 `core.Services` 钩子注入，与 filters 一致。
- 4b 配置快照：P5 需要时一并做。
- 阶段 5 目录/入口搬迁：仅当发行或多入口需要时再做（入口被 Docker/GoReleaser/RPM 写死，收益低）。

依赖守卫测试 `core/deps_test.go` 继续作为约束，可扩展禁止功能包之间互相 import。

## 6. 分期里程碑与验收

| 里程碑 | 内容 | 验收 |
| --- | --- | --- |
| M0 基线 | 建立 benchmark（Header 解析、响应生成、gzip、ByteRow）+ `hey`/`wrk` 静态与代理基线 + pprof | 有可对比的 QPS / P99 / allocs 基线 |
| M1 稳定性 | P0 全部 | `go test ./...`、代理 E2E 通过；异常请求不再打死进程 |
| M2 精简 | 第 3 节依赖与死代码 | `go build ./...`、`go vet ./...`、测试通过；`go mod tidy` 干净 |
| M3 热路径 | P1 + P2 | benchmark 显示分配与 P99 下降；日志可整级关闭 |
| M4 H2/代理 | P3 + P4 | H2 压测无自旋；代理连接复用生效 |
| M5 结构 | 第 5 节耦合 + 4b | 依赖守卫测试通过；无行为变化 |

**通用验收**：`go build ./...`、`go vet ./...`、`go test -p 1 ./...`（Windows）、代理 E2E 全绿；配置 JSON / CLI / 部署目录零破坏。

## 7. 度量方法

- 微基准：`go test -bench`（Header 解析、响应生成、gzip、ByteRow）。
- 端到端：`hey` / `wrk` 压静态与代理，记录 QPS、P50/P99、allocs/op、GC。
- 剖析：`dev` 模式已暴露 `:10000/debug/pprof`（见 `config/consts.go` 注释），用 cpu/mem profile 定位。

## 8. 原则与风险

- 先测后改：M0 基线完成前不做优化，避免无法归因。
- 每阶段独立可回退；先跑受影响 package 测试，再 `go test -p 1 ./...`。
- 不动配置字段、部署契约、CLI 行为。
- H2/代理改动需 E2E + 手工压测；并发改动用 `-race`（Windows 需 cgo/gcc）。
- 高风险项（viper 替换、目录搬迁）放最后且按需。

## 9. 推荐执行顺序

1. **M0** 建立基线与 benchmark。
2. **M1** 稳定性修复（收益最高、风险最低）。
3. **M2** 依赖/死代码精简。
4. **M3** 日志与 HTTP/1 热路径。
5. **M4** H2 与代理 I/O。
6. **M5** 剩余结构收敛与 4b（按需）。
