# 4. 缺陷反馈核验

这份文档记录的是**问题反馈的核对结果**：收到「某某场景会出问题」的反馈后，
逐条确认本项目是否存在同样缺陷、证据是什么、有没有修。

它不是功能计划（那在 [edge-case-hardening-plan.md](../../develop/plans/edge-case-hardening-plan.md)），
也不是覆盖率报告（那在 [coverage-assessment.md](coverage-assessment.md)）。

## 4.1 状态取值

| 标记 | 含义 |
| --- | --- |
| ✅ 无此问题 | 已用自动化用例验证，当前实现不受影响 |
| 🔧 已修复 | 曾经存在，已改代码，用例锁定为回归 |
| ⚠️ 存在待修 | 复现了，尚未改代码，用例先标注跳过并记原因 |
| ❓ 未验证 | 需要特定环境（Windows / 多域名证书 / 高并发），待补 |

## 4.2 核验表

| 编号 | 问题现象 | 本项目状态 | 证据 | 处理 |
| --- | --- | --- | --- | --- |
| B-01 | 请求路径用 `../` 跳出文档根，读到任意文件 | ✅ 无此问题 | `TestStaticPathInRoot`、`TestGetResBytesRejectsTraversal` | 已有防护，无需改 |
| B-02 | 用 `%2e%2e`、`..%2f` 等编码绕过上一条的检查 | ✅ 无此问题 | `TestStaticPathTraversalVectors`（本轮新增） | 解码发生在静态层之前，编码形式到该层已是普通字符 |
| B-03 | 用 `%5c`（编码反斜杠）在 Windows 上再绕过一次 | ✅ 无此问题 | 同上 | 编码形式落到普通文件名分支；裸 `\..\..\` 被 `Clean` 当分隔符拒绝 |
| B-04 | 四点是合法文件名，被误判成 `..` | ✅ 无此问题 | 同上（`/....//etc/passwd`） | 按普通名处理，不误拒 |
| B-05 | 尾点 / 尾空格被 Windows 静默截断，读到别的文件 | 🔧 已修复 | `TestWindowsPathSemantics`、`traversal-vectors.jsonl` 的 Windows 向量 | R-02：Windows 上逐段裁剪尾点 / 尾空格后重新 `Clean` 并复查前缀；`CON`/`COM1` 等保留名一律拒绝 |
| B-06 | 绝对盘符路径（`/C:/Windows/win.ini`）跳出根目录 | ✅ 无此问题 | 同上 | `Clean` 后仍在 root 内，最终 404 |
| B-07 | 重复斜杠（`//etc/passwd`）被当成绝对路径 | ✅ 无此问题 | 同上 | 归一化后仍在 root 内 |
| B-08 | 文档根内的符号链接指向外部，被当普通文件读出 | 🔧 已修复 | `TestSymlinkPolicyRejectEscapingLink` 等 6 个策略用例、`TestLoadFileRejectsSymlinkRootByDefault`、`TestStaticSymlinkEscape` | R-01：新增 `location.symlink_policy`，默认 `check-at-load` 在加载期拒绝 root 下的链接；`reject` 在请求期逐段拒绝（含 Windows junction） |
| B-09 | 未配置证书的域名发起 TLS 握手，进程崩溃 | ✅ 无此问题 | `TestCertSetUnknownSNIFallsBack`、`TestCertSetEmptyReturnsError` | 未知 SNI 与空 SNI 都回落到第一张证书；证书列表为空时返回错误而非 panic |
| B-10 | 通配符证书 `*.example.com` 匹配了 `a.b.example.com` | ✅ 无此问题 | `TestCertSetWildcardMatchesOneLabelOnly` | 走标准库 `VerifyHostname`，通配符只跨一层 |
| B-10b | 大小写或结尾的点导致选不到证书 | ✅ 无此问题 | `TestCertSetNormalizesCaseAndTrailingDot` | `EXACT.EXAMPLE`、`exact.example.` 均命中同一张 |
| B-11 | 客户端伪造 `X-Forwarded-For`，上游看到假链路 | 🔧 已修复 | `TestForwardedChainCases`、`TestForwardedForChainPolicy` | R-05：默认丢弃不受信对端自带的 XFF、只写真实来源；仅来自 `trusted_proxies` 的请求保留入站链并追加 |
| B-12 | hop-by-hop 头（`Connection`、`TE`、`Proxy-Authorization`）被原样转发给上游 | 🔧 已修复 | `TestHopByHopHeaderCases`、`hop-by-hop-headers.json` | R-06：转发前剥离固定清单与 `Connection` 点名的扩展头；客户端要求关闭时给上游补 `Connection: close`，websocket 升级重建 `Connection: Upgrade` / `Upgrade` |
| B-13 | `Accept-Encoding: gzip;q=0` 仍被压缩 | ✅ 无此问题 | `TestEncodingWantedVectors`（27 条向量）、`TestStaticQZeroIsNotCompressed` | `request.QValue` 解析 q 值，q=0 判为拒绝；端到端确认响应不带 `Content-Encoding` |
| B-14 | 上游已带 `Content-Encoding` 时被二次压缩 | ✅ 无此问题 | `TestProxyCompressResponse` 的「上游已压缩不二次压缩」 | 代理压缩前检查 `Content-Encoding`，已有编码时原样透传；静态层不存在上游响应 |
| B-15 | keep-alive 复用连接时，请求间状态泄漏 | ✅ 无此问题 | `TestKeepAliveRequestIsFullyReset`、`TestFlushClearsAuxiliaryFields`、`TestKeepAliveResetsRequestAndResponse` | 请求侧 `Request.Flush` 清 Header/Query/Body；R-08 补上响应侧：重建 `Response` 并复位 `OriginPath`/`PathLocation`/`Upgrade`，上一响应的 `Content-Encoding`/`Set-Cookie` 不再串到下一个响应 |
| B-16 | reload 失败后旧配置失效，服务处于半死状态 | ✅ 无此问题 | `TestReloadInvalidKeepsPreviousConfig`、`TestReloadMissingFileKeepsPreviousConfig`；`management/txn` 的 `TestApplyValidationFails`、`TestApplyReloadFailsRollsBack`、`TestApplyCrashBetweenPhases` | `ReloadFile` 先校验再解析最后 `Store`，任一步失败都不触碰 `Current()` |
| B-17 | `Accept-Encoding: *` 不生效，客户端声明接受任意编码却拿到未压缩响应 | 🔧 已修复 | `TestEncodingWantedVectors`（3 条通配符向量） | 原先只做显式 token 匹配；本轮 `encodingWanted` 增加回落：未显式声明的编码按 `*` 的 q 值决定，显式 `q=0` 仍优先于 `*` |
| B-18 | 压缩后 `Content-Length` 仍是原文件大小，客户端按错误的长度读正文 | 🔧 已修复 | `TestAcceptEncodingCases` 的 `length-rewritten` | R-07：压缩后 `Content-Length` 改用 `int64(len(payload))`；用例逐字节核对声明长度与实际正文 |
| B-19 | 图片、字体、zip 等已压缩类型被二次压缩；几字节的小响应也压 | 🔧 已修复 | `TestAcceptEncodingCases` 的 `compressed-type-skipped`、`min-length` | R-07：内置已压缩类型清单 + `zip_min_length`；`zip_skip_types` 非空时整体替换清单 |
| B-20 | `HEAD` 返回正文，或 `HEAD` 与 GET 的 `Content-Length` 不一致；Range 请求被压缩 | ✅ 无此问题 | `TestAcceptEncodingCases` 的 `head-length-matches-get`、`range-skips-compression`、`TestStaticHeadOmitsBody` | HEAD 只写响应头；压缩路径对 HEAD 同样先算出压缩体，长度与 GET 一致；Range 原本就不压缩 |

