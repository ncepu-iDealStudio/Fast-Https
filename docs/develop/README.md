# 开发文档索引

面向本项目的开发者。产品功能说明在 [`docs/feature/`](../feature/README.md)。

## 开发指南

| 编号 | 文档 | 内容 |
| --- | --- | --- |
| 0 | [0-environment-and-build.md](0-environment-and-build.md) | 环境搭建、构建、运行、Docker |
| 1 | [1-testing.md](1-testing.md) | 测试体系：单元/E2E/-race/基准/架构守卫 |
| 2 | [2-code-standards.md](2-code-standards.md) | 分层约束、并发约定、提交与文档同步规范 |
| 3 | [3-config-development.md](3-config-development.md) | 如何新增一个配置项（schema 到验收） |
| 4 | [4-debugging.md](4-debugging.md) | 日志五路、pprof、常见问题排查 |
| 5 | [5-release.md](5-release.md) | 发布流程：GoReleaser/RPM/Docker 与发布前清单 |

## 过程与规划文档（plans/）

历史演进与专项方案的原始记录，链接已随目录整理更新。

| 文档 | 内容 |
| --- | --- |
| [architecture-convergence-plan.md](plans/architecture-convergence-plan.md) | 架构收敛计划（阶段 0-5，含 4a/4b 状态） |
| [iteration-plan.md](plans/iteration-plan.md) | 迭代计划与回归项（R1-R9） |
| [guardianwaf-adoption.md](plans/guardianwaf-adoption.md) | GuardianWAF 接入方案（已实施） |
| [agent-refactor-plan.md](plans/agent-refactor-plan.md) | Agent 化改造方案（2.0 实施） |
| [performance-slimming-plan.md](plans/performance-slimming-plan.md) | 精简与性能优化计划 |
| [fhs-layout-plan.md](plans/fhs-layout-plan.md) | 运行期目录改成 FHS 分离布局（待实施，仅规划） |

## 参考文档（docs 根目录）

- [architecture-map.md](../architecture-map.md) — 当前 package 盘点与依赖方向
- [about.md](../about.md) — 启动与运行逻辑（含流程图）
- [nginx-core-gap.md](../nginx-core-gap.md) — 与 Nginx 核心功能对比
- [product-design-2.0.md](../product-design-2.0.md) — 2.0 产品设计稿

## 约定速览

- 分支：功能开发在 `develop`，发布合入 `master`。
- 提交：conventional commits，中文主题（`feat(proxy): ...`），一类改动一个 commit。
- 每次提交前：`go build ./... && go vet ./... && go test -race ./...` 全绿。
- 行为变化同步 `plans/iteration-plan.md`；结构变化同步 `../architecture-map.md`；功能变化同步 `../feature/` 对应篇章。
