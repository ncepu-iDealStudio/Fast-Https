# 20. 本机运行快照 `/runtime`

状态：✅ 已实现

## 功能

进程内提供一份只读运行快照，供 CLI 与独立 Monitor 采集。只绑定 IPv4 回环 `127.0.0.1:9088`，路径 `GET /runtime`，无认证、无 TLS。响应不含黑名单、证书路径、Cookie、`Authorization`、请求 URI 或上游地址。

回环地址只在同一个网络命名空间内可达。Monitor 与 Server 需同机同命名空间（容器里放在同一容器或共享网络命名空间）。

## 使用

```powershell
Invoke-RestMethod http://127.0.0.1:9088/runtime | ConvertTo-Json -Depth 5
```

启动早期（写完 PID、证书初始化完成之前）即可访问，此时 `server.state` 为 `starting`。业务端口绑定完成后变为 `running`。9088 被占用时业务端口仍会起来，本接口不可用。

其它路径返回 404。`schema_version` 目前为 `"1"`：只新增字段不升版本；改变已有字段含义或删除字段才升。调用方对不认识的字段忽略，对不认识的版本拒绝。

`request_duration` 始终出现，桶计数和 `count` 为 0 表示还没有请求。`upstream` 由组合根挂入；未装配代理时该对象省略，装配后即使没有代理流量也输出 0（真实的零）。

## 实现方案

- 计数与状态在 `runtime/metrics`：生命周期、连接、请求、状态码、耗时直方图、reload 生成号、监听端口原子副本。快照只做原子 `Load`，不持 `Server.mu`，不读无锁的 `Server.Listens`。
- HTTP 出口在组合根 `cmd/runtime_endpoint.go`。`StartHandler` 在写 PID 之后、`InitSystem` 之前启动监听；`RegisterServices` 把关闭挂到 `core.Services.AfterStop`，并把 `modules/proxy` 的上游计数挂为 `upstream` 分段。
- 分段注册：`metrics.RegisterSection` 供组合根挂入功能包数据，`runtime` 不 import `modules`。
- `request_duration` 口径是进入 `EventHandler` 到返回，不含读请求头；HTTP/2 在 `EventHandler` 返回后才写响应的那部分不在内。桶为累积 `le`（1/5/10/25/50/100/250/500/1000/5000 ms 与 `+Inf`）。
- 上游失败只在错误产生处计数：`dialUpstream` 的拨号失败、`sendToUpstreamServer` 的写超时、`proxy_read.go` 两处读超时。websocket 的 `CircleData` 类型转换失败不算拨号失败。
- `requests.total` 只含进入 `EventHandler` 的请求；解析失败与被拒绝连接单独计，不进入 `responses_*_total`。TCP / WebSocket 会话计入 `connections.active`，升级握手那一次请求计入 `requests.total`。

## 测试

`runtime/metrics/snapshot_test.go`、`runtime/events/duration_test.go`、`modules/proxy/metrics_test.go`、`cmd/runtime_endpoint_test.go`（测试绑定 `127.0.0.1:0`，不占用 9088）、`runtime/server/reload_state_test.go`。进程重启与 `stop` 后 `/runtime` 不可达见 `tests/client_test/runtime_e2e_test.go`（`FASTHTTPS_E2E_RUNTIME=1`）。

## 性能

2026-11-08 按 [Monitor 改造开发计划](fast-https-Monitor改造开发计划.md) 2.2 第 5 条做的前后对比。三个版本用同一份配置、同一静态文件（1KB）、同一并发数，每版跑 4 轮取中位数；32 个 keep-alive 连接，每轮 2s 预热 + 8s 采样，访问日志保持默认开启。

| 版本 | 代码状态 | rps（4 轮） | 中位 rps | p50 | p99 |
| --- | --- | --- | --- | --- | --- |
| a | 第 1 步后：只有连接计数 | 3758 / 3333 / 3832 / 3493 | 3626 | 7.8–8.9 ms | 14.6–18.1 ms |
| b | 第 3 步后：加上请求与状态码计数（第 2 步） | 4322 / 4296 / 3023 / 3245 | 3771 | 7.2–9.8 ms | 11.2–21.9 ms |
| c | 第 4 步后：加上耗时直方图与上游计数（HEAD） | 3679 / 3654 / 3622 / 3787 | 3667 | 7.9–8.2 ms | 14.9–16.5 ms |

中位数差异：第 2 步 +4.0%，第 4 步 +1.1%；而 b 自身 4 轮的极差就有 30%（3023–4322），差异落在测量噪声内。p50/p99 没有系统性偏移。

环境是 Windows 本机回环，单请求约 8 ms 主要由访问日志落盘决定，原子加减的开销在这个量级测不出来。因此不设「退化不超过 N%」的门槛——没有测到退化就不编一个阈值。Linux 上的对照留到隔离环境核对时补测。

## 已知限制

- 端口写死 9088，没有配置项。
- Windows 的 `stop` 走 Kill，不会跑 `AfterStop`，监听由系统回收。
- 第一阶段不探测业务端口是否仍能服务；`/runtime` 能响应不代表站点端口可用。
