# R-12 空 body 上游响应的连接复用 —— 验收报告

- 日期：2026-11-17
- 范围：`docs/develop/archive/edge-case-hardening-plan.md` 的 R-12
- 结论：**通过**。HEAD / 204 / 304 这类协议上必然没有 body 的响应不再等上游关闭，连接可以归还池并复用。

## 1. 验收依据

计划中 R-12 的需求与验收：

1. 在「响应是 1xx/204/304 或 HEAD 请求」时直接判定 body 为空，标记连接可复用，不必等上游关闭。
2. 验收：构造 `HTTP/1.1 204 No Content` 且无 `Content-Length` 的上游响应，断言连接被复用（第二次请求走同一条连接）。

## 2. 逐条验收结果

| 编号 | 验收项 | 结果 | 关键证据 |
| --- | --- | --- | --- |
| R12-1 | 204 无 Content-Length 判定无 body | ✅ | `TestBodylessStatusWithoutLength/204_No_Content`；`tryToParse` 返回 0，`emptyBody=true` |
| R12-2 | 304 无 Content-Length 判定无 body | ✅ | `TestBodylessStatusWithoutLength/304_Not_Modified` |
| R12-3 | HEAD 响应即使带 Content-Length 也不读 body | ✅ | `TestHeadResponseStopsAtHeader`（旧实现会按 CL 等待 1024 字节直到读超时） |
| R12-4 | 空 body 响应后连接被复用 | ✅ | `TestBodylessResponseReusesUpstreamConnection`：两次请求只 accept 一次，第二次 `reused=true` |
| R12-5 | 普通无 CL/TE 响应仍按旧语义读到关闭 | ✅ | `TestStatusWithoutLengthReadsUntilClose`：body 完整、`emptyBody=false`、连接不复用 |
| R12-6 | 1xx interim 中间响应被丢弃，保留最终响应 | ✅ | `TestInterimResponseInSameRead`（100 / 103，同一段数据）、`TestInterimResponseAcrossReads`（分两次到达）、`TestInterimResponseThenBodylessStatus`（100 后接 204） |
| R12-7 | 101 协议升级不按中间响应处理 | ✅ | `TestSwitchingProtocolsNotTreatedAsInterim` |

## 3. 实现位置

| 位置 | 改动 |
| --- | --- |
| `modules/proxy/proxy_read.go` `ReadKeepAlive` | 新增 `emptyBody` 标记；`tryToParse` 先判 HEAD，再判 204/304 |
| `modules/proxy/proxy_read.go` `interimResponse` | 1xx（101 除外）返回专门的中间响应标记；`proxyKeepAlive` 丢弃这段后继续解析剩余字节 |
| `modules/proxy/proxy_read.go` `readUntilEOF` | 无 CL、无 chunked 的普通响应读到 EOF，保持原有「读到连接关闭」语义 |
| `modules/proxy/event_proxy.go` `Proxy.respEmptyBody` | 把「本次响应必然无 body」传给连接回收决策 |
| `modules/proxy/event_proxy.go` `ChangeHeader` | 空 body 响应且上游未显式 close 时清掉 `ProxyNeedClose`，允许入池 |
| `modules/proxy/event_proxy.go` `upstreamCloseRequested` | HTTP/1.1 只看 `Connection: close`；HTTP/1.0 要求显式 `keep-alive` |

## 4. 行为语义

- **HEAD**：响应没有 body，头里的 `Content-Length` 表示 GET 时的长度，不能拿来读正文。
- **204 / 304**：协议规定无 body，收到头部即到响应边界。
- **其他无 CL/TE 的响应**：按 HTTP/1.0 语义读到上游关闭；连接不复用（`ProxyNeedClose` 保持 true）。
- **复用条件**：本次响应必然无 body，且上游没有 `Connection: close`；HTTP/1.0 上游还要求显式 `keep-alive`。
- **1xx interim**：收到 100 Continue / 103 Early Hints 等中间响应时丢弃这段，继续读最终响应；中间响应与最终响应落在同一次读取里时，保留最终响应开头，不会因为「再读一次」而卡到超时。101 是协议升级，不按中间响应处理。

## 5. 测试执行记录

```text
=== RUN   TestHeadResponseStopsAtHeader
--- PASS: TestHeadResponseStopsAtHeader (0.00s)
=== RUN   TestBodylessStatusWithoutLength
=== RUN   TestBodylessStatusWithoutLength/204_No_Content
=== RUN   TestBodylessStatusWithoutLength/304_NotModified
--- PASS: TestBodylessStatusWithoutLength (0.00s)
=== RUN   TestStatusWithoutLengthReadsUntilClose
--- PASS: TestStatusWithoutLengthReadsUntilClose (0.00s)
=== RUN   TestBodylessResponseReusesUpstreamConnection
--- PASS: TestBodylessResponseReusesUpstreamConnection (0.01s)
=== RUN   TestInterimResponseInSameRead
=== RUN   TestInterimResponseInSameRead/100
=== RUN   TestInterimResponseInSameRead/103
--- PASS: TestInterimResponseInSameRead (0.00s)
=== RUN   TestInterimResponseAcrossReads
--- PASS: TestInterimResponseAcrossReads (0.00s)
=== RUN   TestInterimResponseThenBodylessStatus
--- PASS: TestInterimResponseThenBodylessStatus (0.00s)
=== RUN   TestSwitchingProtocolsNotTreatedAsInterim
--- PASS: TestSwitchingProtocolsNotTreatedAsInterim (0.00s)
PASS
ok  	fast-https/modules/proxy	3.718s
```

包级与竞态：

- `go test -count=1 ./modules/proxy/`：通过
- `go test -race -count=1 ./modules/proxy/`：通过
- `go build ./...`、`go vet ./modules/proxy/`：通过

## 6. 已知边界

1. **101 协议升级**：按升级响应处理，不当作 interim 丢弃；websocket 仍走原有的升级分支。
2. **无长度上限**：无 CL/TE 的普通响应按旧语义读到 EOF，没有额外的 body 上限；与同样路径的 `ReadConnectionClose` 保持一致。
3. **连接复用依赖上游未显式 close**：若上游在 204 后仍主动关闭，复用时首次发送失败会由 `p.reused` 分支换新连接重试一次，不会把错误暴露给客户端。

## 7. 结论

R-12 的验收项全部通过：204/304/HEAD 不再等待连接关闭，空 body 响应后连接进入池并被下一次请求复用；1xx interim 被正确跳过，101 保留协议升级语义；普通无 CL/TE 响应维持原有读到关闭的行为。
