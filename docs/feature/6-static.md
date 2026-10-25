# 6. 静态文件服务

状态：✅ 已实现

## 功能

本地文件服务：`root`+`index` 目录索引、`try` 多候选查找、条件请求（304）、Range 分段（206/416）、gzip/brotli 压缩协商、目录穿越防护、自定义错误页。

## 使用

```json
{
  "url": "/", "type": "local", "root": "./httpdoc/root",
  "index": ["index.html"],
  "error_page": { "not_found": "404.html" },
  "zip": ["gzip", "br"]
}
```

## 实现方案（modules/static/static.go）

- **查找**：`dofind` 依次尝试请求路径 → `try.Files` 候选（`filepath.Join` 拼接）→ `try.Next` 回退。
- **安全**：打开前 `staticPathInRoot` 对拼接结果 Clean 并校验仍在 `StaticRoot` 内，`/../` 逃逸一律 404；服务不做 URL 解码，`%2e%2e` 天然不生效。文件句柄全路径关闭（目录请求、客户端中断均不泄漏）。
- **条件请求**：200/206 带 `Last-Modified`（RFC 7231 格式）；`If-Modified-Since` 不早于修改时间回 304 无正文。
- **Range**：仅 GET、单段 `bytes=`；命中回 206 + `Content-Range`（Seek + `io.CopyN`）；畸形/越界回 416 + `bytes */size`；多段范围回 200 全量；带 Range 时跳过压缩。
- **压缩**：`encodingWanted` 内容协商（见 [12-compress.md](12-compress.md)）；压缩路径全量读入内存，Range 优先于压缩。
- **写出**：正文 `io.Copy`（Linux 自动走 sendfile 零拷贝）；h2 时 body 交协议层；配置了压缩的响应带 `Vary: Accept-Encoding`。
- **MIME**：`config/mime.json` 映射，未命中回落 `application/octet-stream`。

## 测试

`modules/static/`：body/压缩协商/条件请求/Range/穿越/目录索引（traversal_test、negotiation_test、conditional_test）。

## 已知限制

- 不支持 ETag/If-None-Match（仅 Last-Modified 系）。
- root 内符号链接指向外部未拦截（对应 nginx disable_symlinks，需单独立项）。
