# CLI `--yes` 确认逻辑审查与收敛

## 1. 背景

上一轮为所有会改动配置的 CLI 命令加入了统一的写操作确认：

- 带 `--yes` 直接执行；
- 标准输入是终端时提示 `即将<动作>。继续？[y/N]`；
- 其他情况（脚本、CI、桌面端）返回 `USAGE_ERROR`（退出码 2），并提示 `--dry-run` 可预览。

本规格记录对现有实现的审查结论，以及需要收敛的缺口。范围仅限确认链路，不改变事务、校验与 dry-run 的既有语义。

## 2. 当前实现（已核对）

### 2.1 核心函数

- `cmd/internal/cli/confirm.go` `ConfirmWrite(cmd, action)`
  - `Confirmed(cmd)` 为真 → 直接通过；
  - `term.IsTerminal(os.Stdin.Fd())` 为假 → `UsageError("<CommandPath> requires --yes; use --dry-run to preview")`；
  - 终端：向 `cmd.ErrOrStderr()` 打印提示，从 `cmd.InOrStdin()` 读一行；`y` / `yes` 通过，其余（含 EOF 无输入）返回 `errors.New("已取消")`。
- `cmd/internal/cli/txn.go` `Confirmed(cmd)`：只读取 `--yes`。

### 2.2 已覆盖的写命令

| 命令 | 确认入口 | dry-run 跳过确认 | 确认时机 |
| --- | --- | --- | --- |
| `site add`（`--domain` 模式） | `siteWriteYes` | 是 | 输入校验之后 |
| `site update` | `siteWriteYes` | 是 | 参数校验之后 |
| `site delete` | `siteWriteYes` | 是 | 执行之前（未先查存在性） |
| `site location add/update/delete/set` | `locationWriteYes` | 是 | add/update 校验后；delete 执行前 |
| `cache add/update/delete` | `cacheWriteYes` | 是 | 校验/查重/引用检查之后 |
| `config proxy` | `cli.ConfirmWrite` | 是（提前 return） | 预校验之后、事务之前 |
| `config tls` | `cli.ConfirmWrite` | 是（提前 return） | 预校验之后、事务之前 |
| `config apply` | `cli.ConfirmWrite` | 是 | 事务之前 |
| `config rollback` | `cli.ConfirmWrite` | 是 | 目标版本解析之后、事务之前 |
| `config versions delete` | `cli.ConfirmWrite` | **无 dry-run** | 存在性与 active 校验之前 |

`site enable/disable` 有意不确认（改名可逆），桌面端所有写调用均已显式追加 `--yes`。

### 2.3 测试覆盖

- Go 单测：`cmd/cli_write_test.go`、`cmd/internal/{sitecmd,configcmd,cachecmd}/*_test.go`。
- 实机脚本：`tests/scripts/cli-confirm.sh`（非终端缺 `--yes`、管道输入、PTY 下 `n`/`y`、配置未被改动）。

## 3. 发现的问题与决定

### P0-1 文档承诺 `--confirm` 与 `--yes` 同义，代码未实现

- 证据：`docs/feature/18-cli-service.md:57`、`docs/feature/fast-https-CLI改造开发计划.md:165`。
- 现状：全仓库没有任何 `--confirm` 注册或解析；`docs/cli/command-spec.md`、`docs/usage.md` 与帮助文案均只写 `--yes`。
- **决定**：不新增 `--confirm` 别名；以 `--yes` 为唯一确认标志，修订上述两处过时表述。理由：命令规范与用户文档已是单一事实来源，新增别名只会扩大需要同步的面。

### P0-2 `config versions delete` 缺少 `--dry-run`

- 证据：`cmd/internal/configcmd/cmd.go:176-215` 只注册了 `--config` 与 `--yes`。
- 影响：与规范「所有写命令必须支持 `--dry-run`」冲突；非终端缺 `--yes` 时统一提示 `use --dry-run to preview`，而该命令根本没有该选项，属于误导。
- **决定**：为 `config versions delete` 增加 `--dry-run`：
  - 先做与删除相同的前置校验（版本名合法、目录存在、不是 active）；
  - 只输出预览（文本 / JSON），不删目录、不写 operation、不要求确认；
  - 非 dry-run 仍在**校验通过之后**再确认。

