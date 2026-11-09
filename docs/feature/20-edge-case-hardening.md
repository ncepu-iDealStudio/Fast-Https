# 边界加固需求清单

> 来源：对标 [caddyserver/caddy](https://github.com/caddyserver/caddy) 社区反馈的高频问题，
> 对照本项目实现逐项排查后形成的待办。
> 测试口径与已完成部分见 `docs/test/edge-case-coverage.md`。

## 优先级说明

- **P0**：影响安全或有崩溃风险，上线前应闭环
- **P1**：语义不一致或平台行为差异，会导致「本地正常、线上异常」
- **P2**：可观测性与体验，允许排期靠后

---

## P0-1 静态文件的符号链接逃逸

**现状**：`modules/static` 的 `staticPathInRoot` 只比较 `filepath.Clean` 后的
字符串前缀，不展开符号链接。root 内若存在指向外部的链接，请求
`/link/secret.txt` 会通过校验并由 `os.Open` 打开外部文件。

**风险边界**：链接由配置者自己放进 root，不是用户可控的 URL 穿越；但在
「文档根目录挂载了用户可写目录」的部署里，等于把任意文件读取交给了目录写入权。

**目标**：明确二选一并写进文档与测试——

| 方案 | 代价 | 收益 |
| --- | --- | --- |
| A. 拒绝（`EvalSymlinks` 后判前缀） | 每请求一次系统调用；需要缓存或按 root 预计算 | root 内链接不能逃逸 |
| B. 保留放行，但配置加载时对 root 做一次「不含符号链接」校验 | 启动期一次性检查 | 无运行时开销，配置者必须显式清理 |

**验收**：`TestStaticSymlinkEscape` 从「记录现状」改为断言固定结果；
Linux/macOS 上必须实际执行（Windows 上无权限，保持 Skip）。

---

## P0-2 未知域名 SNI 不得崩溃

**对应 Caddy**：[#5680](https://github.com/caddyserver/caddy/issues/5680)
（v2.7.x 收到未知域名 SNI 时 panic，86 条评论）。

**现状**：`modules/certmanager` 有 724 行测试，但没有针对「SNI 不在任何站点
证书覆盖范围内」的用例。

**目标**：

- 未知 SNI：回落到默认证书或明确拒绝握手，**不允许 panic**
- 通配符 `*.example.com` 不匹配 `a.b.example.com`（RFC 6125：通配符只跨一层）
- 尾点 `example.com.` 与大小写归一到同一张证书
- SNI 为空（IP 直连）：行为稳定，与文档一致
- 同名多证书（RSA + ECDSA）按客户端能力选择

**验收**：`modules/certmanager/testdata/sni-cases.jsonl` 覆盖上述组合，
并有 `-race` 下的并发选择用例（证书热更新与握手同时发生）。

---

## P1-1 反向代理的 `X-Forwarded-For` 语义

**现状**：`modules/proxy` 有 1349 行测试，`X-Forwarded-For` 的追加/覆盖与
跳数上限未见覆盖。

**目标**：

- 客户端自带 `X-Forwarded-For` 时必须**追加**，不能信任其值
- 跳数上限（避免无限增长）与超出时的处理写进文档
- `Host` 透传 vs 改写：由配置决定，默认行为固定
- hop-by-hop 头必须剥掉：`Connection`、`TE`、`Trailer`、`Transfer-Encoding`、
  `Upgrade`、`Proxy-Authorization`
- 上游中途断连：返回明确错误，不能挂死
- 上游超时：502 还是 504，与 `docs/usage.md` 一致

**验收**：`modules/proxy/testdata/forwarded-chain.jsonl`、
`modules/proxy/testdata/hop-by-hop-headers.json`。

---

## P1-2 压缩协商（当前测试最薄）

**现状**：`modules/compress` 源码 57 行、测试仅 39 行。

**目标**：

- `Accept-Encoding: gzip;q=0` 必须不压（q 值优先级）
- 响应已有 `Content-Encoding` 时不重复压
- `HEAD` 请求不得带 body，`Content-Length` 与 GET 一致
- Range 请求不压（压缩后 Range 语义失效），或明确拒绝
- 已压缩类型（图片、zip、woff2）跳过
- 压缩后补 `Vary: Accept-Encoding`，`Content-Length` 移除或按压缩后长度

**验收**：`modules/compress/testdata/accept-encoding.jsonl`。

**对应 Caddy 反馈**：Caddy 社区多次出现「二次压缩」「q=0 仍压缩」的回归。

---

## P1-3 Windows 路径截断

**现状**：`/file.`、`/file ` 这类请求在 Windows 上会被文件系统静默截断成
`file`，理论上 `/secret.txt.` 可读到 `secret.txt`。

**目标**：静态层对请求路径的尾点、尾空格、保留名（`CON`、`NUL`、`AUX`、
`PRN`、`COM1`…）显式拒绝，或规范化后再查一次。

**验收**：`modules/static/testdata/traversal-vectors.jsonl` 增加保留名向量；
Windows 上实际跑通（当前 CI 只跑 Linux，需要手动验证步骤写进文档）。

---

## P1-4 连接与超时边界

**目标**：

- `Expect: 100-continue`：正确处理，不能直接读 body
- keep-alive 复用连接时，请求间不得泄漏状态（cookie、缓存、认证）
- header 超限 → 431；body 超限 → 413，两者都不能 panic
- reload 时在途请求：处理完再切，或明确中断并告知
- reload 失败后旧配置仍然生效（原子性），且错误可见
- 并发 reload 不互相踩（`-race` 下验证）

**验收**：`runtime/listener` 与 `core` 各新增一组，配合 `-race` 跑。

---

## P2-1 SNI / 路径拒绝的可观测性

**目标**：被拒绝的请求（路径穿越、未知 SNI、超限）要留下可检索的日志字段
（拒绝原因、来源 IP、匹配规则），便于线上排查「是不是被打了」。

**注意**：日志里不得出现完整请求路径的原始编码（可能有敏感参数），
只记归一化后的片段与拒绝原因。

---

## 不做的事

- 不追求覆盖率数字。当前测试规模已不算薄（`proxy` 1349 行、`static` 767 行），
  缺的是**针对性**，不是总量。
- 不为「可能永远不出现」的场景写用例（例如 HTTP/0.9）。
- 不在本批需求里改架构（`--config` 提升为 persistent flag 之类另开议题）。

## 推进方式

每个需求按同一节奏：

1. 先写用例表（`testdata/*.jsonl`），跑出失败或记录现状
2. 需要改代码的，单独一个提交修，用例作为回归
3. 不需要改代码的，用例标注「锁定现状」并说明为什么可以接受

## 参考

- Caddy issue [#5680](https://github.com/caddyserver/caddy/issues/5680)：未知 SNI panic
- Caddy issue [#1157](https://github.com/caddyserver/caddy/issues/1157)：证书加载失败
- Caddy issue [#2407](https://github.com/caddyserver/caddy/issues/2407)：端口映射被忽略
- Caddy issue [#1463](https://github.com/caddyserver/caddy/issues/1463)：代理 header / URI 策略
- RFC 6125：通配符证书只跨一层
