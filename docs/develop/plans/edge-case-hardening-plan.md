# Fast-Https 边界加固功能需求

状态：🚧 **部分实现**（R-01 ~ R-11、R-13 已完成；R-12 待实现）。本文只写**要开发什么能力**；对应的缺陷核验记录（本项目
目前有没有这些问题、修没修）在 [edge-case-coverage.md](../../test/reports/edge-case-coverage.md)。

## 需求来源

同类 HTTP 服务器在公开反馈里反复提到的场景，整理成对本项目的功能需求。
不引用具体条目，只保留「要什么能力」这一层。

## R-01 静态根的符号链接策略可配置

**现状**：`staticPathInRoot` 用字符串前缀判断，root 内的符号链接一律放行。

**需求**：提供显式策略，让部署方选择——

```json
{"root": "./httpdoc", "symlink_policy": "reject" | "allow" | "check-at-load"}
```

策略写在 location 里，与 `root` 同级；只对 `type: local` 生效。

| 取值 | 行为 |
| --- | --- |
| `reject` | 请求路径下命中链接（符号链接或 Windows junction）即 404，运行时逐段检查 |
| `allow` | 保持现状，不做额外检查 |
| `check-at-load` | 启动时校验 root 下无符号链接，有则拒绝加载配置 |

**默认值**：`check-at-load`。理由：运行时零开销，配置者必须显式清理，
与「默认安全但不牺牲性能」的目标一致。

**验收**：三种取值各一个用例；`reject` 在 Linux/macOS 实跑（Windows 上
创建链接需权限，Skip）。

**状态**：✅ 已实现。`symlink_policy` 转小写后校验，空值取 `check-at-load`。
`check-at-load` 在 `LoadFile` 加载层与配置校验阶段都会扫描 root（root 自身是
链接不算违规，加载层兜底保证 `InitSystem` 等绕过校验的入口同样生效）；
`reject` 在静态层对请求路径逐段 `Lstat`，命中任何链接都 404（不再只拦逃出
root 的链接，与 `check-at-load` 的严格度保持一致），索引文件与 `try.Next`
这类请求路径的延伸也逐项检查。链接识别除 `ModeSymlink` 外，还用
`ModeIrregular` + `Readlink` 覆盖 Windows junction——实测 junction 在 Go
1.27/Windows 上既不是 `ModeSymlink`，`EvalSymlinks` 也不会解析它。

## R-02 Windows 保留名与尾点尾空格的显式拒绝

**现状**：`/file.`、`/file ` 落到普通文件名分支，行为取决于目标文件是否存在。

**需求**：在路径规范化阶段显式处理 Windows 语义——

- 尾点、尾空格：拒绝（400）或规范化后重新查一次
- 保留设备名：`CON`、`PRN`、`AUX`、`NUL`、`COM1`–`COM9`、`LPT1`–`LPT9`
  （含带扩展名的形式，如 `CON.txt`）一律拒绝
- 仅在 `GOOS == "windows"` 时生效，其他平台不受影响

**验收**：`traversal-vectors.jsonl` 增加保留名与尾点向量，Windows 实跑。

**状态**：✅ 已实现。尾点/尾空格采用"规范化后重新查一次"：`staticPathInRoot`
在 Windows 上逐段裁剪尾点与尾空格，再 `Clean` 并重做前缀校验（裁剪可能
产生新的 `..`，如 `/.. `）。保留设备名 `CON`/`PRN`/`AUX`/`NUL`/`COM1`–`COM9`/
`LPT1`–`LPT9`（按第一个点判断，`CON.txt` 同样命中）直接拒绝。其他平台不变。
`traversal-vectors.jsonl` 新增 9 条 Windows 向量（新增 `os` 字段控制平台），
`TestWindowsPathSemantics` 精确断言裁剪结果与拒绝名单。

## R-03 未知域名的 TLS 兜底策略

**现状**：**不会崩溃**已确认——未知 SNI 与空 SNI 都回落到证书列表的第一张，
证书列表为空时返回错误而非 panic（`TestCertSetUnknownSNIFallsBack`、
`TestCertSetEmptyReturnsError`）。缺的是「显式指定兜底证书」这个配置能力，
以及回落时的日志。

**需求**：

