# Fast-Https 与 Nginx 核心功能对比

版本：2026-10-13 初稿，2026-11-09 按当前仓库代码修订。对照的是站点日常会用到的能力。本文只做差距和建议，不改服务器。

Nginx Plus 的主动健康检查、动态 API，以及 mail、perl、xslt、image_filter、mp4、dav 这类专用模块，不放进“核心缺口”。应用防火墙和独立 Monitor 进程是本服务多出来的能力，也不算 Nginx 开源核心的对应项。

## 1. 结论

Fast-Https 已经能当一台小型 Web 服务器用：多端口监听、静态文件、TLS、HTTP/1.1、HTTP/2、HTTP/HTTPS 反向代理、WebSocket 升级转发、TCP 字节转发、手工重载、JSON 日志、黑名单、全局限流、Basic 认证，以及 `http-01` / `tls-alpn-01` 的证书申请与续期。默认配置是本地静态站，不是反向代理集群。

它不需要长成 Nginx 的模块集合。

配置预检、停止时收口连接、`HEAD`、分块请求体、按 `Accept-Encoding` 压缩静态正文（gzip 与 brotli）、404 和上游拨号失败的可选错误页，已在 2026-10-13 落地。其后又补上了静态 brotli 协商、`config validate`、本机 `GET 127.0.0.1:9088/runtime` 运行快照，以及读取这份快照的 CLI 与 Monitor。

上游故障转移、自动重载、HTTP/3、FastCGI 和配置表达式语言仍不建议为了“对齐 Nginx”去做。拨号没有超时，这是缺陷，单独补，不放进暂缓。

## 2. 已经有的对应关系

| Nginx 里常见的能力 | Fast-Https 现状 |
| --- | --- |
| `server` / `listen` / `server_name` / `location` | JSONC（注释和尾随逗号）。`listen` 可以是数字、字符串或数组，数组在加载时展开成多个运行期 server。`ssl`、`h2`、`tcp` 选择 TLS、HTTP/2 或 TCP 转发。`server_name` 可空格分隔多个域名，共用 location 和证书。`location.url` 编译成正则，按起点命中和更长匹配排序，没有 `=` / `^~` |
| `root` / `index` / `try_files` | `local` 按 `root`、`index` 和 `try` 找文件。`If-Modified-Since` 回 304，单段 `Range` 回 206 / 416，目录缺斜杠回 301。没有目录列表，没有 ETag。以 `.` 开头的路径段直接 404，只放行 `/.well-known/` |
| `limit_except` | `allow_methods` 非空时限定方法，越界回 405；`HEAD` 随 `GET` 放行 |
| `ssl_certificate` | 站点证书可配置，SNI 选证。同一 SSL 端口重载时不重新绑定，下一次握手用新证书。缺省证书文件时启动前会尝试生成自签名证书 |
| `http2` | `listen` 含 `h2` 时协议栈交给 `golang.org/x/net/http2`：ALPN 协商、HPACK、流状态机与流量控制均由它处理，重载时发 GOAWAY 优雅关闭 |
| `proxy_pass` | 每个 location 一个 `http://` 或 `https://`。可配 `proxy_set_header`。按地址复用空闲连接（每地址最多 8 条、空闲约 30 秒回收），第一次写失败换一条新连接再试一次。上游 chunked 响应会解码后再写出。WebSocket 升级后整条连接双向转发，不进连接池 |
| `stream` 里的 TCP 转发 | `listen` 含 `tcp` 时做双向字节转发，10 秒滚动读写期限，不解析协议 |
| `rewrite` / `return` | `rewrite` 类型固定返回 301，`Location` 来自配置 |
| `gzip` / `brotli` | location 的 `zip` 可写 `gzip`、`br` 或两者。静态响应按 `Accept-Encoding` 协商，两者都开时优先 br，压缩后再写 `Content-Length`、`Content-Encoding` 和 `Vary`。带 `Range` 的请求不压缩。代理响应不在本进程里压缩 |
| `auth_basic` | location 可配用户名和口令，口令写在 JSON 里，比较走常数时间 |
| `limit_req` | 每个客户端 IP 一个令牌桶，加上按 location、按 IP 的计数。黑名单支持单 IP 和地址段。计数和封禁用的是连接地址 |
| `add_header` | `security.headers` 在响应出口补安全头。未配置时仍写 `X-Content-Type-Options: nosniff`，HSTS 等必须显式打开 |
| `include` | `http.include` 引入其它 JSON，`conf.d` 下一个文件一个 server |
| `reload` | `reload` 子命令或 `SIGINT`。无效文件保留旧配置和旧监听。另有 `config validate`，只校验、不应用 |
| 访问日志 | `logs/` 下 system、access、error、safe 四份 JSON 文件，另加一路控制台输出。每条带 `kind` 区分来源，时间为 RFC3339。`log_format` 选择访问日志字段子集。重载成功后按 `log_root` 重新打开。文件不轮转 |
| `stub_status` | `status` 只回答进程在不在。连接数、状态码、耗时和上游失败计数在 `GET 127.0.0.1:9088/runtime`，`runtime stats` 读取同一份快照。不是 Nginx 文本格式 |
| `error_page` | 可选 `error_page.not_found` 与 `error_page.proxy`。前者替换静态 404，后者替换上游拨号失败时的 500 正文。文件不可读时用默认响应。路由没有命中 location 时的 404 仍用默认正文。读写超时直接关连接，不走错误页 |

