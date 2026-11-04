# GuardianWAF 接入架构方案

状态：已实施（2026-10-25， GuardianWAF v1.0.0）。实施结果与方案的差异：钉 v1.0.0 而非 main（检测器为 6 类而非 11 类）；阶段 0 的 monitor 影子验证保留为运营决策。

## 1. 背景与目标

### 1.1 现状

`modules/appfirewall` 是 fast-https 当前的应用层防火墙，存在三个结构性问题：

1. **检测能力薄弱**：`sql` 规则是手写的 11 条保守正则（2026-10-25 落地），覆盖最常见注入语法但极易绕过；`xss` 规则依赖 bluemonday 清洗 body，不查 query/header；其余 9 类常见攻击（路径穿越、命令注入、SSRF、SSTI、NoSQL 注入、请求走私等）完全没有检测。
2. **调用点分散**：`HandleAppFireWall` 在 `proxy.ProxyFilterHandler` 和 `static.HandelSlash` 里各调一次——同一请求只命中一个 location 时恰好一次，但防火墙变成了各 handler 的"自觉行为"，新增 location 类型（rewrite/devmod）默认没有防火墙，靠开发者记得挂。
3. **检测模型原始**：二元正则匹配，没有评分、没有分级响应、没有误报管理机制。

### 1.2 目标

