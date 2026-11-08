# fast-https Monitor 代码扫描报告

日期：2026-11-08。本文回答一个问题：**现有代码是否具备开发独立 Monitor 守护进程的条件。** 对应 [开发前置条件与代码扫描清单](fast-https-Monitor开发前置条件与代码扫描清单.md) 第 7 节要求的评估报告。配套的执行方案见 [Monitor 改造开发计划](fast-https-Monitor改造开发计划.md)。

扫描方法：只读源码，沿调用链确认，不按函数名推断；没有改代码，也没有启动服务。静态阅读确认不了的事项标为「待运行验证」，汇总在第 8 节。首版结论经过一次对照源码的复核，更正和补充记录在附录 A。

整体评级：**C**。进程能启动、能热重载、能写日志，但没有一份独立进程能读到的运行时状态。在补上进程内计数和只读查询接口之前，Monitor 只能看 PID 和日志，分不清「进程活着」和「还能正常服务」。

## 1. 执行摘要

对清单 §7.1 的五个问题逐条回答：

1. **是否具备开发 Monitor 的基础条件？** 不具备。能确认的只有：PID 是否还在、配置是否校验过、端口在启动那一刻是否绑定成功。
2. **能否直接开发 Monitor？** 不能，必须先改 Server 核心。现有 `safe.Snapshot` 只在进程内部，外部进程读不到。
3. **最大的三个缺口：** 没有可查询的生命周期状态；没有连接数、请求数、状态码、耗时的运行时计数；没有回环地址或本机套接字这类受控出口。
4. **稳定性与安全风险：**
   - 监听失败直接 `Fatal` 退出，没有「降级」状态；reload 新增被占用端口也会让整个进程退出。单条连接 panic 只打日志，外面看不出来。
   - 现在没有管理端口。以后若做查询接口，不能把黑名单全文、证书和请求头带出去。
   - `modules/safe/limit.go` 每个新 IP 桶 `println` 一行，`modules/safe/control.go` 的 `applyRateOverlay` 也有一处。内建 `println` 写标准错误，后台模式下进 `daemon.log`，systemd 下进 journald。与监控无关，但会冲淡日志，建议单独清掉。
5. **最小实施路径：** 先加原子计数和生命周期状态，再加一个只绑定本机的只读快照，用测试对一下数字，然后才写独立 Monitor。内存、文件描述符、CPU 由 Monitor 读操作系统，不必塞进 Server。

要点补充：

- 能复用的不多，但可以参考：原子计数写法（`modules/safe`）、生命周期钩子（`core.Services`）、上游错误已分类（`modules/proxy`）、`Restart=on-failure` 已能把 Server 拉起来。
- Server 今天不依赖任何 Monitor，「Monitor 不运行时 Server 照常工作」这一条已经满足。
- Monitor 自己的周期采集、规则和告警不是 Server 缺陷，排在核心计数和查询接口之后。
- 仓库里没有 Monitor 源码。`monitor/` 目录里只有一个未入库的 `monitor.exe`，是旧托盘程序的构建残留，不能当作依据（附录 A）。

## 2. 逐项检查

编号沿用清单 §7.2（A–N）。清单 §3 另有一项「指标语义」（§3 的 G），§7.2 的表里没有，补在表末。

