# 0. 架构总览与请求生命周期

状态：✅ 已实现

## 功能

本文不是独立功能，而是理解其余文档的地图：fast-https 的分层结构，以及一个 HTTP 请求从字节到响应经过的全部环节。

## 分层结构

```
cmd/        组合根：CLI、handler 注册、core.Services 装配
fast-https.go  入口
core/       事件契约：Event/RRcircle、handler 注册表、服务钩子（不依赖 modules）
runtime/    数据面：server（调度）、listener（监听/证书）、events（事件分发、h2）、
            request/response（协议解析与序列化）、filters（路由过滤）、dynlog（访问日志）
modules/    功能面：static、proxy、proxy_tcp、websocket、rewrite、cache、compress、
            safe、auth、appfirewall、dev_mod
config/     配置：JSON schema 解析、快照（atomic）、校验
infra/      基础设施：logging（zerolog 五路）、color、timeutil、banner（彩色横幅）
init/       启动编排：配置→日志→证书→缓存
```

依赖方向单向向下：cmd → modules → core → runtime → config → infra。依赖守卫测试（`tests/arch`）固化边界。

## 一次请求的生命周期（HTTP/1.1）

```
1. serveListener Accept（每端口一个 goroutine，SO_REUSEPORT）
2. serveConn：每连接一个 goroutine + panic 兜底 recover
3. HandleEvent 循环（keep-alive 逐请求复用）：
   a. ConnFilter   —— 连接级：safe 黑名单、per-IP 令牌桶
   b. ListenFilter —— websocket/tcp 代理劫持连接（不进 HTTP 管线）
   c. parseRequest —— 读缓冲（sync.Pool）→ ParseHeader（零拷贝解析，
                      分片头循环读满、畸形 400）→ ParseHost → ParseBody
                      （chunked 走独立解码器）
   d. EventHandler —— RequestFilter 路由匹配（Hosts() 快照）→
                      LocationRejected（计数限流）→ AuthRejected（Basic Auth）→
                      handler 分发（local/proxy/rewrite/devmod）
4. EventWrite —— 响应序列化（generateHeaders 单次预分配）+ 写出
5. EventReuse？回到 3
```

## 关键并发设计

| 共享状态 | 策略 |
| --- | --- |
| 配置快照（config.Current/MIME） | `atomic.Pointer`，reload 整体替换 |
| Listener 路由表（Hosts/Cfgs） | Listener 内 `atomic.Pointer` 快照，热更新走 `updateCfg` |
| 访问日志字段表、四路文件日志器 | `atomic.Pointer`，reload 整体替换 |
| 监听器列表 | `[]*Listener` 指针语义，无数组别名耦合 |
| 读缓冲 / gzip writer | `sync.Pool` |
| 上游连接、缓存热层 | 互斥锁池 |

原则：**reload 与在途请求之间没有任何无同步的共享可变状态**——全部走"构建新快照 → 原子替换"。

## 相关文档

[1-http1.md](1-http1.md)、[4-reload.md](4-reload.md)、[15-appfirewall.md](15-appfirewall.md)、`docs/develop/plans/architecture-convergence-plan.md`
