# 2. HTTP/2

状态：✅ 已实现

## 功能

`listen ... ssl h2` 端口通过 ALPN 协商提供 HTTP/2 服务；分帧、HPACK、流状态机、流量控制、PING/GOAWAY 全部由 `golang.org/x/net/http2` 承担。

## 使用

```json
{ "listen": "443 ssl h2", "server_name": "example.com", ... }
```

只有通告了 h2 的端口才会协商 HTTP/2；`ssl` 不带 `h2` 保持纯 HTTP/1.1（`listener.go` 的 ALPN 逻辑）。

## 实现方案（runtime/events/events_h2.go）

- 每条 TLS 连接接入后，用**只吐一条连接的 `singleConnListener`** 把它喂给一个 `http.Server`——TLS 握手与 ALPN 由标准库完成，`Shutdown` 能发 GOAWAY。
- `ConnState` 在连接关闭/hijack 时放行 Accept，防止 Serve 阻塞泄漏。
- `h2Handler.ServeHTTP` 把 `*http.Request` 翻译成既有 `core.Event`（`fillRequest`：method/path/query/headers/body），进入与 HTTP/1.1 **完全相同**的 EventHandler 管线——模块无需感知协议差异。
- 响应方向：模块写出的 `response.Response` 经 `h2ResponseWriter` 翻译回 `http.ResponseWriter`；跳过逐跳头（Connection/Upgrade 等），Content-Length 交给标准库按实际写出计算（HEAD 保留模块给出的值）。
- 限额与 HTTP/1.1 对齐：`http.servers_limit.max_header_size` 同时写入 `http.Server.MaxHeaderBytes`
  （协议栈据此设置 `SETTINGS_MAX_HEADER_LIST_SIZE`，并留 64 KB 余量让 431 先于连接错误），
  应用层再按 RFC 7540 §6.5.2 的字段开销估算一次，超限回 **431**；`max_body_size` 对声明长度与
  流式正文都生效，超限回 **413**。
- 超时：读头与空闲超时用 `read_timeout`，每次写无进展超过 `write_timeout` 断开；
  正文读取按 `body_timeout` 逐次计时，超时回 **408**。
- `Expect: 100-continue` 由协议栈在首次读 body 时自动回 100；无法满足的其它期望回 **417**。
  读取正文后从转发头里移除 `Expect`，避免上游再回一个 100。
- reload/停机时对每条 h2 连接执行 5s 宽限的 `Shutdown`。
- 约束：`MaxConcurrentStreams` 250；读头超时复用 `servers_limit.read_timeout`；
  标准库内部日志经 `h2LogWriter` 转入项目日志。

## 测试

`tests/client_test/h2_e2e_test.go`：真实 TLS + ALPN 集成测试（子进程起真实服务），覆盖协商、静态正文、404；
`runtime/events/h2_helpers_test.go` 覆盖 431 / 413 / 417；`runtime/events/shutdown_test.go` 覆盖停机。

## 已知限制

- 每连接一个 `http.Server` 实例（对象开销偏大，万级连接场景需评估）。
- 模块设置的 Connection 头会被丢弃（协议要求），依赖它的行为在 h2 下不可见。
