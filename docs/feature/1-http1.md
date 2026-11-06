# 1. HTTP/1.1 服务

状态：✅ 已实现

## 功能

自研的 HTTP/1.1 报文解析与连接服务：请求行/头解析、keep-alive 连接复用、Content-Length 与 chunked body、大头/大 body 防护、畸形请求拒绝、优雅停机。

## 使用

`listen: 80`（或任意端口）即启用；keep-alive 自动。相关上限配置：

```json
"http": {
  "servers_limit": { "max_header_size": 4096, "max_body_size": 524288 }
}
```

## 实现方案

### 解析（runtime/request/parse.go）

- 零拷贝：全程在原始字节切片上切行（`bytes.Index` 定位 CRLF），只为存入 Headers map 的键值分配字符串（2026-10 基准：GET 819ns/288B/9 allocs，约为标准库 `http.ReadRequest` 的 3 倍速）。
- 方法/协议经 `map[string]string` 零分配查找校验并复用常量串；常见头名预置 canonical 表（`commonCanonKeys`）零分配命中。
- 头名统一 canonical 化（HTTP 大小写不敏感语义）；非法/负数 Content-Length、无名头行（`: value`）返回 `InvalidHeaders`。
- 头分片到达：`events.parseRequest` 循环读满（受 `max_header_size` 上限，超限 400）。
- multipart 判结束为增量扫描（记录进度，大上传无 O(n²)）。

### Body（runtime/request/chunked.go）

- Content-Length 按差额循环读；chunked 用自研 `ReadChunked` 解码（含终止块校验、上限保护）。

### 连接（core/event.go、runtime/events/events.go）

- 每连接一个 goroutine，顶层 `recover()` 兜底——模块代码 panic 只关连接不杀进程。
- keep-alive 复用 `Event`/`Request` 对象（`Flush` 清理 Headers/Query/Body/扫描进度）。
- 读缓冲 `sync.Pool`（4KB）；写超时 30s、读超时 30s（body 分块读 1s/轮）。
- 畸形请求统一 400（`DefaultBadRequest`）并关闭。

### 停机（runtime/server/server.go）

- SIGTERM/SIGQUIT/stop：取消监听器 ctx → 等 in-flight（5s 上限）→ 退出。
- accept 瞬时错误（如 fd 用尽）退避 10ms 重试，只有 `net.ErrClosed`/ctx 取消才退出。

## 测试

`tests/unit_test/runtime/request`（解析/畸形/基准）、`runtime/server`（停机/accept 容错）、`runtime/events`（keep-alive/关闭）、`tests/client_test`（E2E）。

## 已知限制

- 不支持 pipelining 深度流水（逐请求串行处理）、100-continue、trailer。
- Date 头已修为 RFC 7231（秒粒度缓存）；`time.Now().String()` 遗留格式仅存于历史日志。
