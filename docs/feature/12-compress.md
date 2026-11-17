# 12. 响应压缩（gzip / brotli）

状态：✅ 已实现

## 功能

静态文件与反向代理响应按客户端 `Accept-Encoding` 与 location `zip` 配置做内容协商压缩，
支持 gzip 与 brotli（`Content-Encoding: br`）。

## 使用

```json
"zip": ["gzip"]        // 仅 gzip
"zip": ["br"]          // 仅 brotli（不回退 gzip）
"zip": ["gzip", "br"]  // 协商：q 值高者优先，q 相同优先 br
"zip_min_length": 1024
"zip_skip_types": ["image/*", "font/woff2"]
```

## 实现方案（modules/compress/、modules/static/、modules/proxy/compress.go）

- **协商**（`compress.Wanted`）：解析 `Accept-Encoding` 的 q 值，q 不同取高者，
  q 相同按服务端偏好 `br > gzip`；`q=0` 视为拒绝，未显式列出的编码回落到 `*` 的 q。
- **跳过规则**（`compress.Skip`）：内置已压缩类型清单；`zip_skip_types` 非空时整体替换。
  规则支持精确类型、`image/*`、`image/p*` 与 `*/*`。
- **gzip**：`klauspost/compress`（标准库 API 平替，更快），writer 经 `sync.Pool` + `Reset` 复用。
- **brotli**：`andybalholm/brotli`，在线压缩用 4 级（体积优于 gzip 默认级别，CPU 不落入离线级别的区间）。
- **分发**：`compress.CompressBytes(data, encoding)`，未知编码报错。
- **静态层**：带 Range、小于 `zip_min_length` 或命中跳过类型时流式原样返回；压缩后改写
  `Content-Length`，并带 `Vary: Accept-Encoding`。
- **代理层**：上游响应读全后压缩；跳过升级连接、HEAD、已有 `Content-Encoding`、
  204/206/304、`Content-Range`、小于阈值与命中跳过类型；压缩后移除
  `Transfer-Encoding` 并改写 `Content-Length`。缓存先存未压缩正文，命中时按
  各请求的 `Accept-Encoding` 重新协商。

## 测试

- `modules/compress/policy_test.go`：q 值择优、通配匹配、内置/自定义跳过清单。
- `modules/static/accept_encoding_test.go` + `testdata/accept-encoding.jsonl`：
  静态链路的十条端到端用例。
- `modules/proxy/compress_test.go`：代理压缩、各跳过条件、chunked 改长度分帧。

## 已知限制

- 在线压缩全量缓冲（无流式），大文件内存翻倍；离线预压缩（.br/.gz 文件直发）未实现。