| 编号 | 前置条件 | 状态 | 代码证据 | 缺口与风险 | 建议 |
| --- | --- | --- | --- | --- | --- |
| A | 生命周期状态 | 部分具备 | 入口 `fast-https.go` 的 `main` 只执行 `cmd.RootCmd`。`cmd.StartHandler`（`cmd/commands.go`）写 PID、初始化、`ServerInit`、`notifyDaemonReady`、`Run`。`Server.stopping` 只在 `drainConnections`（由 `gracefulStop` 调用）里置位（`runtime/server/server.go`），不对外读。`Reload` 没有「重载中」标志，且 `s.Listens = lisAll` 是无锁赋值。监听失败在 `listenTcp`（`runtime/listener/listener.go`）直接 `Fatal`。`InspectProcess`（`cmd/commands.go`）只看 PID 是否还在。Windows 上 `stop` 是 `Process.Kill()`（`cmd/daemon_windows.go` 的 `signalStopProcess`），不走 `gracefulStop` | PID 在不等于可服务。启动成功只通过一次性管道告诉父进程（`cmd/daemon_unix.go` 的 `notifyDaemonReady`），之后查不到。Accept 出错只退避重试（`serveListener`），进程仍显示在跑。连接 panic 被吃掉，主循环不知道。Windows 上观测不到 `stopping` | 进程内放一份原子状态：启动中、运行中、重载中、停止中。记下启动时刻和 `config.CURRENT_VERSION`。reload 不改启动时刻。状态与监听端口列表放在 `runtime/metrics` 自己的原子字段里，快照不去读 `Server.Listens` |
| B | 连接状态统计 | 不具备 | `serveConn` 用 `connWg` 只为停机等待，`WaitGroup` 读不出当前数量。没有接受、关闭、读、写、空闲计数。HTTP/1.1 与 HTTP/2 都走 `serveConn` | 停机等待不能当成活动连接数。客户端断开、读超时都只是 `HandleEvent` 返回（`runtime/events/events.go`），没有计数。TCP 转发与 WebSocket 会话是长连接，会长期占用活动连接数 | 在 `serveConn` 的进入和 `defer` 里做原子加减，得到活动连接、累计接受、累计结束。读/写/空闲阶段先不做 |
| C | 请求与状态码 | 不具备 | 请求循环在 `HandleEvent` / `EventHandler`。状态码只进访问日志 `dynlog.fieldStatus`（`runtime/dynlog/dynlog.go`）。`safe.stats.totalChecked` 在 `Bucket` 里每个连接加一次（`modules/safe/limit.go`），不是每条请求。`locationRejected` 只有字段，没有 `Add`（`modules/safe/safe.go`）。`parseRequest` 返回 0 有多种原因：客户端关闭、头部畸形（已写 400）、请求体读超时、请求体过大、分块解码失败，后三种静默断开。被 `ConnFilter`（`runtime/filters/filters.go` 的 `GConnFilter`）拒绝的连接直接返回，不进入 `EventHandler`。HTTP/1.1 的 `Response` 随连接复用，`static`、`dev_mod`、`rewrite` 就地改写它 | 把限流检查次数当成请求数会错。4xx/5xx 只能事后扫日志，提前断开、解析失败不一定有访问日志。被拒绝的连接不会出现在状态码里。读状态码可能读到上一个请求的残留 | 在 `EventHandler` 入口 `defer` 计数，覆盖 HTTP/1.1 与 HTTP/2。按最终状态码归入 2xx/3xx/4xx/5xx。解析失败、中途静默断开、被拒绝连接各自单独计数 |
| D | 请求耗时 | 不具备 | 连接用 `time.Now()` 设 30 秒读写截止（`runtime/events/events.go`）。上游连接拨通后设 20 秒截止（`modules/proxy/event_proxy.go`）。访问日志字段表没有耗时 | 日志时间是写出时刻，不是处理时长，也不能当直方图。墙钟会受对时影响 | 用 `time.Since` 的单调时钟，在请求 `defer` 里落入固定桶。先不做分位数库 |
| E | 资源指标 | 部分具备 | 生产代码没有 `MemStats`、`NumGoroutine`、`getrlimit`。systemd 单元写了 `LimitNOFILE=65535`（`build/packaging/systemd/fast-https.service`），那是启动限额，不是当前用量 | Server 内部不知道内存和文件描述符。这些可以由 Monitor 对 PID 读操作系统 | Server 不采集。Monitor 以后用 PID 读进程内存、CPU 时间、打开的文件数。读不到就标明未知，不要填 0 |
| F | 上游 | 部分具备 | 每地址一个连接池 `connPool.get`（`modules/proxy/pool.go`），空闲连接在锁里，没有导出数量。拨号失败、读写超时是 `ProxyErrorDialUpstream`、`ProxyErrorSendToUpstreamTimeout`、`ProxyErrorReadFromUpstreamTimeout`（`modules/proxy/event_proxy.go`、`proxy_read.go`），只打日志。没有健康检查，也没有第二个上游 | 池子状态外部不可见。同一个错误值在多处被 `==` 比较，计数若加在比较处会重复。`ProxyErrorDialUpstream` 还在 websocket 的类型转换失败处返回，并非拨号失败。上游地址是内部拓扑，不应输出 | 计数加在错误产生的那一处。空闲数用原子计数在 `get`/`put`/清扫里维护，不在读取时拿池锁，也不输出按地址分组的明细。健康检查仍不做 |
| G | 外部读取接口 | 不具备 | `status` 是另一个进程读 PID 文件（`cmd/commands.go` 的 `statusHandler`）。`safe.Snapshot`（`modules/safe/safe.go`）只在本进程内，还会把整份黑名单拷出去。没有回环 HTTP，没有 Unix 套接字 | 内部变量外部读不到。若直接把 `Snapshot` 暴露出去，会泄漏黑名单，而且持锁拷贝，不适合高频读 | 新增只读快照：只含状态、计数、版本、观测时间。只监听 `127.0.0.1`。默认不影响现有请求端口 |
| H | 故障信号 | 部分具备 | 启动失败：配置、端口、证书在 `PreCheckHandler` 里 `Fatal`，父进程能从管道 EOF 看到。reload 失败留旧配置并打日志。优雅停机超时打一行日志。5xx、解析失败、上游超时没有计数。reload 新增被占用端口时 `listenTcp` 直接 `Fatal` | 信号散落在日志里，只扫日志会漏掉没写访问日志的失败，也会把历史日志当成当前状态。reload 新增被占用端口导致的进程退出，在进程内无法计数 | 先把 reload 失败、停机超时、5xx、解析失败、上游超时变成计数或状态位。进程退出类故障由 Monitor 通过 `started_at`、PID 变化或不可达来识别 |
| I | 事件记录 | 不具备 | reload 起止是控制台日志。没有可轮转的 `server.started` 一类事件文件 | 崩溃前没执行到的日志行会丢。当前文件日志经 `syncWriter` 串行写出，没有单独的事件流 | 第一阶段不做事件总线。Monitor 轮询状态，自己记下「从运行变成不可达」 |
| J | 生命周期管理 | 部分具备 | 包安装用 systemd：`Type=simple`、`ExecStart` / `ExecReload`、`Restart=on-failure`、`RestartSec=2s`。Unix `stop` 发 `SIGTERM`，最多等 `GracefulStopTimeout`（5 秒）。源码直跑会自己转后台。没有 Monitor 单元 | Server 今天不依赖任何 Monitor，这条已经满足。意外退出后没有另一个进程负责记录 | 保持 Monitor 与 Server 分单元、分进程。Server 启动不得等待 Monitor |
| K | 开销与并发 | 不适用 | 请求路径上已有的原子计数只有限流那几项。`Snapshot` 会锁住并复制黑名单 | 若在每次请求里扫连接表，或在读指标时拿 `Server.mu`，会拖垮热路径 | 热路径只做原子加减。读接口只 `Load` 原子值，不遍历连接 |
| L | 日志 | 部分具备 | 四路 JSON，时间是带时区的 RFC3339（`infra/logging/consts.go` 的 `TimeFormat`）。访问日志有状态码和大小，没有耗时、没有请求 ID。轮转靠 logrotate 的 `copytruncate`、单文件 100MB（`build/packaging/logrotate/fast-https`）。写入失败不会让请求返回错误 | 日志能帮忙排障，不能当运行时计数。URI、Host、User-Agent 都是客户端可控内容 | 保持日志为辅。Monitor 只尾读新增行，不要每轮扫全文件 |
| M | 时间与计数语义 | 不具备 | 没有请求计时。没有「本次进程启动时间」。reload 会换配置快照，但没有统计量可被它清掉 | 以后如果 reload 把计数清零，Monitor 会把下降看成负流量 | 计数只在进程重启时归零。reload 不动累计值。快照里带启动时间和 PID，供 Monitor 识别重启 |
| N | 监控配置 | 不具备 | `config/schema.go` 的 `jsonConfig` 没有 Monitor 或运行时接口字段 | 若把告警阈值写进站点 JSON，Monitor 故障会和业务配置缠在一起 | 查询地址先写死在回环上。采样周期、阈值、告警放在 Monitor 自己的配置里 |
| 清单 §3-G | 指标语义 | 不具备 | 没有任何运行时指标，也就没有 Counter/Gauge/Histogram 的语义约定 | 无从判断读取是否阻塞请求、reload 与重启如何处理计数 | 按计划第 2.3 节的计数规则和本文第 5 节的字段表与数据口径确定语义；读取只做原子 `Load` |

