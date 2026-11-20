# 任务清单：CLI `--yes` 确认逻辑收敛

对应规格：[spec.md](./spec.md)

## 状态说明

- `[ ]` 待开始
- `[~]` 进行中
- `[x]` 已完成

## 任务

- [x] T1 新增 `CANCELLED` 错误码
  - 文件：`cmd/internal/cli/errors.go`
  - 常量 `CodeCancelled`、退出码 1
  - 同步 `docs/cli/command-spec.md` 错误码/退出码表
  - 验证：`go test ./cmd/internal/cli/...`

- [x] T2 收敛 `ConfirmWrite`
  - 文件：`cmd/internal/cli/confirm.go`
  - 终端判定改为 `cmd.InOrStdin()` 的 `*os.File` + `term.IsTerminal`
  - 非终端错误走 `T()`
  - 取消返回 `*CLIError`（`CodeCancelled` + `CommandPath()`）
  - 新增 `cmd/internal/cli/confirm_test.go`
  - 验证：非终端缺 `--yes` → `USAGE_ERROR`；`--yes` 放行；取消错误码与退出码 1 已覆盖

- [x] T3 `config versions delete` 支持 `--dry-run`
  - 文件：`management/cfgver/cfgver.go`、`cmd/internal/configcmd/cmd.go`
  - 抽出 `ValidateDelete` 并让 `Delete` 复用
  - `--dry-run`：校验 → 预览 → return；不写盘、不建 operation
  - 非 dry-run：校验通过后再 `ConfirmWrite`
  - 验证：`cmd/internal/configcmd/diff_versions_test.go`（新增 dry-run 成功、active 冲突、非终端缺 `--yes` 三个用例）

- [x] T4 `acme setup` 非交互要求 `--yes`
  - 文件：`cmd/internal/acmecmd/acme.go`
  - `acmeOptions.Confirm` 回调；`setupACME` 在 `buildConfig()` 后、写盘前调用
  - `registerAcmeSetupFlags` 增加 `--yes`
  - 验证：`go test ./cmd/internal/acmecmd/...`；新增「取消不写盘」「dry-run 不确认」用例

- [x] T5 移除死代码与无意义 flag
  - 文件：`cmd/internal/sitecmd/site.go`、`cmd/internal/sitecmd/site_write.go`
  - 删除 `Options.Yes`
  - `--yes` 只注册到 `add/update/delete`
  - 验证：`go test ./cmd/internal/sitecmd/...`、`go test ./cmd/...`

- [x] T6 `cache delete --purge` 提示补充
  - 文件：`cmd/internal/cachecmd/cache.go`
  - `--purge` 且非 dry-run 时动作描述附带目录
  - 验证：`go test ./cmd/internal/cachecmd/...`

- [x] T7 文档同步
  - `docs/feature/18-cli-service.md`、`docs/feature/fast-https-CLI改造开发计划.md`：删除 `--confirm` 同义表述
  - `docs/cli/command-spec.md`、`docs/cli/cli-json-shapes.md`、`docs/usage.md`：`CANCELLED`、`config versions delete --dry-run`、`acme setup --yes`
  - `go run ./tools/docgen` 生成 `docs/usage.html`
  - 验证：`go test ./tools/docgen ./tests/arch ./tests/packaging`

- [x] T8 实机脚本补充与回归
  - 文件：`tests/scripts/cli-confirm.sh`
  - 增加 `config versions delete`：缺 `--yes`、`--dry-run`、终端 `n`/`y`
  - 增加 `acme setup --non-interactive`：缺 `--yes`、`--dry-run`、终端 `n`/`y`
  - 验证：WSL 执行 `bash tests/scripts/cli-confirm.sh`，结果 85 通过 / 0 失败
  - Windows 原生二进制冒烟：`config proxy`、`config versions delete`、`acme setup` 的确认与 dry-run 全部符合预期

- [x] T9 全量验证
  - `go build ./...`
  - `go test ./cmd/... ./tools/... ./tests/...`
  - `cd desktop && go test ./...`
  - `go run ./tools/docgen` 幂等
