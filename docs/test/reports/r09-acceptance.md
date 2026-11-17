# R-09 reload 的原子性与可见性 —— 验收报告

- 日期：2026-11-17
- 范围：`docs/develop/plans/edge-case-hardening-plan.md` 的 R-09，以及同一条 reload 链路上的在途请求可观测性与停机协调
- 结论：**通过**。需求与两条验收项全部落地；真实服务 E2E 在 Windows 上受平台限制无法执行，已记录待 Linux/CI 补跑。

## 1. 验收依据

计划中 R-09 的需求与验收：

1. reload 失败时旧配置继续生效，错误在 CLI 与桌面端可见
2. reload 期间的在途请求：处理完再切换（优雅），或立即中断并在日志标注
3. 并发 reload 串行化，后到的等待或直接返回「正在重载」
4. 验收：构造「新配置非法」的场景，断言旧配置仍在服务
5. 验收：`-race` 下并发触发 10 次 reload，无 panic、无状态错乱

## 2. 逐条验收结果

| 编号 | 验收项 | 结果 | 关键证据 |
| --- | --- | --- | --- |
| R09-1 | 失败保留旧配置，错误可见 | ✅ | `TestReloadInvalidKeepsPreviousConfig`、`TestReloadFileKeepsPreviousConfigWhenInvalid`、`TestReloadFileRejectsMissingCertificate`、`TestReloadMissingCertKeepsListener`、`TestReloadFailureState` |
| R09-2 | 在途请求语义 | ✅ | common 端口：`TestInFlightRequestKeepsCapturedCfgAcrossReload`；移除端口：`TestReloadLogsInFlightRequestsOnRemovedPort`；计数：`TestInFlightRequestsTrackedDuringHandler` |
| R09-3 | 并发 reload 串行化 | ✅ | `TestConcurrentReloadSerialized`（loader）、`TestConcurrentReloadsAreSerialized`（server） |
| R09-4 | 非法配置下旧配置继续服务 | ✅ | `TestReloadInvalidKeepsPreviousConfig` + `TestReloadMissingCertKeepsListener`（旧 socket 仍可连接） |
| R09-5 | `-race` 并发 10 次 reload | ✅ | `go test -race ./runtime/server ./config/internal/loader` 通过，见第 4 节 |

### R09-1 失败语义

- `ReloadFile` 的顺序固定为「校验 → 解析 → 发布快照」，任一步失败都不触碰 `Current()`。
- `Server.Reload` 在 `reloadConfig` 返回错误时记日志、`reload_failed_total` 加一、
  `last_reload_ok=false`，不进入监听器重建，旧监听器保持运行。
- CLI 与桌面端通过 `/runtime` 的 `last_reload_ok` / `reload_failed_total` 读取结果。

### R09-2 在途请求语义

| 场景 | 在途请求 | 日志 |
| --- | --- | --- |
| common 端口热更新（类型与 TLS 通告位不变） | 已进入处理的请求继续用旧快照跑完；新请求（含 keep-alive 下一个请求）读新快照 | debug：`port` + `in_flight_requests`，`in-flight requests keep the previous config` |
| 端口被移除 / 监听类型变化 / TLS 重建 | 请求被终止：h1 取消 ctx 直接断连；h2 走 `Shutdown`，最多等 `H2GracefulShutdownPeriod`（5s） | info：`port` + `in_flight_requests`，`in-flight requests are terminated` |

- 快照不可变：reload 只整体替换 `Listener.cur`，不修改旧 `Cfg`/`HostMap` 切片。
- 计数口径：`Listener.BeginRequest`/`EndRequest`，覆盖 `EventHandler` 全链路（含 panic 展开）；
  h1 每个请求、h2 每个流；websocket/tcp 隧道属于长连接，不计入。
- 同一计数进入 `/runtime`：`listeners[].in_flight_requests`。

### R09-3 并发串行化

- `config/internal/loader`：`LoadFile` 与 `ReloadFile` 共用一把包级锁，校验也在锁内，
  后开始的调用一定后发布快照，避免「先开始、后完成」用旧文件覆盖新内容。
- `runtime/server`：`Reload` 整条流水线（配置 → 钩子 → 监听器 → 指标）持 `reloadMu`，
  后到的 reload 等待前一次完成，不会被丢弃。
- `runtime/listener`：监听器表读取统一走 `SnapshotListeners()`；`processListenData`
  一次取配置快照，避免拼出「新路由 + 旧 XFF 策略」。

### R09-4 非法配置验收

`TestReloadInvalidKeepsPreviousConfig` 先加载合法配置，再写入非法文件并调用 `ReloadFile`，
断言 `Current()` 仍是旧配置；`TestReloadMissingCertKeepsListener` 断言 reload 被拒后
原监听 socket 仍可建立连接。

### R09-5 并发 10 次 reload

| 用例 | 断言 |
| --- | --- |
| `TestConcurrentReloadSerialized` | 10 个并发 `ReloadFile` 全部成功，最终快照的 `server_name` 与 `log_root` 来自同一次加载，不合并 |
| `TestConcurrentReloadsAreSerialized` | 10 个并发 `Server.Reload`：`config_generation` 恰好加 10、状态回到 running、监听器表与 `s.Listens` 一致 |

