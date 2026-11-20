# 全量安全漏洞与程序缺陷修复计划

修订日期：2026-11-20。状态：**待实施**。

本文记录对 `fast-https` 全仓库（`runtime/`、`core/`、`modules/`、`config/`、`cmd/`、`management/`、`init/`、`infra/`、`desktop/`）进行深度代码审计发现的 **30 项安全漏洞与高危程序缺陷**，并给出分阶段的修复方案、改动边界与验收标准。

实施原则：
- **不破坏分层约束**：`core` 只放契约，`runtime` 不反向依赖 `modules`，`management` 不依赖 `runtime`/`core`（由 `tests/arch` 守卫）。
- **不留转发壳与兼容层**（见 `AGENT.md` 硬规则 7）。
- **每批次独立提交与验证**：每完成一个阶段，必须跑通 `go build ./... && go vet ./...`、`go vet -tags=rpm ./...`、`go test -race ./...`，涉及桌面端时同步跑 `cd desktop && go test ./internal/...`。

---

## 1. 漏洞与缺陷总览

| 编号 | 级别 | 所属模块 | 漏洞/缺陷简述 | 位置 |
| --- | --- | --- | --- | --- |
| S-01 | Critical | `runtime/request`, `runtime/events` | HTTP 请求走私：`Transfer-Encoding: chunked;q=0` 绕过分块解码（TE.CL 走私）、重复 `CL`/`TE` 覆盖、无 `CL` 请求夹带 Pipelining 包体转发、前导 `\r\n\r\n` 将请求头拼入 Body 并绕过 `max_header_size` | `runtime/request/chunked.go:32-63`, `runtime/request/parse.go:357-383,483-528`, `runtime/events/events.go:253-342` |
| S-02 | Critical | `modules/proxy`, `modules/cache` | 代理缓存投毒与跨用户越权泄露：`HEAD` 响应将 `GET` 缓存投毒为空包体、`Touch` 读即续期导致永久驻留、忽略 `Authorization`/`Cookie` 与上游 `Cache-Control: private/no-store`、缓存键未区分 `Accept-Encoding` | `modules/proxy/cache.go:38-121,160-171`, `modules/cache/cache_index.go:39-48`, `modules/cache/cache.go:237-267,383-409` |
| S-03 | Critical | `cmd/internal/cachecmd`, `desktop/internal/cacheops`, `config/internal/validate` | `cache delete --purge` 未限制缓存目录边界，`path: "."` 或 `"./config"` 时递归删空整个安装目录或配置树 | `cmd/internal/cachecmd/cache.go:287-326`, `desktop/internal/cacheops/editor.go:107-115`, `config/internal/validate/validate.go:189-208` |
| S-04 | Critical | `runtime/events`, `modules/safe` | HTTP/2 连接触发黑名单或全局限流时 `connEv.EventWrite` 为 `nil` 导致空指针 Panic 与连接 FD 泄漏；`IsInBlacklist`/`Bucket` 在无 ReadDeadline 下调用 `Conn.Read` 导致空连接永久阻塞协程 | `runtime/events/events_h2.go:51-62`, `modules/safe/blacklist.go:123-144`, `modules/safe/limit.go:115-137` |
| S-05 | Critical | `runtime/events`, `modules/proxy`, `modules/websocket`, `modules/proxy_tcp`, `cmd/internal/servicecmd` | H2 请求反代向上游发送非法 `HTTP/2` 请求行；H1 WebSocket 升级误入 `ReadKeepAlive` 死锁 60 秒且不支持 `Connection: keep-alive, Upgrade`；`wss` 上游在 `WebSocketHandler` 中直接读写裸 TCP `ProxyConn` 破坏 TLS 流；WebSocket/TCP 代理在 `io.EOF` 时泄漏上游 FD；TCP 代理因 `LisType(2) != PROXY_TCP(3)` 永远不生效 | `runtime/events/events_h2.go:215`, `runtime/filters/filters.go:30-38`, `modules/proxy/event_proxy.go:254-313`, `modules/websocket/websocket.go:28-104`, `modules/proxy_tcp/event_proxy_tcp.go:25-100`, `cmd/internal/servicecmd/handlers.go:102` |
| S-06 | High | `runtime/request`, `runtime/filters` | `NormalizePath` 未折叠 `/../` 点段且对非 `/` 起始请求目标（如 `http://...`）跳过所有安全检查，可借 `/public/../admin` 绕过 Location 级鉴权、方法限制与 WAF | `runtime/request/normalize.go:22-56`, `runtime/filters/filters.go:87-123` |
| S-07 | High | `runtime/request`, `runtime/events`, `runtime/response` | `ParseHost` 在空 `Host` 头时跳过 `parseQueryParams()` 且对已带 `:80`/`:443` 的 `Host` 重复拼接端口；H2 `fillRequest` 丢弃多条 `Cookie` 头（仅取 `vs[0]`）；`HttpResParse` 覆盖上游多条 `Set-Cookie` 头 | `runtime/request/parse.go:118-130`, `runtime/events/events_h2.go:218-227`, `runtime/response/response.go:102-144` |
| S-08 | High | `runtime/request` | `RequestBodyValid()` 处理 `multipart/form-data` 未检查 `boundary=` 缺失（`po == -1`），不支持带引号或尾随参数的 `boundary`，且跳过 `Content-Length` 校验导致提前截断走私 | `runtime/request/parse.go:492-528` |
| S-09 | High | `modules/proxy`, `runtime/request` | 反代上游读取无内存上限（`ReadConnectionClose`、上游 `Content-Length`、`ReadChunked` 行缓冲）引发 OOM；`ReadKeepAlive` 负数 `NeedRead` 撞上 `-1..-5` 状态码导致响应丢弃或乱序；`Connection: close` 请求不解码上游 chunked 污染缓存；上游 HTTPS 硬编码 `InsecureSkipVerify: true` 且缺 SNI | `modules/proxy/proxy_read.go:90-106,211-310`, `modules/proxy/event_proxy.go:90-97,200-281`, `runtime/request/chunked.go:72-78` |
| S-10 | High | `modules/static` | `location.url` 带结尾 `/`（如 `"/static/"`）时，访问 `/static/` 陷入 `/static//` 无限 301 循环，访问 `/static/file` 因拼接缺少 `/` 变成 `rootfile` 误报 `path_traversal` 并全部 404 | `modules/static/static.go:645-665` |
| S-11 | High | `init`, `runtime/listener`, `modules/certmanager`, `management/siteops`, `cmd/internal/sitecmd`, `management/view` | 多域名站点（`server_name: "a.com b.com"`）下 `CertInit` 只生成主域名 SAN 证书，而 `listener` 强制加载不存在的 `b.com.pem` 导致整端口 0 张证书可握手；`fillCerts` 拼出含空格文件名；ACME 主域名证书路径与自定义 `ssl_certificate` 脱节 | `init/autocert.go:74-84`, `runtime/listener/listener.go:350-358`, `runtime/listener/certs.go:140-166`, `modules/certmanager/sync.go:24-54`, `management/siteops/siteops.go:950-967`, `cmd/internal/sitecmd/site.go:152-163`, `management/view/site_view.go:60-71` |
| S-12 | High | `modules/safe` | `everyFromRate` 整数除法 `1000/rps` 在 `rate > 1000` 时得 0 导致限流完全失效，`state()` 将 `rate.Inf` 强转 `int` 溢出为 `math.MinInt64`；Location 限流 `cl.num > cl.size` 在 `size=0` 或超容量时直接 `return true` 放行；Reload 直接覆写含锁 `g_list` 触发 `RWMutex` 崩溃；连接级限流/黑名单在 keep-alive 与 H2 流上不按请求复检 | `modules/safe/limit.go:36-81`, `modules/safe/counts.go:47,104-127`, `modules/safe/blacklist.go:111-119` |
| S-13 | High | `management/state`, `infra/files` | `state.Dir.Lock` 跨进程创建锁文件与写入 JSON 之间存在空文件窗口，被并发进程误判损坏并 `os.Remove` 抢锁；同进程多协程凭 `PID` 直接重入绕过互斥；Windows 下 `state.WriteFile` 先 `Remove` 后 `Rename` 存在文件丢失窗口，且固定 `.tmp` 后缀存在并发覆盖 | `management/state/dir.go:130-190`, `management/state/atomic.go:12-31`, `infra/files/atomic.go:16-30` |
| S-14 | High | `desktop/internal/view` | 桌面端 `ImportLocalCA` 在 Windows 下用 `Start-Process -ArgumentList` 未给路径加双引号，安装路径含空格必败且含 `"` 时向提权 `certutil` 注入参数；Linux 下 `pkexec sh -c "cp ..."` 跟随软链接可越权复制 root 敏感文件 | `desktop/internal/view/cert_trust.go:82-111` |
| S-15 | High | `management/cfgver`, `management/txn`, `infra/logging` | `cfgver.copyCerts` 与 `RestoreCerts` 硬编码 `0o644`，将备份和回滚后的 `rootCA.key` 及站点 TLS 私钥设为全局可读；事务写回配置文件强制 `0o644` 暴露明文密码与 EAB 密钥；日志文件以 `0o666` 创建 | `management/cfgver/cfgver.go:231,265,326`, `management/txn/files.go:97`, `infra/logging/logger.go:205` |
| S-16 | High | `cmd/internal/cli` | Windows 配置事务回退到 `restartService` 时，若新配置导致服务启动失败，`Reloader` 与 `Health.Check` 双双返回 `nil`，静默吞掉启动错误并跳过回滚，导致服务停摆且坏配置被提交为 active | `cmd/internal/cli/txn.go:95-127`, `cmd/internal/cli/restart_windows.go:23-57` |
| S-17 | High | `runtime/listener` | `listenTcp` 绑定端口失败时直接调用 `Fatal()` (`os.Exit(1)`)，热重载时若新端口冲突或重绑失败会直接杀死整个运行中服务进程 | `runtime/listener/listener.go:493-512`, `runtime/listener/reload.go:16,76,86` |
| S-18 | Medium | `core` | `GetCommandParsedStr` 遍历随机顺序的 `map[string]string` 逐次 `strings.Replace`，请求路径或 `Host` 中构造 `$proxy_host` 等占位符会被二次展开，泄露内部上游地址或引发缓存键碰撞 | `core/commands.go:15-31` |
| S-19 | Medium | `management/operation` | `operation.Store.Get(id)` 未校验 `id` 格式，直接拼接 `id + ".json"`，可通过 `../` 跨目录读取（甚至在 `"status": "running"` 时覆写）任意 `.json` 文件 | `management/operation/operation.go:230-250,332-334` |
| S-20 | Medium | `modules/appfirewall` | `Allowed(_ *listener.ListenCfg, ev *core.Event)` 忽略 `cfg.AppFireWall`，只要任一 location 或全局块开启 WAF，全站所有虚拟主机的所有 location 均被强制拦截 | `modules/appfirewall/waf.go:87-93`, `modules/appfirewall/mapping.go:19-62` |
| S-21 | Medium | `infra/jsonc` | `deleteMember` 未检查 `{` 与首个成员之间是否存在换行，删除单行 JSON 对象的首个字段时会回退到行首把左花括号 `{` 一并切掉，损坏 JSON | `infra/jsonc/path.go:213-228` |
| S-22 | Medium | `runtime/events` | `readH2Body` 对所有请求（含空 body 的 `GET`）无条件起读协程与 32KB 缓冲；当超时或超限提前返回且 `ch`（容量 2）满时，读协程永久阻塞泄漏 | `runtime/events/events_h2.go:229-302` |
| S-23 | Medium | `runtime/server` | `ServerInit` 调用无参 `signal.Notify(sigchnl)` 订阅全部信号（含 Go 运行时高频抢占信号 `SIGURG`），容量为 1 的通道在高负载下会丢弃 `SIGTERM`/`SIGINT`，且停止时信号协程不退出 | `runtime/server/server.go:70-77` |
| S-24 | Medium | `runtime/request` | `ParseHeader` 对未以 `\r\n` 结尾的末尾半截头行提前调用 `parseHeaderLine`，当 TCP 分片恰好落在 `Content-Length: ` 后时因 `strconv.Atoi("")` 报错误返 400 Bad Request | `runtime/request/parse.go:357-361,457-465` |
| S-25 | Medium | `config/internal/paths`, `config/internal/validate`, `config/internal/loader` | `FirstSymlinkUnder` 在 `filepath.WalkDir` 遇到权限错误等非空 `err` 时立即中止遍历，调用方将 `err != nil` 当作无软链接放行，绕过 `symlink_policy: "check-at-load"` | `config/internal/paths/symlink.go:14-40`, `config/internal/validate/symlink.go:31-33`, `config/internal/loader/convert.go:71-73` |
| S-26 | Medium | `modules/cache` | `writeEntryToDisk` 用 `O_TRUNC` 原地截断覆写缓存文件，与并发冷读 `ReadCache` 竞态导致读到半截 gob 报错并误调 `unindex(key)` 删掉有效索引 | `modules/cache/cache.go:136-149,272-296` |
| S-27 | Medium | `modules/proxy` | `sendToUpstreamServer` 单次调用 `p.Write(req_data)` 未循环写完短写（Short Write）剩余字节 | `modules/proxy/event_proxy.go:315-339` |
| S-28 | Medium | `desktop` | `pruneEmptyProxy` 保存设置时误删 `upstream_timeout` 与 `tunnel_idle_timeout` 且抹掉所有 JSONC 注释；`AddSite` 拆成 3 次非原子 CLI 事务导致中途失败留下半成品站点；`QuitApp` 关窗口时杀死 `fast-https` 服务却遗留 `fast-https-monitor` 孤儿进程 | `desktop/internal/configops/editor.go:164-258`, `desktop/internal/siteops/editor.go:133-243`, `desktop/service.go:68-79`, `desktop/tray.go:67-70` |
| S-29 | Low | `modules/static`, `modules/rewrite`, `modules/dev_mod` | 静态 `206 Partial Content` 响应在访问日志中被硬编码记为 `200`；`rewrite` 与 `devmod` 响应漏记访问日志；`dofind` 中 `try.Files`/`try.Next` 在非 `reject` 策略下缺少 `staticPathInRoot` 兜底校验 | `modules/static/static.go:91-120,340,381`, `modules/rewrite/rewrite.go:15-40`, `modules/dev_mod/dev_mod.go:15-20` |
| S-30 | Low-Medium | `runtime/listener`, `cmd`, `desktop/internal/view`, `management/cfgver` | `loadCerts` 按 `ServerName` 去重导致同域名 RSA+ECDSA 双证书无法同时加载；`fast-https.pid` 缺少 `StartKey` 校验存在 PID 复用误杀风险；`SnapshotWithCerts` 仅按 JSON 哈希去重忽略原位更新的证书文件 | `runtime/listener/certs.go:140-161`, `cmd/internal/procctl/pid.go:31-100`, `desktop/internal/view/state.go:272-289`, `management/cfgver/cfgver.go:153-191` |

