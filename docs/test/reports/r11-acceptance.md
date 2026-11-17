# R-11 header 超限返回 431 —— 验收报告

- 日期：2026-11-17
- 范围：`docs/develop/plans/edge-case-hardening-plan.md` 的 R-11
- 结论：**通过**。R-11 随 R-08 一并落地，本次独立复核代码路径、h1/h2 行为、配置文档与日志字段。

## 1. 验收依据

计划中 R-11 的需求与验收：

1. header 超过 `max_header_size` 时返回 **431 Request Header Fields Too Large**（RFC 7231 §6.5.11），不再是 400。
2. 构造超长 header，断言状态行是 431。
3. `max_header_size` 配置项在 `docs/usage.md` 的限额表里有说明。

## 2. 逐条验收结果

| 编号 | 验收项 | 结果 | 关键证据 |
| --- | --- | --- | --- |
| R11-1 | h1 超长 header 返回 431 | ✅ | `TestHeaderTooLargeReturns431`（一次到齐 / 分片到达）、`TestRealTCPHeaderTooLarge431`（真实 TCP） |
| R11-2 | h2 超长 header 返回 431 | ✅ | `TestH2HeaderTooLargeReturns431`；`events_h2.go` 在 `ServeHTTP` 入口按同一份配置判定 |
| R11-3 | 畸形头仍是 400，不被 431 吞掉 | ✅ | `TestMalformedHeaderStillReturns400`、`TestMalformedHeaderIs400` |
| R11-4 | 限额表文档 | ✅ | [usage.md](../../usage.md) 5.2 与 `usage.html` 均有 `max_header_size` 行 |
| R11-5 | 与 R-10 日志字段一致 | ✅ | 拒绝日志写 `reject_reason=header_too_large`、`rule=max_header_size`、`remote_ip` |

## 3. 实现位置

| 位置 | 作用 |
| --- | --- |
| `runtime/events/events.go` `parseRequest` | 头完整（`RequestOk`）与分片（`RequestNeedReadMore`）两条路径都按 `headerLen` 判超限，命中后调 `rejectHeaderTooLarge` |
| `runtime/events/events_h2.go` `ServeHTTP` | h2 在应用层按 `h2HeaderSize` 判 431，与 h1 共用 `maxHeaderSize()` |
| `runtime/response/response_default.go` | `DefaultRequestHeaderFieldsTooLarge` 生成 431 响应 |
| `runtime/events/events.go` `maxHeaderSize` | 配置值 > 0 时用配置，否则回 `DEFAULT_MAX_HEADER_SIZE` |

判定口径：头段长度按 `\r\n\r\n` 终止符计算（`headerLen`），头未读完时按已读字节数；整段头一次到齐也不会跳过检查。

## 4. 行为语义

- h1：回 431 后关闭连接（`WriteResponseClose`），客户端不会把半截请求继续发完。
- h2：回 431 状态，不读正文。
- 400 只保留给真正畸形的请求行/头（例如无名头行），431 专指「头合法但太大」，客户端可据此改配置或精简 Cookie。
- `max_header_size=0` 表示使用默认值 `DEFAULT_MAX_HEADER_SIZE`（4096）。

## 5. 测试执行记录

```text
=== RUN   TestRealTCPHeaderTooLarge431
--- PASS: TestRealTCPHeaderTooLarge431 (0.00s)
=== RUN   TestHeaderTooLargeReturns431
=== RUN   TestHeaderTooLargeReturns431/一次到齐
=== RUN   TestHeaderTooLargeReturns431/分片到达
--- PASS: TestHeaderTooLargeReturns431 (0.00s)
    --- PASS: TestHeaderTooLargeReturns431/一次到齐 (0.00s)
    --- PASS: TestHeaderTooLargeReturns431/分片到达 (0.00s)
=== RUN   TestMalformedHeaderStillReturns400
--- PASS: TestMalformedHeaderStillReturns400 (0.00s)
=== RUN   TestH2HeaderTooLargeReturns431
--- PASS: TestH2HeaderTooLargeReturns431 (0.00s)
=== RUN   TestMalformedHeaderIs400
--- PASS: TestMalformedHeaderIs400 (0.00s)
PASS
ok  	fast-https/runtime/events	4.328s
```

手工链路（真实进程，2026-11-17）：启动服务后发送 5 KB 请求头，`curl -w '%{http_code}'` 返回 `431`，同一条请求在 `safe.log` 留下 `reject_reason=header_too_large`、`rule=max_header_size`、`remote_ip=127.0.0.1`。

## 6. 已知边界

1. **h2 极端超长头**：应用层 431 覆盖到 `max_header_size + 64 KB`；更大的头部会被 `x/net/http2` 以连接错误终止，应用层来不及回 431。属于协议栈行为，已在 `docs/feature/2-http2.md` 记录。
2. **h2 的 `MaxHeaderBytes`**：协议栈的列表上限只作 DoS 兜底，真正的 431 由应用层按 `max_header_size` 判定，保证 h1/h2 状态码一致。
3. **不区分具体超限字段**：431 响应与日志只给出总量超限与规则名，不指出是哪一条 header 导致，避免在响应里回显攻击者可控内容。

## 7. 结论

R-11 的两条验收全部满足，h1/h2 行为一致，畸形请求仍为 400，限额配置与日志字段同步。无需新增代码改动。