## 3. 超清单补强项

以下不属于 R-09 原始三条，但属于同一条 reload 控制链路，已一并落地：

1. **在途请求数可观测**：除日志外，`/runtime` 的 `listeners[].in_flight_requests`
   提供实时值；端口表副本仍由 reload 整体替换，实时值经注册来源填充，`metrics`
   不反向依赖 `runtime/listener`。
2. **reload 与 graceful stop 协调**：`gracefulStop` 与 `Reload` 共用 `reloadMu`；
   `s.Listens` 的读写统一在 `Server.mu` 下；停止开始后到达的 reload 记
   `reload skipped: server is stopping` 并直接返回，不会把已 drain 的端口重新拉起来。

## 4. 测试执行记录

命中 R-09 的用例全部 PASS：

```text
PASS: TestConcurrentReloadSerialized            (config/internal/loader)
PASS: TestReloadMissingFileKeepsSnapshot        (config/internal/loader)
PASS: TestReloadInvalidKeepsPreviousConfig      (config)
PASS: TestConcurrentReloadsAreSerialized        (runtime/server)
PASS: TestReloadLogsInFlightRequestsOnRemovedPort (runtime/server)
PASS: TestRuntimeSnapshotExposesListenerInFlight  (runtime/server)
PASS: TestReloadSkippedWhileStopping            (runtime/server)
PASS: TestReloadAndGracefulStopDoNotRace        (runtime/server)
PASS: TestReloadSuccessState                    (runtime/server)
PASS: TestReloadFailureState                    (runtime/server)
PASS: TestReloadMissingCertKeepsListener        (runtime/server)
PASS: TestListenersAfterReload                  (runtime/server)
PASS: TestReloadAlignsHooksBeforeListeners      (runtime/server)
PASS: TestReloadSkipsHooksWhenConfigRejected    (runtime/server)
PASS: TestInFlightRequestKeepsCapturedCfgAcrossReload (runtime/listener)
PASS: TestHotUpdateSnapshotIsRaceFree           (runtime/listener)
PASS: TestInFlightRequestsTrackedDuringHandler  (runtime/events)
PASS: TestListenersWithInFlightSource           (runtime/metrics)
PASS: TestReplaceListenersCopies                (runtime/metrics)
```

命令与结果：

- `go test -race ./runtime/metrics ./runtime/server ./runtime/listener ./runtime/events`：通过
- `go test -p 1 ./runtime/... ./management/... ./cmd/... ./tests/unit_test/...`：通过
- `go build ./...`：通过

真实服务 E2E：

- 命令：`$env:FASTHTTPS_E2E_RELOAD='1'; go test ./tests/client_test -run TestReloadPortSwitchE2E -v -count=1`
- 结果：**受平台限制未通过**。服务启动、初始端口访问均正常，`reload` 步骤返回：
  `fast-https reload failed: not supported by windows: 无法定向投递 reload 信号，请用重启代替`。
  Windows 上 `reload` 命令本身明确不支持，E2E 的端口切换断言无法完成。
- 待办：在 Linux/CI 上补跑同一用例并回填本节。

## 5. 已知偏差与限制

1. **E2E 平台限制**：如上，Windows 无法投递 reload 信号；热重载语义目前由包级测试锁定。
2. **在途计数不含隧道**：websocket/tcp 隧道是长连接，不计入 `in_flight_requests`；
   移除端口时它们同样会被终止，但不会出现在该计数里。
3. **移除端口的协议差异**：h1 立即断连；h2 先发 GOAWAY，最多等 5s，超过后强制关闭。
4. **计数是瞬时 gauge**：`in_flight_requests` 表示快照时刻正在处理的请求数，不是累计值。

## 6. 提交记录

| 提交 | 内容 |
| --- | --- |
| `62c8135` | reload 串行化（server + loader），监听器表读取走快照，并发 10 次 reload 用例 |
| `59f755f` | 在途请求计数，移除/热更新日志带 `in_flight_requests` |
| `8a49bf8` | 文档状态更新（R-01 ~ R-09 已完成） |
| `de21d21` | `/runtime` 暴露 `listeners[].in_flight_requests`；reload 与 graceful stop 协调 |

## 7. 关键文件

- `config/internal/loader/convert.go`：`loadMu`、`LoadFile`/`ReloadFile` 加锁内层
- `runtime/server/server.go`：`reloadMu`、`Reload`、`gracefulStop`、`drainConnections`、`publishListeners`
- `runtime/listener/listener.go`：`inflight` 计数、`SnapshotListeners`、`processListenData`
- `runtime/listener/reload.go`：common 端口热更新日志、监听器表快照读取
- `runtime/events/events.go`：`EventHandler` 在途计数
- `runtime/metrics/listeners.go`、`snapshot.go`：`in_flight_requests` 与实时来源
- `docs/feature/4-reload.md`、`20-runtime.md`：在途语义与快照字段说明
