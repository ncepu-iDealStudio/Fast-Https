# Fast-Https 与 Nginx 核心功能对比

版本：2026-10-13。对照的是当前仓库代码和 Nginx 开源版里站点日常会用到的能力。本文只做差距和建议，不改服务器。

Nginx Plus 的主动健康检查、动态 API，以及 mail、perl、xslt、image_filter、mp4、dav 这类专用模块，不放进“核心缺口”。

## 1. 结论

Fast-Https 已经能当一台小型 Web 服务器用：多端口监听、静态文件、TLS、HTTP/1.1、HTTP/2、HTTP/HTTPS 反向代理、TCP 字节转发、手工重载、四份日志、黑名单、全局限流和 Basic 认证。默认配置是本地静态站，不是反向代理集群。

它不需要长成 Nginx 的模块集合。
配置预检命令、停止时收口连接、`HEAD`、分块请求体、按 `Accept-Encoding` 压缩正文、以及 404 和上游拨号失败的可选错误页，已在 2026-10-13 落地。
上游故障转移、自动重载、HTTP/3、FastCGI 和配置表达式语言仍不建议为了“对齐 Nginx”去做。

## 2. 已经有的对应关系

| Nginx 里常见的能力 | Fast-Https 现状 |
| --- | --- |
| `server` / `listen` / `server_name` / `location` | JSON 的 `http.server` 和 `location`。`listen` 里的 `ssl`、`h2`、`tcp` 选择 TLS、HTTP/2 或 TCP 转发。`location.url` 编译成正则 |
| `root` / `index` / `try_files` | `local` 类型按 `root`、`index` 和 `try` 找文件。目录没有自动列表 |
| `ssl_certificate` | 站点证书可配置。同一 SSL 端口重载时不重新绑定，下一次握手用新证书。缺省证书文件时启动前会尝试生成自签名证书 |
| `http2` | `listen` 含 `h2` 时走自带的帧、流和 HPACK。未知帧丢掉后续读，非法 HPACK 索引关闭该流 |
| `proxy_pass` | 每个 location 一个 `proxy_pass`，支持 `http://` 和 `https://`。可配 `proxy_set_header` 和一份磁盘缓存 |
| `stream` 里的 TCP 转发 | `listen` 含 `tcp` 时做双向字节转发，注释写明是简单演示 |
| `rewrite` / `return` | `rewrite` 类型固定返回 301，`Location` 来自配置 |
| `gzip` | location 可写 `gzip` / `br`。静态响应会加上 `Content-Encoding: gzip`，正文仍按原文件字节写出 |
| `auth_basic` | location 可配用户名和口令，口令写在 JSON 里 |
| `limit_req` | 全局令牌桶，加上按 location、按 IP 的计数。黑名单支持单 IP 和地址段 |
| `include` | `http.include` 引入其它 JSON，每个文件描述一个 server |
| `reload` | `reload` 子命令或 `SIGINT`。无效文件保留旧配置和旧监听 |
| 访问日志 | `logs/` 下 system、access、error、safe 四份文件。重载成功后按 `log_root` 重新打开 |
| 进程状态 | `status` 只回答进程在不在，不报告连接数和状态码 |

## 3. 核心缺口

“建议”只回答现在要不要做。暂缓表示场景出现后再开，不做表示不纳入产品边界。

| 能力 | Nginx 的作用 | 当前差距 | 建议 |
| --- | --- | --- | --- |
| 配置检查命令 | `nginx -t` 只校验，不监听 | `check` 调用现有校验，不监听、不写 pid | 已落地 |
| 优雅停止 | `nginx -s quit` 停止接受并等在途请求 | `SIGTERM` / `SIGQUIT` 停接受并最多等 5 秒。`stop` 仍发送 `Kill` | 已落地 |
| `HEAD` | 只返回头，不返回正文 | 保留 `Content-Length`，不写正文 | 已落地 |
| 分块请求体 | `Transfer-Encoding: chunked` 的 POST 能读完 | 按块解码，错误或超限关闭连接。`Content-Length` 路径不变 | 已落地 |
| 按编码压缩正文 | 看 `Accept-Encoding`，压缩后再写 `Content-Length` | 静态文件在 location 打开 gzip 且客户端接受时压缩正文 | 已落地 |
| 错误页 | `error_page` 把 404、502 指到本站页面 | 可选 `error_page.not_found` 与 `error_page.proxy`。文件不可读时用默认响应 | 已落地 |
| 运行计数 | `stub_status` 看连接和请求 | 没有连接数、4xx/5xx、限流拒绝计数。2.0 设计依赖这些数 | 随 2.0 做，不单独对齐 `stub_status` 文本 |
| 上游故障转移 | `upstream` 里多个地址，拨号失败试下一个 | 一个 `proxy_pass`。这就是迭代计划里的 R8 | 暂缓 |
| 重写引擎 | `rewrite` 正则替换，`last` / `break`，多种状态码 | 只有固定 301 | 暂缓完整引擎。301 保持现状 |
| 代理超时和缓冲 | `proxy_connect_timeout`、`proxy_read_timeout`、缓冲开关 | 上游连接有固定 20 秒期限，不能按 location 配置 | 暂缓。先保持一个固定期限 |
| 可信代理和真实 IP | `real_ip`、`set_real_ip_from` | 限流和黑名单使用连接上的地址 | 暂缓。出现多层代理再做可信网段 |
| 代理缓存 | `proxy_cache`、过期、陈旧响应、清理 | 已有按 key 落盘的缓存，没有 Nginx 那套锁和清理接口 | 暂缓。不重写 |
| 证书自动申请 | 常配合外部 ACME 客户端 | `shell/.acme.sh` 在仓库里，启动流程不调用 | 暂缓。继续用外部客户端写出 pem |
| HTTP/3 | `http3` / QUIC | 没有 | 暂缓 |
| 二进制热升级 | 新可执行文件接手监听，旧进程退出 | 能热更新配置和同一端口的证书，不能热替换二进制 | 暂缓 |
| 自动重载 | 有人用外部工具监视文件 | `watchConfigChanges` 没有调用方 | 已决定不做。继续手工重载 |
| 多进程 worker | `worker_processes`、`accept_mutex`、共享内存 | 每个连接一个协程。`modules/core/engine` 的 master/slave 不在启动路径上 | 不做 |
| FastCGI、uWSGI、SCGI、gRPC | 把请求交给应用进程或 gRPC 后端 | 没有。HTTP 反向代理可以转到本机 HTTP 端口 | 不做 |
| `auth_request` | 用子请求问认证服务 | 只有配置文件里的 Basic 口令 | 不做 |
| 配置变量语言 | `set`、`map`、`if`、`geo` | JSON 字段，没有表达式 | 不做 |
| WebDAV、SSI、目录列表、地理、切流、视频 | 专用模块 | 没有 | 不做 |
| UDP、`proxy_protocol`、邮件 | `stream` 的其余部分和 mail | TCP 转发是字节管道 | 不做 |

