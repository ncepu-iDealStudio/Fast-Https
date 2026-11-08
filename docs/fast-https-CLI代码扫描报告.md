# fast-https CLI 代码扫描报告

日期：2026-11-08。本文回答一个问题：**现有代码是否具备按 [CLI 命令与接口开发规范](fast-https-CLI开发规范_AI代码智能体版.md)（下称「规范」）开发的条件。** 配套的执行方案见 [CLI 改造开发计划](fast-https-CLI改造开发计划.md)。

**这是开工前的扫描，不是现状说明。** 文中「没有 JSON 封套」「没有 Monitor」「`runtime` 无数据」描述的是当时的仓库。落地后的行为见 [feature/18-cli-service.md](feature/18-cli-service.md) 与 [feature/21-monitor.md](feature/21-monitor.md)。

扫描方法：只读源码，不改业务代码；另跑了一次构建和相关包测试作为基线（第 8 节）。结论以调用链为准，不按函数名推断；静态阅读确认不了的事项标为「待运行验证」。

规范第 11 节要求先回答的 10 个「待确认项」，答案在第 2 节。规范里的 P0/P1 命令逐条评估在第 4 节。

整体评级：**B-**（可以开工，但必须分阶段）。

- 能直接复用的不少：Go + cobra 命令树、双语帮助、配置校验、站点文件读写、原子写文件、PID 与信号管理，都已经有，并且有测试守着。
- 缺的是三块「地基」：统一的 JSON 输出与退出码；落盘的管理核心（配置版本、operation、幂等）；CLI 到运行中服务的通道（运行指标、reload 结果、证书续期）。前两块可以只在 CLI 侧补，不碰数据面；第三块要改 Server 核心，并且已经有 [Monitor 代码扫描报告](fast-https-Monitor代码扫描报告.md) 和 [Monitor 改造开发计划](fast-https-Monitor改造开发计划.md)。
- 规范里约一半命令（只读查询、日志、诊断、配置版本、站点写）不依赖 Server 核心改动，可以先做。`runtime *`、`monitor *`、`cert renew` 必须等 Server 侧能力。

## 1. 执行摘要

1. **具备开工条件，不具备「一把做完」的条件。** 命令框架、配置与站点模块可复用；但 `--json` 封套、退出码、操作记录、配置版本、幂等这些规范核心要求，现有代码一项都没有（仅 `config path --json` 是个孤例）。
2. **最大的三个缺口：** ① CLI 与运行中服务之间只有「PID 文件 + 信号」，没有查询通道，`reload` 发完信号就返回，成败只在日志里；② 没有任何运行时计数，`runtime status/stats` 无数据可读；③ 没有配置版本与 operation 持久化，`config apply/rollback` 没有可依赖的底座。
3. **规范与现状有 4 处命名/语义冲突**（第 5 节）：`status`/`version`/`reload` 已是顶层命令；`config` 组已被桌面端依赖；`site` 现有 `init/remove` 而规范要 `add/update/delete`；站点没有启用/停用概念。处理原则：旧命令一个字不改，新能力以新子命令加进去。
4. **有几处会直接违反规范，必须先补：** 配置加载路径会往 stdout 写 JSON 日志（`--json` 会被污染）；`site show` 原文打印，Basic 认证口令是明文；`main` 对所有错误一律 `exit 1`；`stop`/`reload` 里用 `ConsoleLog.Fatal` 直接退出进程。
5. **桌面端是独立 Go module（`fast-https-desktop`），而且解析主程序的文本输出。** 它里面有可参考的日志查询、证书检查实现，但不能 import；它依赖 `version`、`status`、`stop`、`reload` 的文本和 `config path --json` 的字段，改这些输出会直接弄坏桌面端。
6. **打包有硬约束。** 新增命令必须重新生成 man 页与 `usage.html`（有测试拦）；新增任何运行期目录必须同时改归档与包两套布局常量；新增状态目录要动 systemd 单元与安装脚本。这是配置版本与 operation 落盘的主要「隐性代价」。

## 2. 规范第 11 节「待确认项」逐项回答

