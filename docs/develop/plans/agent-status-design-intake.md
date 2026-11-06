# Agent 状态栏（agent-status-design）设计输入表

> 本文件是 `/agent-status-design` skill 要求的设计输入与对齐留痕，不是待填问卷。
> 原则：能从项目、文档、实测里拿到的自己填，只有必须由人拍板的业务决策才问用户。
>
> **本文件经 3 轮需求澄清定稿，最终结论以第 7 节为准；第 1–5 节为第 1 轮的作废方案，保留仅作演进留痕。**

---

## 1. 自主勘察事实（项目 / 文档 / 实测）—— 第 1 轮，已作废

| 事实 | 取值 | 证据（文件:行） |
| --- | --- | --- |
| 业务场景 | 编码 Agent（DeepSeek Harness / hy4free）在 fast-https（Go Web server）仓库里做插件化改造咨询、多轮长任务 | 本会话上下文；`E:\code\ncepu_bj\fast-https\go.mod` |
| 目标对象 | **不是 fast-https 这个 Go 项目**，而是承载本会话的 Agent 框架（DSH）的上下文组装层 | `agent-status-design` skill：产出是"框架代码（Harness 侧的状态维护与消息拼装）" |
| 工具清单与数量 | 约 50 个（本 Agent 可见）：文件/搜索/命令/调度/子 Agent/插件检查等 | `Tool.listTools` 输出（spill 文件 `4dd2597d0781-...txt`，`"name":` 行计数 50） |
| 是否已有状态栏 | **无** `<agent_status>`；现有的是 system-reminder 里的时间/环境快照与 todo 提示 | 本会话每轮 system-reminder；未见 `<agent_status>` 标签注入 |
| 是否已有计数器 | **无**累计计数/上限检查；只有"工具调用次数"这类散落的提示 | 工具返回结果中未见"第 N 次/已用/上限"标注 |
| 是否已有 TODO 存储 | **有**，但结构不符：`todo_write` 全量替换、无稳定 ID/时间戳；另有 `create_goal`/`update_goal` 目标与 `team_task_*` 共享任务板 | `todo_write`、`create_goal`、`team_task_create` 工具定义 |
| 消息拼装位置 | 框架侧，不在本仓库。扩展点：`systemPrompt.section()` / `agent/pre-step` / `tools/result` / `agent.inject()` / `wire.view` | `Service.listService`、`Event.listEvents`、`practices.md` |
| 模型与服务商 | hy4free（Tencent Hunyuan Hy）；会话内实测缓存定价按 DeepSeek API 页取 | 模型身份声明；[DeepSeek 定价页](https://api-docs.deepseek.com/quick_start/pricing) |
| 缓存单价倍数 α | 0.020（Flash 峰值 0.006/0.30）～0.033（V4-Pro 峰值 0.044/1.32） | [Models & Pricing](https://api-docs.deepseek.com/quick_start/pricing)：cache hit / cache miss 美元价 |
| 典型轮数 N / 轮间后缀 R / 单条状态 S | 12~30 / 800~1500 / 60~120，取自 skill 校准表（**假设**，未实测） | `assets/update_strategy_decision.md:39-41` |
| 症状 | 多轮勘察里同一信息重复查（inspect 带参查询连错 3 次）、长上下文下早期目标易被淹没 | 本会话轨迹；`references/techniques.md` |
| 精确事件契约 | 带 `input` 的 `Event.listEvents` / `Service.listService` 查询被拒（`"input" must be an object`），实现前需退路：无参目录 → `Config.listConfigs` 拿 `packageDir` → 读已装包 `.d.ts`/JSDoc | 本会话 `cordis_inspect_query` 多次报错；`host-plugin.md` |

## 2. 推断的默认方案 —— 第 1 轮，已作废

| 设计项 | 推荐值 | 推断依据 |
| --- | --- | --- |
| 字段集 | 工具调用计数 + 约束 `已用/上限` + TODO + 时间 + 工作目录 + OS/Shell + 错误四层 + 异常 + 原始诉求 | 瞥一眼检验 5 问；`assets/status_render_spec.md:30-42` |
| 计数维度 | 全局 + 按最常重复的参数分组 | skill 默认 |
| 约束检查与超限动作 | 代码比对，`3/3 已达上限 ✗ 禁止再次调用`，`tools/pre-execute` guard 拦截 | `references/techniques.md:20-31` |
| TODO 存储与工具 | 启用；`rewrite_todo_list` / `update_todo_status` | `assets/todo_tool_schema.md` |
| 注入位置与角色 | 末尾追加 `user` 角色消息，`<agent_status>` 包裹，不改 system 前缀 | `references/injection-position.md:23-35` |
| 更新策略 | **持久追加**（成本脚本：α=0.02, S=120, R=1500, N=30 → C_replace=42630 vs C_append=1044；α·S·N/2=36 ≪ (1-α)R=1470） | `cache_cost_estimator.py` 实测输出 |
| 状态栏语言 | 中文 | `README.md`、`docs/develop/*.md` 全中文 |
| 审计日志 | 每次注入的状态快照 + 字段来源落盘 | `references/maintenance-security.md:52-57` |

## 3. 第 1 轮业务决策 —— 已作废

| # | 决策点 | 用户选择 |
| --- | --- | --- |
| 1 | 改造范围 | **自定义**：插件开发是另一个仓库的事；前提是"把我当前程序改造成适合 agent 访问的服务器，而不是让 agent 去数去查" |
| 2 | 上限策略 | **自定义**：想要"写好工具状态的访问，类似 MCP 的状态收集"；运维数据没调研、定时任务未实现 |
| 3 | 更新策略 | **持久追加（推荐项）** |

## 4. 假设记录（沿用至定稿，实现前复核）

| 假设项 | 采用值 | 复核时间 |
| --- | --- | --- |
| S / R / N | 60~120 / 800~1500 / 12~30，skill 校准表 | 实现前用真实 trace 跑 `discover_context.py` |
| α | 0.02（DeepSeek 定价页），本会话模型为 hy4free，服务商未必相同 | 实现前确认实际服务商缓存规则 |
| 带 input 的 inspect 契约 | 待读取已装包 `.d.ts` | 动手编码前 |

## 5. 验收方式

| 项 | 内容 |
| --- | --- |
| 核心指标 | 约束遵从率、完成任务迭代次数、思考 token 量、延迟/成本、遗漏子任务率 |
| 对照实验 | 用 `references/techniques.md` 逐步消融：时间戳 → 计数器 → TODO → 详细错误 → 系统状态 → 全启用 |

---

## 6. 定位修正（第 2 轮，18:09 用户第一次澄清）

> 用户原话要点：① "deepseek harness 插件的开发是另外一个仓库干的事"；② "前提是我要把我当前程序改造成适合 agent 访问的服务器，而不是让 agent 去数去查"；③ "想要写好工具状态的访问，类似 MCP 的状态收集"；④ "对运维手段没有做调研：哪些数据才重要、要提供给大模型才好调整策略"；⑤ "定时任务的抉择这些功能没有实现"。

**修正结论：第 1–5 节的 Harness 侧状态栏方案整体作废（属于另一个仓库的插件工作），本仓库的工作重新定位为「数据面状态供给」。**

skill 在本仓库真正能用的只有两条纪律：**状态必须由代码计算**、**每个字段都要回答「缺了它会做错什么决定」**；注入/更新策略等属于另一个仓库。

**仓库内已有设计（自主勘察新发现，直接回答"哪些数据重要"）**：

- [product-design-2.0.md](../../product-design-2.0.md) §6「观测输入」L195–209 已列出状态接口最小字段集；§7 L213–218 明确"活动连接、4xx/5xx 和限流拒绝没有汇总计数"；§11 失败表 L307–324 给出状态接口不可用时的降级语义。
- [agent-refactor-plan.md](agent-refactor-plan.md) L4 明确"补上设计稿缺失的 Agent 状态栏（依据 `agent-status-design` 技能）"；§5 阶段 A「数据面可观测」L146 是无依赖、可最先做、对运维立竿见影的第一步。
- [v1.5.0-plan.md](v1.5.0-plan.md) L5 明确"1.5.0 只做数据面进程自己能完成、不需要常驻 agent 配合的能力。agent、ctl 通道留给 2.0" —— 即当前版本刻意未做。
- [architecture-map.md](../../architecture-map.md) L11 的 `tests/arch/layering_test.go` 分层约束决定新代码落点。

**定时任务现状（grep 实证）**：已有 `modules/certmanager`（续期调度）、`modules/proxy/pool.go` L87（idleTTL/2 ticker）、`monitor/windows.go` L460（托盘刷新）、`init/init.go` L44（1s sleep）；**没有**给 Agent 供数的窗口聚合、冷门判定、快环规则 ticker。

**第 2 轮业务决策**

| # | 决策点 | 选项 | 用户选择 |
| --- | --- | --- | --- |
| 1 | 状态暴露形态 | A 回环 HTTP JSON / B 内嵌 MCP stdio / C 两者 | **D（自定义）：由 agent 常驻进程干，计数不进数据面** |
| 2 | 定时任务归属 | A 数据面只做无副作用聚合 / B 数据面内置快环 ticker / C 全塞数据面 | **A（但被第 1 项覆盖，见 7.1）** |
| 3 | 窗口口径 | A 单一 60s / B 1m+5m+15m / C 只累计 | **B：1m + 5m + 15m 多窗口** |

---

## 7. 架构定稿（第 3 轮，18:09 用户第二次澄清，以此为准）

> 用户原话：**"我需要一个 agent 常驻进程，来干这个事，我不希望计数这种影响到数据面的正常执行"**

### 7.1 与前两轮的差异（不回避反复）

| 轮次 | 判断 | 状态 |
| --- | --- | --- |
| 第 1 轮 | 改 DSH 框架上下文组装层 | **作废**（用户：插件是另一个仓库的事） |
| 第 2 轮推荐 | 数据面加计数器 + 回环状态接口 + 无副作用聚合 ticker | **部分作废**（用户：计数都不进数据面） |
| 第 3 轮定稿 | **数据面零改动；新建 agent 常驻进程做全部状态收集、聚合、定时器、对外服务、状态栏** | **有效** |

用户在第 3 轮同时选了「数据面只做无副作用聚合」，但自定义回答更彻底且更晚表达，按后者为准：**聚合也不放数据面**。理由：数据面日志的字段名契约本来就是为外部读方设计的。

### 7.2 三层定稿

```text
[另一个仓库] DeepSeek Harness / Cordis 插件  ── 消费状态、做 UI 与上下文注入
                    │ 拉一次（HTTP/MCP resources）
                    ▼
[本仓库·新增]  fast-https agent 常驻进程 = "适合 agent 访问的服务器"
               ├─ 增量读 4 份 JSON 日志 → 聚合计数 / 错误率 / 限流拒绝
               ├─ 1m / 5m / 15m 多窗口（用户已选）
               ├─ 定时器：窗口聚合 / 快环 / 慢环 / 冷门判定
               ├─ 脱敏：查询串、Cookie、Authorization 出本机前删除
               ├─ 状态资源（只读、版本号、幂等；持久追加在消费侧）
               └─ Agent 状态栏：在组装模型请求处注入 <agent_status>
                    │ 增量读、只读，零写入、零信号、零请求路径改动
                    ▼
[本仓库·现有]  fast-https 数据面进程  ——  不做任何改动
               日志 / pid 文件 / config path --json / status CLI 已是现成契约
```

### 7.3 关键证据：数据面早就为"外部控制面"预留了 4 个契约，无需改动

| 契约 | 位置 | 为控制面准备了什么 |
| --- | --- | --- |
| 日志字段名 | `infra/logging/consts.go:31-32` | 注释原文「日志查询与智能体过滤依赖这些名字，改名属于破坏性变更」「值为数字的字段写成 JSON number，比较状态码区间时不需要类型转换」 |
| `kind` 来源区分 | `infra/logging/consts.go:20-27` | 「合并多个日志文件读取时，用它区分来源，无需依赖文件名」 |
| 时间戳可范围比较 | `infra/logging/consts.go:3-7` | 「字典序等于时间序，可以直接做字符串范围比较，不必在查询侧再做时间解析」→ 增量读可直接按 offset/时间切 |
| 安全日志 `reason` | `modules/safe/safe.go:15-17` | `rate_limit` / `location_limit` / `blacklist` 已是受控枚举，不是自由文本 |
| 运行时路径 | `cmd/config_path.go:13-15, 17-23` | 注释原文「config 命令组给**外部控制面**一个稳定的取值入口：控制面是独立进程，不能靠猜」；`--json` 输出 4 个绝对路径 |
| pid 文件 | `cmd/commands.go:484-515` | 已是 JSON `{"pid":N}`，`status` 已能区分 absent/stale/running |
| 配置快照 | `config/config.go:147-180` | reload 失败保留旧快照，外部可对比配置指纹 |

**结论：数据面一行代码都不用改。** 常驻进程只读这些现成产物即可完成全部状态收集，且天然满足"不影响数据面正常执行"。

### 7.4 「定时任务」归属定稿（用户点名未实现的）

全部落在**常驻进程**，数据面不新增任何 ticker：

| 定时器 | 周期 | 依据 | 动作 |
| --- | --- | --- | --- |
| 窗口聚合 | 1s/5s | 用户已选 1m+5m+15m | 增量读日志尾部 → 滚动窗口计数（纯内存，无副作用） |
| 快环 | 秒级 | `product-design-2.0.md` §2 L67、§12 P3 | 本地规则，只调 Rate/Burst/临时黑名单，走 `ReloadFile` |
| 慢环 | 十秒~分钟 | §2 L68、§12 P5 | 脱敏摘要 → 模型 → 结构化变更 |
| 冷门判定 | 窗口驱动 | §5、§12 P4 | 空闲达标才 stop/start；有在途请求不停止 |

现有数据面定时器（`certmanager` 续期、`proxy/pool.go` 连接池、`monitor` 托盘刷新、`init` 缓存清理）保持原样，不给 Agent 供数。

### 7.5 Agent 状态栏的落点（收回第 6 节"移交另一个仓库"的说法）

用户要的常驻进程**自己会调模型**（慢环），所以 `<agent_status>` 的注入点就在**本仓库 `modules/agent` 组装 OpenAI 兼容请求处** —— 与 `agent-refactor-plan.md` §4.7 L122 完全一致。skill 的实现指导在本仓库**重新适用**：

- 计数/TODO/环境/错误摘要/原始诉求由**常驻进程代码**维护，不让 LLM 统计；
- 作为末尾独立 `user` 角色消息、`<agent_status>` 包裹注入，不改 system 前缀；
- 用户已选**持久追加**，需配套「以最后一条状态栏为准」；
- 状态快照落盘 `logs/agent.log`（设计稿 §10），可回放；
- 投毒防护：状态字段只来自受控数据源（聚合器、pid、配置指纹），日志原文脱敏后才进摘要。

另一个仓库的插件消费的是**同一份状态资源契约**，两处渲染各自做，不重复统计。

### 7.6 剩余假设（记录为假设，实现前复核）

| 假设项 | 采用值 | 复核时机 |
| --- | --- | --- |
| 状态资源传输 | 常驻进程回环 HTTP 只读 JSON；MCP 适配留给插件侧 | 与插件仓库对齐接口时 |
| 日志增量读取方式 | 按文件 offset + inode 变化（reload 会重开四份日志，§6 L207） | 编码前；需处理重开后的续读 |
| 快环/慢环默认阈值 | 沿用 `product-design-2.0.md` §9 草案（窗口 60s、黑名单 15min） | 对应阶段开工前 |
| α / S / R / N | 0.02（DeepSeek 定价页）/ 60~120 / 800~1500 / 12~30 | 阶段 C 用真实 trace 实测 |