## 4. 已落地的六项

2026-10-13 已落地。现有 JSON 字段名不变。`error_page` 是可选新字段。

1. **`check`。** 调用 `config.ValidateConfigFile`。成功打印 `config ok` 并以 0 退出。失败把校验错误返回给命令行，进程以非 0 退出。不监听，不写 pid。`start` 预检仍做同一项校验。
2. **优雅停止。** `SIGTERM` 和 `SIGQUIT` 取消各监听上下文，不再接受新连接，并最多等待 5 秒。已跟踪的 HTTP/1.1 和 HTTP/2 处理协程退出后 `Run` 返回。时限到了也退出。`stop` 仍发送 `Kill`。
3. **`HEAD`。** `Event.ResponseBytes` 和静态文件写出都保留 `Content-Length`、`Content-Type`，不写正文。
4. **分块请求体。** `Transfer-Encoding: chunked` 由 `request.ReadChunked` 解码后再进入处理器。块格式错误或解码后超过 `max_body_size` 时关闭连接。带 `Content-Length` 的请求仍按长度读取。
5. **gzip 正文。** location 的 `zip` 为 `gzip` 或同时包含 `gzip` 与 `br`，且 `Accept-Encoding` 接受 gzip 时，静态正文压缩后再写 `Content-Length` 和 `Content-Encoding: gzip`。其它情况不写 `Content-Encoding`。brotli 库仍在，响应路径这一轮不使用它。
6. **错误页。** location 可写 `error_page.not_found` 和 `error_page.proxy`。文件可读时分别替换静态 404 和上游拨号失败的正文。文件缺失或为空时保持原来的默认响应。路由没有命中 location 时的 404 仍用默认正文。

对应测试：`TestCheckFileAcceptsValidConfig`、`TestCheckFileRejectsInvalidConfig`、`TestDrainConnectionsClosesInFlightHandler`、`TestResponseBytesOmitsBodyForHead`、`TestStaticHeadOmitsBody`、`TestReadChunkedSingleRead`、`TestStaticGzipOnlyWhenRequested`、`TestNotFoundUsesConfiguredPage`、`TestDialFailureUsesConfiguredPage`。

## 5. 建议暂缓

- **R8 多上游。** 默认站点没有 `proxy_pass`。单地址代理在上游拒绝连接时失败，这是当前产品的正常结果。有两台可互换后端时再做“拨号失败试下一个”。健康检查和权重仍放在那之后。
- **完整 rewrite。** 内部跳转、正则替换和多种状态码会碰到路由循环。现有 301 够用。
- **代理超时配置、真实 IP、代理缓存增强、ACME、HTTP/3、二进制热升级。** 都有使用场景，但当前发布不依赖它们。
- **运行计数。** 2.0 智能体需要连接数、4xx/5xx 和限流拒绝。做到 2.0 的状态接口里，不必先做一份 Nginx `stub_status` 兼容文本。

## 6. 建议不做

多进程模型和共享内存是 Nginx 的实现方式。本服务用协程接受连接，再复制一套 worker 不会提高正确性。

FastCGI、uWSGI、SCGI 和 gRPC 是另一类后端协议。应用可以自己听 HTTP，再由现有 `proxy_pass` 转发。

`auth_request`、`set` / `map` / `if`、SSI、目录列表、WebDAV、UDP 和邮件会把配置和请求路径变成第二套产品。自动重载已经在 2026-10-09 决定保持手工触发。

## 7. 和现有计划的关系

[iteration-plan.md](iteration-plan.md) 里 R1 到 R6 已落地，R7 暂缓，R8 未做。第 4 节的六项已落地，不改那张状态表。

启动和请求路径以 [about.md](about.md) 为准。2.0 的对话式运维以 [product-design-2.0.md](product-design-2.0.md) 为准，那里的运行计数仍留在 2.0。
