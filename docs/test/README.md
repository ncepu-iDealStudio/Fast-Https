# 测试文档索引

| 编号 | 文档 | 内容 |
| --- | --- | --- |
| 0 | [0-coverage-assessment.md](0-coverage-assessment.md) | 覆盖率评估（2026-10-25 实测）、薄弱模块与补测方向 |
| 1 | [1-boundary-cases.md](1-boundary-cases.md) | 边界测试用例集：协议边界、注入面、数值边界、并发边界 |
| 2 | [2-stress-test-plan.md](2-stress-test-plan.md) | 压力测试计划：4C4G3M 环境下的反代与静态资源负载方案 |
| 3 | [3-distro-verification.md](3-distro-verification.md) | 发行版安装包实机验证记录（2026-11-05 实测）：deb/rpm/apk/升级迁移的结论与已知环境差异 |

日常测试命令与体系说明见 [develop/1-testing.md](../develop/1-testing.md)（本目录聚焦评估报告与专项计划）。

## 历史记录

- 2026-10-25 首轮覆盖评估 + 边界测试：发现并修复裸 CR/LF 头注入三面（`ec425cd`）；清理死代码（DecompressBytesGzip、PathQuery.Del/Set）。
- 2026-11-05 发行版实机验证：ubuntu:24.04 / almalinux:9 / alpine:3.21 容器内装包各跑一遍，四个场景 ALL-PASS；同时补上 logrotate 配置与 `tests/packaging/distro/` 脚本，详见 [3-distro-verification.md](3-distro-verification.md)。