## 4.3 本轮新增的用例

### 4.3.1 SNI 与通配符（B-09、B-10）

`runtime/listener/certs_edge_test.go`，四组用例，全部直接驱动 `certSet.get`：

| 用例 | 断言 |
| --- | --- |
| `TestCertSetUnknownSNIFallsBack` | 未知 SNI、空 SNI 都回落第一张证书，不报错 |
| `TestCertSetEmptyReturnsError` | 空列表与 nil 接收者都只返回错误，不 panic |
| `TestCertSetWildcardMatchesOneLabelOnly` | `a.example.com` 命中通配符；`a.b.example.com` **不**命中，回落第一张 |
| `TestCertSetNormalizesCaseAndTrailingDot` | `EXACT.EXAMPLE`、`Exact.Example.` 与 `exact.example` 命中同一张 |

断言看的是选中证书的 CN，不是「有没有返回证书」——因为跨层匹配会回落到
第一张，只看「有返回」会把错误行为当成通过。

### 4.3.2 代理头处理（B-11、B-12）

`modules/proxy/hopheaders_test.go`，起一个回环上游，把收到的请求头
回显出来：

| 用例 | 结论 |
| --- | --- |
| `TestHopByHopHeaderCases` | 读 `testdata/hop-by-hop-headers.json` 七类输入：单跳头与 `Connection` 点名的扩展头**都不到达上游**；端到端头保留；`Connection: close`、HTTP/1.0（含 keep-alive）补对应的连接头；websocket 重建升级对 |
| `TestForwardedChainCases` | 读 `testdata/forwarded-chain.jsonl`：无 XFF、客户端伪造、多层代理、超跳数四种输入的值符合 R-05 策略 |

