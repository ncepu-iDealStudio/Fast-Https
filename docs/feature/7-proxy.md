# 7. 反向代理

状态：✅ 已实现

## 功能

HTTP/HTTPS 反向代理：转发到 `proxy_pass` 上游、请求头改写、上游连接池复用、响应缓存接入、chunked 响应解码、上游故障的 500/错误页。

## 使用

```json
{
  "url": "/api", "type": "proxy", "proxy_pass": "http://127.0.0.1:9000",
  "proxy_set_header": [ { "HeaderKey": "X-Real-IP", "HeaderValue": "$remote_addr" } ],
  "proxy_cache": { "path": "./cache", "key": "$request_uri", "valid": ["any:1h"], "max_size": 1073741824 }
}
```

## 实现方案（modules/proxy/）

- **转发**：请求经 `ByteRow` 重建后发往上游；`X-Forwarded-For` 由 `core.DefaultParseCommandHandler` 追加真实 IP；`proxy_set_header` 支持 `$remote_addr`/`$request_uri` 等变量插值。
- **上游连接池**（pool.go，2026-10）：按归一化地址（自动补 :80/:443）分组复用空闲连接，省掉每请求 TCP/TLS 握手；每地址空闲上限 8、空闲 30s 惰性+周期回收；复用连接 deadline 借出刷新/归还清除；首次发送失败（上游已关闭）自动换新连接重试一次。websocket 会话整条持有不进池。
- **响应读取**：ReadKeepAlive（Content-Length 精确读/Transfer-Encoding chunked 解码）与 ReadConnectionClose（读到 EOF）；上游错误回 500（可配 `error_page.proxy`），超时关闭。
- **缓存**（cache.go）：`proxy_cache.key` 变量插值取 MD5 为键；命中直接回放（跳过上游）；方法白名单（默认 GET/HEAD）、`no-cache/no-store` 尊重客户端。
- **websocket**：升级请求由握手代理后整条连接交给 WebSocket 模块（见 [9-websocket.md](9-websocket.md)）。

## 测试

`modules/proxy/`（pool_test 连接池语义、proxy_read_test chunked 解码、cache_test 缓存语义）、`tests/client_test/proxy_e2e_test.go`。

## 已知限制

- 上游为单地址，无负载均衡/健康检查/故障转移（见占位 [21-planned-upstream-failover.md](21-planned-upstream-failover.md)）。
- HTTPS 上游 `InsecureSkipVerify: true`（不校验上游证书）。
- 响应不做压缩（客户端接受压缩而上游未压时透传原文）。
