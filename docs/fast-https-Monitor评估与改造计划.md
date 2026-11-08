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

## 6. 落地顺序

按改动从小到大。前五步是 Server 核心，后两步才是 Monitor 进程。

1. **生命周期状态和基础计数。** 只加原子变量和 `defer`，不加端口，也不改配置文件。这是后面所有步骤的数据来源。工作量最小。
2. **状态码分桶和解析失败计数。** 仍然只在进程内。所有提前返回的路径用同一个 `defer` 盖住。
3. **本机只读 `/runtime`。** 这是第一个需要新监听的台阶，也是独立 Monitor 的前提。绑定失败只记日志，不阻止 `start`。
4. **耗时直方图，以及上游失败计数。** 错误类型和请求出口已经在了，比连接状态机小。
5. **用测试把数字对清楚。** 包括 reload 不清计数、进程重启后计数从零开始、快照里能看到新的 PID 和启动时间。这一步只验证，不增加产品能力。
6. **再写独立 Monitor。** 先做周期采集和「连不上 / 进程没了」的事件记录。采样、阈值、告警目标放在 Monitor 自己的配置里。Server 启动不得等待它。
7. **最后才加规则和一种告警。** 先写本地事件文件。内存、文件描述符、CPU 在这一步由 Monitor 读操作系统。连接的读/写/空闲阶段、多上游健康检查，排在这些都稳定之后。

建议的第一版采样周期是 5 秒。接口刚加上时不要用 1 秒把本机打满。告警先做一种渠道。邮件和 Webhook 不在这一版。

## 7. 待运行验证

补上 P0 之后，在测试机上核对，不要在线上做：

- 启动、停止、reload。
- 并发请求下，活动连接和活动请求在结束后回到零，累计值只增。
- 4xx 与 5xx 落在对应计数，而不是扫访问日志才知道。
- 慢请求落在预期的耗时桶。
- 客户端中途断开后，活动连接被减回去。
- `kill -9` 之后 PID 消失，查询接口不再响应。
- 关掉查询监听时，业务端口仍能访问。
- 每秒多次拉取快照时，业务请求延迟没有明显上升。

## 8. 不在 Server 里做的事

以下属于未来 Monitor，或明确不做：

- 周期采集、采集超时、识别计数器重置。
- 5xx 比例、耗时升高、内存增长这类窗口规则。
- 告警去重、恢复事件、重试。第一版只写本地事件文件。
- Prometheus、数据库、消息队列、大模型、自动修复。
- 为了监控而增加多上游或健康检查。