## 3. 核心缺口

“建议”只回答现在要不要做。暂缓表示场景出现后再开，不做表示不纳入产品边界。

| 能力 | Nginx 的作用 | 当前差距 | 建议 |
| --- | --- | --- | --- |
| 配置检查 | `nginx -t` 只校验，不监听 | `start` 与 `reload` 都先校验，失败不监听、不惊动正在服务的进程。`config validate` 可单独校验当前配置或 `--candidate` 目录 | 已落地 |
| 优雅停止 | `nginx -s quit` 停止接受并等在途请求 | `SIGTERM` / `SIGQUIT` 停接受并最多等 5 秒；Unix 上 `stop` 发 `SIGTERM` 走同一条优雅停机路径。Windows 没有信号语义，直接终止进程 | 已落地 |
| `HEAD` | 只返回头，不返回正文 | 保留 `Content-Length`，不写正文 | 已落地 |
| 分块请求体 | `Transfer-Encoding: chunked` 的 POST 能读完 | 按块解码，错误或超限关闭连接。`Content-Length` 路径不变。转发到上游前改成带长度的正文 | 已落地 |
| 按编码压缩正文 | 看 `Accept-Encoding`，压缩后再写 `Content-Length` | 静态文件按 location 的 `zip` 与客户端接受的编码压缩，gzip 与 brotli 都在响应路径上。代理响应不压，也没有 `.gz` / `.br` 预压缩直发 | 在线压缩已落地。代理压缩和预压缩文件暂缓 |
| 错误页 | `error_page` 把指定状态码指到本站页面 | 只有静态 404 和上游拨号失败两处。超时、其它 5xx、未命中 location 的 404 不使用它 | 已落地子集。不扩成任意状态码 |
| 运行计数 | `stub_status` 看连接和请求 | 本机回环 JSON 快照含连接、请求、2xx/3xx/4xx/5xx、耗时直方图和上游失败计数。`status` 仍只回答进程是否存活。限流与黑名单拒绝次数仍在进程内 `Snapshot` | 已落地自有接口。不做 Nginx 兼容文本 |
| 上游故障转移 | `upstream` 里多个地址，拨号失败试下一个 | 一个 `proxy_pass`。连接池按这一个地址复用。这就是迭代计划里的 R8，计划写于 2026-11-02，代码未做 | 暂缓 |
| 重写引擎 | `rewrite` 正则替换，`last` / `break`，多种状态码 | 只有固定 301 | 暂缓完整引擎。301 保持现状 |
| 代理超时和缓冲 | `proxy_connect_timeout`、`proxy_read_timeout`、缓冲开关 | 连接建立后有固定 20 秒读写 deadline，不能按 location 配置。拨号用 `net.Dial`，没有超时，上游不响应时受操作系统 TCP 超时约束。超时关连接，不回错误页。响应整段读入后再写出 | 完整配置暂缓。拨号超时应单独补 |
| 上游 TLS 校验 | `proxy_ssl_verify` 校验 HTTPS 上游证书 | HTTPS 上游固定 `InsecureSkipVerify: true` | 暂缓。现有自签上游依赖跳过校验，不默认改成严格 |
| 可信代理和真实 IP | `real_ip`、`set_real_ip_from` | 限流和黑名单使用连接上的地址。`$remote_addr` 也是这个地址。发往上游的 `X-Forwarded-For` 只是在原值后追加连接地址 | 有反代层时必须做，纯直连可等 |
| 代理缓存 | `proxy_cache`、过期、陈旧响应、清理 | 按 key 落盘，内存热层 LRU（4096 条 / 64MB）。已支持状态码到有效期的匹配、命中回放、`methods` 过滤、`Set-Cookie` 剥离与 `max_size` 软上限。磁盘条目没有 LRU，超限丢弃新条目。仍缺请求合并和 purge | 按需补缺项，不整体重写 |
| 证书自动申请 | 常配合外部 ACME 客户端 | `acme` 配置段内置 ACME：后台申请与续期、签发成功热替换、失败回落自签。只实现 `http-01` 与 `tls-alpn-01` | 已落地。`dns-01`（通配符）仍用外部客户端 |
| HTTP/3 | `http3` / QUIC | 没有 | 暂缓 |
| 二进制热升级 | 新可执行文件接手监听，旧进程退出 | 能热更新配置和同一端口的证书，不能热替换二进制 | 暂缓 |
| 自动重载 | 有人用外部工具监视文件 | 没有配置监视。原先未调用的 `watchConfigChanges` 已删除 | 已决定不做。继续手工重载 |
| 多进程 worker | `worker_processes`、`accept_mutex`、共享内存 | 每个连接一个协程，没有 worker 进程模型和共享内存 | 不做 |
| FastCGI、uWSGI、SCGI、gRPC | 把请求交给应用进程或 gRPC 后端 | 没有。HTTP 反向代理可以转到本机 HTTP 端口 | 不做 |
| `auth_request` | 用子请求问认证服务 | 只有配置文件里的 Basic 口令 | 不做 |
| 配置变量语言 | `set`、`map`、`if`、`geo` | JSON 字段，没有表达式。`proxy_cache.key`、`proxy_set_header` 里的 `$变量` 只替换固定名字（`$remote_addr`、`$request_uri`、`$host`、`$request_method`、`$proxy_host`、`$proxy_add_x_forwarded_for`） | 不做 |
| WebDAV、SSI、目录列表、地理、切流、视频 | 专用模块 | 没有 | 不做 |
| UDP、`proxy_protocol`、邮件 | `stream` 的其余部分和 mail | TCP 转发是字节管道 | 不做 |

