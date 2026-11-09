# Fast-Https 边界加固功能需求

状态：📋 **待实现**。本文只写**要开发什么能力**；对应的缺陷核验记录（本项目
目前有没有这些问题、修没修）在 [4-edge-case-coverage.md](../../test/4-edge-case-coverage.md)。

## 需求来源

同类 HTTP 服务器在公开反馈里反复提到的场景，整理成对本项目的功能需求。
不引用具体条目，只保留「要什么能力」这一层。

## R-01 静态根的符号链接策略可配置

**现状**：`staticPathInRoot` 用字符串前缀判断，root 内的符号链接一律放行。

**需求**：提供显式策略，让部署方选择——

```json
{"static_root": "./httpdoc", "symlink_policy": "reject" | "allow" | "check-at-load"}
```

| 取值 | 行为 |
| --- | --- |
| `reject` | 请求路径命中符号链接即 404（运行时 `EvalSymlinks` 后比对） |
| `allow` | 保持现状，不做额外检查 |
| `check-at-load` | 启动时校验 root 下无符号链接，有则拒绝加载配置 |

**默认值**：`check-at-load`。理由：运行时零开销，配置者必须显式清理，
与「默认安全但不牺牲性能」的目标一致。

**验收**：三种取值各一个用例；`reject` 在 Linux/macOS 实跑（Windows 上
创建链接需权限，Skip）。

## R-02 Windows 保留名与尾点尾空格的显式拒绝

**现状**：`/file.`、`/file ` 落到普通文件名分支，行为取决于目标文件是否存在。

**需求**：在路径规范化阶段显式处理 Windows 语义——

- 尾点、尾空格：拒绝（400）或规范化后重新查一次
- 保留设备名：`CON`、`PRN`、`AUX`、`NUL`、`COM1`–`COM9`、`LPT1`–`LPT9`
  （含带扩展名的形式，如 `CON.txt`）一律拒绝
- 仅在 `GOOS == "windows"` 时生效，其他平台不受影响

**验收**：`traversal-vectors.jsonl` 增加保留名与尾点向量，Windows 实跑。

## R-03 未知域名的 TLS 兜底策略

**现状**：`modules/certmanager` 没有「SNI 不在任何站点证书覆盖范围」的用例。

**需求**：

- 提供 `tls.default_certificate` 配置项：未知 SNI 时用它完成握手
- 未配置时：明确拒绝握手并在日志里记原因，**不允许 panic**
- 握手失败不能影响进程存活，也不能影响其他连接

**验收**：`sni-cases.jsonl` 覆盖「已知 / 未知 / 空 SNI」三态；未知 SNI 场景
在 `-race` 下并发握手，进程不崩。

## R-04 通配符证书的匹配规则

**需求**：按「通配符只跨一层」实现并写进文档——

- `*.example.com` 匹配 `a.example.com`，**不**匹配 `a.b.example.com`
- 尾点 `example.com.` 与大小写差异归一到同一张证书
- 同名多证书（RSA + ECDSA）按客户端 `signature_algorithms` 选择

**验收**：`sni-cases.jsonl` 覆盖上述四类，含反例。

## R-05 代理的 `X-Forwarded-For` 策略

**需求**：

- 默认**追加**客户端地址，绝不信任客户端自带的 `X-Forwarded-For`
- 提供 `proxy.forwarded_for.max_hops`：超过后停止追加并记日志
- 提供「信任代理前缀」配置（`trusted_proxies`）：只有来自这些地址的请求
  才保留其 `X-Forwarded-For`
- `Host` 头：提供 `preserve_host` 开关，默认透传

**验收**：`forwarded-chain.jsonl` 覆盖「无 XFF / 客户端伪造 / 多层代理 /
超跳数」四种输入。

## R-06 hop-by-hop 头的剥离

**需求**：转发前无条件剥掉 `Connection`、`Keep-Alive`、`Proxy-Authenticate`、
`Proxy-Authorization`、`TE`、`Trailer`、`Transfer-Encoding`、`Upgrade`，
以及 `Connection` 里点名的扩展头。

**验收**：`hop-by-hop-headers.json` 逐项核对上游收到的头。

## R-07 压缩协商的完整语义

