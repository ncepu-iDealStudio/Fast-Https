# Fast-Https 当前产品设计

本文只描述仓库里已经实现、并且由测试守住的行为。版本号以 `config/consts.go` 的 `CURRENT_VERSION` 为准，本文不写死具体版本。

[product-design-2.0.md](product-design-2.0.md) 是另一份未来设计稿。那里的常驻智能体、`fast-https agent` 对话、快环 / 慢环、冷门启停、`config/agent.json` 和回环状态端口，当前代码里都没有对应实现。读现状以本文为准；功能细节仍以 [feature/](feature/README.md) 各篇为准。

## 1. 产品是什么

Fast-Https 是一个用 Go 写成的单二进制 Web 服务器和反向代理。进程自己完成监听、HTTP/1.1 解析和响应写出。HTTP/2 的分帧、HPACK、流状态机和流量控制交给 `golang.org/x/net/http2`。TLS 使用 Go 标准库。

一份允许注释和尾随逗号的 JSON 描述监听端口和 location。全部功能编译进同一个可执行文件，运行时不加载动态模块，也不调用大模型。

一条请求的处理顺序是：

```text
连接 → 黑名单 / 全局限流 → location 路由 → location 限流 → Basic 认证 → 应用防火墙
     → 静态文件 / 反向代理 / 301 / 开发模块
```

`listen` 带 `tcp` 的连接，以及升级为 WebSocket 的连接，在进入上述 HTTP 管线之前被劫持，做双向字节转发。

## 2. 边界

已经能承担的场景：单机静态站点、TLS 终止、HTTP/2、把 HTTP 或 WebSocket 转到一个上游、TCP 字节转发、热重载配置和同一端口上的证书。

当前代码明确没有这些能力，文档和配置字段都不把它们写成已经生效：

| 没有的能力 | 代码里的实际情况 |
| --- | --- |
| 多上游、健康检查、故障转移 | 每个 location 只有一个 `proxy_pass` |
| rewrite 引擎、`if` / `map` | `type: rewrite` 只按配置里的地址返回 301 |
| FastCGI、uWSGI、gRPC | 没有对应 location 类型 |
| HTTP/3 | 没有 QUIC 监听 |
| 多进程 worker、二进制热升级 | 单进程，每连接一个 goroutine |
| 可信代理、真实客户端 IP | 限流和黑名单用的是连接地址 |
| 运行计数出口 | `status` 只回答进程是否存活。没有连接数、4xx/5xx 的对外接口 |
| 配置文件自动重载 | 只有显式 `reload` 或收到重载信号 |
| 常驻智能体、对话、自主调参 | 没有 `agent` 子命令，没有模型客户端，请求路径不调用模型 |
| 主从引擎 | `engine` 能被 JSON 解析进结构体，没有任何模块读取它 |

## 3. 分层

依赖方向单向向下。`tests/arch/layering_test.go` 拦住反向依赖。

| 层 | 位置 | 职责 |
| --- | --- | --- |
| 组合根 | `fast-https.go`、`cmd/`、`init/` | 命令行、handler 注册、`core.Services` 装配、启动顺序 |
| 功能 | `modules/*` | 静态、代理、TCP、WebSocket、rewrite、缓存、压缩、认证、限流、防火墙、证书编排、站点文件 |
| 契约 | `core/` | `Event`、处理器注册表、生命周期钩子。不依赖 `modules` |
| 数据面 | `runtime/` | 监听、连接、HTTP/1.1 解析、HTTP/2 适配、路由过滤、响应写出、访问日志字段 |
| 配置 | `config/` | JSONC 解析、校验、路径基准、原子快照 |
| 基础设施 | `infra/` | 日志、原子写文件、JSONC 工具、终端颜色、横幅 |

`desktop/` 是另一个 Go module，不 import 主程序的 `config` 包，也不接管服务进程。

共享可变状态在重载时的做法是：建一份新快照，再用 `atomic.Pointer` 整体换上。在途请求继续读旧快照。配置、监听器上的虚拟主机表、访问日志字段表、四路文件日志器都走这条路径。

## 4. 一次请求

`runtime/server` 为每个监听端口起一个 Accept 循环。每接受一个连接就起一个 goroutine，并有 panic 兜底。

