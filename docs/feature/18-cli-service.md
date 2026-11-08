# 18. 服务管理与 CLI

状态：✅ 已实现

## 功能

`start`（后台服务）、`stop`、`reload`、`status`、`version`，以及 `acme`（交互式配置 ACME 自动签发）、`site`（管理 conf.d 下的站点文件）。`start` 与 `reload` 都先校验配置，失败即停并说明原因。

帮助文本有中英两版：默认中文，`--lang en` 或 `FAST_HTTPS_LANG=en` 切英文。

## 使用

```
fast-https start|stop|reload|status|version
fast-https service version [--json]     # 机器可读的版本查询
fast-https acme [setup|show]     # 交互向导 / 参数式配置 / 查看当前生效值
fast-https site init|list|show|remove   # 管理 conf.d 下的站点文件
```

安装与卸载不通过二进制完成：发行版包（deb / rpm / apk）负责文件落位，systemd 负责常驻，`uninstall` 由包管理器执行。

## 实现方案（cmd/、fast-https.go）

- **CLI**：cobra 子命令，各命令 handler 在 `cmd/commands.go`，启动前置检查（端口占用 `ScanPorts`、handler 齐备性 `core.MissingHandlers`）。
- **服务化**：安装与卸载交给发行版包与 systemd，二进制不再提供 install/uninstall；Windows 下 `reload` 先发 Ctrl+Break 再试 Ctrl+C，不把进程停掉（`cmd/reload_windows.go`）。
- **进程模型**：主进程 `ServerInit().Run()` 阻塞在 WaitGroup；pid 文件由真正提供服务的进程写入，`stop`/`reload`/`status` 经 pid 与信号交互。Unix 后台化走 exec 自身（`cmd/daemon_unix.go`），`stop` 发 `SIGTERM` 触发优雅停机并等进程退出后清理 pid 文件。
- **输出与退出码框架**：`cmd/cli_output.go`（JSON 封套、错误输出）、`cmd/cli_errors.go`（`CLIError`、错误码到退出码的映射）、`cmd/cli_flags.go`（全局 `--json`/`--timeout`/`--request-id`、`Run`）。`main` 调 `cmd.Run()`，错误统一由 `reportError` 输出：旧命令仍是 stderr 上的 `Error: ...`，`--json` 时是 stdout 上的错误封套。`--json` 下用 `logging.SetConsoleOutput` 把控制台日志改到 stderr。不是 `*CLIError` 的错误一律退出码 1。
- **handler 注册**：`cmd.RegisterHandlers` 把 static/proxy/rewrite/devmod 注册进 core 注册表；`RegisterServices` 把 safe（黑名单/限流）、auth、tcp/websocket 劫持、缓存清理装配进 `core.Services` 钩子——core 不反向依赖功能包。
- **`acme` 命令**：`cmd/acme.go` 用 `charmbracelet/huh` 做交互表单（非 TTY 自动降级为逐行问答，`--non-interactive` 全走 flags），写回时走 `config.WriteACMESection` 的文本级局部替换，只改顶层 `acme` 段，注释与其它字段保持原样。
- **`site` 命令组**：`cmd/site.go` 只做参数解析与输出；文件生成、命名校验、原子落盘在 `modules/site`，合并视图与文件级校验复用 `config`（`SiteDirsFromFile`、`ValidateSiteFile`、`ValidateNewSiteFile`、`SnapshotServer`）。`command` 结构新增 `register func() *cobra.Command`，带子命令的命令走它注册，扁平命令保持原样。

## 测试

`tests/unit_test/cmd`（命令行为/未知命令错误）、`cmd/`（foreground/status/check；`cli_test.go` 锁封套与退出码，`compat_test.go` 锁住桌面端依赖的旧命令输出）。

## 已知限制

- `status` 依赖 pid 文件。进程异常退出留下的陈旧 pid 会被识别为 `stale`，`stop` / `reload` 遇到时按「没在跑」处理并清理文件，`status` 仍以非 0 退出。
