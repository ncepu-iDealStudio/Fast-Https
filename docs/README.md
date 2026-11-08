# 文档索引

```
docs/
  README.md          ← 本文件
  feature/           产品功能（按编号排列，只放已实现的能力）
  develop/           开发文档（环境/测试/规范/配置/调试/发布 + plans 未实施方案 + archive 已归档方案）
  help/              帮助文档（构建与安装/目录布局/常见问题/开发问题日志）
  test/              测试专项（覆盖率评估/边界用例集/压测计划/发行版实机验证）
  usage.md               面向部署运维的使用文档（源），生成 usage.html 随发行包分发
  usage.html             由 usage.md 生成，不改源文件就不要手改它
  architecture.html      项目架构图（自包含 HTML，可切换深色）
  images/                架构图 PNG（亮色 / 深色）
  architecture-map.md    当前 package 盘点与依赖方向
  about.md               启动与运行逻辑（含流程图）
  nginx-core-gap.md         与 Nginx 核心功能对比
  product-design-latest.md  当前产品设计（只记录已经实现的行为）
```

`usage.html` 由 `go run ./tools/docgen` 生成，版本号取自 `config.CURRENT_VERSION`。改了 `usage.md` 之后必须重新生成，`tools/docgen` 的测试会在不一致时报错。

## 按角色入口

| 你想做什么 | 去哪 |
| --- | --- |
| 了解某个功能怎么用、怎么实现 | [feature/README.md](feature/README.md) |
| 搭环境、跑测试、发版 | [develop/README.md](develop/README.md) |
| 打包、安装、卸载、服务起不来 | [help/README.md](help/README.md) |
| 看覆盖率、补边界用例、跑压测 | [test/README.md](test/README.md) |
| 装包后行为是否符合预期、要不要重跑实机验证 | [test/3-distro-verification.md](test/3-distro-verification.md) |
| 配证书、让 ACME 签发成功 | [help/tls-certificates.md](help/tls-certificates.md) |
| 新增配置项 | [develop/3-config-development.md](develop/3-config-development.md) |
| 排查线上问题 | [develop/4-debugging.md](develop/4-debugging.md) |
| 看当前产品做成了什么、明确没做什么 | [product-design-latest.md](product-design-latest.md) |
| 看整体架构图（分层与请求链） | [architecture.html](architecture.html)、[images/architecture.png](images/architecture.png) |
| 看代码分层与依赖约束 | [develop/2-code-standards.md](develop/2-code-standards.md)、[architecture-map.md](architecture-map.md) |
| 了解历史决策与演进 | [develop/README.md](develop/README.md)（`plans/` 未实施，`archive/` 已归档） |
| CLI / Monitor 落地后还剩什么 | [develop/plans/cli-monitor-followup.md](develop/plans/cli-monitor-followup.md) |

## 维护约定

- 功能变化 → `feature/` 对应篇章；迭代状态 → `develop/archive/iteration-plan.md`；结构变化 → `architecture-map.md`。
- 方案落地后从 `develop/plans/` 移到 `develop/archive/`，正文不改写。
- 未实现的方案写在 `develop/plans/`；落地后写成 `feature/` 的功能文档，方案本身移入 `develop/archive/`。