## 3. 现有可复用能力

| 能力 | 位置 | 能用到什么程度 |
| --- | --- | --- |
| 进程是否还在 | `cmd.InspectProcess` | 区分没有 PID 文件、PID 已过期、进程仍在。不能说明端口仍在服务 |
| 启动是否绑上端口 | `cmd.notifyDaemonReady` | 在 `ServerInit` 之后通知父进程。一次性的，不是查询接口 |
| 停机与重载 | `Server.gracefulStop`、`Server.Reload` | 配置校验失败会留下旧监听。状态位可以挂在这两条路径上 |
| 生命周期钩子 | `core.Services` 的 `BeforeStop`、`AfterStop`、`AfterListen` | `/runtime` 监听的关闭可挂在 `AfterStop` 上。Windows 的 `Kill` 路径不会触发它，由操作系统回收 |
| 原子计数写法 | `modules/safe` 的 `stats` 与 `Snapshot` | 写法可参考。不要把黑名单拷贝进监控快照。`LocationRejected` 目前一直是 0 |
| 上游错误分类 | `modules/proxy` 的三个 `ProxyError` | 拨号失败、写超时、读超时已经分开，加计数即可（注意计数位置，见第 2 节 F 行） |
| 状态行解析 | `runtime/events/events_h2.go` 的 `statusFromFirstLine` | 解析失败回落 200，用于写 HTTP/2 响应是对的；用于计数会把未知算成 2xx，不能直接复用，计数需要另写返回「是否解析成功」的版本 |
| 日志 | `infra/logging`、logrotate | 留给排障，不作为指标来源 |
| 部署 | `build/packaging/systemd/fast-https.service` | `Restart=on-failure` 能把 Server 拉起来。Monitor 以后用另一个单元 |

