# 2. 代码结构与开发规范

## 分层与依赖

```
cmd(组合根) → modules(功能) → core(契约) → runtime(数据面) → config → infra
```

- 依赖方向单向向下，`tests/arch/layering_test.go` 自动守卫；下层需要上层能力时走 `core.Services` 钩子注入（参考 `RejectWAF` 的接法），**不要让 runtime/core import modules**。
- 第三方依赖的隔离原则：guardianwaf 只出现在 `modules/appfirewall` 与 cmd——换引擎只重写一个包。

## 并发约定（reload 安全）

所有"reload 会换、请求在读"的共享状态一律 **构建新快照 + atomic 替换**，禁止无同步的原地写：

- 配置：`config.Current()` / `MIME()`（atomic.Pointer）
- 监听器路由表：`Listener.Hosts()/Cfgs()`（内部 `cur atomic.Pointer`，热更新走 `updateCfg`）
- 日志字段表：`dynlog.logFields`；四路文件日志器：`logging.System()/Access()/Error()/Safe()`
- WAF 引擎：`appfirewall` 包内 atomic

新增共享状态时先问一句："reload 时谁写、请求谁读？"——答案不是 atomic 就得是锁，且写路径必须幂等。

## 错误处理约定

- 解析/输入层：畸形输入**拒绝并响应**（400/413/416），绝不 panic；每连接协程有 recover 兜底，但那是最后防线不是设计。
- 上游/IO 层：失败关闭连接并回 5xx 或错误页；资源（fd/连接）在所有失败分支显式释放。
- reload 路径禁止 `Fatal`——错误向上返回，保留旧状态继续服务。

## 提交规范

- conventional commits + 中文主题：`feat(proxy): ...`、`fix(request): ...`、`perf(core): ...`、`refactor(config): ...`、`docs(...)`、`build(deps)...`。
- 一类改动一个 commit，正文写"为什么"和关键取舍；行为变化、修复的 bug、基准数据写进正文。
- 基准数据变化随 commit 附带（参考 ParseHeader 零拷贝 commit 的写法）。

## 文档同步（合并门禁）

| 改动 | 必须同步 |
| --- | --- |
| 用户可见行为 | `docs/feature/` 对应篇章 |
| 迭代项状态 | `develop/plans/iteration-plan.md` |
| 目录/依赖结构 | `docs/architecture-map.md` |
| 新增/移除依赖 | 根 `README.md` 依赖表 |

## 反例（历史真实教训，引以为戒）

- `Nodelay` 读错配置 key——复制粘贴的 key 没校验（已修）。
- `RejectWAF` 钩子语义反向（true=放行 vs true=拒绝）——接钩子前先读契约注释。
- 基准里 `bytes.Repeat(…, b.N)` 造成 GC 噪声——流式输入用固定内存 reader。
