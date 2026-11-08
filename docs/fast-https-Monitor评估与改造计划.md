# fast-https Monitor 评估与改造计划

日期：2026-11-08。本文是对 [开发前置条件与代码扫描清单](fast-https-Monitor开发前置条件与代码扫描清单.md) 的扫描结果，以及按工作量从小到大排列的落地顺序。

扫描只读了当前源码，没有改代码，也没有启动服务。结论以调用链为准，不按函数名推断。静态阅读确认不了的事项标为「待运行验证」。

整体评级：**C**。进程能启动、能热重载、能写日志，但没有一份独立进程能读到的运行时状态。在补上进程内计数和只读查询接口之前，Monitor 只能看 PID 和日志，分不清「进程活着」和「还能正常服务」。

## 1. 执行摘要

1. 当前不具备开发可靠 Monitor 的基础条件。能确认的只有：PID 是否还在、配置是否校验过、端口在启动那一刻是否绑定成功。
2. 必须先改 Server 核心，不能直接做 Monitor。现有 `safe.Snapshot` 只在进程内部，外部进程读不到。
3. 最大的三个缺口：没有可查询的生命周期状态；没有连接数、请求数、状态码、耗时的运行时计数；没有回环地址或本机套接字这类受控出口。
4. 稳定性：监听失败直接把进程打退出，没有「降级」状态；单条连接 panic 只打日志，外面看不出来。安全：现在没有管理端口。以后若做查询接口，不能把黑名单全文、证书和请求头带出去。`modules/safe/limit.go` 在每个新 IP 上 `println`，会把标准输出打满，与监控无关，但会干扰运行。
5. 最小路径：先加原子计数和生命周期状态，再加一个只绑定本机的只读快照，用测试对一下数字，然后才写独立 Monitor。内存、文件描述符、CPU 由 Monitor 读操作系统，不必塞进 Server。

Monitor 自己的周期采集、规则和告警不是 Server 缺陷，排在核心计数和查询接口之后。

## 2. 逐项检查

