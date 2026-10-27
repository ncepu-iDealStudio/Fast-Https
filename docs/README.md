# 文档索引

```
docs/
  README.md          ← 本文件
  feature/           产品功能（23 篇，按编号排列，含计划占位）
  develop/           开发文档（环境/测试/规范/配置/调试/发布 + plans 过程规划）
  test/              测试专项（覆盖率评估/边界用例集/压测计划）
  usage.md               面向部署运维的使用文档（源），生成 usage.html 随发行包分发
  usage.html             由 usage.md 生成，不改源文件就不要手改它
  architecture-map.md    当前 package 盘点与依赖方向
  about.md               启动与运行逻辑（含流程图）
  nginx-core-gap.md      与 Nginx 核心功能对比
  product-design-2.0.md  2.0 产品设计稿
```

`usage.html` 由 `go run ./tools/docgen` 生成，版本号取自 `config.CURRENT_VERSION`。改了 `usage.md` 之后必须重新生成，`tools/docgen` 的测试会在不一致时报错。

## 按角色入口

| 你想做什么 | 去哪 |
| --- | --- |
| 了解某个功能怎么用、怎么实现 | [feature/README.md](feature/README.md) |
| 搭环境、跑测试、发版 | [develop/README.md](develop/README.md) |
| 看覆盖率、补边界用例、跑压测 | [test/README.md](test/README.md) |
| 新增配置项 | [develop/3-config-development.md](develop/3-config-development.md) |
| 排查线上问题 | [develop/4-debugging.md](develop/4-debugging.md) |
| 看代码分层与依赖约束 | [develop/2-code-standards.md](develop/2-code-standards.md)、[architecture-map.md](architecture-map.md) |
| 了解历史决策与演进 | [develop/plans/](develop/README.md) |

## 维护约定

- 功能变化 → `feature/` 对应篇章；迭代状态 → `develop/plans/iteration-plan.md`；结构变化 → `architecture-map.md`。
- 计划中的功能在 `feature/` 里有占位文档（编号 20 起），落地后转正。
