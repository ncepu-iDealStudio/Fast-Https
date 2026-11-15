# 5. 上游服务器反馈核验（第二轮）

第一轮（[4-edge-case-coverage.md](4-edge-case-coverage.md)）按风险面自查，这一轮
换输入源：**读主流 HTTP 服务器公开的 issue 与已合并 PR，逐条对照本项目**。

理由很直接——那些条目是别人用真实流量撞出来的，比凭空设计的用例更接近
生产会出的问题；而且「已合并 PR」的标题通常直接说明修了什么，可操作性强于
泛泛的 bug 报告。

## 5.1 筛选口径

参考仓库里的条目绝大多数与本项目无关，先按技术栈过滤：

| 排除类别 | 原因 |
| --- | --- |
| C 层内存管理（缓冲区溢出、指针生命周期、内存泄漏） | 本项目用 Go，不存在同类问题 |
| QUIC / HTTP/3 实现细节 | 本项目未实现 QUIC |
| 特定模块（FastCGI、SSI、image filter、OCSP 缓存键） | 本项目没有对应模块 |
| 构建与包管理 | 与运行时行为无关 |

过滤后剩下三类能落到本项目：**逗号分隔头的 token 匹配**、**空 body 上游响应的
连接复用**、**超大请求的边界处理**。

## 5.2 核验表

| 编号 | 问题现象 | 本项目状态 | 证据 | 处理 |
| --- | --- | --- | --- | --- |
| N-01 | 逗号分隔的头字段（`Connection: keep-alive, upgrade`）被当成整体做精确匹配，导致 `keep-alive` 识别不到 | ✅ 无此问题 | `Request.IsKeepalive` 的既有实现 | 已按 `,` 拆分并逐 token `TrimSpace`，`close` 优先于 `keep-alive`；顺带说明 `request.QValue` 也是同一套拆分逻辑 |
| N-02 | 上游返回空 body 且不带 `Content-Length` / `Transfer-Encoding` 时，连接被永久占用或误判为可复用 | ✅ 保守正确 | `ReadKeepAlive.tryToParse` 的三分支 | 无 framing 信息时归为「读到连接关闭」（返回 −4），会放弃复用而不是挂着等；代价是这类上游连接不复用，属性能取舍，见 R-12 |
| N-03 | 超大请求体的处理：要么静默断连，要么在「一次性发完」的路径上完全绕过限额 | ⚠️ **存在缺陷，已修** | `TestParseRequestBodyTooLargeReturns413` | 见 5.3，本轮发现两处叠加问题 |
| N-04 | header 超过 `max_header_size` 时的状态码 | ⚠️ 存在待修（合规但不精确） | `events.go` 的 `len(byte_row) > maxHeaderSize()` 分支 | 当前回 **400**。RFC 7231 §6.5.11 为这种情况定义了 **431**，用 400 不违规（原文是 MAY），但客户端无法区分「语法错误」与「字段过大」。见 R-11 |
| N-05 | 客户端声明「接受任意编码」却拿到未压缩响应 | 🔧 已修复（上一轮） | `TestEncodingWantedVectors`、B-17 | 见 [4-edge-case-coverage.md](4-edge-case-coverage.md) 的 B-17 |

## 5.3 N-03：两处叠加的缺陷

原始反馈是「超大请求的处理有问题」，追下去发现本项目有两层问题。

**第一层：首包即超限时检查被完全绕过**

`parseRequest` 的读取循环长这样：

```go
for {
    if rr.Req.RequestBodyValid() { break }   // 已读满声明长度就直接退出
    ...读更多...
    if rr.Req.Body.Len() > limit { return 0 }
}
```

`ParseBody` 在前面已经把首包里的 body 全读进 `Req.Body`。如果客户端**一次性**
把超大的 body 发完，`RequestBodyValid()` 立刻为 true，循环一次都不执行，
限额检查形同虚设——请求直接带着超大 body 进了后续管线。

修复：在循环之前统一检查一次，循环内的检查保留（分段到达时仍需要）。

**第二层：超限后静默断连**

原实现只 `return 0`，连接直接关闭。客户端看到的是连接重置，分不清
「服务端拒绝」还是「网络故障」，只能无脑重试——对一个已经超限的请求重试是
最坏的选择。

修复：回 **413 Payload Too Large** 再关闭，新增
`response.DefaultPayloadTooLarge()`。

**副作用**：`net.Pipe` 是同步管道，超限后多写一段 413，如果测试端不读就会把
handler 卡在写上。`request_metrics_test.go` 的 `too_size` 子用例原本不读响应，
已补一个消费者。

## 5.4 结果

```
PASS: TestParseRequestBodyTooLargeReturns413
ok    fast-https/runtime/events
go test ./... 全绿
```

核验结果：**5 条里 2 条确认无此问题、2 条已修复、1 条待修（N-04，合规但可
改进）**。

## 5.5 复跑

```bash
# 请求体超限
go test ./runtime/events/ -run 'TestParseRequestBodyTooLargeReturns413|TestBodyTimeoutAndTooLarge' -v
# keep-alive 的 token 匹配（N-01 相关，随第一轮一起跑）
go test ./runtime/request/ -run 'TestKeepAliveRequestIsFullyReset' -v
go test ./... -count=1
```

## 5.6 与第一轮的关系

| | 第一轮（4-edge-case-coverage） | 第二轮（本篇） |
| --- | --- | --- |
| 输入 | 按风险面自查（路径、SNI、代理头、压缩、连接） | 读主流 HTTP 服务器公开的 issue 与已合并 PR |
| 产出 | 17 条核验，2 处修复 | 5 条核验，2 处修复 |
| 重叠 | 压缩协商（B-13/B-17） | N-05 引用上一轮结论，不重复列 |

后续新增需求见
[edge-case-hardening-plan.md](../develop/plans/edge-case-hardening-plan.md) 的
R-11 ~ R-13。
