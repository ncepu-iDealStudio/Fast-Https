# 开发文档索引

面向本项目的开发者。产品功能说明在 [`docs/feature/`](../feature/README.md)。

## 开发指南

| 编号 | 文档 | 内容 |
| --- | --- | --- |
| 0 | [0-environment-and-build.md](0-environment-and-build.md) | 环境搭建、构建、运行、Docker |
| 1 | [1-testing.md](1-testing.md) | 测试体系：单元/E2E/-race/基准/架构守卫 |
| 2 | [2-code-standards.md](2-code-standards.md) | 分层约束、并发约定、提交与文档同步规范 |
| 3 | [3-config-development.md](3-config-development.md) | 如何新增一个配置项（schema 到验收） |
| 4 | [4-debugging.md](4-debugging.md) | 日志五路、性能剖析、常见问题排查 |
| 5 | [5-release.md](5-release.md) | 发布流程：GoReleaser/RPM/Docker 与发布前清单 |

## 过程与规划文档

按实施状态分成两个目录：`plans/` 只放尚未实施或尚未完成的方案，`archive/` 放已经落地的方案留档。

### 未实施的计划（plans/）

| 文档 | 内容 |
| --- | --- |
| [v1.5.0-plan.md](plans/v1.5.0-plan.md) | 1.5.0 数据面功能收口：ACME、site 命令组、安全加固、测试与性能（当前迭代） |
| [agent-refactor-plan.md](plans/agent-refactor-plan.md) | Agent 化改造方案（2.0，尚未开始） |
| [performance-slimming-plan.md](plans/performance-slimming-plan.md) | 精简与性能优化计划（精简部分已落地，性能与稳定性分期未完成） |
| [fhs-layout-plan.md](plans/fhs-layout-plan.md) | 运行期目录改成 FHS 分离布局（阶段 0、1 已实施：常量拆分 + deb/rpm/apk 走 `/etc`、`/var/log`、`/run`；升级迁移待实测） |
| [upstream-failover-plan.md](plans/upstream-failover-plan.md) | 代理上游故障转移：多地址、健康检查、负载均衡（R8，未排期） |
| [config-autoreload-plan.md](plans/config-autoreload-plan.md) | 配置文件自动重载：watch + 防抖 + 复用 reload 链路（R7，暂缓） |
| [desktop-console-wails3-plan.md](plans/desktop-console-wails3-plan.md) | 桌面管理端：Wails v3 单窗口应用，无托盘，含 NSIS 与 Linux desktop deb 打包（实施中） |

### 已实施的方案（archive/）

已完成的工作移到这里留档，正文保持原样不改写，只作为历史记录。

| 文档 | 内容 |
| --- | --- |
| [architecture-convergence-plan.md](archive/architecture-convergence-plan.md) | 架构收敛计划（阶段 0-4 已完成，4b 快照化已做，阶段 5 按需） |
| [iteration-plan.md](archive/iteration-plan.md) | 迭代计划与回归项（R1-R6、R9 已落地；R7 暂缓，R8 未做） |
| [guardianwaf-adoption.md](archive/guardianwaf-adoption.md) | GuardianWAF 接入方案（2026-10-25 已实施，版本记录以 go.mod 为准） |
| [acme-certmagic-plan.md](archive/acme-certmagic-plan.md) | 用 certmagic 取代 shell/.acme.sh 的 ACME 方案（1.5.0 已实施：`modules/certmanager`） |
| [acme-hot-reload-plan.md](archive/acme-hot-reload-plan.md) | ACME 管理器热重载：reload 按差异重建证书申请/续期任务，不再需要 stop + start（2026-11-05 已实施） |

## 参考文档（docs 根目录）

- [architecture-map.md](../architecture-map.md) — 当前 package 盘点与依赖方向
- [about.md](../about.md) — 启动与运行逻辑（含流程图）
- [nginx-core-gap.md](../nginx-core-gap.md) — 与 Nginx 核心功能对比
- [product-design-2.0.md](../product-design-2.0.md) — 2.0 产品设计稿

## 约定速览

- 分支：功能开发在 `develop`，发布合入 `master`。
- 提交：conventional commits，中文主题（`feat(proxy): ...`），一类改动一个 commit。
- 每次提交前：`go build ./... && go vet ./... && go test -race ./...` 全绿。
- 行为变化同步 `archive/iteration-plan.md`；结构变化同步 `../architecture-map.md`；功能变化同步 `../feature/` 对应篇章。