`modules/safe/control.go` 的加删黑名单和调限流是控制面协议，不是本方案要改的运行时指标。不要把它当成 Monitor 的数据源，也不要为了做监控去删它（AGENT.md 规则 5）。

## 4. 缺失能力与改造清单

对应清单 §7.4。P0 不补就做不出可靠的 Monitor；P1 建议第一阶段跟上；P2 往后。步骤号见 [Monitor 改造开发计划](fast-https-Monitor改造开发计划.md)。

| 优先级 | 编号 | 缺失能力 | 修改模块 | 影响范围与风险 | 对应步骤 |
| --- | --- | --- | --- | --- | --- |
| P0 | 1 | 进程内生命周期：状态、启动时间、版本、PID、监听端口副本、reload 生成号与结果。reload 期间标成重载中，失败回到运行中并保留旧配置 | 新建 `runtime/metrics`；改 `runtime/server` | 只在启动、reload、停机路径上写，热路径不读。reload 不改启动时刻 | 第 1 步 |
| P0 | 2 | 连接与请求计数：活动连接、累计接受、累计结束、被拒绝连接；活动请求、累计请求、2xx/3xx/4xx/5xx、解析失败、中途断开 | `runtime/server`、`runtime/events`、`runtime/metrics` | **热路径**：每连接、每请求几次原子加减，加一次状态行解析。`defer` 覆盖提前返回与 panic。reload 不清零 | 第 1、2 步 |
| P0 | 3 | 本机只读快照：独立小监听，只 `Load` 原子值，带 `schema_version`、观测时间。未知字段省略。绑定失败只记日志 | `cmd`（组合根）、`runtime/metrics` | 新增一个监听端口。启动时机要早于证书初始化。不含黑名单、证书、Cookie、`Authorization` | 第 3 步 |
| P1 | 4 | 请求耗时：固定桶直方图，单调时钟，放在请求 `defer` 里 | `runtime/events`、`runtime/metrics` | 每请求一次 `time.Since` 与一次桶累加 | 第 4 步 |
| P1 | 5 | 上游计数：拨号失败、读超时、发送超时累计，连接池空闲条数 | `modules/proxy`、`cmd` | 错误路径上的原子加一。`modules` 不能 import `runtime/metrics`（分层测试），走通用注册 | 第 4 步 |
| P1 | 6 | 停机与重载结果：停机超时、reload 失败计数 | `runtime/server`、`runtime/metrics` | 只在低频路径上 | 第 1、5 步 |
| P2 | 7 | 连接处于读、写还是空闲 | `runtime/events` | 要改连接状态机，容易漏路径，暂不做 | — |
| P2 | 8 | 文件描述符、内存、CPU | 放在 Monitor，对 PID 读操作系统 | Server 不实现 | — |
| P2 | 9 | 多上游和健康检查 | — | 现在每个 location 只有一个 `proxy_pass`，不要为了监控先做故障转移 | — |

