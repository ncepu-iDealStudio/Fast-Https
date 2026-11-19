# 桌面端开箱即用主配置验证

记录日期：2026-11-17
范围：随包 `config/fast-https.json` 的预设值在桌面端「设置 → 主配置」中的读取与保存行为，
以及同一份配置在 CLI 只读入口（`config show --section security|waf`、`config proxy show`、
`config validate`）下的一致性。

桌面端设置页不维护第二套默认值：读取走 `configops.ReadConfigForm` → `config show`，
保存走 `configops.SaveConfigForm` → 候选配置树 → `config apply` 配置事务（校验、恢复点、
切换、reload，失败回滚）。本次验证直接调用这两个后端入口，参数与设置页提交的字段值一致；
**未点击 GUI 窗口**，字段渲染与事件绑定由 `configSpecs` 单测和前端构建覆盖。

## 环境

| 项 | 值 |
| --- | --- |
| 系统 | Windows 11 |
| 源码 | 提交 `858e1bb`（主配置预设）+ 设置页文案修正 |
| CLI | 从当前源码构建的 `fast-https.exe`，与配置放在同一临时工作目录 |
| 配置基线 | 仓库 `config/fast-https.json`（831 B，SHA256 `A7380768…A2603A22B`） |
| 工作目录 | `%TEMP%\fh-default-verify`，避免改动仓库内实际配置 |
| 操作方式 | 一次性驱动脚本调用 `ReadConfigForm` / `SaveConfigForm` + 文件大小与哈希比对 + CLI 只读复核 |

## 用例与结论

| 编号 | 场景 | 期望 | 结果 |
| --- | --- | --- | --- |
| D-DEF-01 | 打开设置页，读取随包配置 | 十项预设值全部命中，无「缺省」标记 | ✅ |
| D-DEF-02 | 改 4 项后保存 | 事务成功；其余预设原样保留 | ✅ |
| D-DEF-03 | 把 4 项改回随包预设 | 内容恢复；JSON 被规范化重写 | ✅ |
| D-DEF-04 | WAF 模式填非法枚举 | 表单层拦截，配置不被改写 | ✅ |
| D-DEF-05 | 链长上限填 `-1` | `CONFIG_INVALID`，配置不被改写 | ✅ |

### D-DEF-01 读取预设值

`ReadConfigForm` 返回的字段（`missing=false` 表示来自磁盘而不是缺省值）：

| 字段 | 读到的值 |
| --- | --- |
| `log_root` | `./logs` |
| `security.headers.nosniff` | `true` |
| `security.headers.x_frame_options` | `SAMEORIGIN` |
| `security.headers.referrer_policy` | `strict-origin-when-cross-origin` |
| `waf.mode` | `monitor` |
| `proxy.trusted_proxies` | `127.0.0.1/32`、`::1/128`（两行） |
| `proxy.forwarded_for.max_hops` | `10` |
| `http.include` | `./conf.d` |
| `http.servers_limit.max_header_size` | `8192` |
| `http.servers_limit.max_body_size` | `8388608` |

CLI 只读复核：

```text
$ fast-https.exe config show --json --section security
{"security":{"headers":{"content_security_policy":"","hsts":"","nosniff":true,
 "referrer_policy":"strict-origin-when-cross-origin","x_frame_options":"SAMEORIGIN"},
 "host_check":"off"}}

$ fast-https.exe config show --json --section waf
{"waf":{"mode":"monitor"}}

$ fast-https.exe config proxy show
trusted_proxies: 127.0.0.1/32, ::1/128
max_hops:         10
preserve_host:    true
```

`preserve_host` 未写进文件，CLI 显示的是内置默认 `true`，与 R-05 的语义一致。

### D-DEF-02 修改后保存

- 提交字段：`x_frame_options=DENY`、`referrer_policy=`（留空）、`waf.mode=enforce`、
  `max_body_size=16777216`。
- 界面等价提示：

  ```text
  已保存 4 个字段并应用（配置事务完成）（操作 op_20261117_001，配置版本 v002，恢复点 v001）
  ```