## 4. 已落地的六项

2026-10-13 已落地。现有 JSON 字段名不变。`error_page` 是可选新字段。

1. **配置校验。** 调用 `config.ValidateConfigFile`。`start` 在预检阶段做，`reload` 在发信号之前做；失败把校验错误返回给命令行、以非 0 退出，且不监听、不写 pid、不影响正在服务的进程。其后增加的 `config validate` 用同一套校验，可以检查当前配置或一棵候选目录，不应用、不重载。
2. **优雅停止。** `SIGTERM` 和 `SIGQUIT` 取消各监听上下文，不再接受新连接，并最多等待 5 秒。已跟踪的 HTTP/1.1 和 HTTP/2 处理协程退出后 `Run` 返回。时限到了也退出。Unix 上 `stop` 发送 `SIGTERM`，走同一条路径并在退出后清理 pid 文件；Windows 没有信号语义，直接终止进程。
3. **`HEAD`。** `Event.ResponseBytes` 和静态文件写出都保留 `Content-Length`、`Content-Type`，不写正文。
4. **分块请求体。** `Transfer-Encoding: chunked` 由 `request.ReadChunked` 解码后再进入处理器。块格式错误或解码后超过 `max_body_size` 时关闭连接。带 `Content-Length` 的请求仍按长度读取。
5. **压缩正文。** location 的 `zip` 含 `gzip` 和/或 `br`，且 `Accept-Encoding` 接受对应编码时，静态正文压缩后再写 `Content-Length` 和 `Content-Encoding`。两者都配置时优先 br，否则回退 gzip。配置了压缩的响应带 `Vary: Accept-Encoding`。客户端不接受、或请求带 `Range` 时不压缩、不写 `Content-Encoding`。
6. **错误页。** location 可写 `error_page.not_found` 和 `error_page.proxy`。文件可读时分别替换静态 404 和上游拨号失败的正文。文件缺失或为空时保持原来的默认响应。路由没有命中 location 时的 404 仍用默认正文。

