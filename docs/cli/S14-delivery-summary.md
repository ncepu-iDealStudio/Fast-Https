# S14 交付总结（CLI 第一阶段收口）

日期：2026-11-08。**范畴**以 [开发计划 S14（§860–877）](../feature/fast-https-CLI改造开发计划.md) 为准：文档收口、全量回归、DSH 手工、安全复核、实现总结；**不展开** S13 `cert renew`、Monitor 计划第 6–7 步（进程侧）。

## 与计划「执行」五步对照

| 步骤 | 计划要求 | 状态 | 说明 |
|:---:|---|---|---|
| **1 文档** | `usage.md` §10 + docgen；`18-cli-service`；`architecture-map`、`AGENT.md`；`docs/cli/*`；manpage | **基本完成** | §10.3 / §13 已在 `usage.md`；`AGENT.md` 已有 `management/` 行；新增 `command-spec.md`、`examples.md`；`architecture-map` 去重一行；`docgen`/`manpage` 已跑 |
| **2 全量回归** | `go test -p 1 ./...`、`-race ./...`、双 tag `vet`、packaging、distro 脚本、goreleaser、desktop | **部分** | vet、核心包 `-p 1`、`packaging`、`svcctl -race` 通过；`./...` 因 `infra/banner.TestPrintLogoColored` 失败；全量 `-race`、distro Docker、goreleaser（未安装）、desktop（缺 `frontend/dist`）**未跑** |
| **3 DSH 联调** | SSH 只读 JSON 五项 | **未做** | 需目标环境与只读账户，本机未执行 |
| **4 安全复核** | `TestNoExecInProduction`、口令夹具、日志注入 | **通过（自动化）** | `TestNoExecInProduction`；`TestConfigShowAndSiteViewsRedact`、`TestDiagnoseNoSecrets`；`TestMonitorEventsQuery` |
| **5 实现总结** | 规范 §10 十项 + 未完成项 | **见下文** | 本文件 + [command-spec](./command-spec.md) |

**验收（§8.5 十条）**：见文末对照表；未手工/未跑环境处如实标注。

---

## 规范 §10 交付十项

## 1. 现状盘点

- 语言：Go；CLI：cobra（`cmd/`），入口 `cmd.Run()`。
- 配置：`config/fast-https.json` + `http.include` 站点；reload 信号 + 运行期快照。
- 管理核心：`management/state`、`txn`、`svcctl`、`view`、`rtclient`、`monview` 等。
- 证书：`modules/certmanager` + `cert list/check`；续期写通道未做。
- Runtime：`127.0.0.1:9088/runtime`，`schema_version: "1"`。
- Monitor：独立进程在 `monitor/`，CLI 只读其状态文件。打包与 systemd 见 [feature/21-monitor.md](../feature/21-monitor.md)。本摘要写于收口当时，其中「第 6–7 步未做」已过时，以功能文档与 [遗留改进计划](../develop/plans/cli-monitor-followup.md) 为准。

## 2. 实现文件清单（S14 本轮）

| 路径 | 用途 |
|---|---|
| `docs/cli/command-spec.md` | 命令契约、P0/P1 状态、退出码、测试映射 |
| `docs/cli/examples.md` | 本机实测 JSON 示例 |
| `docs/architecture-map.md` | 去掉重复的 `management/state` 行 |
| `docs/feature/18-cli-service.md` | 测试列表补 `cli_s12`、`monview` |
| `docs/usage.md` / `usage.html` | §10.3、§13 限制（此前已具备）；docgen 已重跑 |
| man 页 | `go run ./tools/manpage` 已重跑 |

S11/S12 代码见历史提交（`management/rtclient`、`cmd/runtime.go`、`management/monview`、`cmd/monitor.go` 等）。

## 3. 命令完成表

| 优先级 | 未实现 |
|---|---|
| P0 | 无 |
| P1 | `cert renew`；`logs access`/`logs query` 若规范列出的子能力有缺口以 usage 为准 |
| 其它 | 无（Monitor 进程与 CLI 只读查询均已落地；未做的是验证与 `cert renew`） |

## 4–5. 接口与安全

见 [command-spec.md](./command-spec.md)、[usage.md](../usage.md) §10、`management/redact`。无 `--exec`；`tests/arch.TestNoExecInProduction` 通过。

## 6. 测试结果（本次实际运行）

| 命令 | 结果 |
|---|---|
| `go vet ./...`、`go vet -tags=rpm ./...` | 通过 |
| `go test -p 1 ./cmd/... ./management/... ./tests/arch/... ./tools/... ./tests/packaging/...` | 通过 |
| `go test -race ./management/svcctl/...` | 通过 |
| `go test -p 1 ./...` | **失败**：`infra/banner`（`TestPrintLogoColored`，Windows 环境已知问题） |
| `go test -race ./...` | 未运行（全量时长与环境） |
| `go test ./tests/packaging` | 通过（含于上文） |
| `sh tests/packaging/distro/run.sh all` | 未运行（需 Docker/Linux） |
| `goreleaser check` | 未运行（本机未安装 `goreleaser`） |
| `desktop/` 构建与测试 | 未运行 |
| DSH SSH 只读 JSON 联调 | 未运行（手工） |
| `FASTHTTPS_E2E_RUNTIME=1` runtime E2E | 未运行（需显式环境变量） |

## 7. 未完成项

- S13 `cert renew`（明确不做；连 `NOT_SUPPORTED` 命令壳也未加）。
- 全量 `-race ./...`、distro 容器、desktop、goreleaser、DSH 手工联调。
- Monitor 第 6–7 步代码已在 `monitor/`；本条在收口当时误记为未做。后续验证见遗留改进计划。

## 8. 运行示例

见 [examples.md](./examples.md)。

## 9. 兼容性

- 旧 `start`/`stop`/`reload`/`status` 文案与退出行为由 `compat_test.go` 锁定。
- 新增状态目录：归档 `./state`，RPM 布局 `/var/lib/fast-https`；两套 `base_dir` 已存在。
- 桌面端依赖的旧命令未改输出格式。

## 10. 后续建议（仅第一阶段相关）

1. 在 Linux CI 跑 `go test -p 1 ./...` 与 distro 脚本，规避 Windows banner 测试噪声。
2. 再做 `cert renew` 前单独安全评审（规范要求）。后续条目见 [cli-monitor-followup.md](../develop/plans/cli-monitor-followup.md)。

---

## §8.5 十条验收对照

| # | 标准 | 证据 |
|---|---|---|
| 1 | P0 可执行且有测试 | P0 表；`cmd/cli_s2/s8/s11` 等 |
| 2 | JSON 与退出码稳定 | `cli_test.go`；examples 实测 |
| 3 | Runtime 来自真实 `/runtime` | `rtclient`、`cli_s11_test.go`；E2E 需 `FASTHTTPS_E2E_RUNTIME=1` |
| 4 | apply/rollback 经管理核心 | `management/txn`、`cli_s8_test.go` |
| 5 | operation 持久化 | `management/operation`、`operation list/get` 测试 |
| 6 | request-id 幂等 | txn/operation 故障注入测试 |
| 7 | 日志注入不执行 | `cli_s2_test.go`、`cli_s12_test.go`、`logquery` |
| 8 | 无任意 Shell | `TestNoExecInProduction` |
| 9 | DSH SSH JSON | **未手工验证**（设计满足只读 `--json`） |
| 10 | 文档说明限制 | `usage` §13、`18-cli-service`、本目录 `command-spec` |