---

## 2. 分阶段修复方案

按「先堵高危安全面与崩溃 → 再修核心协议与功能阻断 → 再修管理面/事务一致性 → 最后收口桌面端与可观测性」分为五个阶段（Phase 1 ~ Phase 5）。

### Phase 1：HTTP 协议解析、走私防护与 H2 连接级稳定性（覆盖 S-01, S-04, S-06, S-07, S-08, S-22, S-23, S-24）

#### 1.1 请求走私与帧解析加固（S-01, S-08, S-24）
- **`Transfer-Encoding` 与 `Content-Length` 严格校验（`runtime/request/parse.go`, `runtime/request/chunked.go`, `runtime/events/events.go`）**：
  1. 在 `parseHeaderLine` 中禁止重复的 `Content-Length`（若已存在则回 `InvalidHeaders` 400）；若收到多行 `Transfer-Encoding`，按逗号合并到同一键值（`r.Headers["Transfer-Encoding"] = prev + ", " + val`），避免后一行覆盖前一行。
  2. 新增专用的 `HasTransferEncodingToken(header, token string) bool`（不解析 `;q=0` 质量参数；若存在任何 `Transfer-Encoding` 头但最后一个 transfer-coding 不是 `chunked`，或者含不支持的编码/参数，按 RFC 9112 §6.1 直接回 400/501 拒绝，绝不将含 `Transfer-Encoding` 的原始头与 `Content-Length` 一起透传给上游）。
  3. 修正前导 `\r\n` 处理：`ParseHeader` 剥除开场空行时，在 `parseRequest` 中同步推进 `byte_row` 切片起点（跳过开头的连续 `\r\n`），确保 `headerLen(byte_row)` 和 `ParseBody(byte_row)` 定位到的 `\r\n\r\n` 必定是请求头结束边界，而非开场空行。
  4. 修正 `ParseHeader` 分片尾行逻辑（S-24）：当 `nl := bytes.Index(rest[pos:], crlf)` 为 `< 0`（即最后一段尚无 `\r\n`）时，只检查其中是否含有孤立 `\r`/`\n`，**不再对未收完的半截行提前调用 `r.parseHeaderLine`**，直接返回 `RequestNeedReadMore`；同时每次 `ParseHeader` 重入前清空已写入的 `r.Headers`，避免分片重解析残留半截键值。
  5. 修正 `ParseBody` 与 `RequestBodyValid` 的包体边界：
     - 当请求不含 `Transfer-Encoding: chunked` 且不含 `Content-Length`（或 `Content-Length: 0`）时，`r.Body` 必须为空，不把同一 TCP 包里 `\r\n\r\n` 后的多读字节写进 `r.Body`。
     - 当 `Content-Length: n`（`n > 0`）时，只读取恰好 `n` 字节进入 `r.Body`（若首次 `Read` 读入的字节超过 `n`，截断至 `n` 字节或拒绝夹带数据），并将 `RequestBodyValid` 中的 `r.Body.Len() != n` 改为 `r.Body.Len() >= n`。
     - 修正 `multipart/form-data`（S-08）：若请求已声明合法 `Content-Length`，统一按 `Content-Length` 判定包体读完，不再因包体中途出现 `--` 提前结束读取；若无 `Content-Length`，使用 `mime.ParseMediaType(contentType)` 解析 `params["boundary"]`（自动处理引号、分号参数及缺失 `boundary` 的情况，缺失或为空时返回 400）。
  6. `ReadChunked` 加固（`runtime/request/chunked.go`）：
     - 为单行 chunk size（含 chunk extension）设置最大行长限制（如 4096 字节），超过未见 `\r\n` 直接返回 `ErrChunkedBody`，防止无 `\r\n` 流打爆内存。
     - 当 `n == 0` 时，支持跳过可选的 trailer header 行直至终止空行 `\r\n`。