HTTP/1.1 在同一连接上按 keep-alive 逐个请求处理：

1. 连接级过滤：`modules/safe` 的 per-IP 令牌桶，然后是 IP 黑名单。拒绝时写安全日志并关掉连接。
2. 监听级分流：TCP 代理或 WebSocket 升级在这里把连接交出去，不再解析后续 HTTP。
3. 解析：请求行和头是零拷贝解析；`Content-Length` 按长度读完；`Transfer-Encoding: chunked` 单独解码。头或体超过 `servers_limit` 的上限时关闭连接。
4. 路由：按 `server_name` 和端口找虚拟主机，再按配置顺序用 `location.url` 的正则匹配。`server_name` 可以写空格分隔的多个域名，它们共用同一组 location 和证书。
5. location 级过滤：计数限流、Basic 认证、应用防火墙。防火墙在 `waf.mode` 为 `monitor` 时只记录。
6. 按 location 的 `type` 从 `core` 的注册表取出处理函数。`cmd.RegisterHandlers` 在监听前登记四类：`local`、`proxy`、`rewrite`、`devmod`。缺登记时返回 500，不空指针崩溃。
7. 写出响应。`HEAD` 保留 `Content-Length`，不写正文。需要复用连接时回到第 3 步。

`listen` 含 `h2` 时，ALPN 协商到 HTTP/2 后由 `x/net/http2` 管帧。每个流再适配成同一个 `core.Event`，后面的路由和处理与 HTTP/1.1 相同。重载时对该连接发 GOAWAY。

`security.host_check` 默认 `off`：Host 对不上时回落到该端口的第一个 server。`warn` 只记录，`enforce` 拒绝。`security.headers` 在响应出口写安全头；未配置时仍然写 `X-Content-Type-Options: nosniff`，HSTS 必须显式打开。

## 5. 配置

主文件默认是 `config/fast-https.json`，只放系统级开关。站点放在 `http.include` 指向的目录里，默认 `config/conf.d/`，一个文件一个站点。解析由 `infra/jsonc` 完成，注释和尾随逗号会被去掉，字段顺序在就地改写时保留。

系统级字段：

| 字段 | 生效方式 |
| --- | --- |
| `log_root` | 四路日志的目录。重载成功后按新值重新打开；新目录打不开则继续写原来的文件 |
| `security` | Host 校验和安全响应头 |
| `waf.mode` | `enforce` / `monitor` / `disabled`。未配置且没有任何 location 写 `appfirewall` 时不构建引擎 |
| `http.include` | 额外的站点 JSON |
| `http.default_type` | 未识别扩展名时的 Content-Type |
| `http.servers_limit` | 头和体的上限，以及全局限流的 `limit` / `burst` |
| `http.blaklist` | 永久黑名单。键名是历史拼写，配置兼容保留 |
| `http.log_format` | 访问日志字段子集 |
| `acme` | 自动签发。缺省或 `enabled: false` 时不申请证书 |
| `pid`、`engine` | 能解析，运行期没有读取方 |

一个站点是一个 `server`：`listen`、`server_name`、可选的证书路径、若干 `location`。`listen` 可以是数字、字符串，或数组。数组会在加载时展开成多个运行期 server，端口和路由表本身不知道「一个块听多个口」。标记写在字符串里：`443 ssl h2`、以及 `tcp`。

location 的 `type`：

| type | 行为 |
| --- | --- |
| `local` | 用 `root`、`index`、`try` 找文件。支持条件请求 304、单段 Range 的 206/416、目录缺斜杠时 301。不生成目录列表。路径穿越和隐藏路径在打开文件前拒绝 |
| `proxy` | 一个 `http://` 或 `https://` 上游。连接池按地址复用，空闲上限 8、空闲约 30 秒回收，第一次写失败换一条新连接再试一次。可配 `proxy_set_header` 和 `proxy_cache`。HTTPS 上游不校验对端证书 |
| `rewrite` | 固定 301，`Location` 来自配置里的 `rewrite` |
| `devmod` | 开发用的 location 处理 |

`zip` 只作用于静态响应：按 `Accept-Encoding` 协商 gzip 或 brotli，压缩后再写 `Content-Length` 和 `Vary`。Range 请求优先于压缩。代理响应不在本进程里压缩。

