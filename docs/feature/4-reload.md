# 4. 配置热重载

状态：✅ 已实现

## 功能

`SIGINT`（或 `reload` 命令）在不停止服务的前提下加载新配置：配置、MIME 表、路由表、日志文件、证书全部原子切换；校验失败保留整份旧状态。

## 使用

```
fast-https reload        # 或向进程发 SIGINT
```

## 实现方案（runtime/server/server.go Reload、runtime/listener/reload.go、config）

### 快照原子切换（架构收敛计划阶段 4b，2026-10 完成）

| 状态 | 策略 |
| --- | --- |
| 配置/MIME | `config.Current()`/`MIME()`（atomic.Pointer），校验全过后整体 Store |
| Listener 路由表 | `Listener.cur atomic.Pointer`，请求路径经 `Hosts()`/`Cfgs()` 读取 |
| 访问日志字段表 | `dynlog.logFields` atomic 整体替换 |
| 四路文件日志器 | `logging.Open` 经 `System()/Access()/Error()/Safe()` 原子换入 |

在途请求与 reload 协程之间没有无同步的共享可变状态；读取端要么看到完整旧状态、要么完整新状态。

### 监听器增删改（listener/reload.go）

- **新增端口**：新建 `*Listener`（新 Lfd/Ctx）→ 发布快照 → `RunAdded` 启动 serveListener。
- **移除端口**：取消 ctx + 关闭 Lfd，serveListener 以 `ErrClosed` 干净退出。
- **common 端口（类型不变）**：直接复用旧 `*Listener` 指针，`updateCfg` 整体替换配置快照——在途 keep-alive 连接即刻读到新路由。
- **类型变化**（http→ssl）：旧监听器关闭、新监听器按新增启动（2026-10 修复：此前新监听器从未被启动、端口死掉）。

### 失败语义

`config.Reload` 校验失败 → 记日志、**保留旧配置与旧监听器**；mime/include 文件出错不 Fatal（2026-10 修复）。

### 配套行为

- reload 成功后按新 `log_root` 重新打开四路日志（新目录打不开则继续写旧文件）。
- `safe.Init`/`dynlog.LogRegister`/`cache.LoadCache` 在每次监听器创建后重跑。

## 测试

`config/reload_window_test.go`（-race 并发读 + 50 次 reload 零撕裂）、`tests/unit_test/runtime/listener/reload_common_test.go`、`runtime/listener/hot_update_race_test.go`、`runtime/server/reload_cert_test.go`、E2E：`FASTHTTPS_E2E_RELOAD=1`。

## 已知限制

- reload 期间新建连接与新配置之间有毫秒级窗口（先切配置后切监听）。