- 提供 `tls.default_certificate` 配置项：未知 SNI 时用它完成握手。
  未配置时保持现状（回落第一张证书），但**记一条可检索的日志**
- 日志字段见 R-10（`reject_reason` 取 `unknown_sni`）

**验收**：配置了默认证书时，未知 SNI 返回该证书；未配置时回落第一张且日志
可查。

**状态**：✅ 已实现。新增顶层 `tls.default_certificate`（`certificate` +
`key` 一对路径，相对路径按配置目录解析），校验阶段要求成对且文件存在。
`certSet` 增加 `defaultCert`，未知 SNI（含空 SNI）优先返回它，命中的域名
行为不变；未配置时仍回落第一张，并输出
`reject_reason=unknown_sni`、`sni=<name>` 的 Warn 日志。配置了兜底证书后
不再告警（属于预期流量）。`reload` 会重新加载兜底证书；读取失败只告警并
回落到第一张，不会让 TLS 端口起不来。

## R-04 通配符证书的匹配规则

**状态**：✅ 已实现。`certSet.get` 现在收集全部同域名命中的证书，用
`ClientHelloInfo.SupportsCertificate` 按客户端能力（`signature_algorithms`、
`supported_groups` 等）挑兼容的那张，缺扩展或全不兼容时退回第一张；
`TestCertSetSameNamePicksByClientCapabilities` 覆盖 RSA / ECDSA 两种客户端。
匹配规则已写进 `docs/usage.md` 第 7 章。

**原现状**：前两条**实现已正确**（走标准库 `VerifyHostname`，已由
`TestCertSetWildcardMatchesOneLabelOnly` 与
`TestCertSetNormalizesCaseAndTrailingDot` 锁定）；缺的是文档说明，以及
「同名多证书按客户端能力选择」这条。

**需求**：

- `*.example.com` 匹配 `a.example.com`，**不**匹配 `a.b.example.com`（已实现）
- 尾点 `example.com.` 与大小写差异归一到同一张证书（已实现）
- 同名多证书（RSA + ECDSA）按客户端 `signature_algorithms` 选择（已实现）
- 把上述规则写进 `docs/usage.md` 的 TLS 章节（已实现）

**验收**：多证书场景补一条用例；文档段落与实现一致。

## R-05 代理的 `X-Forwarded-For` 策略

**需求**：

- 默认**追加**客户端地址，绝不信任客户端自带的 `X-Forwarded-For`
- 提供 `proxy.forwarded_for.max_hops`：超过后停止追加并记日志
- 提供「信任代理前缀」配置（`trusted_proxies`）：只有来自这些地址的请求
  才保留其 `X-Forwarded-For`
- `Host` 头：提供 `preserve_host` 开关，默认透传

**验收**：`forwarded-chain.jsonl` 覆盖「无 XFF / 客户端伪造 / 多层代理 /
超跳数」四种输入。

**状态**：✅ 已实现。顶层 `proxy` 段全局生效，构建期拷进每个
`listener.ListenCfg`。`X-Forwarded-For` 只在直连对端命中 `trusted_proxies`
时保留入站链并追加对端地址，否则整条丢弃、只写真实对端；链长达到
`forwarded_for.max_hops`（0 表示不限）后停止追加并输出含 `hops` /
`max_hops` 的 Warn。解析阶段的 `$proxy_add_x_forwarded_for` 与转发阶段共用
同一策略，`proxy_set_header` 的显式配置仍可覆盖。`preserve_host=false`
时 Host 改写为上游 `host:port`。CLI 提供 `config proxy show` /
`config proxy set`，桌面端设置页有对应三项，全部回默认值时删除整段。
`modules/proxy/testdata/forwarded-chain.jsonl` 覆盖四类输入。

## R-06 hop-by-hop 头的剥离

**需求**：转发前无条件剥掉 `Connection`、`Keep-Alive`、`Proxy-Authenticate`、
`Proxy-Authorization`、`TE`、`Trailer`、`Transfer-Encoding`、`Upgrade`，
以及 `Connection` 里点名的扩展头。

**验收**：`hop-by-hop-headers.json` 逐项核对上游收到的头。