| 编号 | 前置条件 | 状态 | 代码证据 | 缺口与风险 | 建议 |
| --- | --- | --- | --- | --- | --- |
| A | 生命周期状态 | 部分具备 | 入口 `fast-https.go` 的 `main` 只执行 `cmd.RootCmd`。`cmd.StartHandler`（`cmd/commands.go`）写 PID、初始化、`ServerInit`、`notifyDaemonReady`、`Run`。`Server.stopping` 只在 `gracefulStop` 里置位（`runtime/server/server.go`），不对外读。`Reload` 没有「重载中」标志。监听失败在 `listenTcp`（`runtime/listener/listener.go`）直接 `Fatal`。`InspectProcess`（`cmd/commands.go`）只看 PID 是否还在 | PID 在不等于可服务。启动成功只通过一次性管道告诉父进程（`cmd/daemon_unix.go` 的 `notifyDaemonReady`），之后查不到。Accept 出错只退避重试（`runtime/server/server.go` 的 `serveListener`），进程仍显示在跑。连接 panic 被吃掉，主循环不知道 | 进程内放一份原子状态：启动中、监听中、运行中、重载中、停止中、已退出。记下启动时刻和 `config.CURRENT_VERSION`。reload 不改启动时刻 |
| B | 连接状态统计 | 不具备 | `serveConn` 用 `connWg` 只为了停机等待。`WaitGroup` 读不出当前数量。没有接受、关闭、读、写、空闲计数。HTTP/1.1 与 HTTP/2 都走 `serveConn` | 停机等待不能当成活动连接数。客户端断开、读超时都只是 `HandleEvent` 返回（`runtime/events/events.go`），没有计数 | 在 `serveConn` 的进入和 `defer` 里做原子加减，得到活动连接、累计接受、累计结束。读/写/空闲阶段先不做 |
| C | 请求与状态码 | 不具备 | 请求循环在 `HandleEvent` / `EventHandler`。状态码只进访问日志 `dynlog.fieldStatus`（`runtime/dynlog/dynlog.go`）。`safe.stats.totalChecked` 在 `Bucket` 里每个连接加一次（`modules/safe/limit.go`），不是每条请求。`locationRejected` 只有字段（`modules/safe/safe.go`），没有 `Add` | 把限流检查次数当成请求数会错。4xx/5xx 只能事后扫日志，而且提前断开、解析失败不一定有访问日志 | 请求进入 `EventHandler` 时用 `defer` 计数，按最终状态码归入 2xx/3xx/4xx/5xx。解析失败单独计数 |
| D | 请求耗时 | 不具备 | 连接用 `time.Now()` 设 30 秒读写截止（`runtime/events/events.go`）。上游固定 20 秒（`modules/proxy/event_proxy.go`）。访问日志字段表没有耗时 | 日志时间是写出时刻，不是处理时长，也不能当直方图。墙钟会受对时影响 | 用 `time.Since` 的单调时钟，在请求 `defer` 里落入固定桶。先不做分位数库 |
| E | 资源指标 | 部分具备 | 生产代码没有 `MemStats`、`NumGoroutine`、`getrlimit`。systemd 单元写了 `LimitNOFILE=65535`（`build/packaging/systemd/fast-https.service`），那是启动限额，不是当前用量 | Server 内部不知道内存和文件描述符。这些可以由 Monitor 对 PID 读操作系统 | Server 不采集。Monitor 以后用 PID 读进程内存、CPU 时间、打开的文件数。读不到就标明未知，不要填 0 |
| F | 上游 | 部分具备 | 每地址一个连接池 `connPool.get`（`modules/proxy/pool.go`），空闲连接在锁里，没有导出数量。拨号失败、读写超时是 `ProxyErrorDialUpstream`、`ProxyErrorSendToUpstreamTimeout`、`ProxyErrorReadFromUpstreamTimeout`（`modules/proxy/event_proxy.go`），只打日志。没有健康检查，也没有第二个上游 | 池子状态外部不可见。超时和拨号失败无法按上游累计 | 在现有三条错误上各加一个原子计数，并给出池内空闲连接数。健康检查仍不做 |
| G | 外部读取接口 | 不具备 | `status` 是另一个进程读 PID 文件（`cmd/commands.go` 的 `statusHandler`）。`safe.Snapshot`（`modules/safe/safe.go`）只在本进程内，还会把整份黑名单拷出去。没有回环 HTTP，没有 Unix 套接字 | 内部变量外部读不到。若直接把 `Snapshot` 暴露出去，会泄漏黑名单，而且持锁拷贝，不适合高频读 | 新增只读快照：只含状态、计数、版本、观测时间。只监听 `127.0.0.1` 或权限收紧的 Unix 套接字。默认不影响现有请求端口 |
| H | 故障信号 | 部分具备 | 启动失败：配置、端口、证书在 `PreCheckHandler` 里 `Fatal`，父进程能从管道 EOF 看到。reload 失败留旧配置并打日志。优雅停机超时打一行日志。5xx、解析失败、上游超时没有计数 | 这些信号散落在日志里。只扫日志会漏掉没写访问日志的失败，也会把历史日志当成当前状态 | 先把监听失败退出、reload 失败、停机超时、5xx、上游超时变成计数或状态位。资源类告警交给 Monitor 看操作系统 |
| I | 事件记录 | 不具备 | reload 起止是控制台日志。没有可轮转的 `server.started` 一类事件文件 | 崩溃前没执行到的日志行会丢。当前文件日志经 `syncWriter` 串行写出，没有单独的事件流 | 第一阶段不要做事件总线。Monitor 轮询状态，自己记下「从运行变成不可达」 |
| J | 生命周期管理 | 部分具备 | 包安装用 systemd：`Type=simple`、`ExecStart` / `ExecReload`、`Restart=on-failure`。Unix `stop` 发 `SIGTERM`，最多等 `GracefulStopTimeout`（5 秒）。源码直跑会自己转后台。没有 Monitor 单元 | Server 今天不依赖任何 Monitor，这条已经满足。意外退出后没有另一个进程负责记录 | 保持 Monitor 与 Server 分单元、分进程。Server 启动不得等待 Monitor |
| K | 开销与并发 | 不适用 | 请求路径上已有的原子计数只有限流那几项。`Snapshot` 会锁住并复制黑名单 | 若在每次请求里扫连接表，或在读指标时拿 `Server.mu`，会拖垮热路径 | 热路径只做原子加减。读接口只 `Load` 原子值，不遍历连接 |
| L | 日志 | 部分具备 | 四路 JSON，时间是带时区的 RFC3339（`infra/logging/consts.go` 的 `TimeFormat`）。访问日志有状态码和大小，没有耗时、没有请求 ID。轮转靠 logrotate 的 `copytruncate`、单文件 100MB（`build/packaging/logrotate/fast-https`）。写入失败不会让请求返回错误 | 日志能帮忙排障，不能当运行时计数。URI、Host、User-Agent 都是客户端可控内容 | 保持日志为辅。Monitor 只尾读新增行，不要每轮扫全文件 |
| M | 时间与计数语义 | 不具备 | 没有请求计时。没有「本次进程启动时间」。reload 会换配置快照，但没有统计量可被它清掉 | 以后如果 reload 把计数清零，Monitor 会把下降看成负流量 | 计数只在进程重启时归零。reload 不动累计值。快照里带启动时间和 PID，供 Monitor 识别重启 |
| N | 监控配置 | 不具备 | `config/schema.go` 的 `jsonConfig` 没有 Monitor 或运行时接口字段 | 若把告警阈值写进站点 JSON，Monitor 故障会和业务配置缠在一起 | 查询地址先写死在回环上。采样周期、阈值、告警放在 Monitor 自己的配置里 |

