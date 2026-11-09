# 边界用例测试报告

## 背景

对标 [caddyserver/caddy](https://github.com/caddyserver/caddy) 社区反馈的高频问题，
对本项目（fast-https）逐模块补充边界用例。目的不是提高行覆盖率，而是覆盖
**同类服务器实际踩过的坑**，尤其是那些本地开发跑不出来、上线后才暴露的情况。

Caddy 侧可参考的反馈类型：

| 类型 | 典型 issue | 本项目对应模块 |
| --- | --- | --- |
| 未知域名 SNI 导致崩溃 | #5680（86 条评论） | `modules/certmanager` |
| 证书加载 / 匹配失败 | #1157（95 条评论） | `modules/certmanager` |
| 端口映射被忽略 | #2407 | `modules/listener` |
| 代理的 header / URI 策略 | #1463 | `modules/proxy` |

> Caddy 的 issue 列表以功能请求为主（GUI、WSGI、PHP 之类），能转化成测试的
> 主要是上面几类崩溃与语义问题，以及社区反复讨论的通用坑（见下）。

## 测试组织

- 用例表放各模块的 `testdata/`，格式为 `.jsonl`（一行一条「输入 → 期望」），
  加用例只改数据文件，不动测试代码
- 需要按临时目录/端口改值的用 `.tmpl` 模板，由 `renderTestdata` 渲染
- 驱动测试按主题命名（`edge_cases_test.go`），不按开发阶段编号

运行方式：

```bash
go test ./modules/static/ -run TestStaticPathTraversalVectors -v
go test ./... -count=1
```

## 阶段 1：静态文件与路径安全

### 覆盖内容

`modules/static/testdata/traversal-vectors.jsonl`，11 条向量：

| 向量 | 期望 | 说明 |
| --- | --- | --- |
| `/../etc/passwd` | reject | 裸 `..` 逃逸 |
| `/a/../../etc/passwd` | reject | 多层 `..` 逃逸 |
| `/a/..%2f..%2fetc%2fpasswd` | name | 编码斜杠，未解码时是普通文件名 |
| `/..%2fetc%2fpasswd` | name | 同上，单层 |
| `/a%5c..%5c..%5cetc%5cpasswd` | name | 编码反斜杠 |
| `/a\..\..\etc\passwd` | reject | 裸反斜杠（Windows 上是分隔符） |
| `/....//etc/passwd` | name | 四点是合法文件名 |
| `/file.` | name | Windows 会截断尾点 |
| `/file ` | name | Windows 会截断尾空格 |
| `//etc/passwd` | name | 重复斜杠 |
| `/C:/Windows/win.ini` | name | 绝对盘符路径 |

`reject` 与 `name` 在安全性上等价（都读不到 root 外的文件），分开断言是为了
让回归时能立刻分辨「校验生效」和「解码行为变了」。

### 结果

```
PASS: TestStaticPathTraversalVectors  (11 个子用例)
SKIP: TestStaticSymlinkEscape         (Windows 无符号链接权限)
ok    fast-https/modules/static
```

### 结论

- **URL 解码发生在静态层之前**：`%2e%2e` 到这一层已是普通字符，因此既不会
  被 `..` 检查拒绝，也读不到越界文件。这条链路是安全的，但依赖「上层先解码」
  这个前提——若将来在解析层改成延迟解码，需要重跑这组向量。
- **反斜杠**在 Windows 上被 `filepath.Clean` 当作分隔符，因此裸 `\..\..\` 会被
  正确拒绝；编码形式 `%5c` 不会，落到普通文件名分支。
- **`staticPathInRoot` 只看 Clean 后的字符串前缀，不 EvalSymlinks**。root 内
  一个指向外部的符号链接因此能通过校验。这不是 URL 穿越（链接由配置者自己
  放入），但行为随平台变化，`TestStaticSymlinkEscape` 用来锁定现状。

### 待定问题

1. 符号链接：当前实现放行。是否要改成 `EvalSymlinks` 后再判前缀？
   代价是每次请求多一次系统调用，收益是「root 内链接不能逃逸」。
2. 尾点/尾空格（`/file.`、`/file `）：Windows 文件系统会静默截断，理论上
   `/secret.txt.` 能读到 `secret.txt`。当前行为是当普通名，取决于目标文件
   是否存在——本地测试环境无法稳定复现，需要在 Windows 上单独验证。

## 后续阶段（未执行）

| 阶段 | 目标模块 | 计划用例 |
| --- | --- | --- |
| 2 | `modules/proxy` | `X-Forwarded-For` 追加 vs 覆盖、跳数上限、hop-by-hop 头过滤、Host 透传、chunked + trailer、上游中途断连、超时语义 |
| 3 | `modules/compress`（现有测试最薄：57 行源码 / 39 行测试） | `Accept-Encoding` q 值协商、已压缩内容不重复压、`HEAD` 无 body、Range 与压缩互斥 |
| 4 | `modules/certmanager` | 未知 SNI 不 panic、通配符不跨层匹配、尾点与大小写、空 SNI、过期窗口、RSA + ECDSA 同名多证书 |
| 5 | `runtime/listener`、`core` | `Expect: 100-continue`、keep-alive 状态泄漏、431/413 边界、reload 时在途请求、并发 reload |

## 参考

- Caddy issue [#5680](https://github.com/caddyserver/caddy/issues/5680)：未知域名 SNI panic
- Caddy issue [#1157](https://github.com/caddyserver/caddy/issues/1157)：证书加载失败
- 需求清单见 [20-edge-case-hardening.md](../feature/20-edge-case-hardening.md)