hop-by-hop 一项由 R-06 从「原样转发」改成「转发前剥离」，用例从锁定
现状改为反向断言。XFF 一项已由 R-05 从「不追加」改成「按可信代理
策略追加」，核心策略另有 `core/forwarded_test.go` 的
`TestForwardedForChainPolicy` 逐条锁定。

### 4.3.4 压缩协商（B-13、B-17 ~ B-20）

`modules/static/testdata/accept-encoding.jsonl` 十条用例，`TestAcceptEncodingCases`
逐条驱动真实的 `getResBytes` 响应链路：

| 用例 | 断言 |
| --- | --- |
| `q-zero` | `gzip;q=0` 明确拒绝，响应不带 `Content-Encoding` |
| `wildcard-fallback` | 只给 `*` 时回落到 `*` 的 q 值并压缩 |
| `head-length-matches-get` | HEAD 无正文，`Content-Length` / `Content-Encoding` 与 GET 一致 |
| `range-skips-compression` | `206` 且不压缩，正文是请求的那一段 |
| `compressed-type-skipped` | `logo.png` 默认跳过，`Content-Type: image/png` |
| `length-rewritten` | 压缩后声明的 `Content-Length` 等于压缩体字节数，并带 `Vary: Accept-Encoding` |
| `min-length` | 小于 `zip_min_length` 的响应不压 |
| `skip-wildcard` | `application/*` 命中整类通配 |
| `skip-partial-wildcard` | `application/j*` 命中部分通配 |
| `skip-global-wildcard` | `*/*` 命中全局通配 |

「响应已带 `Content-Encoding` 时跳过」由代理压缩路径覆盖（B-14）：
`modules/proxy/compress.go` 在压缩前检查该头，已有编码时原样透传。

代理响应压缩另由 `modules/proxy/compress_test.go` 的 `TestProxyCompressResponse`
覆盖：q 值择优、已压缩类型与部分通配跳过、阈值、HEAD、206、websocket、
chunked 改为长度分帧。

### 4.3.5 静态路径（B-01 ~ B-07）

`modules/static/testdata/traversal-vectors.jsonl` —— 11 条向量：

```jsonl
{"path":"/../etc/passwd","want":"reject","note":"裸 .. 逃逸"}
{"path":"/a/..%2f..%2fetc%2fpasswd","want":"name","note":"百分号编码的斜杠"}
{"path":"/a\\..\\..\\etc\\passwd","want":"reject","note":"裸反斜杠"}
{"path":"/file.","want":"name","note":"Windows 会截断尾点"}
...
```

`reject` 与 `name` 两种期望分开断言：安全性等价（都读不到 root 外），但对应
代码路径不同，回归时能立刻分辨是「校验生效」还是「解码行为变了」。

`modules/static/edge_cases_test.go` —— 遍历上面这张表；另有
`TestStaticSymlinkEscape` 覆盖 B-08（Windows 无符号链接权限时 Skip）。

## 4.4 本轮结论

```
PASS: TestStaticPathTraversalVectors      (11 个子用例)
SKIP: TestStaticSymlinkEscape             (Windows 无符号链接权限)
PASS: TestCertSetUnknownSNIFallsBack
PASS: TestCertSetEmptyReturnsError
PASS: TestCertSetWildcardMatchesOneLabelOnly
PASS: TestCertSetNormalizesCaseAndTrailingDot
PASS: TestHopByHopHeaderCases                  (7 条 hop-by-hop 输入)
PASS: TestForwardedChainCases                    (4 条 XFF 策略输入)
PASS: TestForwardedForChainPolicy                (受信/不受信/上限/0 不限)
PASS: TestEncodingWantedVectors                  (27 条协商向量)
PASS: TestAcceptEncodingCases                    (10 条压缩协商输入)
PASS: TestProxyCompressResponse                  (代理压缩/跳过/头部改写)
PASS: TestKeepAliveRequestIsFullyReset
PASS: TestFlushClearsAuxiliaryFields
PASS: TestReloadInvalidKeepsPreviousConfig
PASS: TestReloadMissingFileKeepsPreviousConfig
```

