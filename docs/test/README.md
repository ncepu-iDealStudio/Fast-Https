# 测试文档索引

| 编号 | 文档 | 内容 |
| --- | --- | --- |
| 0 | [0-coverage-assessment.md](0-coverage-assessment.md) | 覆盖率评估（2026-10-25 实测）、薄弱模块与补测方向 |
| 1 | [1-boundary-cases.md](1-boundary-cases.md) | 边界测试用例集：协议边界、注入面、数值边界、并发边界 |
| 2 | [2-stress-test-plan.md](2-stress-test-plan.md) | 压力测试计划：4C4G3M 环境下的反代与静态资源负载方案 |

日常测试命令与体系说明见 [develop/1-testing.md](../develop/1-testing.md)（本目录聚焦评估报告与专项计划）。

## 历史记录

- 2026-10-25 首轮覆盖评估 + 边界测试：发现并修复裸 CR/LF 头注入三面（`ec425cd`）；清理死代码（DecompressBytesGzip、PathQuery.Del/Set）。
