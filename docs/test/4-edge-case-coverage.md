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
| B-09 | 未配置证书的域名发起 TLS 握手，进程崩溃 | ❓ 未验证 | 无用例 | `modules/certmanager` 缺「SNI 不在覆盖范围」的用例，见 R-03 |
| B-10 | 通配符证书 `*.example.com` 匹配了 `a.b.example.com` | ❓ 未验证 | 无用例 | 需按「通配符只跨一层」补用例，见 R-04 |
| B-11 | 客户端伪造 `X-Forwarded-For`，上游看到假链路 | ❓ 未验证 | 无用例 | 需确认是追加还是覆盖，见 R-05 |
| B-12 | hop-by-hop 头（`Connection`、`TE`、`Upgrade`）被原样转发给上游 | ❓ 未验证 | 无用例 | 见 R-06 |
| B-13 | `Accept-Encoding: gzip;q=0` 仍被压缩 | ❓ 未验证 | 无用例 | `modules/compress` 现有测试仅 39 行，见 R-07 |
| B-14 | 上游已带 `Content-Encoding` 时被二次压缩 | ❓ 未验证 | 无用例 | 同上 |
| B-15 | keep-alive 复用连接时，请求间状态泄漏 | ❓ 未验证 | 无用例 | 需 `-race` 下验证，见 R-09 |
| B-16 | reload 失败后旧配置失效，服务处于半死状态 | ❓ 未验证 | 无用例 | 见 R-10 |

## 4.3 本轮新增的用例

`modules/static/testdata/traversal-vectors.jsonl` —— 11 条向量，覆盖 B-01 ~ B-07：

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
PASS: TestStaticPathTraversalVectors  (11 个子用例)
SKIP: TestStaticSymlinkEscape         (Windows 无符号链接权限)
ok    fast-https/modules/static
```

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
go test ./modules/static/ -run 'TestStaticPathTraversalVectors|TestStaticSymlinkEscape' -v
go test ./modules/static/ -count=1
go test ./... -count=1
```

B-05、B-08 需要在 Windows / Linux 各跑一次，结果才有意义（`EvalSymlinks`
与文件名截断都是平台相关行为）。

## 4.6 后续核验

B-09 ~ B-16 八条还没验证，需要按 `modules/certmanager`、`modules/proxy`、
`modules/compress`、`runtime/listener` 分四批补。

每批的做法一致：先在 `testdata/` 写用例表 → 跑出结论 → 在本表里把 ❓ 改成
✅ 或 ⚠️ → 需要改代码的单独提交，用例作为回归。