## 3. 可以复用的

| 能力 | 位置 | 能用到什么程度 |
| --- | --- | --- |
| 进程是否还在 | `cmd.InspectProcess` | 区分没有 PID 文件、PID 已过期、进程仍在。不能说明端口仍在服务 |
| 启动是否绑上端口 | `cmd.notifyDaemonReady` | 在 `ServerInit` 之后通知父进程。一次性的，不是查询接口 |
| 停机与重载 | `Server.gracefulStop`、`Server.Reload` | 配置校验失败会留下旧监听。状态位可以挂在这两条路径上 |
| 原子计数写法 | `modules/safe` 的 `stats` 与 `Snapshot` | 写法可参考。不要把黑名单拷贝进监控快照。`LocationRejected` 目前一直是 0 |
| 上游错误分类 | `modules/proxy` 的三个 `ProxyError` | 拨号失败、写超时、读超时已经分开，加计数即可 |
| 日志 | `infra/logging`、logrotate | 留给排障，不作为指标来源 |
| 部署 | `build/packaging/systemd/fast-https.service` | `Restart=on-failure` 能把 Server 拉起来。Monitor 以后用另一个单元 |

`modules/safe/control.go` 的加删黑名单和调限流是控制面协议，不是本计划要改的运行时指标。不要把它当成 Monitor 的数据源，也不要为了做监控去删它。

## 4. 核心代码还缺什么

### P0：不补就做不出可靠的 Monitor

1. **进程内生命周期。** 状态、启动时间、版本、PID。改 `runtime/server`。reload 期间标成重载中，失败回到运行中并保留旧配置。热路径不读这组字段。
2. **连接与请求计数。** 活动连接、累计接受、累计结束；活动请求、累计请求、2xx/3xx/4xx/5xx、解析失败。用 `defer` 覆盖提前返回。reload 不清零。
3. **本机只读快照。** 独立小监听，只 `Load` 原子值，返回观测时间、单位和 `schema_version`。未知字段用 `null`，不用 0。不包含黑名单、证书、Cookie、`Authorization`。这个监听绑定失败只记日志，业务端口照常服务。