`error_page.not_found` 和 `error_page.proxy` 可选。文件可读时分别替换静态 404 和上游拨号失败的正文。路由没匹配到 location 时的 404 仍是默认正文。

`fast-https site` 负责 `conf.d` 下站点文件的生成、查看、校验和删除。改已有文件时按路径就地替换已知字段，不丢掉文件里还有、但命令没建模的字段。

相对路径在加载时变成绝对路径，基准由编译 tag 决定，不在运行时猜测安装方式：

| 构建 | tag | 配置与证书 | 站点与错误页 | 日志 | pid |
| --- | --- | --- | --- | --- | --- |
| 归档、Docker、Windows、macOS | 无 | `./config` | `.` | `./logs` | `.` |
| deb、rpm、apk | `-tags=rpm` | `/etc/fast-https` | `/usr/share/fast-https` | `/var/log/fast-https` | `/run/fast-https` |

`fast-https config path --json` 输出这四个目录的绝对路径。

## 6. 进程与命令

入口是根目录的 `fast-https.go`，只执行 `cmd.RootCmd()`。无参数等同 `start`。帮助默认中文，可用语言开关切到英文。

| 命令 | 行为 |
| --- | --- |
| `start` | 先校验配置、扫描本进程已有监听、确认没有另一份活着的 pid。Unix 上在容器和 systemd 之外会重新 exec 自己转到后台，父进程等到子进程报告端口已听。真正服务的进程写 pid，然后初始化、登记 handler、监听 |
| `stop` | Unix 发 `SIGTERM`，最多等待优雅停机时限后再清 pid。没有 pid 或进程已不在时视为已经停止。Windows 没有这套信号，直接结束进程 |
| `reload` | 发信号之前用同一套校验检查配置。校验失败直接把原因打到终端，不惊动正在服务的进程。服务没在跑时只报告配置没问题 |
| `status` | 区分没有 pid 文件、pid 已过期、进程仍在。不报告连接数和状态码 |
| `version` | 打印 `fast-https` 和 `CURRENT_VERSION` |
| `acme` | 交互式或参数式写入顶层 `acme` 段，保留文件里的注释和其它字段。`acme show` 查看当前配置和将要申请的域名，密钥打码 |
| `config path` | 打印四个运行期绝对路径 |
| `site` | `init` / `list` / `show` / `check` / `remove` |

没有独立的 `check` 子命令。`start` 和 `reload` 都先走 `config.ValidateConfigFile`。也没有 `install` / `uninstall`：安装和卸载交给系统包。

启动顺序在 `init.InitSystem`：读配置和 MIME、打开四路日志、准备本地 CA 并给缺少文件的 SSL 站点生成自签证书、从磁盘恢复代理缓存。限流、黑名单和防火墙不在这一步建，它们挂在 `core.Services.AfterListen`，监听器创建之后和每次重载成功之后各跑一次。

`SIGTERM` / `SIGQUIT` 停止 Accept，并最多等待 5 秒让已跟踪的处理协程退出。`SIGINT` 在各平台触发进程内重载。`reload` 子命令在 Unix 上发的就是这个信号；Windows 上依次尝试 Ctrl+Break 和 Ctrl+C。

重载成功时：

1. 新配置校验通过后整体替换快照。失败则旧配置和旧监听都保持不动。
2. 按 `log_root` 重新打开四路日志。
3. 对比端口：新增的开始 Accept，去掉的关闭，端口和监听类型都没变的复用原 socket，只换路由表。
4. 同一 SSL 端口不重新绑定。新证书能解析时，下一次握手用新证书。证书文件缺失则这次重载被拒绝。
5. 再次执行 `AfterListen`：重建限流和黑名单底座、按需重建防火墙、按 `acme` 段差异决定是否重建证书申请任务。

控制面在运行时改过的黑名单和全局限流，会在这一步重建之后再套回去。见第 8 节。

## 7. 证书

没配 ACME 时，SSL 站点缺少证书文件会在启动前用本地 CA 生成自签证书，服务照常起来。