Monitor 自身需要实现的能力（清单第 4 节，不是 Server 缺陷）：周期采集、规则检测、事件记录，对应计划第 6、7 步。

## 5. Runtime 接口建议

对应清单 §7.5，按真实代码得出的最小设计。

**传输方式与理由：** 只绑定 `127.0.0.1` 的 HTTP，一个路径 `/runtime`。Monitor 是另一个进程，本机 HTTP 最好写、最好测。Unix 套接字在 Windows 上还要另做一套，不作为第一步。不占用现有业务端口，避免和站点路由缠在一起。

**监听地址与权限：**

- 地址先写死 `127.0.0.1:9088`，不进站点 JSON（全仓没有其它地方使用 9088）。以后要改端口，再加一个与站点无关的配置项，默认仍是回环。
- 只绑 IPv4 回环。本机其它用户都能读到，所以响应里不能有任何敏感内容，由禁止词测试守住。
- 回环地址只在同一个网络命名空间内可达，Monitor 与 Server 需同机同命名空间（容器场景要放在同一个容器或共享网络命名空间）。

**响应字段：**

| 字段 | 类型 | 来源 |
| --- | --- | --- |
| `schema_version` | Info | 常量，字段含义变化时才升 |
| `observed_at` | Info | 快照读出时的绝对时间 |
| `server.state` / `pid` / `version` / `started_at` | Info / Gauge | 生命周期（P0-1） |
| `server.config_generation` | Counter | 每次 reload 成功加一，重启归零 |
| `server.last_reload_ok` / `last_reload_at` | Info | 最近一次 reload 的结果与时间 |
| `server.reload_failed_total` / `stop_timeout_total` | Counter | `Reload` 失败返回、`drainConnections` 超时 |
| `listeners` | Info | 当前监听端口与是否 ssl/h2，只含端口，不含证书路径；由 reload 时整体替换的原子副本提供 |
| `connections.active` / `accepted_total` / `closed_total` | Gauge / Counter | `serveConn` 的进入与退出 |
| `connections.rejected_total` | Counter | `ConnFilter` 拒绝 |
| `requests.active` / `total` | Gauge / Counter | `EventHandler` 的 `defer` |
| `requests.parse_errors_total` / `aborted_total` | Counter | 解析失败写出 400；请求中途静默断开 |
| `responses_2xx_total` 至 `responses_5xx_total`、`responses_unknown_total` | Counter | 最终写出的状态码 |
| `request_duration` | Histogram | 单调时钟，固定桶，单位毫秒 |
| `upstream.dial_fail_total` / `send_timeout_total` / `read_timeout_total` / `idle_conns` | Counter / Gauge | 错误产生处；连接池原子计数 |

示例（数据形态，字段以上表为准）：

```json
{
  "schema_version": "1",
  "observed_at": "2026-11-08T01:00:00+08:00",
  "server": {"state": "running", "pid": 1234, "version": "V1.6.1",
             "started_at": "2026-11-07T08:00:00+08:00", "config_generation": 3},
  "listeners": [{"port": "8080"}, {"port": "443", "ssl": true, "h2": true}],
  "connections": {"active": 128, "accepted_total": 90000, "closed_total": 89872, "rejected_total": 12},
  "requests": {"active": 8, "total": 1250000, "parse_errors_total": 3, "aborted_total": 1},
  "responses_2xx_total": 1200000, "responses_4xx_total": 30000, "responses_5xx_total": 20000
}
```

**采集成本：** 几次原子读，不遍历连接，不拿 `Server.mu`，不持锁复制。读取失败或字段不可用时省略并在响应中说明，不用 0 冒充未知。

**版本兼容策略：**