#### 1.2 路径规范化与 `Host` / `Cookie` 处理（S-06, S-07）
- **`NormalizePath`（`runtime/request/normalize.go`）**：
  1. 对不以 `/` 开头的 `target`（除 `OPTIONS *` 外）：若为 `http://` 或 `https://` 绝对 URI，提取其 path+query 部分再走同一套解码与规范化流程；其余非法形式直接返回 `("", false)`（400 Bad Request）。
  2. 在 `mergeSlashes(decoded)` 之后，按 RFC 3986 §5.2.4 / `path.Clean` 折叠 `.` 与 `..` 段（同时保留原始结尾 `/` 语义），且若 `..` 试图越出根 `/` 则规范化为根下路径或直接拒绝，杜绝 `/public/../admin` 在路由匹配时命中 `/public` 而绕过 `/admin` 策略。
- **`ParseHost` 与多值 `Cookie`/`Set-Cookie`（`runtime/request/parse.go`, `runtime/events/events_h2.go`）**：
  1. 将 `r.parseQueryParams()` 移出 `ParseHost` 的 `if r.Headers["Host"] == ""` 早退分支（或独立在 `ParseHeader` 完成后调用），保证无 `Host` 头的请求也能正确解析 `r.Query`。
  2. `ParseHost` 追加 `:80` / `:443` 前先检查 `Host` 是否已包含端口（注意兼容 IPv6 `[::1]:80`），已带端口时不再重复追加。
  3. HTTP/2 `fillRequest`（`events_h2.go:218-222`）：遍历 `r.Header` 时，对 `Cookie` 头用 `"; "` 连接（`strings.Join(vs, "; ")`），其余多值头用 `", "` 连接，且不允许普通 `r.Header` 覆盖已由 `:authority`（`r.Host`）设置的 `req.Headers["Host"]`。