### P1：第一阶段建议跟上

4. **请求耗时。** 固定桶直方图，单调时钟，放在请求 `defer` 里。
5. **上游计数。** 拨号失败、读超时、写超时累计，以及连接池空闲条数。
6. **停机与重载结果。** 停机超时、reload 失败各留一个计数，避免只能去翻日志。

### P2：往后

7. 连接处于读、写还是空闲。要改连接状态机，容易漏路径。
8. 文件描述符、内存、CPU 放在 Monitor 里对 PID 采集，Server 不实现。
9. 多上游和健康检查。现在每个 location 只有一个 `proxy_pass`，不要为了监控先做故障转移。

## 5. 查询接口

用只绑定 `127.0.0.1` 的 HTTP，一个路径，例如 `/runtime`。Monitor 是另一个进程，本机 HTTP 最好写、最好测。Unix 套接字在 Windows 上还要另做一套，不作为第一步。不要占用现有业务端口，避免和站点路由缠在一起。

监听地址先写死在回环上，不进站点 JSON。以后要改端口，再加一个与站点无关的配置项，默认仍是回环。

响应只放这些字段：

| 字段 | 类型 | 来源 |
| --- | --- | --- |
| `schema_version` | Info | 常量，字段含义变化时才加 |
| `observed_at` | Info | 快照读出时的绝对时间 |
| `server.state` / `pid` / `version` / `started_at` | Info / Gauge | 第 4 节 P0 的生命周期 |
| `connections.active` / `accepted_total` / `closed_total` | Gauge / Counter | `serveConn` 的进入与退出 |
| `requests.active` / `total` | Gauge / Counter | `EventHandler` 的 `defer` |
| `responses_2xx_total` 至 `responses_5xx_total` | Counter | 最终写出的状态码 |
| `request_duration_buckets` | Histogram | 单调时钟，固定桶 |
| `upstream.dial_fail_total` 等 | Counter | 现有三个 `ProxyError` |

采集就是几次原子读，不遍历连接。版本不兼容时加 `schema_version`，不改旧字段的含义。

## 6. 落地执行计划

第 4 节只说明缺什么。本节按依赖从少到多拆成七步，每一步都能单独合并，并且在进入下一步之前用本节给出的测试收口。不要把后一步的接口或进程提前做进来。

分层约束：`runtime` 不能 import `modules`（`tests/arch/layering_test.go`）。连接和请求计数放在 `runtime` 里。上游计数留在 `modules/proxy`，由 `cmd` 在组装时注册到快照，不能让 `runtime/server` 去引用代理包。`modules/safe.Snapshot` 继续只服务控制面，不拿来当监控出口。

计数规则贯穿各步：

- Counter 只增。Gauge 是当前值，用 `defer` 减回去，覆盖提前返回和 panic。
- reload 不清任何累计值，也不改 `started_at`。进程重启后内存计数自然从零开始，快照用新的 PID 和 `started_at` 让 Monitor 认出重置。
- 没有测到的字段不要写 `0`。接口还没做出来之前，测试直接读进程内快照函数。

每一步结束时至少跑：

```bash
go test ./runtime/... ./modules/proxy/... ./cmd/...
go test -race ./runtime/server/ ./runtime/events/ ./runtime/metrics/ ./modules/proxy/
go test ./tests/arch/
```

Windows 上全量测试用 `go test -p 1 ./...`。改动没碰到的包不必为了这一步重跑打包和发行版安装。

### 第 1 步：生命周期和连接计数

只加进程内数据，不加端口，不改 JSON 配置。