**状态**：✅ 已实现。`modules/proxy/hopheaders.go` 在 `ChangeHead` 里先
删除固定清单与 `Connection` 点名的扩展头，再应用 `proxy_set_header`；
额外覆盖非标准的 `Proxy-Connection`。三个必要的重建：客户端要求关闭
（HTTP/1.0 或 `Connection: close`）时给上游补 `Connection: close`，
否则代理按「读到连接结束」读取响应会等到超时；HTTP/1.0 的 keep-alive
客户端补回 `Connection: keep-alive`；websocket 升级时重建
`Connection: Upgrade` 与 `Upgrade`，其余 token 仍剥离。
`TestHopByHopHeaderCases` 读取 `testdata/hop-by-hop-headers.json` 的七类
输入，逐项断言上游实际收到的头。

## R-07 压缩协商的完整语义

**现状**：`modules/compress` 提供字节压缩辅助，协商与跳过规则集中在
`modules/compress/policy.go`，静态与代理共用。

**需求**：

- 按 q 值协商：`gzip;q=0` 必须不压，`q` 不同取高者，相同按服务端偏好定序
- `*` 通配符：显式未命中时回落到 `*` 的 q 值；显式 `q=0` 优先于 `*`
- 响应已带 `Content-Encoding` 时跳过（代理响应路径）
- `HEAD` 不产生 body，且 `Content-Length` 与 GET 一致
- Range 请求跳过压缩（压缩后 Range 语义失效）
- 已压缩类型（图片、zip、woff2、mp4）跳过，列表可配置
- 补 `Vary: Accept-Encoding`；压缩后移除或改写 `Content-Length`
- 提供最小压缩阈值（`min_length`），小响应不压

**验收**：`accept-encoding.jsonl` 覆盖上述七条，每条一个用例。

**状态**：✅ 已实现。协商与跳过规则抽到 `modules/compress/policy.go`，
静态与代理共用同一套策略：

- `Wanted` 按 q 值取高者，q 相同按服务端偏好 br > gzip；`q=0` 拒绝，
  未显式列出的编码回落到 `*` 的 q。
- `Skip` 内置已压缩类型清单（jpeg/png/gif/webp/avif、mp4/webm/quicktime、
  mp3/ogg/aac、zip/gzip/7z/rar/bz2、woff/woff2 等）；`zip_skip_types` 非空时
  **整体替换**该清单。规则支持精确类型、`image/*`、`image/p*` 与 `*/*`。
- 静态层在 `zip_min_length` 不足、命中跳过类型或带 Range 时流式原样返回；
  压缩后 `Content-Length` 改用压缩体长度。
- 代理层在 `modules/proxy/compress.go` 压缩上游响应：跳过升级连接、HEAD、
  已有 `Content-Encoding`、204/206/304、`Content-Range`、小于阈值与命中
  跳过类型；压缩后移除 `Transfer-Encoding` 并改写 `Content-Length`。
  缓存先存未压缩正文，命中时按各请求的 `Accept-Encoding` 重新协商。
- 配置项贯通 schema → loader → model → `listener.ListenCfg`，并同步
  `site update --zip-min-length` / `--zip-skip-types` 与桌面端站点编辑；
  `site add` 新建站点默认写入 br+gzip、1 KB 阈值与整类跳过清单（local 与
  proxy 都适用），可用同名 flag 覆盖，`--zip=` 关闭。
- `TestAcceptEncodingCases` 读取 `modules/static/testdata/accept-encoding.jsonl`
  的十个用例；`TestEncodingWantedVectors` 覆盖 27 条协商向量；
  `TestProxyCompressResponse` 覆盖代理侧的压缩、跳过与头部改写。

## R-08 连接与超时的边界行为

**需求**：

- `Expect: 100-continue`：先回 `100`，再读 body
- keep-alive 复用连接时，请求间不残留状态（cookie、认证、缓存键）
- header 超限 → 431；body 超限 → 413；两者都不 panic
- 配置项沿用既有的 `max_header_size`、`max_body_size`

**验收**：`runtime/listener` 新增一组，`-race` 下跑。

**状态**：✅ 已实现。

- `Expect: 100-continue`：HTTP/1.1 请求在读正文前先写 `100 Continue`；声明的
  Content-Length 已超限时不发 100，直接 413；列表形式（`100-continue, x`）按包含关系识别；
  其它无法满足的期望回 417；HTTP/1.0 按 RFC 忽略。期望在本机满足后从转发头里移除。
