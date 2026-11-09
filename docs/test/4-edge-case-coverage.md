# 4. 缺陷反馈核验

这份文档记录的是**问题反馈的核对结果**：收到「某某场景会出问题」的反馈后，
逐条确认本项目是否存在同样缺陷、证据是什么、有没有修。

它不是功能计划（那在 [edge-case-hardening-plan.md](../develop/plans/edge-case-hardening-plan.md)），
也不是覆盖率报告（那在 [0-coverage-assessment.md](0-coverage-assessment.md)）。

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
| B-05 | 尾点 / 尾空格被 Windows 静默截断，读到别的文件 | ⚠️ 存在待修 | 同上（`/file.`、`/file `） | 当前当普通名，结果取决于目标文件是否存在；需在 Windows 上单独验证并决定是否显式拒绝 |
| B-06 | 绝对盘符路径（`/C:/Windows/win.ini`）跳出根目录 | ✅ 无此问题 | 同上 | `Clean` 后仍在 root 内，最终 404 |
| B-07 | 重复斜杠（`//etc/passwd`）被当成绝对路径 | ✅ 无此问题 | 同上 | 归一化后仍在 root 内 |
| B-08 | 文档根内的符号链接指向外部，被当普通文件读出 | ⚠️ 存在待修 | `TestStaticSymlinkEscape`（Windows 上 Skip） | `staticPathInRoot` 只看字符串前缀，不 `EvalSymlinks`；需决定「拒绝」还是「配置期校验」 |
| B-09 | 未配置证书的域名发起 TLS 握手，进程崩溃 | ✅ 无此问题 | `TestCertSetUnknownSNIFallsBack`、`TestCertSetEmptyReturnsError` | 未知 SNI 与空 SNI 都回落到第一张证书；证书列表为空时返回错误而非 panic |
| B-10 | 通配符证书 `*.example.com` 匹配了 `a.b.example.com` | ✅ 无此问题 | `TestCertSetWildcardMatchesOneLabelOnly` | 走标准库 `VerifyHostname`，通配符只跨一层 |
| B-10b | 大小写或结尾的点导致选不到证书 | ✅ 无此问题 | `TestCertSetNormalizesCaseAndTrailingDot` | `EXACT.EXAMPLE`、`exact.example.` 均命中同一张 |
| B-11 | 客户端伪造 `X-Forwarded-For`，上游看到假链路 | ✅ 无此问题 | `TestProxyDoesNotAddXForwardedFor` | 代理不添加该头，伪造无从谈起；**代价是上游拿不到真实来源**，属功能缺失，见 R-05 |
| B-12 | hop-by-hop 头（`Connection`、`TE`、`Proxy-Authorization`）被原样转发给上游 | ⚠️ 存在待修 | `TestProxyForwardsHopByHopHeadersVerbatim` | 代理按字节转发，未做剥离。用例先锁定现状，等 R-06 实现后改成反向断言 |
| B-13 | `Accept-Encoding: gzip;q=0` 仍被压缩 | ✅ 无此问题 | `TestEncodingWantedVectors`（21 条向量）、`TestStaticQZeroIsNotCompressed` | `request.QValue` 解析 q 值，q=0 判为拒绝；端到端确认响应不带 `Content-Encoding` |
| B-14 | 上游已带 `Content-Encoding` 时被二次压缩 | ✅ 不适用 | — | 压缩只发生在「读静态文件」这一步，不存在响应中间件链路，没有二次压缩的入口 |
| B-15 | keep-alive 复用连接时，请求间状态泄漏 | ✅ 无此问题 | `TestKeepAliveRequestIsFullyReset`、`TestFlushClearsAuxiliaryFields` | `Request.Flush` 清 Header/Query/Body；本轮补齐 `Encoding` / `HeaderLen` / `BodyLen` 三个此前未清的辅助字段 |
| B-16 | reload 失败后旧配置失效，服务处于半死状态 | ✅ 无此问题 | `TestReloadInvalidKeepsPreviousConfig`、`TestReloadMissingFileKeepsPreviousConfig`；`management/txn` 的 `TestApplyValidationFails`、`TestApplyReloadFailsRollsBack`、`TestApplyCrashBetweenPhases` | `ReloadFile` 先校验再解析最后 `Store`，任一步失败都不触碰 `Current()` |
| B-17 | `Accept-Encoding: *` 不生效，客户端声明接受任意编码却拿到未压缩响应 | 🔧 已修复 | `TestEncodingWantedVectors`（3 条通配符向量） | 原先只做显式 token 匹配；本轮 `encodingWanted` 增加回落：未显式声明的编码按 `*` 的 q 值决定，显式 `q=0` 仍优先于 `*` |

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

