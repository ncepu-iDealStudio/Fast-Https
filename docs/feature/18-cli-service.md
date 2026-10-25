# 18. 服务管理与 CLI

状态：✅ 已实现

## 功能

`start`（后台服务）、`dev`（前台开发模式）、`stop`、`reload`、`install`/`uninstall`（系统服务）、`status`、`check`（只校验配置）、`version`。

## 使用

```
fast-https start|dev|stop|reload|install|uninstall|status|check
```

## 实现方案（cmd/、fast-https.go）

- **CLI**：cobra 子命令，各命令 handler 在 `cmd/commands.go`，启动前置检查（端口占用 `ScanPorts`、handler 齐备性 `core.MissingHandlers`）。
- **服务化**：`kardianos/service` 跨平台安装为系统服务；Windows 下 `reload` 先发 Ctrl+Break 再试 Ctrl+C，不把进程停掉（`cmd/reload_windows.go`）。
- **进程模型**：主进程 `ServerInit().Run()` 阻塞在 WaitGroup；pid 文件记录运行中进程，`stop`/`reload`/`status` 经 pid 与信号交互。
- **handler 注册**：`cmd.RegisterHandlers` 把 static/proxy/rewrite/devmod 注册进 core 注册表；`RegisterServices` 把 safe（黑名单/限流）、auth、tcp/websocket 劫持、缓存清理装配进 `core.Services` 钩子——core 不反向依赖功能包。
- **dev 模式**：前台运行 + trace 级控制台 + pprof（`0.0.0.0:10000`）。

## 测试

`tests/unit_test/cmd`（命令行为/未知命令错误）、`cmd/`（foreground/status/check）。

## 已知限制

- `status` 依赖 pid 文件，进程异常退出留下陈旧 pid 需手工清理。