### P1-1 取消操作被当成 `INTERNAL_ERROR`

- 证据：`confirm.go:30,36` 返回普通 `error`；`cmd/internal/cli/output.go` 对非 `CLIError` 统一填 `INTERNAL_ERROR`。
- 影响：终端里按 `n` 后，文本模式退出码为 1（可接受），但 `--json` 模式会输出 `{"code":"INTERNAL_ERROR","message":"已取消"}`，脚本无法区分「用户取消」与真实内部错误。
- **决定**：新增错误码 `CANCELLED`，映射退出码 1；`ConfirmWrite` 取消时返回
  `&CLIError{Code: CodeCancelled, Command: cmd.CommandPath(), Message: T("已取消","cancelled")}`。
  同步 `docs/cli/command-spec.md` 的退出码/错误码表与 `docs/cli/cli-json-shapes.md`。

### P1-2 `acme setup --non-interactive` 绕过确认

- 证据：`cmd/internal/acmecmd/acme.go:164-173` 非交互分支只校验邮箱，`setupACME` 随后直接写配置；交互分支有自己的「确认写入配置文件？」。
- 影响：`fast-https acme setup --email ... --non-interactive` 在脚本/CI 中会静默改写 `acme` 段，与「所有改配置命令需 `--yes`」不一致。
- **决定**：`acme setup` 增加 `--yes`；非交互且非 `--dry-run` 时，在 `buildConfig()` 校验通过之后、事务之前调用 `cli.ConfirmWrite`。交互向导保持现有末屏确认，不重复询问。
- 实现方式：`acmeOptions` 增加 `Confirm func(action string) error`；`runAcmeSetup` 注入 `func(a string) error { return cli.ConfirmWrite(cmd, a) }`，`setupACME` 在非交互路径调用。单测不设置该字段时不触发（保持既有单测语义），CLI 路径必然设置。

### P2-1 终端判定与读取源不一致

- 证据：`confirm.go:24` 用 `os.Stdin` 判定终端，`confirm.go:28` 却从 `cmd.InOrStdin()` 读取。
- 影响：测试或嵌入调用通过 `cmd.SetIn` 注入输入时，判定与实际输入源可能不是同一个；无法交互时还可能等待错误的输入。
- **决定**：改为对 `cmd.InOrStdin()` 做类型断言，只有它是 `*os.File` 且 `term.IsTerminal(f.Fd())` 才走交互分支；否则按非终端处理。错误文案统一走 `cli.T()`。

### P2-2 死代码与无意义的 flag

- `cmd/internal/sitecmd/site.go:34,128-130` 的 `Options.Yes` 只赋值、从未读取。
- `cmd/internal/sitecmd/site_write.go:144-149` 给 `enable/disable` 也注册了 `--yes`，但两者从不确认；仓库内没有调用方给这两个子命令传 `--yes`。
- **决定**：删除 `Options.Yes` 字段及赋值；`--yes` 只在 `add/update/delete` 上注册，`enable/disable` 只保留 `--dry-run`。

### P2-3 确认时机不一致（保留现状）

`site delete`、`site location delete` 在存在性校验之前确认；`cache`、`config proxy/tls`、`site update` 在校验之后确认。存在性判断位于 `siteops` 事务内部，提前到命令层会重复一套查找逻辑。

- **决定**：本次只调整 `config versions delete`（随 P0-2 一起，校验后确认）；其余保持现状，不扩大改动面。

### P2-4 `cache delete --purge` 的提示未提及目录删除

- 现状：动作描述只有「删除缓存区 X」，但 `--purge` 还会 `RemoveAll` 缓存目录。
- **决定**：`--purge` 且非 dry-run 时，动作描述追加目录路径，避免用户只看到「删配置」就确认。

## 4. 明确不做

- 不新增 `--confirm` 别名。
- 不改变 `--dry-run` 的「不写盘、不 reload、不建 operation」语义。
- 不为 `site enable/disable` 增加确认。
- 不改桌面端：`desktop/internal/{siteops,cacheops,configops}` 已全部追加 `--yes`。
- 不重构 `RunTxn` / `RunSectionTxnCmd`。

## 5. 实施清单