| # | 待确认项 | 现状 | 证据 |
| --- | --- | --- | --- |
| 1 | 语言与 CLI 框架 | Go 1.27，cobra v1.10.2；命令描述用 `t(中, 英)` 双语，`TestEveryCommandTranslated` 兜底；`cmd.RootCmd()` 同时供 man 页与 `usage.html` 生成器使用 | `go.mod`；`cmd/commands.go`；`cmd/text_test.go`；`tools/manpage`、`tools/docgen` |
| 2 | 配置格式、目录、reload 机制 | JSONC 主配置 `fast-https.json` + `http.include` 目录（默认 `conf.d`）；运行期路径由 build tag 决定两套布局；reload = 向 PID 发 `SIGINT`（Windows 发 Ctrl 事件），**没有返回结果** | `config/consts.go`、`config/base_dir*.go`；`cmd/commands.go` 的 `reload`；`cmd/reload_windows.go`；`runtime/server/server.go` 的 `SigHandler` |
| 3 | 站点是否有独立实体 | 有：站点 = `conf.d/<name>.json` 一个文件，ID 是文件名主干；主配置 `http.server[]` 里内联的站点没有 ID，`site` 命令改不了；没有启用/停用 | `modules/site/site.go`；`cmd/site.go` |
| 4 | 证书模块现有命令与状态 | `acme setup/show` 只改写/展示配置的 `acme` 段；续期与签发全部在 **Server 进程内** 的 `certmanager`（certmagic + 数据面钩子应答挑战）；没有 `cert` 命令组，没有证书状态查询接口 | `cmd/acme.go`；`modules/certmanager/manager.go` |
| 5 | Runtime 指标能否获取 | 不能。没有连接数、请求数、状态码、耗时计数；`safe.Snapshot` 只在进程内，且 `totalChecked` 是每连接一次的限流检查，不是请求数。独立 Monitor 进程不存在（`monitor/` 里只有一个未入库的 `monitor.exe`，是提交 `5ab84d6` 移除的旧托盘程序的构建残留，不作依据） | [Monitor 扫描报告](fast-https-Monitor代码扫描报告.md) 第 2 节；`modules/safe/safe.go` |
| 6 | 日志格式、轮转、敏感字段 | 四路单行 JSON（`access`/`error`/`system`/`safe`），时间 RFC3339 带毫秒，字典序即时间序；logrotate `copytruncate`、单文件 100MB。**`error.log` 只收 `logging.PrintErr` 的几行**，真正的错误（reload 失败、ACME 失败、连接 panic）走 `ConsoleLog`，只到 stdout / `daemon.log` / journald。`path` 含查询串，可能带 token；没有任何脱敏实现 | `infra/logging/*.go`；`build/packaging/logrotate/fast-https`；`cmd/daemon_unix.go` 的 `daemonLogPath` |
| 7 | 是否由 systemd 管理 | 包安装用 systemd（`Type=simple`、`ExecReload=… reload`、`RuntimeDirectory`）；单元文件里没有 `User=`（grep 无命中，待运行验证：推断以 root 运行）；源码直跑自己转后台；容器/systemd 下保持前台 | `build/packaging/systemd/fast-https.service`；`cmd/commands.go` 的 `runInForeground` |
| 8 | Monitor 是否已存在 | 不存在（见第 5 项）；已有扫描报告与分步改造计划，未实施 | `docs/fast-https-Monitor代码扫描报告.md`、`docs/fast-https-Monitor改造开发计划.md` |
| 9 | operation 持久化 | 没有。`grep` 全仓无 `operation_id`、`request_id`、audit 相关实现 | 全仓搜索 |
| 10 | 配置版本与回滚 | 没有。只有桌面端的单文件 `.bak`（写前备份一份、站点删除改名为 `.bak`），不是版本体系，也不在主模块里 | `desktop/config_editor.go`；`desktop/site_editor.go` |

## 3. 规范通用约定的符合度（规范第 2 节）