- keep-alive 复用：请求侧 `Request.Flush` 清 Headers/Query/Body，响应侧重建
  `Response`，并复位 `OriginPath` / `PathLocation` / `Upgrade` / `ProxyConnInit` /
  `CircleData`。静态层直接往 `rr.Res.Headers` 写头，不重建会把上一响应的
  `Content-Encoding`、`Set-Cookie` 带给下一个响应。
- header 超限回 431：既覆盖「整段头一次到齐」（按 `\r\n\r\n` 计算头段长度），
  也覆盖「分片到达」（累计字节超限）；畸形头仍回 400。body 超限回 413。
- HTTP/2 对齐：`max_header_size` 写入 `http.Server.MaxHeaderBytes`（协议栈据此
  设置 `SETTINGS_MAX_HEADER_LIST_SIZE`），应用层再按 RFC 7540 的字段开销判一次 431；
  `max_body_size` 对声明长度与流式正文都生效（413）；未知期望回 417。
- 配置沿用既有 `http.servers_limit.max_header_size` / `max_body_size`，默认
  4096 / 524288 字节。
- 用例：`runtime/events/connection_limits_test.go`（单测）、`connection_e2e_test.go`
  （真实 TCP：100、431、413、keep-alive，以及客户端 Expect → 代理 → 上游的完整链路）、
  `h2_helpers_test.go`（h2 431/413/417）；`-race` 下通过。
- 超时可配置：`http.servers_limit.read_timeout` / `write_timeout` / `body_timeout`
  （单位秒，0 用默认 30 / 30 / 1），负数在加载期报错；HTTP/2 的读头超时同样读
  `read_timeout`。代理到上游的整体超时由 `proxy.upstream_timeout` 控制（默认 20s）；
  websocket / tcp 隧道的空闲断开由 `proxy.tunnel_idle_timeout` 控制（默认 0，不自动断开）。
- 验收报告：[r08-acceptance.md](../../test/reports/r08-acceptance.md)。

## R-09 reload 的原子性与可见性

**现状**：第一项**已实现并有测试锁定**——`ReloadFile` 先校验、再解析、
最后 `Store`，任一步失败都不触碰 `Current()`；事务层另有
`TestApplyValidationFails`、`TestApplyReloadFailsRollsBack`、
`TestApplyCrashBetweenPhases` 覆盖回滚与崩溃窗口。剩余的是「在途请求怎么
处理」与「并发 reload 串行化」两项目前没有明确语义。

**需求**：

- reload 失败 → 旧配置继续生效，错误在 CLI 与桌面端可见（已完成）
- reload 期间的在途请求：处理完再切换（优雅），或立即中断并在日志标注
- 并发 reload 串行化，后到的等待或直接返回「正在重载」

**验收**：构造「新配置非法」的场景，断言旧配置仍在服务；`-race` 下并发
触发 10 次 reload，无 panic、无状态错乱。

**状态**：✅ 已实现。`server.Reload` 与 `config.LoadFile`/`ReloadFile` 各持
互斥锁，后到的 reload 等待前一次完成；在途请求按端口变化分两种：common 端口
热更新时用旧快照处理完，移除/重建端口时终止并在日志带 `port` +
`in_flight_requests` 字段。`TestReloadInvalidKeepsPreviousConfig` 锁定非法配置
保留旧快照；`TestConcurrentReloadSerialized`（loader）与
`TestConcurrentReloadsAreSerialized`（server，-race）并发 10 次 reload；
`TestInFlightRequestsTrackedDuringHandler` 与
`TestReloadLogsInFlightRequestsOnRemovedPort` 锁定在途请求计数与日志。
验收报告：[r09-acceptance.md](../../test/reports/r09-acceptance.md)；
实现细节见 [4-reload.md](../../feature/4-reload.md)。

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

**状态**：✅ 已实现。统一字段由 `infra/logging.RejectWarn` / `RejectDebug`
写入安全日志：路径穿越（`path_traversal` / `static_root`）、隐藏路径
（`hidden_path` / `hidden_path_segment`）、超限（`header_too_large` /
`max_header_size`、`body_too_large` / `max_body_size`，h1 与 h2 共用）、
未知 SNI（`unknown_sni` / `tls.sni`）、压缩跳过（`compression_skipped` /
`zip_min_length`、`zip_skip_types`，debug 级）。日志只保留客户端 IP 与规则名，
不记原始请求路径。`logs query` 新增 `--reject-reason`，返回记录带
`reject_reason`、`rule`、`remote_ip`；桌面端日志查询同步该参数。
真实链路 E2E：`TestRejectLogE2E`（`FASTHTTPS_E2E_REJECT_LOG=1`）起真实进程触发
穿越/超限/压缩跳过/未知 SNI，再按 `reject_reason` 查回记录；已加入 CI。

