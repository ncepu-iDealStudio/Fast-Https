# 0. 覆盖率评估

实测日期：2026-10-25（develop 分支）。数据来自：

```bash
go test ./... -coverpkg=./... -coverprofile=cover.out
go tool cover -func=cover.out
```

> 2026-10-25 二次实测：完成三大补测缺口 + 公开语料覆盖（见
> [1-boundary-cases.md §1.8](1-boundary-cases.md#18-公开语料覆盖真实用例)）后，
> 合并覆盖率 50.8% → **55.7%**。
>
> 2026-10-25 三次实测：**全功能模块补测**（auth/rewrite/dev_mod/safe-counts/
> static 主流程/proxy 主流程与缓存策略/websocket/proxy_tcp/events EventHandler
> 与 h2 助手/dynlog/config 解析/listener/core hooks/color/autocert）后，
> 合并覆盖率 55.7% → **68.5%**，通过测试的包 30 → 35。本轮发现并修复
> 1 个 P0 级缺陷（proxy 发送路径对非 net.Error 写错误走 Fatal=os.Exit，
> 一条上游连接异常即可打死整个进程）。

## 总览

| 口径 | 数值 |
| --- | --- |
| 合并覆盖率（-coverpkg=./...） | **68.5%**（语料回打前 55.7%，首轮 50.8%） |
| 包数 / 通过 `-race` | 35 / 35 |
| 关键热点函数 | ParseHeader 93.7%、ReadBytes 100%、LoadCache 95.5% |

## 分包数据与解读（2026-10-25 三轮补测后）

| 包 | 状态 | 解读 |
| --- | --- | --- |
| infra/banner | 高 | 彩色/无色双路径全覆盖 |
| modules/safe | 高 | 控制面 API + overlay + per-IP 计数限流（counts 全语义）已测 |
| modules/auth | 已补测（原 0%） | Basic 解析畸形矩阵 + 常数时间比较 + 401 路径 |
| modules/appfirewall | 高 | 引擎生命周期/检测入口/未知规则忽略已测 |
| modules/cache | 高 | 热层/LRU/过期/磁盘恢复（95.5%）已测 |
| modules/rewrite、dev_mod | 已补测（原 0%） | 301 重定向与 dev 应答主路径 |
| modules/websocket、proxy_tcp | 已补测 | 回显搬运 + 坏 CircleData 保护 + 拨号失败 + 上游断开收尾 |
| modules/static | 已补测 | 301/keep-alive/close/404 主流程；gzip 大文件路径未测 |
| modules/compress | 中 | 双算法往返已测；DecompressBytesGzip 死代码已删 |
| config | 已补测 | include 目录遍历（嵌套/非 json/坏 json/空目录/缺失）、parsePathType/parseZip、Reload 失败保护快照 |
| runtime/events | 已补测 | EventHandler 全链路分发 + GFilterHostPath 回退 + h2 助手（fillRequest/statusFromFirstLine/hop-by-hop/writer） |
| runtime/listener | 已补测 | SortByPort 构建/去重/LisType、certMatches（通配/预解析/坏 DER） |
| runtime/dynlog | 已补测 | pathTypeName 全枚举 + fieldTime 占位 |
| core | 已补测 | Services 钩子注入/nil 安全/撤销注册、DefaultParseCommandHandler、EventReuse |
| runtime/server | 中 | 信号/停机/accept 已测；后台化 exec 自身仅 Linux 手工验证 |
| cmd | 低（保留） | 命令 handler 靠 unit_test/cmd 行为测试；writePidTo / waitProcessGone 已单测，CLI 主循环与平台后台化不做单测 |
| init | 部分 | randomSerialNumber 已测；InitSystem/newRoot/newCert 为启动编排与 ACME 路径，靠 E2E |
| monitor | 不测 | Windows 托盘 GUI 封装 |

## 已识别并清零的死代码（2026-10-25）

- `compress.DecompressBytesGzip`（无生产调用方；三轮补测中再次确认后删除）
- `PathQuery.Del/Set`（无调用方）
- 此前轮次已删：`validWAFMode`、`Parse()`（chunked 死实现）、`setConnCfg`、`allow` 包装等

## 补测优先级（按风险 × 缺口）

> **2026-10-25 更新：前三项已全部补测完成**，并叠加了一轮公开语料
> （httparse 向量 + payload-box 走私 payload）覆盖性回打，合计修掉 5 个
> 真实缺陷（分片误拒 400、冒号前空白放行、响应 reason 截断、
> ReadBytes 自旋/半截交付、过期缓存条目回放）。明细见
> [1-boundary-cases.md §1.8](1-boundary-cases.md#18-公开语料覆盖真实用例)。

1. ~~runtime/events.parseRequest 的分片读分支~~ ✅ `runtime/events/fragment_test.go`
2. ~~proxy getDataFromServer 的上游异常矩阵~~ ✅ `modules/proxy/upstream_matrix_test.go`（M1-M7：半响应/关闭/真实 RST/慢/对照/close 路径）
3. ~~cache LoadCache 磁盘恢复~~ ✅ `modules/cache/load_cache_test.go`（合法/损坏 gob/旧格式/过期/坏目录）
4. **config include 的目录遍历分支**（空目录/非 json 文件/嵌套）。
5. cmd 的 CLI 参数解析（cobra 层）：低优先，行为已由 commands_test 锁定。

## 约定

- 覆盖率是手段不是 KPI：注入面、并发边界（`-race`）、协议边界的用例价值高于数字。
- 新增功能合并门禁：对应包不因新代码**下降**（见 [develop/1-testing.md](../develop/1-testing.md)）。