| 规范要求 | 现状 | 差距 |
| --- | --- | --- |
| 命令格式 `fast-https <group> <command>` | 部分符合：`site`、`acme`、`config` 已是组；`start/stop/reload/status/version` 是顶层 | 新增 `service`、`runtime`、`operation`、`logs`、`cert`、`monitor` 组；`diagnose` 顶层。旧顶层命令保留 |
| 全局 `--json` | 只有 `config path` 有自己的局部 `--json`；没有全局 flag | 需要根命令 PersistentFlags，并统一输出层 |
| `--timeout`、`--request-id`、`--config` | 只有 `site`/`acme` 的局部 `--config`；其余无 | 新增全局 flag；`--request-id` 只对修改命令生效 |
| 不支持的参数必须报错 | cobra 默认对未知 flag 报错并退出 1 | 退出码需改为 2（见下） |
| stdout 只输出一个 JSON | **不满足。** `ConsoleLog` 写 stdout；`config.LoadFile` 路径上有 `ConsoleLog.Info`（`config/convert.go`）和 `ConsoleLog.Warn`（`config/acme.go`）；cobra 错误走 stderr 是对的 | `infra/logging` 增加「控制台输出重定向」能力，CLI 的 JSON 模式把它改到 stderr |
| JSON 封套 `ok/command/request_id/data/error/warnings` | 无 | 新增统一输出层 |
| 退出码 0–11 | `fast-https.go` 对所有错误 `os.Exit(1)`；`StopHandler`/`reload` 内部 `ConsoleLog.Fatal` 直接 `os.Exit(1)` | 新增错误类型到退出码的映射；`Fatal` 路径重构为返回 error（保持旧命令对外行为不变） |
| 错误码与退出码一一对应 | 无 | 新增错误码表，测试锁定映射 |
| 时间 RFC 3339 带时区 | 日志已是；`InspectProcess` 等无时间 | 新输出统一使用 |
| `--dry-run`、`--confirm` | 无 dry-run。确认机制现为 `site remove --yes`（`acme setup`/`site init` 另有 `--non-interactive`） | dry-run 随事务引擎实现；确认标志建议 `--yes` 与 `--confirm` 同义 |
| 敏感信息不外泄 | 现有 `site show` 原文输出，含 `auth.pswd` 明文；`cmd/acme.go` 有 `maskSecret` 可复用 | `config show`、`site show --json` 必须走脱敏层，不改旧 `site show` 文本行为 |

## 4. 命令逐条评估

难度档：S 小（几个文件、只在 `cmd` 或新包内）；M 中；L 大（需要事务/落盘/重构旧路径）；XL 需要 Server 核心改动或新进程。

### 4.1 P0

| 命令 | 现状与可复用 | 缺口 | 依赖 | 档 |
| --- | --- | --- | --- | --- |
| `service status` | 有顶层 `status`：读 PID、判活，区分 运行/残留 PID/无 PID（`InspectProcess`）。桌面端也用端口探测 | 无 JSON；无 `degraded`；`uptime`、`config_version`、`started_at` 取不到（PID 文件里只有 pid） | 仅基础字段：无；完整字段：Monitor 计划第 1、3 步 | S（基础）/ 依赖 Server（完整） |
| `service version` | 顶层 `version` 打印编译期常量 | `server_version` 无法从运行中的进程取得（磁盘上的二进制可能已被升级而进程没重启） | `/runtime` | S（只返回 CLI 版本，`server_version` 标明不可用） |
| `runtime status` | 无 | 无任何数据源 | Monitor 计划第 1–3 步 | XL |
| `runtime stats` | 无 | 无请求/状态码/耗时计数 | Monitor 计划第 2、4 步 | XL |
| `config show` | 无；`site show`（原文，不脱敏）、`acme show`（有掩码）、`config.SnapshotServer` | 脱敏层；「当前生效配置」只在进程内存里，CLI 只能读磁盘，需标注 `source: disk` 并给出 `config_dirty`（桌面端有 mtime 启发式可参考） | 无 | S–M |
| `config validate` | `config.ValidateConfigFile(path)` 现成，`check` 命令已用 | ① 错误是 `fmt.Errorf` 文本，没有字段路径结构；② 警告只写日志，不返回；③ 候选目录隔离问题（G7） | 无 | S（校验当前或候选主文件）/ M（候选目录树） |
| `config diff` | 无；`config/site_diff.go` 是把改动写回文件的补丁器，不是差异比较 | 配置树差异（JSONC → 对象 → 路径级新增/删除/修改）与敏感值遮蔽 | 无（版本对比需配置版本） | M |
| `config apply` | 无事务；`modules/site` 的「写 `.tmp` → 校验 → rename」是单文件原子 | 整套管理核心：恢复点、journal、reload、健康验证、补偿恢复、幂等 | 配置版本 + operation + 受控 reload | L |
| `config versions` | 无 | 版本存储（含状态目录与两套布局常量） | 状态目录 | M |
| `config rollback` | 无 | 同 apply | 同 apply | L |
| `operation list/get` | 无 | journal 存储与对账（执行者进程已死时识别 `unknown`） | 状态目录 | M |
| `logs error` | 部分：日志是结构化 JSON，可按字段过滤；桌面端 `queryLogs` 逻辑可参考（但在独立 module） | 数据源不全（G6）；脱敏；截断；限制 | 数据源补全属另一步 | M |
| `diagnose` | 无；各检查项的部件大多已有 | 组合与分级（失败 vs 不可用） | 前述只读命令 | M |