## R-11 header 超限返回 431

**现状**：header 超过 `max_header_size` 时回 **400 Bad Request**。

**需求**：改成 **431 Request Header Fields Too Large**（RFC 7231 §6.5.11）。
400 不违规（原文是 MAY），但客户端拿不到可区分的信号——是「请求语法错了、
改代码」还是「这条头太长、改配置或精简 cookie」，处理方式完全不同。

**验收**：构造超长 header，断言状态行是 431；`max_header_size` 配置项在
`docs/usage.md` 的限额表里补一行。

**状态**：✅ 已实现（随 R-08）。新增 `DefaultRequestHeaderFieldsTooLarge`，
`events.parseRequest` 在头完整与分片两条路径上都按 `headerLen` 判 431；
`docs/usage.md` 5.2 已补 `max_header_size` / `max_body_size` 限额表。

## R-12 空 body 上游响应的连接复用

**现状**：上游响应既没有 `Content-Length` 也没有 `Transfer-Encoding: chunked`
时，代理按「读到连接关闭」处理（`ReadKeepAlive.tryToParse` 返回 −4），
然后放弃复用这条上游连接。

**风险边界**：这是**保守但正确**的行为，不会挂死，只是浪费连接。多数合规
上游会带 `Content-Length: 0`，不受影响；只有「HTTP/1.0 风格、靠关闭连接
表示响应结束」的老服务端会命中。

**需求**：如果要对齐主流实现，可以在「响应是 1xx/204/304 或 HEAD 请求」时
直接判定 body 为空、标记连接可复用，而不必等关闭。

**验收**：构造 `HTTP/1.1 204 No Content` 且无 `Content-Length` 的上游响应，
断言连接被复用（第二次请求走同一条连接）。

## R-13 请求体限额与 100-continue 配合

**现状**：超过 `max_body_size` 时回 413（本轮已修）；但客户端如果带了
`Expect: 100-continue`，目前仍会先读完 body 再判断。

**需求**：解析出 `Content-Length` 后先与限额比对，超限就直接回 413（不发送
`100 Continue`），让客户端在上传前就知道被拒；没有 `Content-Length` 的
chunked 上传仍按累计长度在读取过程中拒绝。

**验收**：`Expect: 100-continue` + 声明长度超限时，响应里不出现 `100 Continue`，
只有 413。

**状态**：✅ 已实现（随 R-08）。`TestExpectContinueOversizedDeclaredBodyGets413`
断言响应只有 413、没有 100 Continue。

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
| R-11 header 超限返回 431 | P2 | 现行 400 合规，只是客户端拿不到可区分信号 |
| R-12 空 body 上游连接复用 | P2 | 纯性能取舍，现状保守但正确 |
| R-13 限额与 100-continue 配合 | P2 | 省一次无用的上传，非正确性问题 |

## 不做的事

- 不追求覆盖率数字。缺的是**针对性**，不是总量。
- 不为「可能永远不出现」的场景做能力（例如 HTTP/0.9 兼容开关）。
- 不在本计划里改架构（`--config` 提升为 persistent flag 之类另开议题）。

## 推进方式

每个需求按同一节奏：

1. 先在 `testdata/` 写用例表，跑出当前的失败或记录现状
2. 在 [缺陷核验表](../../test/reports/edge-case-coverage.md) 里把对应行从 ❓ 改成 ✅ 或 ⚠️
3. 需要改代码的单独一个提交，用例作为回归
4. 只补用例不改代码的，标注「锁定现状」并说明为什么可以接受

## 相关文档

- 缺陷核验记录见 [edge-case-coverage.md](../../test/reports/edge-case-coverage.md)
- 按输入域组织的用例集见 [boundary-cases.md](../../test/cases/boundary-cases.md)
- 覆盖率评估见 [coverage-assessment.md](../../test/reports/coverage-assessment.md)
