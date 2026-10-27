# 20. GuardianWAF 集成（占位）

状态：🚧 集成已落地（2026-10-25，commit 见 git log），待线上灰度观察后从 monitor 收敛到 enforce 的运营决策

> 实现进度已超出本占位文档：功能现状见 [15-appfirewall.md](15-appfirewall.md)，
> 完整方案见 `docs/develop/plans/guardianwaf-adoption.md`。以下为立项时的原始规划，留作对照。

## 功能（目标）

以 [GuardianWAF](https://github.com/GuardianWAF/GuardianWAF) v1.1.0 替换内部正则检测，成为应用防火墙的检测引擎：11 类攻击检测器（sqli、xss、路径穿越、命令注入、XXE、SSRF、SSTI、NoSQL、请求走私、开放重定向、GraphQL）+ 分词评分模型（block/log 双阈值）+ 误报管理。

## 方案

完整架构方案见 `docs/develop/plans/guardianwaf-adoption.md`。要点：

- **只用库模式**（`Engine.Check(*http.Request)`），fast-https 本身是网关，不用 standalone/sidecar。
- **适配层**：`core.Event` 合成检测专用 `*http.Request`（body 已缓冲、零拷贝引用、不做 URL 解码）。
- **检查点收敛**：从 proxy/static 各自 FilterHandler 收敛到 `EventHandler` 统一检查，每请求恰好一次。
- **零开销默认**：未配置 `appfirewall`/`waf` 的 location 引擎不构建，行为与今天一致。
- **配置兼容**：`appfirewall: ["sql","xss"]` 映射到检测器启用集；新增 `waf` 配置块（mode=monitor/enforce/disabled、阈值、敏感度）。
- **生命周期**：引擎 `atomic.Pointer` 持有，reload 幂等重建（`initModules` 同节奏），`AfterStop` 释放。
- **灰度**：先 `ModeMonitor` 影子验证误报，再切 `ModeEnforce`。

## 依赖与许可

`github.com/guardianwaf/guardianwaf`，MIT，零外部依赖（go.mod 净增 1 行），Go 1.26.6+（本项目 1.27 满足）。版本钉死不跟踪 latest。

## 验收

- 11 类攻击样本集全部拦截（monitor→enforce 语义正确）、正常样本零误杀。
- 防火墙检查每请求恰好一次；reload 前后在途请求无 nil 引擎。
- 干净请求 p99 劣化 ≤ 10%（硬门槛）。
- `go test -race ./...` 全绿，三个 E2E 不回归。

## 实施切分

1. 依赖接入 + arch 守卫（guardianwaf 只许出现在 modules/appfirewall 与 cmd）
2. 适配层 + 引擎封装 + 单测（未接线）
3. 检查点收敛 + 旧调用删除
4. `waf` 配置块 + 校验器
5. 文档同步 + 基准报告

完成后本占位文档转"已实现"，[15-appfirewall.md](15-appfirewall.md) 的内部实现降级为回退路径。
