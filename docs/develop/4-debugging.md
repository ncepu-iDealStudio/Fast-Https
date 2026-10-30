# 4. 调试与可观测

## 日志五路（logs/ 下 JSON）

| 文件 | 内容 | 典型排查 |
| --- | --- | --- |
| system.log | 启动/reload/监听/证书 | "为什么端口没起来"、"reload 到了哪一步" |
| access.log | 逐请求（字段按 log_format） | 请求有没有进来、状态码、路由类型 |
| error.log | 错误 | 上游失败、证书问题 |
| safe.log | 安全拒绝，带 `reason` | blacklist / rate_limit / location_limit / waf |
| daemon.log | 后台进程的控制台输出（每次 `start` 覆盖） | `start` 立刻失败、进程起来就没了 |
| console（stdout） | 与文件并行的控制台回显 | dev 模式前台观察 |

排查顺序建议：access.log 确认请求到达与状态码 → safe.log 看是否被安全层拒绝（reason 字段直接给出层级）→ error.log 找内部错误 → system.log 看生命周期事件。

后台模式的两点约定：

- 子进程的 stdout/stderr 指向 `logs/daemon.log` 而不是 `/dev/null`：绑端口失败、配置校验失败这类启动期 Fatal 只会往 stdout 写，丢了就只剩「进程没了」。
- 子进程绑定完全部端口后才向父进程报告就绪。父进程读到「就绪」才打印 started，读到 EOF（子进程启动期退出）就返回非零并把 `daemon.log` 尾部打出来。所以 `start` 的退出码是可信的，不需要再 `status` 复查。

## WAF 观察

```json
"waf": { "mode": "monitor" }
```

monitor 模式检测但不阻断，safe.log 的 `reason=waf` 记录（含 detector/score）即为误报分析素材；`enforce` 模式被阻断的请求在 access.log 里是 403。

## pprof（dev 模式）

`go run . dev` 在 `0.0.0.0:10000` 挂标准库 mux：

```
go tool pprof http://127.0.0.1:10000/debug/pprof/heap
go tool pprof http://127.0.0.1:10000/debug/pprof/goroutine
curl http://127.0.0.1:10000/debug/pprof/profile?seconds=30 > cpu.prof
```

关注点：每请求分配（heap/alloc_objects）、goroutine 数量（连接数基线）、CPU 火焰（压缩/解析）。

## 常见问题

| 现象 | 排查 |
| --- | --- |
| 端口拒绝连接 | system.log 看 Listen 行是否存在；`check` 命令校验配置；端口被占会在 `start` 阶段就 Fatal 并指明端口号 |
| `start` 返回成功但 `status` 说 stale | 4b 之后不应发生：父进程按子进程的就绪信号判定成败。复现则先看 `logs/daemon.log` 尾部 |
| `start` 报 port ... already in use | `ss -ltnp \| grep :<port>` 找占用者；这个检查在 fork 之前完成，命中时不会留下后台子进程 |
| reload 后行为没变 | console 看 reload start/end 与 "reload kept previous config"（校验失败保留旧配置是设计行为） |
| reload 时进程退出 | 4b 之后不应发生——mime/include 失败返回错误而非 Fatal；若复现按 bug 处理 |
| 连接被静默关闭（EOF） | 查安全层：safe.log 的 reason；历史上 Reject 钩子语义反向出过这种事故 |
| 404 但文件存在 | 路径穿越防护把 Clean 后逃出 root 的路径按 404 处理；核对 root 与 URL 拼接结果 |

## 性能定位

基准入口见 [1-testing.md](1-testing.md)；热路径优化的既有结论：解析 819ns、每请求分配已压到个位数 allocs——新代码进入热路径前先跑基准对比。