改动落点：新建 `runtime/metrics`，在包初始化时把状态写成 `starting`。字符串的零值是空串，不能把空串当成 `starting`。在 `ServerInit` 记下 `started_at`、`os.Getpid()` 和 `config.CURRENT_VERSION`。状态只用这几个值：`starting`、`running`、`reloading`、`stopping`。`ServerInit` 在监听建立后进入 `running`。测试之间要清掉进程内计数，不要用 `t.Parallel` 跑这些用例，否则全局原子量会串。`Reload` 开头进入 `reloading`，无论配置成功还是失败，函数返回前都回到 `running`（失败时旧监听仍在服务）。`gracefulStop` 进入 `stopping`。监听失败仍保持现在的 `Fatal` 退出，这一步不发明「降级」状态。

连接只加三个数：`accepted_total`、`closed_total`、`active`。在 `serveConn` 确认不是停机丢弃后加接受数和活动数，在处理协程的 `defer` 里加结束数并减活动数。HTTP/1.1 和 HTTP/2 都走 `serveConn`，所以两种连接都算进去。停机时尚未进入处理的连接直接关掉，不计入接受数。

这一步不做请求数、状态码、耗时、上游和 HTTP 接口。

测试放在 `runtime/metrics` 与 `runtime/server`：

- 初始状态是 `starting`，`ListenWithCfg` 成功后的测试替身调用后变为 `running`。
- `Reload` 失败（沿用现有「保留旧配置」路径）结束后状态是 `running`，`started_at` 不变，已有计数不变。
- 两个连接先后进入并退出后，`accepted_total` 为 2，`closed_total` 为 2，`active` 为 0。
- 处理函数 panic 时，`defer` 仍然把 `active` 减回去。
- `go test -race` 下并发加减，`active` 不为负。

做完才算通过：上面的测试全绿，业务请求路径的行为测试没有新增失败，`go test ./tests/arch/` 通过。

### 第 2 步：请求数、状态码和解析失败

仍然只在进程内。

一条连接上的多次 keep-alive 请求要逐条计数，不能把活动连接当成活动请求。计数点放在 `runtime/events`：

- `parseRequest` 返回 1 之后、调用 `EventHandler` 之前，活动请求和累计请求加一；用 `defer` 在 `EventHandler` 返回时减活动数。
- `parseRequest` 写出 400 的那条路径计入 `parse_errors_total`。客户端主动关掉（读到空或读错误）只算连接结束，不算解析失败。
- 状态码在 `defer` 里读取。`response.Response` 没有单独的状态码字段，`SetFirstLine` 只把码写进 `FirstLine` 字符串。从这行解析前三位数字；解析不出时计入 `responses_unknown_total`，不要算进 2xx。若在 `SetFirstLine` 里另存一个整数，也必须以 `FirstLine` 为准，避免两个来源不一致。
- HTTP/2 进入 `EventHandler` 的请求走同一套计数。`h2Handler.ServeHTTP` 里 `fillRequest` 返回 false 时已经写了 400 且没有进入 `EventHandler`，这条计入 `parse_errors_total`，不计入 `requests_total`。其它还分不清的 H2 流错误先不要猜，记在实现说明里留到后面。

`modules/safe` 的 `totalChecked` 仍是每条连接的限流检查次数，测试里断言它和 `requests_total` 不是同一个数。

测试：

- 同一连接上两轮请求：`requests_total` 为 2，结束后 `requests.active` 为 0，`connections.active` 仍按连接算。
- 静态 200、未命中 location 的 404、处理函数的 500，分别只增加对应分桶。
- 畸形请求头走 400 时 `parse_errors_total` 加 1，且不增加 `requests_total`。
- 读到一半客户端断开：活动连接回到 0，`parse_errors_total` 不增加。

做完才算通过：第 1 步的测试仍然全绿，新测试全绿。这一步还没有对外接口，不写 E2E。

### 第 3 步：本机只读快照

这是独立 Monitor 的前提，也是第一次增加监听。

在 `cmd` 里于业务监听之外再起一个只绑 `127.0.0.1` 的 HTTP 服务，路径固定为 `/runtime`，默认端口 `9088`。测试使用 `:0`，不占用固定端口。绑定失败只打错误日志，`StartHandler` 照常进入业务服务。不读取站点 JSON，不挂到现有 location 上。