- 引入 [GuardianWAF](https://github.com/GuardianWAF/GuardianWAF)（v1.1.0，MIT）作为检测引擎，一次性获得 11 类攻击检测与评分模型。
- 防火墙检查收敛为**每请求恰好一次**，与 location 类型解耦。
- 保持现有 `appfirewall: ["sql", "xss"]` 配置语义兼容，新增能力以新配置面渐进开放。
- 不引入传递依赖（GuardianWAF 零外部依赖，go.mod 净增 1 行）。

### 1.3 非目标

- 不使用 GuardianWAF 的 standalone / sidecar 部署模式——fast-https 本身就是网关，再叠一层反代毫无意义，只用**库模式**。
- 不替换 `safe` 模块：连接级黑名单、per-IP 限流、location 级计数限流是连接/流量层防护，与 WAF 的请求内容检测是不同层次，边界见 3.6。
- 不引入 GuardianWAF 的 dashboard/MCP/REST 管理面（那是 standalone 模式的配套），配置仍走 fast-https 的 JSON 配置。
- 不做完整 WAF 能力承诺：GuardianWAF 的评分模型同样存在绕过可能，见第 6 节风险。

## 2. GuardianWAF 概况

| 项 | 值 |
| --- | --- |
| module path | `github.com/guardianwaf/guardianwaf` |
| Go 要求 | 1.26.6+（本项目 go 1.27.0，满足） |
| 依赖 | 零外部依赖，仅标准库 |
| 许可 | MIT |
| 成熟度 | v1.1.0，963 commits，单一维护者（见 6.1 供应链风险） |
| 性能 | 干净请求 < 1ms p99；全检测器 < 500µs p95；库模式内存 < 5MB |

**检测器（11 个）**：sqli、xss、路径穿越（lfi）、命令注入（cmdi）、xxe、ssrf、ssti、nosqli、HTTP 请求走私、开放重定向、GraphQL 深度/复杂度。

**检测模型**：分词 + 评分，不是二元规则。请求分解为 token 按检测器加权计分，按阈值分级：达到 `block` 阈值（默认 50）阻断、达到 `log` 阈值（默认 25）仅记录；bot 流量可选返回 SHA-256 工作量证明挑战。每个检测器可独立开关并调敏感度倍数（`WithDetector("sqli", true, 1.5)`）。

**库模式 API**（实施时以源码 `guardianwaf.go` / `options.go` 为准，README 仅示例）：

```go
waf, err := guardianwaf.New(guardianwaf.Config{...},
    guardianwaf.WithMode(guardianwaf.ModeEnforce),
    guardianwaf.WithThreshold(60, 30),
    guardianwaf.WithDetector("sqli", true, 1.0),
    guardianwaf.WithIPWhitelist("10.0.0.0/8"),
)
defer waf.Close()

result := waf.Check(req)        // result.Blocked / TotalScore / Findings
waf.OnEvent(func(e guardianwaf.Event) { ... })   // e.Method / Path / ClientIP / Score / Action
```

## 3. 架构设计

### 3.1 适配层：core.Event → *http.Request

**核心阻抗点**：GuardianWAF 库模式的 API 面向 `*http.Request`（net/http），而 fast-https 的 HTTP/1.1 路径用的是自研解析器产出的 `core.Event` + `request.Request`。

方案：在 `modules/appfirewall` 内新增适配函数，从已解析的 `request.Request` 合成一个最小 `*http.Request`：

```go
// toHTTPRequest 把自研解析结果适配成 GuardianWAF 需要的 *http.Request。
// body 在 parseRequest 阶段已完整缓冲，这里零拷贝引用。
func toHTTPRequest(ev *core.Event) *http.Request {
    req := ev.RR.Req
    r := httptest-style 合成：
      - Method = req.Method
      - URL = url.URL{Path: req.Path 解析出的路径, RawQuery: query}
      - Header：逐条写入（canonical 键已保证）
      - Body = io.NopCloser(bytes.NewReader(req.Body.Bytes()))
      - RemoteAddr = ev.Conn.RemoteAddr()
}
```

要点：

- **时机在 body 缓冲之后**：`parseRequest` 已把 body 完整读入 `req.Body`（并受 `MaxBodySize` 上限约束，默认 512KB），合成 Request 的 Body 是内存引用，无二次读 socket、无内存放大。
- **语义差异声明**：合成的 Request 仅供 WAF 检测使用，不进入任何响应路径；`RequestURI`、TLS 状态等字段按 GuardianWAF 实际读取的字段补齐（实施第一步是核对其检测器实际消费哪些字段，写进适配层的注释与测试）。
- h2 路径（`events_h2.go` 的 `fillRequest`）产出同一个 `request.Request` 结构，天然走同一条适配路径，无需单独处理。

### 3.2 调用点收敛：每请求一次

防火墙检查从各 handler 的 FilterHandler 中移除，收敛到 `runtime/events` 的 `EventHandler`：在 location 匹配成功之后、`LocationRejected`/`AuthRejected` 之前，统一执行：

```go
if !appfirewall.Allowed(cfg, ev) {   // 内部写拒绝响应，返回 false
    return
}
```

- `proxy.ProxyFilterHandler` 与 `static.HandelSlash` 里的 `HandleAppFireWall` 调用删除。
- 收益：所有 location 类型（含 rewrite/devmod）默认受保护；检查次数恒为 1；拒绝路径统一（403 + 安全日志）。
- websocket/tcp 代理路径维持现状（连接被 hijack，内容不经过 HTTP 检测管线；websocket 握手请求本身在升级前会经过检查点）。

### 3.3 配置映射与兼容

现有配置语义保持不变：

```json
"appfirewall": ["sql", "xss"]
```

映射规则：`"sql"` → 启用 `sqli` 检测器；`"xss"` → 启用 `xss` 检测器。二者均配置且评分模型给出判定，行为与现状对齐（检测到即拒绝，回 403 `DefaultForbidden`）。

新增可选配置块（不写则用安全默认：enforce 模式、默认阈值、全部 11 个检测器开启）：

```json
"waf": {
  "mode": "enforce",              // enforce | log（log 用于灰度观察，见第 7 节）
  "threshold": { "block": 50, "log": 25 },
  "bot_challenge": false,          // 工作量证明挑战，默认关闭（可能影响 API 客户端）
  "sensitivity": { "sqli": 1.0, "xss": 1.0 },
  "disabled": ["graphql"],
  "ip_whitelist": ["10.0.0.0/8"]
}
```

- 优先级约定：`appfirewall` 数组是"按 location 的最小启用集"（只开列出的检测器）；`waf` 块是全局策略。两者同时存在时取并集，location 粒度可通过 `disabled` 之外再引入 per-location 覆盖——首版不做，配置面按需生长。
- **bluemonday 的去留**：GuardianWAF 是检测+阻断语义，不做"清洗后转发"。现有 `xss` 规则在 bluemonday 清洗后依旧按检测结果拒绝，清洗本身没有对外语义。接入 GuardianWAF 后清洗路径废弃，`microcosm-cc/bluemonday` 依赖随之移除（go.mod 再减一行）。

### 3.4 生命周期与 reload

- WAF 实例在 `cmd.RegisterHandlers` 后（`initModules` 同一时机）构建，reload 时在 `initModules` 里重建——与 safe 模块的 `AfterListen` 钩子同节奏。
- 重建策略：幂等替换包级 `atomic.Pointer[guardianwaf.WAF]`；在途请求持旧实例完成本次检测（与配置快照同语义）。reload 配置校验失败则旧实例继续服务。
- 每次重建都 `Close()` 旧实例（释放其内部缓存），失败不阻断 reload。

### 3.5 事件与日志

`waf.OnEvent` 回调统一接到 `infra/logging.Safe()`：

```
{"level":"warn","kind":"safe","remote_addr":"...","reason":"waf",
 "detector":"sqli","score":85,"action":"block","path":"..."}
```

- 拒绝响应复用现有 `response.DefaultForbidden` 与 `logReject` 路径，`reason` 取 `waf`（区别于现有 `blacklist`/`rate_limit`）。
- Findings 明细（命中的 token/规则）记入 debug 级别，避免 info 级日志被攻击流量刷爆。

### 3.6 与 safe 模块的边界

| 层 | 模块 | 时机 | 判定依据 |
| --- | --- | --- | --- |
| 连接接入 | safe（黑名单/限流） | ConnFilter，每连接 | 来源 IP，不看内容 |
| 位置访问控制 | safe（计数限流/认证） | location 匹配后 | IP + 配置阈值 |
| 请求内容 | appfirewall（GuardianWAF） | location 匹配后，认证前 | 请求内容评分 |

顺序：黑名单 → 限流 → （location 匹配）→ 认证 → WAF。WAF 放在认证之后是因为凭据错误产生的噪音请求不值得花检测成本；如实施中发现探测型攻击先打 WAF 更划算，可调换，属实现细节。

### 3.7 覆盖矩阵

| 路径 | 经过 WAF | 说明 |
| --- | --- | --- |
| HTTP/1.1 静态/代理/rewrite/devmod | 是 | 3.2 收敛点 |
| HTTP/2（x/net 协议栈） | 是 | 同一 `request.Request`，同一检查点 |
| websocket 握手 | 是 | 升级前的普通请求 |
| websocket 帧 / tcp 代理 | 否 | 非 HTTP 语义内容，不在 WAF 职责内 |
| 代理响应（上游→客户端） | 否 | 响应向检测是独立课题，不在本期 |

## 4. 依赖与版本策略

- `go get github.com/guardianwaf/guardianwaf@v1.1.0`，go.mod **钉死具体版本**，不跟踪 @latest。
- 依赖零传递：tidy 后 go.mod 应只净增这一行，CI 中可加守卫。
- 升级走显式 PR，跑全量回归 + WAF 检测用例集（见 7.4）。

## 5. 分阶段落地

| 阶段 | 内容 | 出口标准 |
| --- | --- | --- |
| 0 影子验证 | 配置 `mode: log`（若上游无 log 模式，则 Findings 全量记录、不阻断——以 options.go 源码为准，列为实施首个确认项）；线上流量观察 1-2 周 | 误报清单成文，阈值确定 |
| 1 替换 | enforce 模式接管 `sql`/`xss` 规则语义；调用点收敛到 EventHandler；适配层 + 合成 Request 的单测 | 旧配置行为兼容测试全绿 |
| 2 扩展 | `waf` 配置块开放；11 检测器默认全开；事件日志接 SafeLog；性能基准对比（干净请求 p99 不劣化超过 10%） | 基准报告 |
| 3 清理 | 删除内部 sql 正则与 bluemonday 清洗路径，go.mod 移除 bluemonday | 依赖守卫测试更新 |

每阶段一个或多个独立 commit，可单独 revert。

## 6. 风险与回退

### 6.1 供应链风险（最大风险）

GuardianWAF 是单维护者项目（35 stars，Ersin Koc），存在停止维护、被接管或破坏性变更的可能。缓解：

- 全部调用隔离在 `modules/appfirewall` 包内，`core`/`runtime` 不出现 guardianwaf import（依赖守卫测试固化该边界）；
- 检测入口只有一个接口方法 `Allowed(cfg, ev) bool`，替换引擎（如 Coraza）只需重写该包；
- 版本钉死；不接受自动升级。

### 6.2 行为差异风险

评分模型与正则的误报/漏报画像不同：正常查询串（含 `-`、`'`、SQL 关键词的文案）可能得分越线。缓解：阶段 0 影子验证先行；`sensitivity` 倍数 + 阈值可调；误报用排除规则处理；`waf.mode` 一键回 log 模式即等价于关闭 WAF。

### 6.3 性能风险

库模式标称 < 500µs p95（全检测器），叠加 fast-https 每请求的合成 Request 分配。验收线：干净请求端到端 p99 劣化 ≤ 10%（以阶段 2 的前后基准为准）；不达标则裁剪默认启用的检测器集合。

### 6.4 回退

任何阶段 revert 到内部正则实现都是一次 `git revert` + 重启；配置层面 `waf.mode: log` 或删除 `waf` 块即可让拦截行为降级为观察。

## 7. 验收标准

1. 旧配置 `appfirewall: ["sql","xss"]`：注入/XSS 样本集全部拒绝，正常样本集全部放行（样本集固化为表驱动测试，样本覆盖 11 类攻击各至少 3 例）。
2. 防火墙检查每请求恰好一次（以计数钩子或日志验证）。
3. reload 前后在途请求不出现 nil WAF、不重复 Close。
4. go.mod 除 guardianwaf 外无新增依赖；bluemonday 在阶段 3 移除。
5. 基准：干净请求 p99 劣化 ≤ 10%，攻击请求不影响其他连接的延迟分布。
6. `go test -race ./...` 全绿；proxy/reload/h2 三个 E2E 不回归。
