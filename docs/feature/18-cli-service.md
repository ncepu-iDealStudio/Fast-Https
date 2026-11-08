# 18. 服务管理与 CLI

状态：✅ 已实现

## 功能

`start`（后台服务）、`stop`、`reload`、`status`、`version`，以及 `acme`（交互式配置 ACME 自动签发）、`site`（管理 conf.d 下的站点文件）。`start` 与 `reload` 都先校验配置，失败即停并说明原因。`service reload`、`config apply`、`config rollback`、`site add/update/delete/enable/disable` 走管理核心的受控事务（恢复点、operation、幂等、失败补偿）。

帮助文本有中英两版：默认中文，`--lang en` 或 `FAST_HTTPS_LANG=en` 切英文。

## 使用

```
fast-https start|stop|reload|status|version
fast-https service version|status|reload [--json]   # 机器可读的版本 / 状态 / 受控 reload
fast-https runtime status|stats [--json]  # 读本机 /runtime 的瞬时状态与累计统计
fast-https monitor status|events [--json] # 只读 Monitor 状态文件与事件文件（未安装为 not_installed）
fast-https config path|show|validate|versions|diff|apply|rollback [--json] # 路径 / 脱敏配置 / 校验 / 版本 / 差异 / 应用 / 回滚
fast-https cert list|check --domain <d> [--json] # 只读的证书状态 / 检查
fast-https acme [setup|show]     # 交互向导 / 参数式配置 / 查看当前生效值
fast-https site init|list|show|remove   # 旧命令：只写文件并提示 reload
fast-https site add|update|delete|enable|disable [--json]  # 受控站点写操作（走配置事务）
```

安装与卸载不通过二进制完成：发行版包（deb / rpm / apk）负责文件落位，systemd 负责常驻，`uninstall` 由包管理器执行。

## 实现方案（cmd/、fast-https.go）

- **CLI**：cobra 子命令，各命令 handler 在 `cmd/commands.go`，启动前置检查（端口占用 `ScanPorts`、handler 齐备性 `core.MissingHandlers`）。
- **服务化**：安装与卸载交给发行版包与 systemd，二进制不再提供 install/uninstall；Windows 下 `reload` 先发 Ctrl+Break 再试 Ctrl+C，不把进程停掉（`cmd/reload_windows.go`）。
- **进程模型**：主进程 `ServerInit().Run()` 阻塞在 WaitGroup；pid 文件由真正提供服务的进程写入，`stop`/`reload`/`status` 经 pid 与信号交互。Unix 后台化走 exec 自身（`cmd/daemon_unix.go`），`stop` 发 `SIGTERM` 触发优雅停机并等进程退出后清理 pid 文件。
- **输出与退出码框架**：`cmd/cli_output.go`（JSON 封套、错误输出）、`cmd/cli_errors.go`（`CLIError`、错误码到退出码的映射）、`cmd/cli_flags.go`（全局 `--json`/`--timeout`/`--request-id`、`Run`）。`main` 调 `cmd.Run()`，错误统一由 `reportError` 输出：旧命令仍是 stderr 上的 `Error: ...`，`--json` 时是 stdout 上的错误封套。`--json` 下用 `logging.SetConsoleOutput` 把控制台日志改到 stderr。不是 `*CLIError` 的错误一律退出码 1。
- **handler 注册**：`cmd.RegisterHandlers` 把 static/proxy/rewrite/devmod 注册进 core 注册表；`RegisterServices` 把 safe（黑名单/限流）、auth、tcp/websocket 劫持、缓存清理装配进 `core.Services` 钩子——core 不反向依赖功能包。
- **`acme` 命令**：`cmd/acme.go` 用 `charmbracelet/huh` 做交互表单（非 TTY 自动降级为逐行问答，`--non-interactive` 全走 flags），写回时走 `config.WriteACMESection` 的文本级局部替换，只改顶层 `acme` 段，注释与其它字段保持原样。
- **`site` 命令组**：`cmd/site.go` 只做参数解析与输出；文件生成、命名校验、原子落盘在 `modules/site`，合并视图与文件级校验复用 `config`（`SiteDirsFromFile`、`ValidateSiteFile`、`ValidateNewSiteFile`、`SnapshotServer`）。`command` 结构新增 `register func() *cobra.Command`，带子命令的命令走它注册，扁平命令保持原样。`site add/update/delete/enable/disable`（`cmd/site_write.go`）先把线上配置树克隆到临时候选（`management/siteops`），在副本上调用 `modules/site`，再交给 S8 事务引擎；`--root` 由 `management/pathguard` 规范化并拒绝配置/证书/状态/日志目录。停用是把 `<name>.json` 改名为 `<name>.json.disabled`。旧的 `site init` / `site remove` 输出与退出码不变。
- **只读视图与脱敏**：`management/view` 把磁盘配置、站点与进程状态整理成输出数据（纯只读，不依赖 `cmd`），`management/redact` 按键名遮蔽口令、令牌、EAB 密钥和敏感请求头的值，路径不算秘密。`management` 是分层测试里的独立一层，只依赖 `infra`、`config`、`modules/site`、`modules/certmanager`。`cmd/service_status.go`、`cmd/config_cmd.go`、`cmd/site_json.go` 只做参数解析与输出，`cmd/view_errors.go` 把视图层错误转成错误码。
- **`service status` 判定**：进程在且至少一个配置端口回环可连是 `running`，进程在但端口都连不上是 `degraded`，没进程也没端口是 `stopped`（pid 文件残留时带 `stale_pid`），证据互相矛盾（没进程却有端口可连、pid 文件读不出）是 `unknown`。端口取自 `config.ListenPortsFromFile`，只读、不替换运行期快照。读得到 `/runtime` 时补 `uptime_seconds`、`started_at`、`version`；`config_version` 来自状态目录里当前 active 的配置快照。
- **`runtime status` / `runtime stats`**：`management/rtclient` 用标准库 HTTP 读 `127.0.0.1:9088/runtime`（可用 `FASTHTTPS_RUNTIME_URL` 覆盖）。只做客户端，不另写计数。`schema_version` 必须为 `"1"`，否则 `UNMET_DEPENDENCY`。连不上是 `SERVICE_UNREACHABLE`。`runtime stats` 的窗口固定为 `since_process_start`；`--since` / `--domain` 返回 `NOT_SUPPORTED`。
- **reload 强验证**：`management/svcctl.WaitRuntime` 要求快照可连、`state` 为 `running`、pid 与 `started_at` 不变、`config_generation` 加一。读不到快照时退回 `WaitBasic`。`started_at`/pid 变化视为重启失败（`ErrReloadedAsRestart`）；生成号不加一则 `ErrReloadNotApplied`，apply 走补偿回滚。
- **`monitor status` / `monitor events`**：`management/monview` 只读 `<日志目录>/monitor/status.json` 与 `events.jsonl`（测试可用 `FASTHTTPS_MONITOR_DIR` 覆盖）。不连接 Monitor 进程。状态文件缺失为 `not_installed`；`schema_version` 不是 `"1"` 时退出码 11。事件只返回 `time`、`type`、`severity`、`message`。
- **`config validate` 的告警**：`config.ValidateConfigFileWithWarnings` 把非致命告警作为返回值交出来；旧的 `ValidateConfigFile` 仍写同一条控制台日志。
- **`cert` 命令组**：`cmd/cert.go` 只做参数与输出，`management/view/cert_view.go` 把 `config.SSLServersFromFile`（按运行期规则把 server 块展开成「域名 → 证书文件」：第一个域名用 `ssl_certificate`，其余域名用 `<证书目录>/<域名>.pem`）与 `modules/certmanager.Inspect`（解析 PEM）合成输出。“是否由 ACME 管理”直接复用 `certmanager.Plan`，与运行期判断一致。私钥只在 `CheckKeyPair` 里在内存中参与一次配对计算，返回固定枚举值，不带出底层错误文本。

