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
- **分发**：`compress.CompressReader(io.Reader, encoding)` 按块读入压缩、返回完整压缩体；
  `CompressBytes` 只是把字节切片包成 reader 的入口，未知编码报错。
- **静态层**：带 Range、小于 `zip_min_length` 或命中跳过类型时流式原样返回；需要压缩时
  直接从文件流式读取压缩，避免原文整体进内存；压缩后改写 `Content-Length`，并带
  `Vary: Accept-Encoding`。
- **代理层**：上游响应读全后压缩；跳过升级连接、HEAD、已有 `Content-Encoding`、
  204/206/304、`Content-Range`、小于阈值与命中跳过类型；压缩后移除
  `Transfer-Encoding` 并改写 `Content-Length`。缓存先存未压缩正文，命中时按
  各请求的 `Accept-Encoding` 重新协商。

## 测试

- `modules/compress/policy_test.go`：q 值择优、通配匹配、内置/自定义跳过清单。
- `modules/compress/stream_test.go`：逐字节 reader 验证输入按块读取、空输入、读错误上抛。
- `modules/static/accept_encoding_test.go` + `testdata/accept-encoding.jsonl`：
  静态链路的十条端到端用例。
- `modules/proxy/compress_test.go`：代理压缩、各跳过条件、chunked 改长度分帧。
- `modules/proxy/proxy_compress_e2e_test.go`：完整 `ProxyEvent` + 回环上游，覆盖无缓存
  gzip 压缩，以及缓存命中后按各请求 `Accept-Encoding` 重新协商（无编码回原文、br 本机重压）。

## 已知限制

- 压缩输出仍需整体缓冲后才能写 `Content-Length`：静态层只保留压缩体一份内存（原文按块读），
  代理层的上游响应本来就整体读入 `ev.RR.Res.Body`，压缩时无法避免一次全量拷贝。
- 离线预压缩（直接发 .br/.gz 文件）未实现。