`GET /runtime` 返回 JSON，`Content-Type: application/json`。第一版字段只有第 1、2 步已经有的量，加上 `schema_version`（字符串 `"1"`）、`observed_at`、`server.state`、`server.pid`、`server.version`、`server.started_at`。耗时和上游还没有时，省略这两个对象，不要填 0。响应里不得出现黑名单、证书路径、Cookie、`Authorization`、请求 URI。

只允许来自回环地址的连接。其它路径返回 404，不带业务站点内容。读取过程只调用原子 `Load`，不拿 `Server.mu`，不遍历连接。

测试：

- `httptest` 或本地 `:0`：字段与进程内计数一致，`schema_version` 为 `"1"`。
- 绑定一个已经被占用的地址时，函数返回错误，调用方可以继续跑业务监听。用现有的小监听测试代替真正的 `start`。
- 非 `/runtime` 得到 404。
- 响应体用固定的禁止词扫描，确认没有 `blacklist`、`ssl_certificate`、`cookie`、`authorization`。

做完才算通过：上述测试全绿，并且 `tests/client_test` 里现有的代理与 HTTP/2 集成测试仍然通过。不在这一步加 CLI 子命令；以后的 `runtime status` 只读取这个接口，不另写一套计数。

### 第 4 步：耗时直方图和上游计数

请求耗时：在第 2 步那个 `defer` 里用开始时的 `time.Now()` 做 `time.Since`。`time.Since` 用的是单调时钟，不受中途对时影响。桶边界固定为 1ms、5ms、10ms、25ms、50ms、100ms、250ms、500ms、1s、5s，另加一个 `+Inf`。上报各桶的累计个数和总次数，单位是毫秒。不引入 Prometheus 客户端。

上游：在 `ProxyErrorDialUpstream`、`ProxyErrorSendToUpstreamTimeout`、`ProxyErrorReadFromUpstreamTimeout` 三处各加一个只增计数。连接池在已有的锁内顺便给出空闲连接数，不要为了读数再扫一遍所有业务连接。这三个数由 `cmd` 注册进 `/runtime` 的 `upstream` 对象。没有代理流量时计数是 0，这是真实的零，不是未知。

`/runtime` 从这一步起带上 `request_duration` 和 `upstream`。旧字段的名字和含义不变。

测试：

- 立即返回的请求落在最小桶；测试里把一个处理函数睡过 30ms，只增加 25ms 以上的桶。
- 拨号到没有人监听的地址，`dial_fail_total` 加 1，业务仍返回现有的上游失败响应。
- 用现有上游读超时测试，确认读超时计数加 1，且不会把同一次失败再记进拨号失败。
- reload 之后这些累计值不变。

做完才算通过：`go test ./modules/proxy/ ./runtime/metrics/ ./runtime/events/` 与 `-race` 全绿。

### 第 5 步：停机、重载和重启的语义收口

这一步不增加新指标种类，只把前面几步在真实启停路径上对死，避免 Monitor 以后把 reload 看成重启。

在已有测试上补这三件：

- reload 成功：状态经历 `reloading` 后回到 `running`，`started_at` 与各项 Counter 不变，Gauge 仍反映当前连接。
- reload 失败：状态回到 `running`，`reload_failed_total` 加 1。这个计数在 `Reload` 返回错误时加，和状态放在一起，实现可以落在第 1 步的同一个包里，但验收放在这里。
- 停机超时：`drainConnections` 走到超时分支时 `stop_timeout_total` 加 1。用一个不返回的处理函数和很短的超时，不要真的等 5 秒。

进程重启不在进程内测试。用 `tests/client_test` 起两个短生命周期子进程：第二个进程的 `/runtime` 必须是新 PID、新的 `started_at`，`requests_total` 从 0 开始。测试结束要清掉子进程和临时配置，不改仓库里的 `config/fast-https.json`。