### 4.2 P1

| 命令 | 现状与可复用 | 缺口 | 档 |
| --- | --- | --- | --- |
| `site list` | `site list` 有文本表；`site.List` 返回结构体 | JSON、启用状态、证书状态 | S |
| `site show` | `site show` 有（原文 / `--merged`） | JSON 与脱敏 | S |
| `site add` | `site init --non-interactive` 已可用，`site.Create` 先 `.tmp`、校验、再 rename | dry-run、request-id、事务与 reload、`--root` 路径边界校验 | M（依赖事务引擎） |
| `site update` | `site init` 同名即更新，`PatchSiteFile` 保留注释与未建模字段 | 规范要求「只允许明确支持的字段」；事务；只能改 include 文件，不能改内联站点 | M |
| `site delete` | `site remove --yes` 直接 `os.Remove`，不碰内容目录（符合规范） | 事务、dry-run；现在不可恢复（桌面端是改名 `.bak`） | M |
| `site enable/disable` | 无概念 | `include` 只收 `*.json`（`config/validate.go` 的 `includeFiles`），可用改后缀实现，且可恢复；需与 `.bak` 约定协调 | M |
| `cert list/check` | 无；`certmanager.Plan`、桌面端 `inspectPEM`（读 PEM 得颁发者、有效期、SAN、是否自签）可参考 | 在主模块里实现一份；域名匹配；私钥不输出 | S–M |
| `cert renew` | 无 | 续期只在 Server 进程内，挑战由数据面钩子应答（`core.Services.HandleEarlyRequest`、`listener.RegisterTLSALPNProvider`）；CLI 进程不能自己续期，也没有触发通道 | XL（需 Server 控制通道） |
| `logs access` / `logs query` | 同 `logs error` | 白名单过滤字段；`--domain` 依赖 `log_format` 含 `host`（默认配置含） | M |
| `monitor status/events` | Monitor 不存在 | 全部 | 阻塞 |

## 5. 与现有契约的冲突和兼容约束

