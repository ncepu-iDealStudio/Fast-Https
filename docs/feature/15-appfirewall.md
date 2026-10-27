# 15. 应用防火墙

状态：✅ 已实现（GuardianWAF 引擎，2026-10-25 接入）

## 功能

对经过 HTTP 管线的请求做应用层攻击检测：基于 [GuardianWAF](https://github.com/GuardianWAF/GuardianWAF) v1.0.0 的分词评分引擎，检测 SQL 注入、XSS、路径穿越、命令注入、XXE、SSRF（按配置启用），命中即 403。

## 使用

```json
"appfirewall": ["sql", "xss"]          // location 级：规则名映射到检测器（sql→sqli）
"waf": { "mode": "monitor" }           // 全局：enforce（默认）/ monitor / disabled
```

未配置 `appfirewall` 且无 `waf` 块的容器不构建引擎，请求零开销放行。

## 实现方案（modules/appfirewall/）

- **引擎封装**（waf.go）：`BuildWAF` 按配置快照构建（AfterListen 钩子，reload 幂等重建），`atomic.Pointer` 持有，`ShutdownWAF` 在 AfterStop 释放；构建失败保留旧引擎。
- **适配层**（adapter.go）：把已解析的 `core.Event` 合成检测专用 `*http.Request`（body 零拷贝引用、不做 URL 解码），guardianwaf 依赖被隔离在本包与 cmd。
- **检查点**：`events.EventHandler` 中 location 匹配与认证之后、handler 分发之前，经 `core.Services.RejectWAF` 钩子注入（runtime 不反向依赖功能包），每请求恰好一次。
- **拒绝语义**：403 + 安全日志（reason=waf，含 detector/score/action）+ 访问日志。
- **历史**：内部正则 sql/xss 实现已被引擎替代并删除（样本集保留在 waf_test.go 作为回归）。

## 测试

`modules/appfirewall/`：adapter 字段映射、enforce 阻断/monitor 放行/disabled/零开销默认、XSS 检测、reload 并发重建（-race）、11 类攻击样本回归。

## 已知限制

- v1.0.0 引擎含 6 类检测器（SSTI/NoSQL/请求走私等在 main 分支，升级另行评估）。
- 评分模型存在绕过可能；`mode: monitor` 可随时降级为只记录。
- 单维护者上游项目，版本钉死、接口隔离（换引擎只需重写本包）。