做完才算通过：`go test ./runtime/server/ ./tests/client_test/ -count=1` 全绿。Windows 上这一步若 E2E 因进程权限失败，记下具体错误，不要改成跳过还宣称通过。

### 第 6 步：独立采集进程

Server 核心到第 5 步就可以被外部读取。这一步才新建 Monitor，建议放在仓库里单独的 `monitor/` 目录，独立 `go.mod` 也可以，只要它不进 Server 的请求路径、不被 `fast-https` 的 `start` import。

第一版只做三件事：按配置的周期 `GET 127.0.0.1:9088/runtime`，把原始快照追加到本地 JSON 行文件，在连续失败时写一条「不可达」事件。默认周期 5 秒，超时 1 秒。失败要区分连不上、HTTP 非 200、JSON 不合 schema。PID 或 `started_at` 变化时写一条「进程已重启」，并且不要把计数变小算成负流量。

Monitor 自己的配置单独一个文件，例如采样周期、快照地址、事件文件路径。不写入站点 `fast-https.json`。Server 的 systemd 单元不 `Requires=` Monitor。Monitor 没启动时，第 3 步的测试仍然能直接访问 `/runtime`。

测试用 `httptest` 模拟快照服务：正常一条、连不上、错误 JSON、PID 变化。不要在单元测试里真的循环 5 秒，把时钟换成测试替身。

做完才算通过：Monitor 包测试全绿，同时 `go test ./runtime/...` 不因 Monitor 的依赖变红。

### 第 7 步：一条规则和一种记录

规则先做确定性的，不做模型。第一版只实现两条，并且只写本地事件文件：

- 连续 3 次采集失败，写「不可达」；之后有一次成功，写「恢复」。
- 最近 1 分钟的 5xx 增量除以请求增量，超过配置阈值且请求增量至少 20，写「5xx 升高」。请求增量不足 20 时不告警，避免除以很小的数。

同一条规则在触发后的冷却时间内不重复写。冷却默认 5 分钟。事件含时间、规则名、严重级别和触发时的快照摘要。不发邮件，不调 Webhook。

内存、文件描述符和 CPU 仍不在 Server 里做。这一步如果要做，只在 Monitor 里对快照中的 PID 读操作系统；读不到就在事件里写未知，不要当成 0。连接的读/写/空闲、多上游健康检查不在这七步里。

做完才算通过：用假的计数序列驱动规则，断言不可达、恢复、样本不足不告警、冷却期内不重复。不需要把规则接到线上进程才能合并这一步。

## 7. 每一步之外仍需隔离环境核对的事项

第 1 到第 5 步的单测覆盖功能正确性。下面几项要在测试机上对真实进程做一次，不在生产上做，也不是每一笔提交都做：

- 第 3 步合并后：业务端口正常响应时，关掉 `9088` 或让它绑定失败，业务端口仍然可访问。
- 第 5 步合并后：对运行中的进程发 reload，再发 `SIGTERM`，`/runtime` 在进程退出后连接失败，PID 文件按现有 `stop` 行为清理。
- 第 5 步合并后：几十个并发连接打完，`connections.active` 与 `requests.active` 回到 0。
- 第 6 步合并后：停掉 Monitor 进程，再请求业务端口，响应与 `/runtime` 都不受影响。
- 第 3 步之后抽查一次：每秒拉取 `/runtime` 约 10 次、持续 30 秒，业务请求没有出现新增的错误。这一项没有现成基准，只记录有没有错误和延迟是否明显变长，不设一个还没测过的毫秒门槛。

## 8. 不在 Server 里做的事

以下属于未来 Monitor，或明确不做：

- 周期采集、采集超时、识别计数器重置。这些在第 6 步，不进 Server。
- 耗时升高、内存增长这类窗口规则。第 7 步只做「不可达」和「5xx 比例」两条，耗时和内存不做。
- 邮件、Webhook、自动重试。第 7 步的去重和恢复只写本地事件文件。
- Prometheus、数据库、消息队列、大模型、自动修复。
- 为了监控而增加多上游或健康检查。