## 测试

`tests/unit_test/cmd`（命令行为/未知命令错误）、`cmd/`（foreground/status/check；`cli_test.go` 锁封套与退出码，`compat_test.go` 锁住桌面端依赖的旧命令输出，`cli_s2_test.go` 覆盖只读命令与脱敏，`cli_s8_test.go` 覆盖 apply/reload 用法与旧 reload 文案，`cli_s9_test.go` 覆盖站点写命令用法与旧 `site init/remove`，`cli_s11_test.go` 覆盖 `runtime status/stats` 与强验证相关 CLI 契约，`logs_test.go` 覆盖 `logs error` 可读控制台镜像）、`management/redact`、`management/view`、`management/rtclient`、`management/svcctl`、`management/txn`（故障注入）、`management/siteops`、`management/pathguard`、`config/validate_warnings_test.go`。`tests/client_test/runtime_e2e_test.go` 在 `FASTHTTPS_E2E_RUNTIME=1` 下核对真实进程的累计请求数。

## 已知限制

- `status` 依赖 pid 文件。进程异常退出留下的陈旧 pid 会被识别为 `stale`，`stop` / `reload` 遇到时按「没在跑」处理并清理文件，`status` 仍以非 0 退出。
- `config show` 的来源是磁盘，不是运行中进程实际生效的配置；`config_dirty_hint` 只是按修改时间的提示，且只在进程确实在跑时给出。
- `config validate --candidate` 校验整棵候选目录（主文件 + `conf.d`），不再读线上 `conf.d`。
- `service reload` / `config apply` 在 `/runtime` 可达时是强验证（`verification.level: runtime`，`reload_confirmed` 在生成号加一时为 true）；快照不可达时退回弱验证（`basic`）。`--dry-run` 始终是 basic。
- Monitor 采集进程在 `monitor/`，作为独立可执行程序随 deb / rpm / apk 与归档发布，有独立的 systemd 单元。CLI 只读它的 `status.json` 与 `events.jsonl`，不启停它；文件不存在时 `monitor status` 是 `not_installed`。
- `cert renew` 未实现。续期要在 Server 里加写通道，本阶段明确不做；`cert list` 与 `cert check` 覆盖读取。
- 命令字段、错误码与退出码见 [docs/usage.md](../usage.md) 第 10 节；示例只收录实际跑过的命令输出。
- `config apply --dry-run` 不写正式配置、不 reload、默认不创建 operation。非 dry-run 必须 `--yes`（`--confirm` 同义）。`site add/update/delete/enable/disable` 同样要求 `--yes`（除非 `--dry-run`）。
- 旧顶层 `reload` 仍保留原文案；`FindProcess`/发信号失败时仍 `Fatal`。新增端口被占用时改为命令失败，避免 Server 在 reload 时 `Fatal` 退出。
- `config show` 只展开 `http.include` 目录下直接的 `*.json`（与 `site list` 一致）；include 写成单个文件或不存在的目录时会给出告警。
- 不带 `--json` 的 `site show` 仍打印站点文件原文，包含写在文件里的口令；要脱敏输出请加 `--json`。
- `service status` 的端口探测会对配置端口各发起一次回环 TCP 连接，TLS 端口上服务端会看到一次未完成的握手。
- `cert list/check` 只读取配置里写明的证书路径，不遍历证书目录；`days_left` 向零取整（剩余 9.9 天显示 9）；不做证书链验证，`chain.checked` 恒为 false；当前用户读不了私钥时配对检查标为 `checked:false`，不算失败。
