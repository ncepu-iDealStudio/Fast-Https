# R-08 连接与超时边界行为 —— 验收报告

- 日期：2026-11-17
- 范围：`docs/develop/archive/edge-case-hardening-plan.md` 的 R-08，以及与连接/超时直接相关的后续补强
- 结论：**通过**。需求清单全部落地，扩展项已完成，待修项清零。

## 1. 验收依据

计划中 R-08 的需求与验收：

1. `Expect: 100-continue`：先回 `100`，再读 body
2. keep-alive 复用连接时，请求间不残留状态（cookie、认证、缓存键）
3. header 超限 → 431；body 超限 → 413；两者都不 panic
4. 提供 `max_header_size`、`max_body_size` 配置项（沿用既有键名）
5. 验收：新增一组真实连接用例，`-race` 下跑

## 2. 逐条验收结果

| 编号 | 验收项 | 结果 | 关键证据 |
| --- | --- | --- | --- |
| R08-1 | `Expect: 100-continue` | ✅ | `TestExpectContinueSends100BeforeBody`、`TestExpectContinueChunkedBody`、`TestExpectListAcceptsContinue`、`TestRealTCPExpectContinueAndKeepAlive` |
| R08-2 | keep-alive 状态隔离 | ✅ | `TestKeepAliveRequestIsFullyReset`、`TestFlushClearsAuxiliaryFields`、`TestKeepAliveResetsRequestAndResponse` |
| R08-3 | 431 / 413 / 不 panic | ✅ | `TestHeaderTooLargeReturns431`（一次到齐 + 分片）、`TestMalformedHeaderStillReturns400`、`TestChunkedBodyTooLargeReturns413`、`TestRealTCPHeaderTooLarge431`、`TestRealTCPBodyTooLarge413` |
| R08-4 | 限额配置项 | ✅ | `config/internal/model`、`config/internal/validate`、`config` 的加载/校验用例 |
| R08-5 | 真实连接 + `-race` | ✅ | `runtime/events/connection_e2e_test.go`；`go test -race ./runtime/events` |

### R08-1 `Expect: 100-continue`

- HTTP/1.1：读正文前直接写 `HTTP/1.1 100 Continue`（`writeContinue`）。
- 声明长度超过 `max_body_size`：**不回 100**，直接 413（`TestExpectContinueOversizedDeclaredBodyGets413`）。
- chunked 上传：先回 100，再按块解码（`TestExpectContinueChunkedBody`）。
- 头值是列表（`100-continue, x`）：按包含关系识别（`TestExpectListAcceptsContinue`）。
- 无法满足的期望：回 417（`TestUnknownExpectationReturns417`、`TestH2UnknownExpectationReturns417`）。
- HTTP/1.0：忽略期望并移除头（`TestHTTP10IgnoresExpect`）。
- 代理链路：本机满足期望后从转发头删除 `Expect`，上游确认收不到（`TestRealTCPProxyDropsSatisfiedExpect`）。
- HTTP/2：协议栈在首次读 body 时自动回 100；应用层只负责 417 与移除转发头。

### R08-2 keep-alive 状态隔离

- 请求侧：`Request.Flush` 清 Headers / Query / Body / Encoding / 计数与 multipart 扫描进度。
- 响应侧：复用连接时重建 `Response`，并复位 `OriginPath`、`PathLocation`、`Upgrade`、`ProxyConnInit`、`CircleData`。
- 覆盖场景：上一请求的 `Cookie`、`Authorization`、`Accept-Encoding`、查询串、body，以及上一响应的 `Content-Encoding`、`Set-Cookie` 均不会串到下一请求。
- 缓存键来自当前请求的 `$host` / `$request_uri`，无跨请求残留。

### R08-3 431 / 413 / 不 panic

| 场景 | 结果 |
| --- | --- |
| header 一次到齐且超限 | 431 |
| header 分片到达、累计超限 | 431 |
| 畸形头（无名头行等） | 400，不被 431 吞掉 |
| Content-Length 超限 | 413 |
| chunked 解码后超限 | 413（独立 `ErrBodyTooLarge`，与格式错误区分） |
| HTTP/2 声明长度 / 流式正文超限 | 413 |
| HTTP/2 头部超限 | 应用层按 RFC 7540 字段开销判 431 |

拒绝路径只返回响应并结束连接，没有 panic；连接级 panic 另由 `runtime/server` 的 `recover` 兜底。

### R08-4 配置项

| 键 | 默认 | 说明 |
| --- | --- | --- |
| `http.servers_limit.max_header_size` | 4096 | 431；HTTP/1.1 与 HTTP/2 共用 |
| `http.servers_limit.max_body_size` | 524288 | 413；声明长度、chunked、h2 流式正文都检查 |
| `http.servers_limit.read_timeout` | 30 | 读请求头/首包；h2 读头与空闲超时 |
| `http.servers_limit.write_timeout` | 30 | 写响应；静态文件直写路径同样生效 |
| `http.servers_limit.body_timeout` | 1 | 读取 body 的单次读超时；h2 超时回 408 |
| `proxy.upstream_timeout` | 20 | 代理到上游的整体超时 |
| `proxy.tunnel_idle_timeout` | 0 | websocket / tcp 隧道双向静默超时后断开；0 表示不断开 |

超时写 `0` 表示使用默认值；负数在加载/校验时直接报错（`TestValidateLimitsRejectsNegativeTimeouts`、`TestValidateProxy`）。