| 约束 | 内容 | 对改造的要求 |
| --- | --- | --- |
| 命名冲突 | `status`/`version`/`reload`/`stop`/`start` 已是顶层命令，`systemd` 的 `ExecReload` 调用的就是 `fast-https reload` | 旧命令保持原样；新增 `service` 组。`service status` 与 `status` 并存，二者共用同一内部函数，只是输出形态不同 |
| `config` 组已被桌面端依赖 | 桌面端调用 `config path --json` 并读 `config/cert/log/pid/root_crt` 五个字段 | `config` 组下新增子命令，`path` 不改 |
| 桌面端解析文本 | 桌面端取 `version` 输出的第一行当版本；`status`/`stop`/`reload`/`start` 的文本回显到界面；桌面端 `stopServer` 依赖 `stop` 在无 PID 时「按没在跑处理」 | 新增 `cmd/compat_test.go`，把这几条旧命令的输出形态锁住（本改造的第 1 步就加） |
| 打包两套布局 | 归档布局的路径「一个字都不能变」（`config/paths_layout_test.go` 逐字锁定）；包布局由 `-tags=rpm` 决定；`base_dir.go` 与 `base_dir_rpm.go` 必须同改 | 新增状态目录常量时两处同时加；两种 tag 都要 `go vet` 与测试 |
| 生成物 | 改了命令树必须重新生成 man 页与 `usage.html`，否则 `tools/manpage`、`tools/docgen` 的测试失败 | 每个新增命令的步骤都带「重新生成」检查项 |
| 分层测试 | `tests/arch/layering_test.go` 限定 `infra`/`config`/`core`/`runtime`/`modules` 的依赖方向；`cmd`、`init`、根是唯一允许装配全部层的地方；未归类的新目录不受约束 | 新增管理核心目录 `management/` 时，同时把它登记为一层（只依赖 `infra`、`config`、`modules/site`），防止以后被乱依赖 |
| 双语 | 命令描述必须用 `t(中, 英)`，缺中文会让 `TestEveryCommandTranslated` 失败 | 新命令的 `Short`/`Long` 一律双语 |
| `modules/safe/control.go` | AGENT.md 明确禁止删除或精简；它是将来 agent 控制面协议，且是进程内 API | 本次 CLI 不动它。CLI 是独立进程，调不到它；规范第一阶段也不含黑名单/限流命令 |
| Windows | 项目同时发 Windows 版；reload 用 `GenerateConsoleCtrlEvent`；停止是 `TerminateProcess`（无优雅停机）；全量测试要 `-p 1` | 新代码必须 Windows 可编译；信号相关逻辑放 `_unix.go`/`_windows.go`；`service start/stop/restart` 不进第一阶段（规范也允许） |

## 6. 关键缺口详述

**G1 没有统一输出与退出码。** `fast-https.go` 的 `main` 只有 `os.Exit(1)`；`config path --json` 自己 new 一个 `json.Encoder`；`site`/`status`/`acme` 直接 `Fprintf`。`StopHandler`、`reload` 里多处 `logging.ConsoleLog.Fatal()`，进程直接退出，上层拿不到 error。

**G2 stdout 会被日志污染。** `ConsoleLog` 是 `zerolog.New(os.Stdout)`。`config.LoadFile` → `processIncludeCfg` 会写 `Include config dir: …`（`config/convert.go`），`acme` 校验会写 http-01 端口告警（`config/acme.go`）。任何加载配置的 CLI 命令在 `--json` 下都会输出多余的 JSON 日志行，违反规范 2.1。修法很小：`infra/logging` 加一个设置控制台输出目标的函数，JSON 模式下指向 stderr。

**G3 没有管理核心。** 版本、operation、request-id 幂等、审计都没有。可复用的只有：`infra/files.WriteAtomic`（写 `.tmp` 再 rename）、`modules/site` 的单文件事务、`config.ValidateConfigFile`。规范 5.2 明确说「单次 rename 不能保证整个配置事务原子」，所以必须自建恢复点与 journal。

**G4 CLI 到运行中服务没有查询通道。** 目前只有 PID 文件与信号：`SIGINT` = reload，`SIGTERM`/`SIGQUIT` = 优雅停机。`reload` 的 CLI 端先校验配置再发信号，之后是否成功只能靠日志。Server 侧 `Reload` 失败只打 `ConsoleLog.Error("reload kept previous config")`。后果：`service reload` 与 `config apply` 的「reload 后做健康验证」只能做弱验证（进程仍存活 + 配置端口可连通），要强验证需 Monitor 计划的 `/runtime`。另一个风险：reload 新增一个已被占用的端口时，`listenTcp` 用 `Fatal`，整个进程退出（Monitor 扫描报告第 2 节 A、H 行），所以 apply 前必须做「新增端口空闲」预检。

