# 测试文档索引

本目录存放「测过什么、结论是什么、怎么复跑」的记录，按用途分三类：
**报告**（结论）、**用例与方案**（怎么测）、**实机验证**（真跑过的结果）。

日常测试命令与测试分层说明见 [develop/1-testing.md](../develop/1-testing.md)。

## 目录结构

```text
docs/test/
├── reports/          评估与核验结论：覆盖率、缺陷反馈核验
├── cases/            怎么测：边界用例集、压力测试方案
└── verification/     实测记录：发行版装包、桌面端手工验证
```

## 文档索引

| 类别 | 文档 | 内容 | 最近结果 |
| --- | --- | --- | --- |
| 报告 | [reports/coverage-assessment.md](reports/coverage-assessment.md) | 覆盖率评估与薄弱模块清单 | 2026-10-25 合并覆盖率 68.5%，35 个包通过 `-race` |
| 报告 | [reports/edge-case-coverage.md](reports/edge-case-coverage.md) | **缺陷反馈核验（第一轮）**：按风险面自查 B-01 ~ B-17，逐条给证据与复跑命令 | 18 条核验：3 条已修复（B-05 / B-08 / B-17），1 条待修（B-12），其余有回归用例锁定 |
| 报告 | [reports/upstream-feedback-coverage.md](reports/upstream-feedback-coverage.md) | **缺陷反馈核验（第二轮）**：对照主流 HTTP 服务器公开 issue/PR，过滤 C 层与 QUIC 专用条目 | 5 条核验、2 处修复 |
| 用例 | [cases/boundary-cases.md](cases/boundary-cases.md) | 边界测试用例集：协议边界、注入面、数值边界、并发边界、公开语料 | 用例已全部落库，见文内对应测试文件 |
| 方案 | [cases/stress-test-plan.md](cases/stress-test-plan.md) | 压力测试方案：4C4G3M 环境下的反代与静态资源负载 | 仅方案，压测脚本未实现（见 [v1.5.0-plan](../develop/plans/v1.5.0-plan.md)） |
| 实测 | [verification/distro-packages.md](verification/distro-packages.md) | 发行版安装包实机验证：deb / rpm / apk / 升级迁移 | 2026-11-05 四个场景 ALL-PASS |
| 实测 | [verification/desktop-tls-config.md](verification/desktop-tls-config.md) | 桌面端 TLS 默认证书配置：GUI 写入、清除、成对校验 | 2026-11-17 三项全部符合预期 |

## 测试类型与入口

| 层级 | 命令 | 守什么 | 结论记在哪 |
| --- | --- | --- | --- |
| 单元 / 包内 | `go test ./...`（Windows 加 `-p 1`） | 白盒逻辑、解析边界 | 各包 `*_test.go`；覆盖率见 [coverage-assessment](reports/coverage-assessment.md) |
| 竞态 | `go test -race ./...` | 共享状态、热重载并发 | 同上 |
| 架构守卫 | `go test ./tests/arch` | 分层依赖方向 | [develop/1-testing.md](../develop/1-testing.md) |
| 打包守卫 | `go test ./tests/packaging` | LF 行尾、安装清单、logrotate 覆盖、脚本结构 | 同上 |
| 实机装包 | `sh tests/packaging/distro/run.sh all`（需 docker） | 真装、启动、轮转、卸载 | [verification/distro-packages.md](verification/distro-packages.md) |
| 桌面端 | `cd desktop && go test ./internal/...` | 设置页字段、站点参数翻译、CLI 转交 | [verification/desktop-tls-config.md](verification/desktop-tls-config.md) |
| 前端 | `cd desktop/frontend && npm run typecheck && npm run i18n:check` | 类型、双语字典键对齐 | 同上 |
| 手工验证 | 按 verification/ 下的步骤操作 GUI 或容器 | 图形界面、安装包这类自动化覆盖不到的部分 | `verification/` |

## 最近一次全量检查（2026-11-17）

命令与结果（`-race` 未在本次运行，提交前仍按 [develop/1-testing.md](../develop/1-testing.md) 执行）：

| 命令 | 结果 |
| --- | --- |
| `go test ./runtime/listener/ ./config/... ./management/... ./cmd/...` | 全部 `ok`（含 R-01 ~ R-04 边缘加固回归） |
| `cd desktop && go test ./internal/...` | `cli` / `configops` / `siteops` / `view` 全部 `ok` |
| `go test ./tests/packaging` | `ok` |
| `cd desktop/frontend && npm run typecheck` | 通过 |
| `cd desktop/frontend && npm run i18n:check` | 两份字典各 619 键，代码引用 286 键，全部覆盖 |
| 桌面端 TLS 配置手工验证 | 写入 / 清除 / 只填一项报错 三项符合预期，见 [verification/desktop-tls-config.md](verification/desktop-tls-config.md) |

## 历史记录

- 2026-10-25 首轮覆盖评估 + 边界测试：发现并修复裸 CR/LF 头注入三面（`ec425cd`）；清理死代码（DecompressBytesGzip、PathQuery.Del/Set）。覆盖率 50.8% → 68.5%。
- 2026-11-05 发行版实机验证：ubuntu:24.04 / almalinux:9 / alpine:3.21 容器内装包各跑一遍，四个场景 ALL-PASS；同时补上 logrotate 配置与 `tests/packaging/distro/` 脚本，详见 [verification/distro-packages.md](verification/distro-packages.md)。
- 2026-11-17 边缘加固 R-01 ~ R-04 落地：符号链接策略、Windows 保留名/尾点、TLS 兜底证书、同名证书按客户端能力选择；每项都有回归用例，CLI 与桌面端入口同步，见 [edge-case-hardening-plan.md](../develop/plans/edge-case-hardening-plan.md)。
- 2026-11-17 桌面端 TLS 默认证书配置手工验证：GUI 写入成功、两项清空即删 `tls` 段、只填一项被成对校验拦下且配置未改写，详见 [verification/desktop-tls-config.md](verification/desktop-tls-config.md)。

## 旧文件名对照

早期文档按 0 ~ 5 编号，2026-11-17 按类别重组。文件名去掉了编号，正文标题与章节号（如 `1.8`、`4.2`、`5.6`）保持原样，方便与历史提交、旧链接和文内交叉引用对应。

| 旧路径 | 新路径 |
| --- | --- |
| `docs/test/0-coverage-assessment.md` | `docs/test/reports/coverage-assessment.md` |
| `docs/test/1-boundary-cases.md` | `docs/test/cases/boundary-cases.md` |
| `docs/test/2-stress-test-plan.md` | `docs/test/cases/stress-test-plan.md` |
| `docs/test/3-distro-verification.md` | `docs/test/verification/distro-packages.md` |
| `docs/test/4-edge-case-coverage.md` | `docs/test/reports/edge-case-coverage.md` |
| `docs/test/5-upstream-feedback-coverage.md` | `docs/test/reports/upstream-feedback-coverage.md` |

## 约定

- 本目录只放「结论」和「怎么复跑」，不复制代码；每条结论必须给出可执行的复跑命令或测试名。
- 新增验证记录：放到对应的 `verification/` 文件，或按主题新建一篇并在上面的索引表登记。
- 一次验证的结果要写清**环境、时间、产物版本/校验和、逐项结论**，方便判断是否需要重跑。