#### 1.3 HTTP/2 连接级拒绝、Body 读取与信号订阅（S-04, S-22, S-23）
- **`H2HandleEvent` 与 `safe` 连接拒绝（`runtime/events/events_h2.go`, `modules/safe/blacklist.go`, `modules/safe/limit.go`）**：
  1. 在 `H2HandleEvent` 中为 `connEv` 设置安全的 `connEv.EventWrite = func(ev *core.Event, _ []byte) error { return nil }`（H2 握手前直接关闭底层连接即可，不能往 TLS 流里裸写 HTTP/1.1 文本响应），并确保 `ConnFilter` 返回 `false` 时一定关闭 `conn`。
  2. 移除 `IsInBlacklist`（`blacklist.go:136-137`）和 `Bucket`（`limit.go:130-133`）中在无 `ReadDeadline` 下执行的 `ev.Conn.Read(drain)`，或在设置短超时（如 50ms）且仅在非 TLS/H2 连接上执行；在 `Bucket` 调用 `dynlog.Log` 前确保 `ev.RR.Req` 非 nil。
- **`readH2Body` 协程泄漏与空 Body 短路（`runtime/events/events_h2.go:229-302`）**：
  1. 在 `fillRequest` 中，当 `r.Body == nil || r.Body == http.NoBody || (r.ContentLength == 0 && len(r.TransferEncoding) == 0 && (r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodDelete || r.Method == http.MethodOptions))` 时，直接跳过 `readH2Body`。
  2. 在 `readH2Body` 中引入 `done := make(chan struct{})` 并在函数返回时 `defer close(done)`，读协程发送 `ch <- chunk{...}` 时使用 `select { case ch <- ...: case <-done: return }`，彻底消除超时/超限退出时的读协程阻塞泄漏。
- **信号订阅精确化（`runtime/server/server.go:70-77`）**：
  将 `signal.Notify(sigchnl)` 改为只订阅 `syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGINT`，并在 `gracefulStop` 中调用 `signal.Stop(sigchnl)` 结束信号监听协程。

---

### Phase 2：反向代理、缓存安全、WebSocket 与 TCP 代理修复（覆盖 S-02, S-05, S-09, S-18, S-26, S-27）

#### 2.1 代理缓存防投毒与磁盘原子写（S-02, S-26）
- **`modules/proxy/cache.go`**：
  1. **缓存键（`CacheKey`）**：包含请求方法（或仅对 `GET` 写入缓存，`HEAD` 只允许读命中 `GET` 缓存并在返回时剥去 Body、不允许 `HEAD` 响应调用 `pc.Store` 写入空包体！）以及协商后的编码维度。
  2. **禁止缓存私有/鉴权响应**：在 `pc.Serve` 与 `pc.Store` 中，若请求包含 `Authorization` 头，或上游响应包含 `Cache-Control: private` / `no-store` / `no-cache` 或 `Set-Cookie` 或 `Vary: *`，默认不写入共享缓存。
  3. **深拷贝热缓存数据**：`cachedResponse` 返回给 `ev.RR.Res.Body` 时或下游修改 `Res.Body` 前确保不原地篡改 `hotEntry` 的共享切片。