**G5 没有运行时指标。** 与 Monitor 扫描报告结论一致：没有连接数、请求数、状态码、耗时；`/runtime` 只读快照在 Monitor 改造计划第 3 步。CLI 的 `runtime *` 只该是这个接口的客户端，不另写一套计数。

**G6 日志数据源不全。** `error.log` 只由 `logging.PrintErr` 写（缓存写失败、代理读错误等几处）。`ConsoleLog` 里的 reload 失败、ACME 失败、连接 panic、配置错误只到 stdout：Unix 后台模式下是 `daemon.log`（每次 `start` 清空），systemd 下是 journald，Windows 前台则是控制台。因此 `logs error` 第一版只能如实返回 `error.log`，并在 `warnings` 里说明覆盖不全；要让它真正有用，需要把 `ConsoleLog` 的 Warn/Error 同时写入文件日志（`infra/logging` 小改，但影响全进程日志量，单独成步）。另外 `access.log` 的 `path` 带查询串，需要做脱敏；日志里没有 Authorization/Cookie（访问日志字段是固定白名单，见 `infra/logging/consts.go`），但 `user_agent`、`path` 都是客户端可控内容。

**G7 候选配置无法隔离校验。** `ValidateConfigFile(path)` 能校验任意主文件，但 `http.include` 与证书路径用 `absETCPath` 按运行期 ETC 基址解析（带 `.` 兜底），不是相对候选文件所在目录。所以拿一棵候选目录树去校验，读到的 `conf.d` 其实是线上那份。解决方式是给 `config` 包加一个带基准目录参数的校验入口（现有调用点不变），属于对核心模块的小而真实的改动，由 `config/paths_layout_test.go`、`config/abs_path_test.go` 等守着。另外校验错误只有文本，规范要的「字段路径」只能先放进 `message`，结构化需另行重构。

**G8 证书续期无法从 CLI 进程发起。** `certmanager` 的 `activeManager` 是进程内单例，挑战应答挂在 Server 的钩子上，`cert renew` 要么在 Server 里加控制通道（本机套接字或带令牌的回环接口），要么不做。规范允许不支持的功能返回 `NOT_SUPPORTED`。`cert list/check` 只读 PEM，不受影响。

**G9 站点模型与规范有出入。** 站点就是 `conf.d/<name>.json`；`site init` 同名即更新；`remove` 永久删除；没有启用/停用。`site.Update` 对 include 文件做就地补丁并保留注释，这点比规范要求更好。规范里的「站点 ID 由管理核心生成」在这里直接取文件名主干（`site.ValidateName` 已限制为 `[A-Za-z0-9._-]`，天然防路径穿越）。

**G10 权限模型。** 现状没有角色，全靠操作系统文件权限。pid、配置默认 0644，新写文件 `0o644`，证书私钥 `0o600`。规范第一阶段允许沿用 OS 权限，但要保证「只读用户不能改配置」。改造时需要在写命令入口做一次可写性预检并映射为 `PERMISSION_DENIED`（退出码 3），而不是等写到一半才失败。Windows 上权限语义是 ACL，只能做到尽力而为（待运行验证）。

**G11 敏感字段。** 配置里的敏感项：location 认证的 `pswd`（明文）、ACME 的 `eab_kid`/`eab_hmac_key`、证书私钥路径（路径本身不算秘密，内容算）。`cmd/acme.go` 的 `maskSecret` 只用于 `acme show`。新增统一脱敏层，并用「把已知口令写进夹具，断言所有新命令输出都不含它」的测试守住。

## 7. 安全面扫描