- 只新增字段不升 `schema_version`；改变已有字段含义或删除字段才升。
- 调用方（CLI、Monitor）对不认识的字段忽略，对不认识的 `schema_version` 拒绝。
- 数据口径写进字段说明：`requests.total` 只含进入 `EventHandler` 的请求；`responses_*_total` 只含这些请求的最终状态码；解析失败、被拒绝连接单独计，不进入 `responses_*_total`；TCP 转发与 WebSocket 会话进入 `connections.active`，不进入 `requests.active`（升级握手那一次请求会计一次）。

**不暴露的内容：** 黑名单、证书路径、上游地址、请求内容、Cookie、`Authorization`。

## 6. 与现有契约的冲突和兼容约束

| 约束 | 内容 | 对改造的要求 |
| --- | --- | --- |
| 分层测试 | `tests/arch/layering_test.go`：`runtime` 不能依赖 `modules`；`modules` 只能依赖 `runtime/{listener,request,response,dynlog}`，不能依赖新建的 `runtime/metrics` | 连接和请求计数放 `runtime/metrics`；上游计数留在 `modules/proxy`，由 `cmd`（组合根）在组装时通过通用注册函数交给快照；新包要登记在分层测试里 |
| 测试里的全局状态 | 计数是进程级原子量；`runtime/events` 里有测试直接调 `EventHandler`；`parseRequest` 的返回值（1 与 0）被测试断言 | 测试之间清空计数，不用 `t.Parallel`；`parseRequest` 的返回值契约不变，在各出口处加计数 |
| 数据面热路径 | 每个请求都经过 `EventHandler` | 计数只用原子加减；改动前后做同一压测对比并记录差异；`-race` 必跑 |
| `modules/safe/control.go` | AGENT.md 规则 5：不得删除或精简 | 本方案不动它，也不把 `Snapshot` 当监控出口 |
| 日志字段契约 | `infra/logging/consts.go` 的字段名是对外契约 | 本方案不改日志字段，耗时只进 `/runtime`，不进访问日志 |
| 旧 Monitor 残留与命名 | 提交 `5ab84d6`（「移除 monitor 托盘程序」）移除了旧托盘 Monitor 的源码、README 与 `docs/feature/19-monitor.md`；`monitor/monitor.exe`（5.1MB）是被 `.gitignore` 的 `*.exe` 忽略的构建残留；`config/consts.go` 里没有任何引用的 `MONIITOR_LOG_FILE_PATH`（拼写也是错的）同属残留；托盘现在由 `desktop/` 承担 | 新建 Monitor 前删掉残留文件；新 Monitor 不沿用该常量；文档和打包里不要再用「Monitor」指代托盘 |
| 打包的五种形态 | AGENT.md 规则 1、2：安装包、systemd 单元、安装脚本、两套目录布局、`tests/packaging` | 本方案前几步不改运行期路径与打包；Monitor 进安装包单独成步（计划第 8 步），要过 `tests/packaging` 与容器安装验证 |
| Windows 与容器 | Windows 的 `stop` 是 `Kill`，没有优雅停机；Windows 没有 `daemonize`；容器内回环只在本命名空间可达 | `stopping` 与 `stop_timeout_total` 在 Windows 上不出现；`SIGTERM` 相关测试只在 Unix 跑；第一阶段只写 Linux systemd 单元，Windows 与容器由使用者自行托管 |
| 启动顺序 | `StartHandler` 顺序为 PreCheck → 写 PID → 横幅 → `InitSystem`（含首次生成 RSA 3072 的 CA 与自签证书）→ 装配 → `ServerInit` → `notifyDaemonReady` → `Run`；Unix 后台模式下父进程在 `daemonize` 后就返回 | `/runtime` 监听放在写 PID 之后、`InitSystem` 之前，才能让 Monitor 看到 `starting`；放在 `ServerInit` 之后，`starting` 对外永远不可见 |

## 7. Monitor MVP 可行性与盲区

对应清单 §7.6。