- **`modules/cache/cache_index.go` 与 `modules/cache/cache.go`**：
  1. `CacheIndex.Touch(key)` 只校验 `now <= node.Expire`（必要时惰性剔除已过期项），**不再每次读命中都无限顺延 `node.Expire`**（TTL 应自写入时刻起算，与 `valid` 语义一致）。
  2. `writeEntryToDisk` 改为调用 `files.WriteAtomic`（临时文件 + 原子 `Rename`），消除 `O_TRUNC` 原地写与并发 `ReadCache` 的竞态（S-26）。
  3. 按缓存区 `Path` 独立统计容量，且当 `CacheChan` 队列满丢弃新写入时，若该 key 原先已有旧条目在索引中，恢复旧条目的 `previous` 大小而非直接 `delete(CC.sizes, key)`。

#### 2.2 WebSocket、HTTP/2 反代与 TCP 代理修复（S-05）
- **HTTP/2 请求反代协议号转换（`runtime/request/parse.go:269-282`）**：
  在 `byteRow(target)` 生成发往 HTTP/1.1 上游的请求行时，若 `r.Protocol == "HTTP/2"` 或 `r.Protocol == "HTTP/3"`，在序列化到上游报文时使用 `"HTTP/1.1"`，避免上游 HTTP/1.1 服务器因看到 `GET / HTTP/2` 返回 400。
- **WebSocket 升级识别与握手读取（`runtime/filters/filters.go`, `modules/proxy/event_proxy.go`, `modules/proxy/proxy_read.go`）**：
  1. `GHttpParseFilter`（`filters.go:30-38`）：使用 `request.HasToken(rr.Req.HeaderFold("Connection"), "upgrade")` 且 `strings.EqualFold(strings.TrimSpace(rr.Req.HeaderFold("Upgrade")), "websocket")`，兼容浏览器的 `Connection: keep-alive, Upgrade` 与大小写写法。
  2. `readFromUpstreamServer`（`event_proxy.go:254-313`）与 `ReadKeepAlive.tryToParse`（`proxy_read.go:68-90`）：
     当 `ev.Upgrade == "websocket"` 或上游状态码为 `101 Switching Protocols` 时，`tryToParse` 读完响应头 `\r\n\r\n` 后立即返回 `0`（并将紧随 101 头之后同一包到达的残留帧字节保留在 `rka.body` 中一并转发给客户端），**绝不返回 `-4` 进入 `readUntilEOF()`**。
- **`WebSocketHandler` 支持 TLS 上游与双向连接完整关闭（`modules/websocket/websocket.go`, `modules/proxy_tcp/event_proxy_tcp.go`）**：
  1. 在 `WebSocketHandler` 中定义 `upstreamConn net.Conn = proxy.ProxyConn`，当 `proxy.ProxyType == config.PROXY_HTTPS && proxy.ProxyTlsConn != nil` 时使用 `upstreamConn = proxy.ProxyTlsConn` 进行 `SetDeadline`/`Read`/`Write`/`Close`。
  2. 在 `WebSocketHandler` 和 `ProxyEventTcp` 入口处增加 `defer upstreamConn.Close()`（及 `defer conn2.Close()`）和 `defer ev.Close()`，任一方向的读写循环因 `io.EOF` 或错误退出时主动关闭双端连接，使另一方向的阻塞 `Read` 立即返回，彻底消除上游 FD 与协程泄漏。
- **修复 TCP 代理 `LisType` 常量不一致（`cmd/internal/servicecmd/handlers.go:102`, `runtime/listener/listener.go:264`）**：
  统一 `listener` 中的 TCP 监听类型常量与 `handleListen` 的判断条件（并在 `handleListen` 中检查 `len(ev.LisInfo.Cfgs()) > 0` 后再取 `Cfgs()[0].ProxyAddr`）。

#### 2.3 反代上游读取限额、状态机与短写修复（S-09, S-18, S-27）
- **`ReadKeepAlive` 与 `ReadConnectionClose`（`modules/proxy/proxy_read.go`）**：
  1. 将 `tryToParse` 的返回值拆分为明确的 `(status parseState, needRead int)`，不再用负数 `needRead` 兼任 `-1..-5` 状态枚举，彻底解决上游多发字节或负数 `Content-Length` 撞上 `-3`/`-4`/`-5` 的问题；当已读 body 长度超过 `contentLength` 时截断到 `contentLength` 并将 `p.ProxyNeedClose` 置为 `true`。
  2. 上游响应头阶段（未见 `\r\n\r\n`）限制最大头部缓冲（如 `maxHeaderSize()`），上游 `Content-Length` 和 `ReadConnectionClose.proxyReadAll()` 均检查 `maxChunkedBody()` 上限，超限返回 `ProxyErrorUpstreamBodyTooLarge`（502 Bad Gateway），防止恶意或大文件上游打爆内存。
  3. 统一 `Connection: close` 与 `Keep-Alive` 的上游响应读取逻辑（均走支持 `Content-Length`、`Transfer-Encoding: chunked` 解码和 EOF 读取的 `ReadKeepAlive`），消除 `ReadConnectionClose` 未解码 chunked 导致的响应损坏与缓存污染。
- **上游 HTTPS SNI 设置（`modules/proxy/event_proxy.go:90-97`）**：
  在 `dialUpstream` 构造 `tls.Config` 时，从 `p.ProxyAddr` 提取主机名（`host, _, _ := net.SplitHostPort(p.ProxyAddr)`）填入 `ServerName: host`。
- **`sendToUpstreamServer` 循环写全（S-27，`modules/proxy/event_proxy.go:315-339`）**：
  改为 `for len(req_data) > 0` 循环调用 `p.Write(req_data)` 直至全部写出或报错。
- **`GetCommandParsedStr` 单遍扫描替换（S-18，`core/commands.go:25-31`）**：
  改为单遍扫描 `inputString` 中的 `$<ident>` 占位符并查表替换（或按固定长名优先顺序一次性构造 `strings.NewReplacer`），保证已被替换出的变量值（如 `$request_uri`、`$host`）不会被二次展开。