`acme.enabled` 为真且写了邮箱之后，`modules/certmanager` 用 certmagic 和 acmez 做申请、续期和 PEM 落盘。默认先 Let's Encrypt，失败再 ZeroSSL，都失败则回落自签并记日志，已有监听不中断。挑战只有 `http-01` 和 `tls-alpn-01`：前者走 `core.Services.HandleEarlyRequest`，在 location 匹配之前应答；后者由监听器在握手时查控制面注册的临时证书。两者都不必为挑战另开端口。`dns-01` 和通配符不在进程内实现。

改 `acme` 段后 `reload` 即可。配置没有差异时不打断正在跑的续期调度。签发成功的证书覆盖 PEM 之后，在原 socket 上热替换。

## 8. 安全与进程内控制面

数据面上的拒绝都在 `modules/safe`：

- 黑名单：单 IP，或 `起-止` 的地址段。段会展开，过大的段直接拒绝，避免一次展开耗尽内存。命中写 `reason=blacklist`。
- 全局限流：每个客户端 IP 一个令牌桶，桶之间不共享突发额度。空闲桶会清扫。命中写 `reason=rate_limit`。
- location 限流：按 location 编号和客户端 IP 做秒级计数，允许次数与配置的 `rate` 一致。命中写 `reason=location_limit` 并返回 429。

应用防火墙是 GuardianWAF，由 location 的 `appfirewall` 选择规则类别。构建失败时保留上一份引擎，不让这次重载把防护变成空的。

除此之外，`modules/safe/control.go` 提供一组进程内函数，给后续的外部工具调用。它不是网络服务，也没有对应的子命令：

| 函数 | 作用 |
| --- | --- |
| `AddBlacklist` / `RemoveBlacklist` / `ListBlacklist` | 立刻改黑名单。来源标成 `runtime`，和配置文件里的永久项分开 |
| `SetGlobalRateLimit` / `GetGlobalRateLimit` | 立刻改全局 per-IP 的速率和突发 |
| `Snapshot` | 采样检查次数、黑名单拒绝次数、全局限流拒绝次数、当前黑名单和限流参数 |

这些运行时变更记在内存里的 overlay 中。`reload` 用配置文件重建黑名单和限流之后，overlay 会再应用一遍，所以热重载不会把它们冲掉。变更写安全日志，`reason=control`。要落盘或在进程重启后回放，由调用方自己做，服务器不写回 JSON。

`Snapshot` 里的 `LocationRejected` 字段目前没有写入点，读出来是 0。location 限流的拒绝只进安全日志。

这组函数不进入 Accept、请求解析和响应写出。连接路径上用的仍是上面三道数据面检查。

## 9. 日志

`infra/logging` 是唯一出口，基于 zerolog，每条都是一行 JSON。

| 输出 | kind | 去向 |
| --- | --- | --- |
| 控制台 | `console` | 标准输出，默认 info |
| 系统 | `system` | `system.log` |
| 访问 | `access` | `access.log`，字段由 `http.log_format` 选择 |
| 错误 | `error` | `error.log` |
| 安全 | `safe` | `safe.log`，拒绝和防火墙带 `reason` |

时间是带毫秒的 RFC3339。访问日志的 `status` 和 `size` 是 JSON 数字。每个文件由单独的 writer 串行写出，避免并发把两行拼到一起。

## 10. 交付

同一次源码要产出五种运行形态：源码直跑、归档包、Docker 镜像、Windows 安装包、Linux 的 deb / rpm / apk。路径差异只来自第 5 节的编译 tag。

桌面管理端是 Wails v3 的独立程序：单窗口，关窗可以收到托盘。它读写站点文件、调用主程序的命令行，不把请求处理放进界面进程。Windows 安装包和 Linux 的 desktop 包可以带上它；图形程序不替代 `fast-https` 服务。

## 11. 和未来设计稿的分界

[product-design-2.0.md](product-design-2.0.md) 规划的是另一个控制进程：多轮对话改配置、按错误率和连接数自动调参、等到没人访问再停机拉起。那些模块、命令、配置文件和状态端口都还没有。

当前已经有的、可以被外部工具使用的只有第 8 节的进程内函数：加删黑名单、调全局限流、采样安全计数。外部工具如果要做，调用的是这些函数所表达的语义，而不是在请求路径里嵌模型，也不是再写一套直接改内存、绕过重载的配置入口。热更新配置仍然只有 `reload` 这一条路：先校验，失败就留着旧的。