1. `cmd/internal/cli/errors.go`：新增 `CodeCancelled = "CANCELLED"`，`exitCodes` 映射为 1。
2. `cmd/internal/cli/confirm.go`：
   - 终端判定改用 `cmd.InOrStdin()`（`*os.File` + `term.IsTerminal`）；
   - 非终端错误用 `T()` 包一层；
   - 取消返回 `*CLIError`（`CodeCancelled`，带 `Command`）。
3. `management/cfgver/cfgver.go`：抽出 `ValidateDelete(dir state.Dir, version, liveMain string) error`（版本名合法性、目录存在、非 active），`Delete` 复用，避免 dry-run 与真实删除校验漂移。
4. `cmd/internal/configcmd/cmd.go` `newConfigVersionsDeleteCmd`：
   - 注册 `--dry-run`；
   - `RunE` 顺序：取参数 → `cfgver.ValidateDelete` → 错误映射（`ErrActiveVersion`=10、`ErrVersionNotFound`=9）→ dry-run 输出预览并 return → `ConfirmWrite` → `cfgver.Delete` → 输出。
5. `cmd/internal/acmecmd/acme.go`：
   - `registerAcmeSetupFlags` 增加 `--yes`；
   - `acmeOptions` 增加 `Confirm func(string) error`；
   - `setupACME` 在 `buildConfig()` 之后、`DryRun` 分支之前，若 `NonInteractive && !DryRun && Confirm != nil` 则调用；
   - `runAcmeSetup` 注入基于 `cli.ConfirmWrite` 的回调。
6. `cmd/internal/sitecmd/site.go`：删除 `Yes` 字段与赋值。
7. `cmd/internal/sitecmd/site_write.go`：`--yes` 只注册到 `add/update/delete`。
8. `cmd/internal/cachecmd/cache.go`：`cache delete --purge` 时动作描述带上目录。
9. 文档：
   - `docs/feature/18-cli-service.md`、`docs/feature/fast-https-CLI改造开发计划.md` 删除 `--confirm` 同义表述；
   - `docs/cli/command-spec.md`、`docs/cli/cli-json-shapes.md`、`docs/usage.md` 增加 `CANCELLED`、`config versions delete --dry-run`、`acme setup --yes`；
   - `go run ./tools/docgen` 重新生成 `docs/usage.html`。
10. 测试：
    - `cmd/cli_write_test.go`：`config versions delete --dry-run` 不删除、active 版本仍报冲突。
    - `cmd/internal/configcmd/diff_versions_test.go`：补齐 dry-run 文本 / JSON 断言。
    - `cmd/internal/acmecmd/acme_test.go`：非交互 + `Confirm` 返回取消时不写配置；确认回调被调用一次。
    - `cmd/internal/cli/` 新增 `confirm_test.go`：非终端缺 `--yes` 为 `USAGE_ERROR`；带 `--yes` 放行；EOF 无输入为 `CANCELLED`。
    - `tests/scripts/cli-confirm.sh`：增加 `config versions delete` 的「缺 `--yes` 退出 2」「`--dry-run` 不删且不询问」「终端 `n` 取消 / `y` 删除」。
    - 如 `cmd/i18n_test.go` 或命令规范测试枚举 flag，同步期望。

## 6. 验证步骤

```bash
go build ./...
go test ./cmd/... ./tools/... ./tests/...
go run ./tools/docgen
cd desktop && go test ./...
```

实机（Linux/WSL）：

```bash
bash tests/scripts/cli-confirm.sh
```

实机（Windows 原生二进制）：

```powershell
go build -o fast-https.exe .
.\fast-https.exe config versions delete v2026.11.18.1 --config .\config\fast-https.json   # 退出码 2
.\fast-https.exe config versions delete v2026.11.18.1 --dry-run --config .\config\fast-https.json
```

## 7. 验收标准

- 非终端缺 `--yes`：退出码 2，错误包含 `fast-https <命令路径> requires --yes`。
- 终端 `n`：不写盘，`--json` 输出 `code=CANCELLED`，退出码 1；`y`：正常执行。
- 所有 dry-run：不提示、不写盘、不建 operation。
- `config versions delete --dry-run`：active / 不存在版本给出与真实删除一致的错误码。
- `acme setup --non-interactive`：缺 `--yes` 时非终端拒绝，终端询问 `y/N`。
- 上述构建与测试命令全部通过。