**现状**：`modules/compress` 源码 57 行、测试 39 行，是最薄的一块。

**需求**：

- 按 q 值协商：`gzip;q=0` 必须不压，`q` 相同时按服务端偏好定序
- 响应已带 `Content-Encoding` 时跳过
- `HEAD` 不产生 body，且 `Content-Length` 与 GET 一致
- Range 请求跳过压缩（压缩后 Range 语义失效）
- 已压缩类型（图片、zip、woff2、mp4）跳过，列表可配置
- 补 `Vary: Accept-Encoding`；压缩后移除或改写 `Content-Length`
- 提供最小压缩阈值（`min_length`），小响应不压

**验收**：`accept-encoding.jsonl` 覆盖上述七条，每条一个用例。

## R-08 连接与超时的边界行为

**需求**：

- `Expect: 100-continue`：先回 `100`，再读 body
- keep-alive 复用连接时，请求间不残留状态（cookie、认证、缓存键）
- header 超限 → 431；body 超限 → 413；两者都不 panic
- 提供 `max_header_bytes`、`max_body_bytes` 配置项

**验收**：`runtime/listener` 新增一组，`-race` 下跑。

## R-09 reload 的原子性与可见性

**需求**：

- reload 失败 → 旧配置继续生效，错误在 CLI 与桌面端可见
- reload 期间的在途请求：处理完再切换（优雅），或立即中断并在日志标注
- 并发 reload 串行化，后到的等待或直接返回「正在重载」

**验收**：构造「新配置非法」的场景，断言旧配置仍在服务；`-race` 下并发
触发 10 次 reload，无 panic、无状态错乱。

## R-10 拒绝行为的可观测性

**需求**：被拒绝的请求（路径穿越、未知 SNI、超限、压缩跳过）留下结构化
日志字段：

| 字段 | 内容 |
| --- | --- |
| `reject_reason` | 枚举值，如 `path_traversal`、`unknown_sni`、`header_too_large` |
| `rule` | 命中的规则或配置项名 |
| `remote_ip` | 来源地址 |

**约束**：日志里不得出现完整请求路径的原始编码（可能含敏感参数），只记
归一化后的片段。

**验收**：`logquery` 能按 `reject_reason` 过滤出这些记录。

## 优先级

| 需求 | 优先级 | 理由 |
| --- | --- | --- |
| R-01 符号链接策略 | P0 | 部署方可能把用户可写目录挂进 root |
| R-03 未知 SNI 兜底 | P0 | 握手崩溃会带走整个进程 |
| R-05 XFF 策略 | P1 | 影响上游看到的真实来源，安全相关 |
| R-07 压缩语义 | P1 | 现有测试最薄，回归风险高 |
| R-09 reload 原子性 | P1 | 直接影响可用性 |
| R-02 Windows 显式拒绝 | P1 | 只在 Windows 暴露，CI 覆盖不到 |
| R-06 hop-by-hop 剥离 | P1 | 协议正确性 |
| R-04 通配符规则 | P2 | 现有实现可能已正确，缺用例 |
| R-08 连接边界 | P2 | 需要压力才能暴露 |
| R-10 可观测性 | P2 | 排查效率，不影响功能 |

## 不做的事

- 不追求覆盖率数字。缺的是**针对性**，不是总量。
- 不为「可能永远不出现」的场景做能力（例如 HTTP/0.9 兼容开关）。
- 不在本计划里改架构（`--config` 提升为 persistent flag 之类另开议题）。

## 推进方式

每个需求按同一节奏：

1. 先在 `testdata/` 写用例表，跑出当前的失败或记录现状
2. 在 [缺陷核验表](../../test/4-edge-case-coverage.md) 里把对应行从 ❓ 改成 ✅ 或 ⚠️
3. 需要改代码的单独一个提交，用例作为回归
4. 只补用例不改代码的，标注「锁定现状」并说明为什么可以接受

## 相关文档

- 缺陷核验记录见 [4-edge-case-coverage.md](../../test/4-edge-case-coverage.md)
- 按输入域组织的用例集见 [1-boundary-cases.md](../../test/1-boundary-cases.md)
- 覆盖率评估见 [0-coverage-assessment.md](../../test/0-coverage-assessment.md)
