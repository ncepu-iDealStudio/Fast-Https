# 20. 本机运行快照 `/runtime`

状态：✅ 已实现（第 1–3 步字段；耗时与上游见后续步骤）

## 功能

进程内提供一份只读运行快照，供 CLI 与独立 Monitor 采集。只绑定 IPv4 回环 `127.0.0.1:9088`，路径 `GET /runtime`，无认证、无 TLS。响应不含黑名单、证书路径、Cookie、`Authorization`、请求 URI 或上游地址。

回环地址只在同一个网络命名空间内可达。Monitor 与 Server 需同机同命名空间（容器里放在同一容器或共享网络命名空间）。

## 使用

```powershell
Invoke-RestMethod http://127.0.0.1:9088/runtime | ConvertTo-Json -Depth 5
```

启动早期（写完 PID、证书初始化完成之前）即可访问，此时 `server.state` 为 `starting`。业务端口绑定完成后变为 `running`。9088 被占用时业务端口仍会起来，本接口不可用。

其它路径返回 404。`schema_version` 目前为 `"1"`：只新增字段不升版本；改变已有字段含义或删除字段才升。调用方对不认识的字段忽略，对不认识的版本拒绝。

尚未实现的段（`request_duration`、`upstream`）不会出现，不会用 `0` 冒充未知。

## 实现方案

- 计数与状态在 `runtime/metrics`：生命周期、连接、请求、状态码、reload 生成号、监听端口原子副本。快照只做原子 `Load`，不持 `Server.mu`，不读无锁的 `Server.Listens`。
- HTTP 出口在组合根 `cmd/runtime_endpoint.go`。`StartHandler` 在写 PID 之后、`InitSystem` 之前启动监听；`RegisterServices` 把关闭挂到 `core.Services.AfterStop`。
- 分段注册：`metrics.RegisterSection` 供组合根挂入功能包数据（例如以后的 upstream），`runtime` 不 import `modules`。
- `requests.total` 只含进入 `EventHandler` 的请求；解析失败与被拒绝连接单独计，不进入 `responses_*_total`。TCP / WebSocket 会话计入 `connections.active`，升级握手那一次请求计入 `requests.total`。

## 测试

`runtime/metrics/snapshot_test.go`、`cmd/runtime_endpoint_test.go`（测试绑定 `127.0.0.1:0`，不占用 9088）。

## 已知限制

- 端口写死 9088，没有配置项。
- Windows 的 `stop` 走 Kill，不会跑 `AfterStop`，监听由系统回收。
- 第一阶段不探测业务端口是否仍能服务；`/runtime` 能响应不代表站点端口可用。