| 项 | 结果 |
| --- | --- |
| 任意 Shell 执行 | 主模块 `cmd` 里没有 `exec.Command` 调用外部命令（后台化是重新执行自身，`daemonize`）。桌面端有 `exec.Command` 调主程序与系统打开命令，不在本次范围。新增命令不得引入 Shell 拼接 |
| 路径穿越 | 站点名有白名单正则；`modules/static` 只拦 `../`。新命令的 `--root`、`--candidate` 需要「规范化 + 目录边界」校验，这部分现在没有统一实现，需新增 |
| 日志注入 | 日志是 JSON，字段值是数据；CLI 输出也走 JSON 编码，天然不会被解释成指令。仍需按规范 8.3 加测试 |
| 敏感信息 | 见 G11；另外证书落盘权限 `0o600` 已经对 |
| 并发写配置 | 现在没有锁。`site init`、桌面端、手工编辑可以同时写。事务引擎需要一把跨进程文件锁，并在文档里说明「手工编辑不受保护」，通过内容哈希识别漂移 |
| SSH 断线 | 断线会给 CLI 进程发 `SIGHUP`，默认会杀掉进程。执行中的修改命令必须在关键区忽略 `SIGHUP`/`SIGPIPE`（Unix），并靠 journal 在下次调用时对账 |

## 8. 基线测试结果

2026-11-08 在 Windows 上执行：

- 本机默认 `GOROOT` 是 go1.26.3，而 `go.mod` 要求 1.27.1 并自动切换工具链，结果每个包都报 `compile: version "go1.26.3" does not match go tool version "go1.27.1"`。这是环境问题（架构地图第 8 节已有同样记录），不是代码问题。
- 把 `GOROOT` 指到模块缓存里的 `golang.org/toolchain@v0.0.1-go1.27.1.windows-amd64`、`GOTOOLCHAIN=local` 后：`go build ./...` 成功；`go test -p 1 -count=1 ./cmd/... ./config/... ./modules/site/... ./modules/certmanager/... ./runtime/server/... ./tests/arch/... ./tools/...` 全部通过。
- 未运行：全量 `go test ./...`、`-race`、`-tags=rpm` 构建、`tests/packaging`、容器安装验证（`tests/packaging/distro/run.sh`）、桌面端测试。这些在改造计划的各步验收里逐步纳入。

结论：基线是绿的，可以在其上做增量改造，不需要先清理存量。

## 9. 需要你拍板的决策点

计划里已经给出推荐值，没有异议就按推荐执行。

| # | 决策 | 选项 | 推荐 |
| --- | --- | --- | --- |
| D1 | 管理状态（配置版本、operation）放哪 | A. 新增独立状态目录（归档 `./state`，包布局 `/var/lib/fast-https`）；B. 放在配置目录下的隐藏子目录 | A：职责清楚，代价是要改两套布局常量与安装/卸载脚本及其测试 |
| D2 | 新命令是否也走全局退出码 2（用法错误） | 只对新命令组；或对整棵命令树 | 整棵树：非 0 不变，只是 1 变 2，风险低 |
| D3 | 确认标志 | 沿用 `--yes`；新增 `--confirm`；二者同义 | 二者同义 |
| D4 | request ID 保留期 | 7 天 / 30 天 / 永久 | 7 天，可配置 |
| D5 | `cert renew` | 第一阶段返回 `NOT_SUPPORTED`；或本次就加 Server 控制通道 | 先 `NOT_SUPPORTED` |
| D6 | `ConsoleLog` 的 Warn/Error 是否同时写 `error.log` | 做；不做 | 做，但放在可写命令之后单独一步，并评估日志量 |
| D7 | 候选配置是否支持整棵目录树 | 第一版只支持单个主文件；或一次做整棵树 | 先单文件（只读校验阶段），写入阶段再做整棵树 |
| D8 | `service start/stop/restart` | 第一阶段不做；或封装现有 start/stop 并在 systemd 下拒绝 | 不做 |

## 10. 结论

- **是否具备条件：具备，需分阶段。** 先做不碰 Server 核心的部分（输出框架、只读命令、日志、诊断、配置版本与事务），这一段几乎全部是新增代码，对数据面零影响。
- **哪些不能现在做：** `runtime status/stats`、`monitor *`、reload 的强验证依赖 Monitor 计划；`cert renew` 需要 Server 控制通道。
- **最该先动手的三件小事：** `infra/logging` 加输出重定向；`main` 加退出码映射；给桌面端依赖的旧命令加兼容测试。三件加起来改动很小，却是后面所有步骤的前提。

执行顺序、每一步的改动范围和验收测试见 [CLI 改造开发计划](fast-https-CLI改造开发计划.md)。