- **能否启动独立 Monitor：** 能，且 Server 不依赖它。
- **如何取得状态：** 周期 `GET 127.0.0.1:9088/runtime`；进程资源（内存、文件描述符、CPU 时间）由 Monitor 按快照里的 PID 读操作系统，读不到就标未知。
- **Server 崩溃如何发现：** `/runtime` 连续失败，同时 PID 文件指向的进程不存在，判定为崩溃；`Restart=on-failure`、`RestartSec=2s` 拉起后，`started_at` 与 PID 变化，判定为「已重启」，计数下降不算负流量。
- **采样周期依据：** 一次采集只做几次原子读，成本很低；默认 5 秒、超时 1 秒、连续 3 次失败告警，崩溃发现延迟约 15 秒，可配置。没有实测数据，这些是起点，不是承诺。
- **Monitor 自身故障：** 由服务管理器重启；Monitor 停止期间的状态变化无法补记，恢复后从当前快照继续，并记一条「采集中断」。
- **第一阶段能检测的：** 不可达与恢复、进程重启、5xx 比例升高；可选的业务端口不可达、疑似卡死提示。`/runtime` 由独立小监听提供，它能响应不代表业务端口还能服务，所以业务端口探测值得做成可选规则。
- **当前检测不到的：**
  - 业务端口开着但个别请求持续很慢（耗时规则不在第一阶段，直方图有了之后再做）；
  - 内存缓慢增长、文件描述符逼近上限（Monitor 读操作系统的规则也不在第一阶段）；
  - 上游健康（没有健康检查，只有失败计数）；
  - reload 新增被占用端口导致的进程退出，只能事后以「重启」形式发现；
  - Windows 与容器环境；
  - 证书即将过期（属于 CLI 的 `cert check`，不是 Monitor 第一阶段的规则）。
- **告警渠道：** 第一阶段只写本地事件文件，之后若加渠道，先加一种，且失败不阻塞采集。

## 8. 静态无法确认、需运行验证的事项

对应清单 §7.7。验证在隔离测试环境进行，不在生产上做；具体步骤与对应的计划步骤见 [Monitor 改造开发计划](fast-https-Monitor改造开发计划.md) 第 4 节。

| 事项 | 为什么静态确认不了 | 对应步骤 |
| --- | --- | --- |
| 并发请求下计数是否准确、活动数是否回到 0 | 取决于各出口路径的实际调用顺序，包括 panic 与客户端中途断开 | 第 1、2、5 步 |
| 同一条 keep-alive 连接上连续请求的状态码是否串号 | `Response` 对象随连接复用，各模块是否都会设置状态行要靠测试证实 | 第 2 步 |
| `/runtime` 绑定失败时业务端口是否照常服务 | 涉及启动顺序与错误处理 | 第 3 步 |
| 启动期能否读到 `starting` | 取决于 `InitSystem` 实际耗时，首次生成 CA 才有窗口 | 第 3 步 |
| reload 新增被占用端口时进程退出并被 systemd 拉起 | 涉及 `Fatal` 与 `Restart=on-failure` 的实际行为 | 第 5 步 |
| Unix 上 reload 后 `SIGTERM` 的停机行为；Windows 上 `stop` 后的残留 | 平台差异 | 第 5 步 |
| Monitor 不运行时 Server 是否正常 | 需要真实进程 | 第 6 步 |
| 高频读取 `/runtime` 对业务请求的影响 | 没有现成基准 | 第 3 步之后抽查 |
| 计数加入热路径后的吞吐与延迟变化 | 没有现成基准 | 第 2、4 步合并前后对比 |

## 9. 需要你确认的决策点

计划里已经给出推荐值，没有异议就按推荐执行。编号用 MD 前缀，避免与 CLI 扫描报告的 D1–D8 混淆。

| # | 决策 | 选项 | 推荐 |
| --- | --- | --- | --- |
| MD1 | `/runtime` 监听的启动时机 | A. `InitSystem` 之前（能看到 `starting`）；B. `ServerInit` 之后（改动更保守） | A |
| MD2 | `/runtime` 端口 | 写死 9088；或加独立配置项 | 先写死，需要时再加配置项 |
| MD3 | 业务端口探测、疑似卡死提示 | 第 7 步做；不做 | 做，作为可选规则 |
| MD4 | 清掉两处 `println` | 单独提交；不处理 | 单独提交，放在第 0 步 |
| MD5 | 删除 `monitor/monitor.exe` 残留与 `MONIITOR_LOG_FILE_PATH` 常量 | 第 6 步前删；保留 | 删 |
| MD6 | Monitor 是否进安装包 | 第一阶段只交付源码；同步做打包（第 8 步） | 先源码，打包单独评审 |

## 10. 结论