### R08-5 验收方式

- `runtime/events/connection_e2e_test.go` 使用真实 `net.Listen` + `HandleEvent`，覆盖 100、431、413、keep-alive、以及「客户端 Expect → 代理 → 回环上游」完整链路。
- `go test -race ./runtime/events ./runtime/request ./runtime/server ./core ./modules/static ./modules/proxy ./modules/websocket ./modules/proxy_tcp` 通过。

## 3. 超清单补强项

以下不属于 R-08 原始四条，但属于同一「连接与超时」边界，已一并落地：

1. **超时全链路可配置**：h1 读写与 body、h2 读头/空闲/写、静态文件直写、代理上游、隧道空闲。
2. **HTTP/2 限额对齐**：`MaxHeaderBytes` 接入配置；正文超限 413；未知期望 417。
3. **chunked 正文超限 413**：此前会静默断连，客户端分不清「服务端拒绝」与「网络故障」。
4. **隧道空闲断开**：websocket 与 tcp 隧道双向共享最后活动时间，单向静默不会误杀。
5. **静态文件写超时**：绕过 `EventWrite` 的直写路径也会刷新写 deadline。

## 4. 测试执行记录

命中 R-08 的用例全部 PASS：

```text
PASS: TestRealTCPExpectContinueAndKeepAlive
PASS: TestRealTCPProxyDropsSatisfiedExpect
PASS: TestRealTCPHeaderTooLarge431
PASS: TestRealTCPBodyTooLarge413
PASS: TestExpectContinueSends100BeforeBody
PASS: TestExpectContinueOversizedDeclaredBodyGets413
PASS: TestExpectContinueChunkedBody
PASS: TestExpectListAcceptsContinue
PASS: TestUnknownExpectationReturns417
PASS: TestHTTP10IgnoresExpect
PASS: TestHeaderTooLargeReturns431 (一次到齐 / 分片到达)
PASS: TestMalformedHeaderStillReturns400
PASS: TestChunkedBodyTooLargeReturns413
PASS: TestReadTimeoutIsConfigurable
PASS: TestWriteTimeoutIsConfigurable
PASS: TestKeepAliveResetsRequestAndResponse
PASS: TestReadH2Body
PASS: TestH2HeaderTooLargeReturns431
PASS: TestH2UnknownExpectationReturns417
PASS: TestH2BodyTooLargeReturns413 (声明长度 / 未知长度)
PASS: TestStaticWriteTimeout
PASS: TestWebSocketHandlerIdleTimeout
PASS: TestProxyEventTcpIdleTimeout
PASS: TestTunnelIdleDisabled
PASS: TestTunnelIdleExpiresAndResets
PASS: TestServerLimitTimeoutDuration
PASS: TestProxyUpstreamTimeoutDuration
PASS: TestProxyTunnelIdleTimeoutDuration
PASS: TestValidateLimitsRejectsNegativeTimeouts
PASS: TestConfigProxySetShowAndClear
```

全量验证：

- `go test ./...`：全绿
- `go test -race ./runtime/events ./runtime/request ./runtime/server ./core ./modules/static ./modules/proxy ./modules/websocket ./modules/proxy_tcp`：通过
- 桌面端 `go test ./internal/...`：通过
- `go run ./tools/docgen`：`usage.html` 已同步

## 5. 已知偏差与限制

1. **用例位置**：计划写的是「`runtime/listener` 新增一组」，实际放在逻辑所在的 `runtime/events`（`connection_e2e_test.go`）。这是有意的位置调整，不改变验收内容。
2. **h2 极端超长头**：应用层 431 覆盖到 `max_header_size + 64 KB`；更大的头部会被 x/net/http2 以连接错误终止，应用层来不及回 431。属于协议栈行为，已在 `docs/feature/2-http2.md` 记录。
3. **隧道空闲默认关闭**：`proxy.tunnel_idle_timeout` 默认 `0`，保持历史「不断开」行为；需要主动配置才会断开。

## 6. 提交记录

| 提交 | 内容 |
| --- | --- |
| `618b468` | R-08 初版：100-continue、keep-alive 复位、431/413 |
| `0a320d2` | 补真实 TCP E2E、chunked/列表/417、h2 限额 |
| `66f5d5a` | 超时配置化（read/write/body） |
| `6d60357` | 删除未被调用的 `setConnCfg` |
| `61b82fa` | 超时覆盖 h2、静态直写、代理上游 |
| `5add2b2` | websocket / tcp 隧道空闲断开 |
| `b27da56` | chunked 正文超限回 413 |

## 7. 关键文件

- `runtime/events/events.go`：解析、Expect、431/413、超时
- `runtime/events/events_h2.go`：h2 限额、Expect、正文超时 408、写/空闲超时
- `runtime/events/connection_limits_test.go`、`connection_e2e_test.go`
- `runtime/request/chunked.go`：`ErrBodyTooLarge`
- `runtime/server/server.go`：连接级 panic 兜底
- `core/tunnel.go`：隧道空闲跟踪
- `config/internal/model/types.go`、`config/internal/loader/convert.go`、`config/internal/validate/validate.go`
- `modules/static/static.go`、`modules/proxy/event_proxy.go`、`modules/websocket/websocket.go`、`modules/proxy_tcp/event_proxy_tcp.go`
- `docs/feature/1-http1.md`、`2-http2.md`、`7-proxy.md`、`8-proxy-tcp.md`、`9-websocket.md`、`docs/usage.md`
