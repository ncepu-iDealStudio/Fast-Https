# 12. 响应压缩（gzip / brotli）

状态：✅ 已实现

## 功能

静态响应按客户端 `Accept-Encoding` 与 location `zip` 配置做内容协商压缩，支持 gzip 与 brotli（`Content-Encoding: br`）。

## 使用

```json
"zip": ["gzip"]        // 仅 gzip
"zip": ["br"]          // 仅 brotli（不回退 gzip）
"zip": ["gzip", "br"]  // 协商：客户端认 br 优先 br，否则回退 gzip
```

## 实现方案（modules/compress/、modules/static/static.go）

- **协商**（`encodingWanted`）：`request.HasToken` 解析 Accept-Encoding（q=0 视为不接受），三种配置语义如上；配置了压缩的响应带 `Vary: Accept-Encoding`。
- **gzip**：`klauspost/compress`（标准库 API 平替，更快），writer 经 `sync.Pool` + `Reset` 复用。
- **brotli**：`andybalholm/brotli`，在线压缩用 4 级（体积优于 gzip 默认级别，CPU 不落入离线级别的区间）。
- **分发**：`compress.CompressBytes(data, encoding)`，未知编码报错。
- **与 Range 的关系**：带 Range 的请求跳过压缩（分段语义优先）。

## 测试

`modules/compress/brotli_test.go`（往返/分发拒绝）、`modules/static/negotiation_test.go`（br/gzip/both × 客户端编码的协商矩阵 + Vary）。

## 已知限制

- 仅静态路径压缩，代理响应不压。
- 在线压缩全量缓冲（无流式），大文件内存翻倍；离线预压缩（.br/.gz 文件直发）未实现。