---

### Phase 3：静态服务路由、限流/黑名单、WAF 与多域名证书修复（覆盖 S-10, S-11, S-12, S-17, S-20, S-29, S-30）

#### 3.1 静态文件路由拼接与斜杠重定向修复（S-10, S-29）
- **`modules/static/static.go`**：
  1. 在 `HandelSlash`（`static.go:645-652`）中：先剥离 `OriginPath` 中的 `?query` 再判断是否为空；若匹配的请求路径前缀 `ev.RR.Req.Path[loc[0]:loc[1]]` **已经以 `/` 结尾**（例如 `cfg.Path` 为 `"/static/"`），则**不再触发 301 重定向**，消除 `/static/` -> `/static//` 的无限 301 循环；重定向时保留原始 `?query`。
  2. 在 `StaticEvent`（`static.go:657-665`）中：拼接 `cfg.StaticRoot` 与 `rr.OriginPath` 时，若 `rr.OriginPath != ""` 且不以 `/` 或 `?` 开头，补齐 `/` 分隔符（或使用安全的路径拼接），确保 `cfg.Path = "/static/"` 时请求 `/static/app.js`（`OriginPath = "app.js"`）能正确映射到 `<StaticRoot>/app.js`。
  3. 在 `dofind` 中对 `try.Files` 和 `try.Next` 拼接出的候选路径统一调用 `staticPathInRoot(staticRoot, candidate)` 校验，不依赖 `symlink_policy == "reject"`。
  4. 修正 `getResBytes` 第 340 行和第 381 行的日志状态码：当 `rngStart >= 0` 时记录 `log.Status(206)` 而非硬编码 `200`；在 `rewrite` 和 `dev_mod` 中补齐 `dynlog.Log` 访问日志记录（S-29）。

#### 3.2 多域名 TLS 证书生成、加载与 ACME 路径对齐（S-11, S-30）
- **`runtime/listener/listener.go`, `runtime/listener/certs.go`**：
  1. 在 `loadCerts`（`certs.go:140-166`）中：加载 `item.ExtraCerts`（多域名别名证书 `config/cert/<alias>.pem`）时，若别名独立证书文件尚不存在（例如启动初期 `CertInit` 生成的是覆盖所有别名的主域名 SAN 证书，或用户配置了单张多域名/通配符证书），当主证书 `crt` 已存在时跳过缺失的 `ExtraCerts` 文件（仅记 Debug 日志，不返回错误中止整个端口的证书加载）。
  2. `processListenData`（`listener.go:353-358`）在生成 `ExtraCerts` 文件名时对 `alias` 做小写归一化（与 `certmanager` 的 `normalizeName` 一致）。
  3. `loadCerts` 的去重键由 `item.ServerName` 改为 `item.SSL.SslKey + "\x00" + item.SSL.SslValue`，允许同一 `ServerName` 在多个 server 块中分别配置 RSA 与 ECDSA 证书并由 `chooseCert` 按客户端能力挑选（S-30）。
- **`management/siteops/siteops.go`, `cmd/internal/sitecmd/site.go`, `management/view/site_view.go`, `modules/certmanager/sync.go`**：
  1. `siteops.fillCerts`（`siteops.go:954`）与 `sitecmd.fillSiteCerts` / `buildSiteInput`（`site.go:158,304`）统一取 `primaryName(in.ServerName)`（第一个域名）生成默认证书文件名 `<primary>.pem` / `<primary>-key.pem`。
  2. `site_view.SiteList`（`site_view.go:66`）按 `strings.Fields(strings.ToLower(e.ServerName))` 切分域名查询 `certByDomain`，修复多域名站点 `cert_status` 为空的问题。
  3. `certmanager/sync.go` 的 `certPaths(domain)`：当 `domain` 命中某个 server 块的主域名（`names[0]`）且该块显式配置了 `ssl_certificate` / `ssl_certificate_key` 时，优先写入该配置路径（或同时同步），确保 `ReloadCerts()` 能立即加载新签发的主域名证书。

#### 3.3 限流器、黑名单并发安全与 WAF Location 隔离（S-12, S-17, S-20）
- **`modules/safe/limit.go`, `modules/safe/counts.go`, `modules/safe/blacklist.go`**：
  1. `everyFromRate(rps int)` 改为 `if rps <= 0 { return rate.Inf }; return rate.Limit(rps)`，彻底消除 `1000/rps` 整数除法在 `rps > 1000` 时归零的问题；`state()` 中判断 `if il.rate > 0 && il.rate != rate.Inf { rate = int(math.Round(float64(il.rate))) }`，消除 `int(rate.Inf)` 溢出为 `-9223372036854775808` 的 Bug。
  2. `countsInit()` 与 `(*CountLimit).Insert`（`counts.go`）：
     - 若某个 location 未配置限流（`path.Limit.Rate <= 0`），`CountLimitFor(cfg.ID)` 直接返回 `nil`（不建表、不加锁）。
     - 若配置了 `path.Limit.Rate > 0` 但未写 `path.Limit.Size`（`Size <= 0`），赋予合理的默认桶容量上限（如 `defaultLimitSize = 65536` 个 IP）。
     - 当 `cl.num >= cl.size` 且新 IP 不在 `cl.gMap` 中时，先尝试清理已过期的条目，若仍满则**拒绝新 IP（返回 `false`）或淘汰最旧项**，绝不能在 `Insert` 开头无条件 `if cl.num > cl.size { return true }` 放行所有请求；被拒绝时调用 `stats.locationRejected.Add(1)`。
  3. 修复 `g_list` 与 `g_ips` 的 Reload 数据竞争：`g_list` 改为在 `g_list.mu.Lock()` 保护下替换 `g_list.entries`（绝不直接覆写正在被并发读锁持有的 `sync.RWMutex` 结构体），`g_ips` 改为 `atomic.Pointer[ipLimiters]`。
  4. 统一使用 `net.SplitHostPort`（或 `core.ClientIP(ev)`）提取客户端 IP，修复 `safe.RemoteIP` 与 `rejectLocation` 在 IPv6 下残留方括号 `[::1]` 的问题。