- 配置文件：831 B → 885 B。落盘内容同时满足：

  ```text
  "x_frame_options": "DENY"              ✓
  "referrer_policy": ""                  ✓（留空是合法取值，不删键）
  "mode": "enforce"                      ✓
  "max_body_size": 16777216              ✓
  "max_header_size": 8192                ✓（未被提交，保持预设）
  "trusted_proxies" / "max_hops": 10     ✓（同样保持）
  "nosniff": true                        ✓（同样保持）
  ```

- 只提交 4 个字段时，其余字段按原值进入候选树，没有出现「只写提交项、其余被重置」的情况。

### D-DEF-03 改回预设

- 提交字段：`x_frame_options=SAMEORIGIN`、`referrer_policy=strict-origin-when-cross-origin`、
  `waf.mode=monitor`、`max_body_size=8388608`。
- 界面等价提示：

  ```text
  已保存 4 个字段并应用（配置事务完成）（操作 op_20261117_002，配置版本 v004，恢复点 v003）
  ```

- 文件体积 885 B → 921 B，四项取值与随包配置一致。
- 体积没有回到 831 B 是预期行为：事务按 JSON 对象重写，顶层键按字母排序、数组展开成多行；
  **内容语义一致**，只是格式被规范化。手工改回这一项时不要用字节数判断是否恢复成功。

### D-DEF-04 非法 WAF 模式

- 提交：`waf.mode=block`。
- 错误（表单枚举校验，未进入事务）：

  ```text
  WAF 模式：取值必须是  / enforce / monitor / disabled
  ```

- 配置文件 SHA256 前后相同（`SHA_UNCHANGED_1 = true`），未产生恢复点。

### D-DEF-05 链长上限为负

- 提交：`proxy.forwarded_for.max_hops=-1`。
- 错误（配置事务校验）：

  ```text
  CONFIG_INVALID: proxy.forwarded_for.max_hops must be >= 0: -1
  ```

- 配置文件 SHA256 不变（`SHA_UNCHANGED_2 = true`），事务未提交。

## 结论

- 桌面端读到的就是随包文件里的值，没有隐藏的第二套默认值；用户打开设置页即可看到并直接改。
- 保存只提交用户改动的字段，其余预设（安全头、代理信任、请求上限）不会被重置。
- 留空的安全头以空字符串落盘，语义等同关闭；是否删除整键由各段的 prune 规则决定，
  `security.headers` 不做整段删除，避免误删 `nosniff` 等兄弟字段。
- 非法值分两层拦截：枚举错误在表单层、取值错误在配置校验层，两者都发生在写盘前，
  配置与恢复点均不受影响。
- 保存会按对象重写 JSON（键排序、数组展开），属于格式变化，不影响语义。

## 复跑方式

手工（GUI）：

1. 构建主程序与桌面端，与 [desktop-tls-config.md](desktop-tls-config.md) 相同：
   `go build -o desktop\bin\fast-https.exe .`，再 `cd desktop\frontend && npm run build`，
   然后 `cd .. && go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui" -o bin\fast-https-desktop.exe .`
2. 启动桌面端，进入「设置 → 主配置」，按 D-DEF-01 的表格逐项核对；再做 D-DEF-02 ~ 05 的改动。
3. 每次保存后用下列命令复核：

   ```powershell
   Get-Item .\bin\config\fast-https.json | Select-Object Length,LastWriteTime
   Get-FileHash .\bin\config\fast-https.json -Algorithm SHA256
   .\bin\fast-https.exe config show --section security
   .\bin\fast-https.exe config show --section waf
   .\bin\fast-https.exe config proxy show
   .\bin\fast-https.exe config validate
   ```

4. 结束前把四项改回随包预设（D-DEF-03），避免把 `DENY` / `enforce` 留在本机配置里。

自动化回归（读写管道，不需要 GUI）：

```powershell
cd desktop
go test ./internal/configops -run "Config|Field|Proxy|Prune" -v
```

本次的取值核对由一次性驱动脚本完成，脚本未入库；长期回归依赖上面的 `configops` 单测
与前端 `npm run typecheck` / `npm run i18n:check`。

## 相关文档

- 预设项说明与字段含义：[docs/usage.md](../../usage.md) 的 5.1、5.2、5.5、5.6 节
- 配置变更提交：`858e1bb`（主配置改为开箱即用的安全与代理预设）
- 同类桌面端验证：[desktop-proxy-config.md](desktop-proxy-config.md)、[desktop-tls-config.md](desktop-tls-config.md)