`modules/proxy/forwarded_headers_test.go`，起一个回环上游，把收到的请求头
回显出来：

| 用例 | 结论 |
| --- | --- |
| `TestProxyForwardsHopByHopHeadersVerbatim` | `Connection`、`X-Hop`、`Proxy-Authorization`、`TE` **全部原样到达上游** |
| `TestProxyDoesNotAddXForwardedFor` | 客户端自带的 XFF 原样透传，代理不追加真实来源 |

这两个是「锁定现状」的用例，不是期望行为。R-06 实现后第一个要改成反向
断言；R-05 实现后第二个要改成断言「追加了真实地址」。

### 4.3.3 静态路径（B-01 ~ B-07）

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
PASS: TestProxyForwardsHopByHopHeadersVerbatim   (锁定现状：未剥离)
PASS: TestProxyDoesNotAddXForwardedFor           (锁定现状：未追加)
PASS: TestEncodingWantedVectors                  (21 条协商向量)
PASS: TestStaticQZeroIsNotCompressed
PASS: TestStaticQZeroFallsBackToOtherEncoding
PASS: TestKeepAliveRequestIsFullyReset
PASS: TestFlushClearsAuxiliaryFields
PASS: TestReloadInvalidKeepsPreviousConfig
PASS: TestReloadMissingFileKeepsPreviousConfig
```

核验结果：**17 条反馈里，14 条确认无此问题，2 条存在待修，1 条已修复，0 条未验证**。

本轮修掉的两处：

| 问题 | 改动 |
| --- | --- |
| `Accept-Encoding: *` 不生效（B-17） | `runtime/request` 拆出 `QValue`（返回 q 值与是否出现），`encodingWanted` 在显式未命中时回落到 `*`；显式 `q=0` 仍优先于 `*` |
| `Request.Flush` 漏清三个辅助字段（B-15 的一部分） | 补 `Encoding` / `HeaderLen` / `BodyLen`。三者当前没有读取方，但属于「随请求变化」的状态，留着等于给将来埋跨请求残留的坑 |


- **URL 解码发生在静态层之前**：`%2e%2e` 到这一层已是普通字符，因此既不会
  被 `..` 检查拒绝，也读不到越界文件。这条链路是安全的，但依赖「上层先解码」
  这个前提——若将来在解析层改成延迟解码，需要重跑这组向量。
- **反斜杠**在 Windows 上被 `filepath.Clean` 当作分隔符，裸 `\..\..\` 会被
  正确拒绝；编码形式 `%5c` 不会，落到普通文件名分支。
- **两个待修项**（B-05 尾点尾空格、B-08 符号链接）都涉及「配置者自己放进
  root 的东西」或「Windows 文件系统行为」，没有明确的产品判断，先记录，
  处理方式见 [计划里的 R-01 / R-02](../develop/plans/edge-case-hardening-plan.md)。

## 4.5 复跑方式

```bash
# 静态路径
go test ./modules/static/ -run 'TestStaticPathTraversalVectors|TestStaticSymlinkEscape' -v
# SNI 与通配符
go test ./runtime/listener/ -run TestCertSet -v
# 代理头
go test ./modules/proxy/ -run 'TestProxyForwardsHopByHop|TestProxyDoesNotAddXForwardedFor' -v
# 内容编码协商
go test ./modules/static/ -run 'TestEncodingWantedVectors|TestStaticQZero' -v
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

两个 ⚠️ 待修（B-05 Windows 尾点尾空格、B-08 符号链接逃逸）需要先定产品判断：
「拒绝」还是「配置期校验」，方案见
[edge-case-hardening-plan.md](../develop/plans/edge-case-hardening-plan.md) 的 R-01 / R-02。

后续再收到新的问题反馈，按同样节奏处理：

1. 先在对应包的 `testdata/` 写用例表（一行一条「输入 → 期望」）
2. 跑出结论，在本表里按 ✅ / 🔧 / ⚠️ / ❓ 归档
3. 需要改代码的单独提交，用例作为回归；只锁定现状的，在用例注释里写明为什么可以接受
