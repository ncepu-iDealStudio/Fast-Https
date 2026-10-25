# 0. 覆盖率评估

实测日期：2026-10-25（develop 分支）。数据来自：

```bash
go test ./... -coverpkg=./... -coverprofile=cover.out
go tool cover -func=cover.out
```

## 总览

| 口径 | 数值 |
| --- | --- |
| 合并覆盖率（-coverpkg=./...） | **50.8%** |
| 单包覆盖率均值 | 36.8% |
| 包数 / 通过 `-race` | 30 / 30 |

## 分包数据与解读

| 包 | 覆盖率 | 解读 |
| --- | --- | --- |
| infra/banner | 93.8% | 彩色/无色双路径全覆盖 |
| modules/safe | 92.6% | 控制面 API + overlay 语义全覆盖 |
| modules/appfirewall | 86.9% | 引擎生命周期与检测入口已测 |
| modules/cache | 73.6% | 热层/LRU/过期已测；LoadCache 磁盘恢复未测 |
| modules/static | 67.7% | 协商/Range/穿越已测；gzip 大文件路径未测 |
| modules/compress | 65.1% | 双算法往返已测 |
| config | 61.4% | reload/paths/校验已测；include 展开分支部分未测 |
| runtime/server | 50.3% | 信号/停机/accept 已测；Daemon fork 仅 Linux |
| modules/proxy_tcp、websocket | 45.7% | 搬运循环部分分支未测 |
| core | 25.4% | 契约层多为胶水；DefaultParseCommandHandler 未测 |
| runtime/request | 25.9%* | *单包口径；合并口径下解析主路径覆盖率高（injection/parse/chunked 全测） |
| runtime/response | 26.7%* | 同上，序列化主路径已测 |
| runtime/events | 18.7% | h1 事件循环主体靠 E2E 覆盖，单测缺口在 parseRequest 分片读分支 |
| runtime/listener | 19.8% | 端口增删靠 reload E2E；单测缺口在 SortBy/processHostMap |
| modules/proxy | 20.3% | 连接池单测完备；getDataFromServer 读路径靠 proxy E2E |
| cmd | 14.4% | 命令 handler 靠 unit_test/cmd 的行为测试；CLI 主循环未测 |
| init | 17.0% | InitSystem 编排链靠 E2E |

## 已识别并清零的死代码（2026-10-25）

- `compress.DecompressBytesGzip`（无生产调用方）
- `PathQuery.Del/Set`（无调用方）
- 此前轮次已删：`validWAFMode`、`Parse()`（chunked 死实现）、`setConnCfg`、`allow` 包装等

## 补测优先级（按风险 × 缺口）

1. **runtime/events.parseRequest 的分片读分支**（头跨多个 TCP 段到达）：目前只有整包到达的用例，分片路径是 keep-alive 真实网络形态。
2. **proxy getDataFromServer 的上游异常矩阵**（连接重置/半响应/超时重试）：现有 E2E 只覆盖健康上游。
3. **cache LoadCache 磁盘恢复**：启动时索引重建的损坏文件/旧格式分支。
4. **config include 的目录遍历分支**（空目录/非 json 文件/嵌套）。
5. cmd 的 CLI 参数解析（cobra 层）：低优先，行为已由 commands_test 锁定。

## 约定

- 覆盖率是手段不是 KPI：注入面、并发边界（`-race`）、协议边界的用例价值高于数字。
- 新增功能合并门禁：对应包不因新代码**下降**（见 [develop/1-testing.md](../develop/1-testing.md)）。