- **`modules/appfirewall/waf.go`（S-20）**：
  在 `Allowed(cfg *listener.ListenCfg, ev *core.Event) bool` 中检查 `cfg`：若 `cfg == nil` 或当前 location 未配置 `appfirewall`（且全局并非全站强制模式），直接放行；若当前 location 指定了具体的 `cfg.AppFireWall` 规则列表（如 `["sqli"]`），则仅当 `result.Findings` 命中的 `Detector` 属于当前 location 启用的检测器集合时才执行拦截与计数。
- **`runtime/listener/listener.go`（S-17）**：
  为 `listenTcp` / `listenSsl` / `openListener` 增加返回 `error` 的内部实现：启动期绑定失败仍 `Fatal` 退出，但在 `ReloadListenCfg()` 热重载路径中绑定失败时记录 Error 日志并保留旧监听器，不再 `os.Exit(1)` 杀死整个运行中进程。

---

### Phase 4：配置校验、事务锁、文件权限与 Windows 重载回滚修复（覆盖 S-03, S-13, S-15, S-16, S-19, S-21, S-25）

#### 4.1 缓存目录边界校验与 `operation get` 路径校验（S-03, S-19）
- **缓存区 `path` 安全守卫（`config/internal/validate/validate.go`, `cmd/internal/cachecmd/cache.go`）**：
  1. 在 `validateCacheZones` 与 `cachecmd` 中，校验 `config.DataPath(z.Path)`：禁止 `path` 等于或包含应用根目录（`DataDir()`）、配置目录（`FAST_HTTPS_ETC_DIR`）、证书目录（`CERT_DIR`）、日志目录（`LogRoot`）、状态目录（`StateDir`）以及文件系统根目录（被保護目录不得是 `cache.path` 的自身或子目录）。
  2. 在 `purgeDir` 中复核上述边界：若目标目录是上述任何受保护目录或其父目录，拒绝执行 `os.RemoveAll` 并返回 `INVALID_ARGUMENT`。