对应测试：`TestValidateConfigAcceptsValidConfig`、`TestValidateConfigRejectsInvalidConfig`、`TestReloadStopsOnInvalidConfig`、`TestDrainConnectionsClosesInFlightHandler`、`TestResponseBytesOmitsBodyForHead`、`TestStaticHeadOmitsBody`、`TestReadChunkedSingleRead`、`TestStaticGzipOnlyWhenRequested`、`TestNotFoundUsesConfiguredPage`、`TestDialFailureUsesConfiguredPage`。brotli 协商见 `modules/static/negotiation_test.go`。

## 5. 2026-10-13 之后改变差距判断的项

- **运行快照。** `runtime/metrics` 在 `127.0.0.1:9088` 提供 `GET /runtime`。字段含生命周期、连接、请求、状态码、耗时直方图、reload 生成号和上游失败计数。`fast-https runtime status` / `runtime stats` 读这份快照。独立 Monitor 周期采集并写事件，不进请求路径。端口写死，无认证，只在本机回环可达。
- **代理连接池与 WebSocket。** 普通 HTTP 代理按上游地址复用连接。`Upgrade: websocket` 的请求握手后整条双向转发，会话不进池。这两项缩小的是单上游代理的可用性，不改变“一个 `proxy_pass`”的边界。
- **静态条件请求与 Range。** 304、单段 206 / 416、目录补斜杠 301 已在静态路径上。没有 ETag，没有目录列表。

## 6. 建议暂缓

- **R8 多上游。** 默认站点没有 `proxy_pass`。单地址代理在上游拒绝连接时失败，这是当前产品的正常结果。有两台可互换后端时再做“拨号失败试下一个”。健康检查和权重仍放在那之后。
- **完整 rewrite。** 内部跳转、正则替换和多种状态码会碰到路由循环。现有 301 够用。
- **代理超时的完整配置、HTTP/3、二进制热升级、代理响应压缩、预压缩文件、任意状态码的错误页、上游证书校验。** 都有使用场景，当前发布不依赖它们。拨号超时是缺陷，单独补，不在暂缓之列。
- **代理缓存剩余缺项。** 请求合并、purge，以及磁盘条目的淘汰，按需补，不整体重写已有实现。
- **真实 IP。** 只要前面挂 CDN 或负载均衡就必须做：现在限流和黑名单取的是连接地址，多层代理下会把所有客户端识别成同一个 IP，按 IP 限流失效、黑名单会误封。没有代理层时可以等。

## 7. 建议不做

多进程模型和共享内存是 Nginx 的实现方式。本服务用协程接受连接，再复制一套 worker 不会提高正确性。

FastCGI、uWSGI、SCGI 和 gRPC 是另一类后端协议。应用可以自己听 HTTP，再由现有 `proxy_pass` 转发。

`auth_request`、`set` / `map` / `if`、SSI、目录列表、WebDAV、UDP 和邮件会把配置和请求路径变成第二套产品。自动重载已经在 2026-10-09 决定保持手工触发，监视代码已删除。

`stub_status` 的文本格式不做。连接和状态码看 `/runtime`。

## 8. 和现有计划的关系

[iteration-plan.md](develop/archive/iteration-plan.md) 里 R1 到 R6 已落地，R7 暂缓，R8 未做。第 4 节的六项已落地，不改那张状态表。R8 的实施草案在 [upstream-failover-plan.md](develop/plans/upstream-failover-plan.md)，状态仍是未实施。

启动和请求路径以 [about.md](about.md) 为准。产品边界以 [product-design-latest.md](product-design-latest.md) 为准；该文第 2 节仍写着没有连接数和状态码的对外接口，这部分以本文和 [20-runtime.md](feature/20-runtime.md) 为准。各能力的用法和限制以 [feature/](feature/README.md) 为准。