核验结果：**21 条核验项里，14 条确认无此问题，7 条已修复，待修项清零**。

本轮修掉的两处：

| 问题 | 改动 |
| --- | --- |
| `Accept-Encoding: *` 不生效（B-17） | `runtime/request` 拆出 `QValue`（返回 q 值与是否出现），`encodingWanted` 在显式未命中时回落到 `*`；显式 `q=0` 仍优先于 `*` |
| `Request.Flush` 漏清三个辅助字段（B-15 的一部分） | 补 `Encoding` / `HeaderLen` / `BodyLen`。三者当前没有读取方，但属于「随请求变化」的状态，留着等于给将来埋跨请求残留的坑 |
| 压缩后 `Content-Length` 用错长度（B-18） | `modules/static` 在 `!stream` 分支把 `payloadLen` 改成 `int64(len(payload))` |
| 已压缩类型与小响应仍被压（B-19） | 新增 `zip_min_length` / `zip_skip_types`，内置已压缩类型清单；配置后整体替换清单 |


- **URL 解码发生在静态层之前**：`%2e%2e` 到这一层已是普通字符，因此既不会
  被 `..` 检查拒绝，也读不到越界文件。这条链路是安全的，但依赖「上层先解码」
  这个前提——若将来在解析层改成延迟解码，需要重跑这组向量。
- **反斜杠**在 Windows 上被 `filepath.Clean` 当作分隔符，裸 `\..\..\` 会被
  正确拒绝；编码形式 `%5c` 不会，落到普通文件名分支。
- **B-08 符号链接**：已由 R-01 落地 `location.symlink_policy`，默认
  `check-at-load`（加载时拒绝 root 下的链接），可切 `reject`（请求期逐段
  拒绝任意链接，含 Windows junction）或 `allow`，
  见 [edge-case-hardening-plan.md](../../develop/plans/edge-case-hardening-plan.md)。
- **B-05 尾点尾空格**：已由 R-02 处理——Windows 上逐段裁剪尾点 / 尾空格后
  重新 `Clean` 并复查前缀（裁剪可能产生新的 `..`，如 `/.. `），存量保留设备名
  `CON`/`PRN`/`AUX`/`NUL`/`COM1`–`COM9`/`LPT1`–`LPT9` 直接拒绝；
  `TestWindowsPathSemantics` 逐条锁定，其他平台行为不变。

## 4.5 复跑方式

```bash
# 静态路径
go test ./modules/static/ -run 'TestStaticPathTraversalVectors|TestStaticSymlinkEscape' -v
# 符号链接策略（R-01）
go test ./config/ ./modules/static/ -run 'TestCheckAtLoadRejectsSymlink|TestSymlinkPolicy' -v
# SNI 与通配符
go test ./runtime/listener/ -run TestCertSet -v
# 代理头
go test ./modules/proxy/ ./core/ -run 'TestHopByHopHeaderCases|TestForwardedChainCases|TestForwardedForChainPolicy' -v
# 内容编码协商
go test ./modules/static/ -run 'TestEncodingWantedVectors|TestStaticQZero|TestAcceptEncodingCases' -v
# keep-alive 状态隔离
go test ./runtime/request/ -run 'TestKeepAliveRequestIsFullyReset|TestFlushClearsAuxiliaryFields' -v
# reload 原子性
go test ./config/ -run 'TestReloadInvalidKeepsPreviousConfig|TestReloadMissingFileKeepsPreviousConfig' -v
# 全量
go test ./... -count=1
```

B-05、B-08 需要在 Windows / Linux 各跑一次，结果才有意义（`EvalSymlinks`
与文件名截断都是平台相关行为）。

## 4.6 后续核验

当前收集到的反馈已全部核验完毕，没有 ❓ 项。

B-05、B-08 已分别由 R-02、R-01 处理，B-12 由 R-06 处理，B-18/B-19 由 R-07
处理，均有回归用例；当前核验表里没有待修项。

后续再收到新的问题反馈，按同样节奏处理：

1. 先在对应包的 `testdata/` 写用例表（一行一条「输入 → 期望」）
2. 跑出结论，在本表里按 ✅ / 🔧 / ⚠️ / ❓ 归档
3. 需要改代码的单独提交，用例作为回归；只锁定现状的，在用例注释里写明为什么可以接受