- **`operation.Store.Get(id)` 标识符校验（`management/operation/operation.go:230-250,332-334`）**：
  在 `pathOf(id)` / `Get(id)` 中校验 `id`：不允许包含 `/`、`\`、`..` 或路径分隔符（仅允许 `[A-Za-z0-9._-]+` 且不以 `.` 开头），非法 `id` 直接返回 `ErrNotFound`。

#### 4.2 状态目录锁 `state.Dir.Lock` 与原子写文件修复（S-13）
- **`management/state/dir.go`**：
  1. **消除跨进程空锁文件被删竞态**：创建锁文件时，先将锁记录 JSON 写入同目录下的唯一临时文件（`os.CreateTemp`），写完 `Close()` 后通过 `os.Link(tmp, lockPath)`（Unix/Windows 均支持硬链接原子建锁，若目标已存在则 `os.Link` 失败返回 `ErrExist`）或当观察到 `readJSON` 失败且锁文件 `ModTime` 距当前不足 500ms 时重试等待而非立即 `os.Remove(path)`。
  2. **补齐进程内互斥**：在 `state.Dir`（或包级按规范化锁路径索引的表）中增加进程内 `sync.Mutex`，只有当前调用链显式传入已持锁上下文时才允许事务内嵌套，防止同进程两个并发协程因 `held.PID == rec.PID` 同时穿过锁。
- **`management/state/atomic.go` 与 `infra/files/atomic.go`**：
  1. 使用 `os.CreateTemp(dir, filepath.Base(path)+".*.tmp")` 生成唯一临时文件，消除并发写同一路径时的 `.tmp` 碰撞。
  2. 移除 `management/state/atomic.go:22-24` 中在 `os.Rename(tmp, path)` 之前执行的 `if runtime.GOOS == "windows" { _ = os.Remove(path) }`（Go 标准库在 Windows 下的 `os.Rename` 底层即为 `MoveFileExW(..., MOVEFILE_REPLACE_EXISTING)`，无需预删目标文件；若因瞬时占用失败可做短退避重试，绝不先删原文件）。

#### 4.3 私钥/配置权限保护与 Windows 事务回滚修复（S-15, S-16, S-21, S-25）
- **文件权限最小化（S-15，`management/cfgver/cfgver.go`, `management/txn/files.go`, `infra/logging/logger.go`）**：
  1. 在 `cfgver.copyCerts`（第 231 行）与 `RestoreCerts`（第 265 行）中，保留源文件权限或对私钥文件（`*.key`、`*-key.pem`）统一使用 `0o600`（证书公钥用 `0o644`），快照目录 `certs/` 使用 `0o700`。
  2. `txn/files.go` 的 `switchFiles` 若目标文件已存在则保留其原有 `FileMode`（不存在时默认 `0o600` 或 `0o640`）。
  3. `infra/logging/logger.go:205` 打开日志文件的权限由 `0o666` 收紧为 `0o640`（或 `0o644`）。
- **Windows 事务 Reload 失败回滚（S-16，`cmd/internal/cli/txn.go:86-128`）**：
  在 `NewTxnEngine` 中记录事务开始前服务是否正在运行；当 Windows `restartService(pidPath)` 返回非空 `restartErr` 时，**直接将 `restartErr` 作为 `Reloader` 的错误返回给 `txn.Engine.Apply`**，从而触发 `e.compensate()` 自动回滚配置文件并重新拉起旧配置下的服务进程，而不是返回 `nil` 吞掉错误。
- **`jsonc.PatchPath` 单行对象删除修复（S-21，`infra/jsonc/path.go:213-228`）**：
  在 `deleteMember` 中与 `deleteElem` 对齐：先获取所属 `Object` 的 `Open`（`{` 的偏移），仅当 `containsNewline(masked, obj.Open, m.KeyStart)` 为真时才使用 `lineStart(masked, m.KeyStart)`，否则从 `m.KeyStart` 开始裁剪，防止切掉同一行上的 `{`。
- **`FirstSymlinkUnder` 遍历错误处理（S-25，`config/internal/paths/symlink.go`, `config/internal/validate/symlink.go`, `config/internal/loader/convert.go`）**：
  当 `FirstSymlinkUnder(root)` 返回非空 `err`（且不是根目录自身不存在）时，`validateSymlinkPolicy` 和 `checkLoadTimeSymlinks` 应将该错误上报为校验失败（或在 `WalkDir` 中记录权限错误并返回），不再将 `err != nil` 静默视为“无符号链接”。

---

### Phase 5：桌面端 (Desktop) 提权安全、配置保存与生命周期修复（覆盖 S-14, S-28）

#### 5.1 `ImportLocalCA` 安全加固（S-14，`desktop/internal/view/cert_trust.go`）
- **Windows**：
  1. 校验 `p`（`rootCA.crt`）确实是普通文件（非软链接/重解析点）且 PEM 解码为有效 X.509 CA 证书。
  2. 定位 `%SystemRoot%\System32\WindowsPowerShell\v1.0\powershell.exe` 与 `%SystemRoot%\System32\certutil.exe` 绝对路径；在构造 `Start-Process` 的 `-ArgumentList` 时，对证书路径参数外层包裹转义双引号（`'"' + psQuote + '"'`），确保路径含空格（如 `C:\Program Files\...`）时不会被 `certutil` 拆成多个参数，同时拒绝含 `"` 或控制字符的非法路径。
- **Linux**：
  在调用 `pkexec` 前先用 `os.Lstat(p)` 确认 `rootCA.crt` 不是符号链接且为合法 PEM 证书；脚本中用 `install -m 0644 --no-target-directory`（或 `cp --no-dereference`）替代裸 `cp`。

#### 5.2 设置页保存、站点新建原子性与退出行为对齐（S-28）
- **`desktop/internal/configops/editor.go`**：
  1. `pruneEmptyProxy`（第 235-258 行）补齐对 `upstream_timeout` 和 `tunnel_idle_timeout` 的非零判断（与 `config/internal/proxyconf/write.go` 的 `IsDefault` 对齐），不再误删用户设置的代理超时。
  2. `buildCandidate` 处理非 `conf.d` 的相对 `http.include` 路径时同样重写或按原相对目录结构复制到候选树中。
- **`desktop/internal/siteops/editor.go`**：
  在 `AddSite` 中，若后续的 `site update` 或 `site location set` 步骤失败，自动补偿执行 `site remove <domain> --yes` 清理第 1 步落盘的半成品站点（或扩展 `site add` 一次性接收完整参数单事务落盘），保证用户在弹窗修正错误后重试不会撞上 `site already exists`。
- **`desktop/service.go` 与 `desktop/tray.go`**：
  对齐 `QuitApp`、托盘“退出”菜单与 `HelpDialog.vue` 的契约：退出桌面控制台仅关闭 GUI 进程（或若用户明确要求停止服务再退出，则同时调用 `view.StopMonitor(s.baseDir)` 停止监控进程，不留孤儿 `fast-https-monitor`）。

---

## 3. 验证与回归清单

每个阶段合入前均需满足以下自动化与回归检查：

1. **编译与静态检查（双布局）**：
   ```bash
   go build ./... && go vet ./...
   go vet -tags=rpm ./...
   cd desktop && go build ./... && go test ./internal/...
   ```
2. **新增专项回归单测**：
   - `runtime/request`：`Transfer-Encoding: chunked;q=0`、重复 `Content-Length`、前导 `\r\n\r\n`、`Content-Length:` 分片截断、`multipart/form-data` 无 `boundary=` 及带引号 `boundary`、`/public/../admin` 路径折叠、`Host: example.com:80` 端口幂等。
   - `runtime/events`：H2 连接触发黑名单/限流不 Panic 且关闭 `conn`；H2 多 `Cookie` 头合并；`readH2Body` 超时/超限退出无 goroutine 泄漏。
   - `modules/proxy` & `modules/cache`：`HEAD` 请求不污染 `GET` 缓存；带 `Authorization`/`Cache-Control: private` 不进共享缓存；H2 请求反代发送 `HTTP/1.1` 请求行；`Connection: close` 下上游 chunked 响应正确解码；上游超长 `Content-Length` 按 502 拦截不 OOM。
   - `modules/websocket` & `modules/proxy_tcp`：H1 `Connection: keep-alive, Upgrade` 握手不死锁；`wss` 上游经 `ProxyTlsConn` 读写；`io.EOF` 退出时双端 `Close()` 被调用；TCP 监听端口正常触发 `ProxyEventTcp`。
   - `modules/static` & `runtime/listener`：`location.url = "/static/"` 下访问 `/static/` 与 `/static/file` 正常返回 200；多域名 `server_name: "a.com b.com"` 在单张主域名 SAN 证书下 TLS 握手成功。
   - `modules/safe` & `modules/appfirewall`：`rate = 2000` 正常限流且 `Snapshot().GlobalRate == 2000`；Location 限流 `size = 0` 时不放行超额请求；并发 `Reload` 与 `IsInBlacklist`/`Bucket` 在 `go test -race` 下零竞争；WAF 仅在配置了 `appfirewall` 的 location 生效。
   - `management` & `cmd` & `desktop`：`cache delete --purge` 拒绝 `.` 与 `./config`；`operation get ../...` 被拒绝；`state.Dir.Lock` 并发抢锁测试通过；快照中 `*.key`/`*-key.pem` 权限为 `0o600`；`jsonc.PatchPath` 删除单行对象首键不吞 `{`；桌面端 `pruneEmptyProxy` 保留 `upstream_timeout`。
3. **全量与竞态测试**：
   ```bash
   go test -p 1 ./...
   go test -race ./...
   go test ./tests/packaging
   ```