- **是否具备条件：不具备，必须先改 Server 核心。** 这和 CLI 不同：CLI 前半段几乎全是新增代码，而 Monitor 的前置条件（计数、状态、快照）全部落在数据面上，其中请求计数在热路径。
- **可复用的：** 原子计数写法、生命周期钩子、上游错误分类、systemd 的 `Restart=on-failure`。**必须改造的：** `runtime/server`、`runtime/events`、`modules/proxy` 的几处，以及 `cmd` 起一个本机只读监听。
- **推荐先做什么：** 先做第 1 步（生命周期与连接计数）。它只在连接入口和出口各加一次原子加减，影响最小，也是后面所有步骤的底座。
- **不要提前做的事：** 事件总线、Prometheus、数据库、告警渠道、自动修复、多上游健康检查。

执行顺序、每一步的改动范围和验收测试见 [Monitor 改造开发计划](fast-https-Monitor改造开发计划.md)。

## 附录 A 复核记录（对照源码，2026-11-08）

首版结论（评级 C，先改 Server 再做 Monitor）复核后成立。复核时逐处核对了源码，并结合 [CLI 改造开发计划](fast-https-CLI改造开发计划.md) 对 `/runtime` 的需求，修正和补充了下表各项，正文已体现。

| 类别 | 内容 | 影响的位置 |
| --- | --- | --- |
| 更正 | `println` 写的是标准错误，不是标准输出；而且有两处（`modules/safe/limit.go` 每个新 IP 桶一行，`modules/safe/control.go` 的 `applyRateOverlay` 一行） | 第 1 节第 4 点 |
| 更正 | 计数不能复用 `statusFromFirstLine`：它解析失败时回落 200，会把未知状态算进 2xx | 第 3 节、计划第 2 步 |
| 更正 | 上游计数不能挂在 `ProxyError*` 每一处引用上：同一个错误值在多处被比较，会重复计数；`ProxyErrorDialUpstream` 还在 websocket 的类型转换失败处返回，并非拨号失败 | 第 2 节 F 行、计划第 4 步 |
| 更正 | `monitor/monitor.exe` 不是「来历不明」：提交 `5ab84d6` 移除了旧的 monitor 托盘程序，这个文件是被 `*.exe` 忽略的构建残留 | 第 1、6 节 |
| 遗漏 | `parseRequest` 返回 0 有多种原因，除 400 和客户端关闭外，还有请求体读超时、请求体过大、分块解码失败，这些静默断开连接且没有响应，首版没有归类 | 第 2 节 C 行、计划第 2 步 |
| 遗漏 | HTTP/1.1 的 `Response` 对象随连接复用，`static`、`dev_mod`、`rewrite` 就地修改它，读状态码可能读到上一个请求的残留 | 第 2 节 C 行、计划第 2 步 |
| 遗漏 | 计数点放在 `EventHandler` 内部才能同时覆盖 HTTP/1.1 与 HTTP/2，也能覆盖它的各个提前返回 | 第 2 节 C 行、计划第 2 步 |
| 遗漏 | 被 `ConnFilter` 拒绝（黑名单、限流）的连接不进入 `EventHandler`，不会出现在请求和状态码计数里 | 第 2 节 C 行 |
| 遗漏 | 首版把 `/runtime` 放在 `ServerInit` 之后启动，`starting` 状态对外永远不可见；而启动期要生成 CA 和自签证书，窗口真实存在 | 第 6 节、计划第 3 步 |
| 遗漏 | reload 新增被占用的端口时 `listenTcp` 直接 `Fatal`，进程退出，`reload_failed_total` 记不到这种失败 | 第 2 节 H 行、计划第 5 步 |
| 遗漏 | `Reload` 里 `s.Listens = lisAll` 没有加锁，快照不能直接读 `s.Listens` | 第 2 节 A 行、计划第 1 步 |
| 遗漏 | Windows：`stop` 是 `Process.Kill()`，没有优雅停机，`stopping` 与 `stop_timeout_total` 观测不到；`SIGTERM` 验证项不适用 | 第 2 节 A 行、第 6 节 |
| 遗漏 | 清单 §3 的「指标语义」一项在 §7.2 的表里没有对应行；清单 §7.5、§7.6 要求的接口建议与 MVP 可行性首版没有单独成节 | 第 2、5、7 节 |
| 补充 | CLI 的强验证需要 `config_generation`、`last_reload_ok`、`last_reload_at`、`listeners` 等字段 | 第 5 节 |
