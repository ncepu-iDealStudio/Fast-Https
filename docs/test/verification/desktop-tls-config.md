# 桌面端 TLS 默认证书配置验证

记录日期：2026-11-17
范围：Windows 桌面端「设置 → 主配置 → TLS」分组，对应配置项 `tls.default_certificate`
（`certificate` + `key`），以及同一能力的 CLI 入口 `config tls` / `config show --section tls`。

桌面端是 CLI 的图形前端：读取走 `config show`，保存把改动写成候选配置后走
`config apply` 事务（校验 → 恢复点 → 切换 → reload，失败回滚），不在 GUI 里另写一套写盘逻辑。
因此本记录验证的是「CLI 语义在 GUI 上是否被正确暴露」，不是新实现。

## 环境

| 项 | 值 |
| --- | --- |
| 系统 | Windows 11 |
| 桌面端 | `desktop/bin/fast-https-desktop.exe`（production 构建：`frontend/dist` + `go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"`） |
| CLI | `desktop/bin/fast-https.exe`（与桌面端同目录，GUI 按相对路径调用） |
| 主配置 | `desktop/bin/config/fast-https.json` |
| 证书素材 | `desktop/bin/config/cert/localhost.pem`、`localhost-key.pem` |
| 操作方式 | 桌面端 UI 实际操作 + 配置文件 stat/hash 比对 + CLI 只读命令复核 |

初始状态：主配置里没有 `tls` 段（735 B），`config tls show` 输出「未配置」。

## 用例与结论

| 编号 | 场景 | 期望 | 结果 |
| --- | --- | --- | --- |
| D-TLS-01 | 两项都填，点「保存更改」 | 走配置事务成功，配置写入 `tls.default_certificate` | ✅ |
| D-TLS-02 | 两项都清空，点「保存更改」 | 视为关闭兜底证书，删掉整段 `tls` | ✅ |
| D-TLS-03 | 只填「默认证书」，私钥留空，点「保存更改」 | 成对校验报错，配置文件不被改写 | ✅ |

### D-TLS-01 写入

- 操作：TLS 分组填「默认证书」「默认证书私钥」，点「保存更改」。
- 界面提示：`已保存 2 个字段并应用（配置事务完成）（操作 op_20261117_001，配置版本 v063，恢复点 v062）`。
- 配置文件：684 B → 977 B，新增：

  ```json
  "tls": {
    "default_certificate": {
      "certificate": "E:\\code\\ncepu_bj\\fast-https\\desktop\\bin\\config\\cert\\localhost.pem",
      "key": "E:\\code\\ncepu_bj\\fast-https\\desktop\\bin\\config\\cert\\localhost-key.pem"
    }
  }
  ```

- CLI 复核：`fast-https.exe config tls show --config <主配置>` 与
  `fast-https.exe config show --section tls` 均输出上述两项，双入口语义一致。

### D-TLS-02 清除

- 操作：两项清空后保存。
- 界面提示：`已保存 2 个字段并应用（配置事务完成）（操作 op_20261117_002，配置版本 v065，恢复点 v064）`。
- 配置文件：977 B → 735 B，`tls` 键整体消失。
- CLI 复核：`config tls show` 回到「未配置（未知 SNI 回落到该端口第一张证书）」。
- 说明：桌面端保存后会对候选树做规范化（键排序、补齐默认字段），再经事务提交，所以清除后的文件
  比最初的手写版本略大（735 B vs 684 B），语义一致。这是事务路径的既有行为，不是本次引入的差异。

### D-TLS-03 只填一项

- 操作：只填「默认证书」= `desktop\bin\config\cert\localhost.pem`，私钥留空；顶部改动计数显示 `1`，点「保存更改」。
- 界面提示（红色错误条）：

  ```text
  CONFIG_INVALID: tls.default_certificate: certificate and key must be set together
  ```

- 配置未被改写：
  - 文件大小仍为 735 B，修改时间仍为 `2026-11-17 16:24:46`；
  - SHA256 与保存前一致：`AED804527ADF004BF0663EAA356FD53953F0630EDD6E55B2B2CAB5400E5B5754`；
  - `config tls show` 仍为「未配置」，没有产生新的配置版本与操作记录。
- 恢复现场：点「重新读取」后表单回到磁盘状态，错误提示消失，「保存更改」因无改动而置灰。

## 结论

- 成对校验由 `config/internal/tlsconf` 实现，桌面端只回显事务错误原文，没有独立的第二套判断；
  因此 CLI 与 GUI 的报错文案、拦截时机一致。
- 「两项都空」与「只填一项」是两条不同分支：前者在保存前由 `pruneEmptyTLS` 删除空段，后者进入校验并失败，
  两条都有单测锁定（`desktop/internal/configops/editor_test.go` 的 `TestPruneEmptyTLS`、
  `TestConfigSpecsCoverTLSDefaults`）。

## 复跑方式

1. 构建并启动桌面端：

   ```powershell
   cd desktop\frontend
   npm run build
   cd ..
   go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui" -o bin\fast-https-desktop.exe .
   .\bin\fast-https-desktop.exe
   ```

2. 在「主配置 → TLS」分组按 D-TLS-01 / 02 / 03 操作，每次保存后用下列命令复核：

   ```powershell
   Get-Item .\bin\config\fast-https.json | Select-Object Length,LastWriteTime
   Get-FileHash .\bin\config\fast-https.json -Algorithm SHA256
   .\bin\fast-https.exe config tls show --config .\bin\config\fast-https.json
   ```

3. 结束前把两个字段清空并保存，使配置回到无 `tls` 段的干净状态。

自动化回归（不需要 GUI）：

```powershell
cd desktop
go test ./internal/configops -run "TLS|Prune" -v
```

## 相关文档

- 配置项与 CLI 行为：[docs/usage.md](../../usage.md) 的「`config tls`」（第 9 节命令表）与 7.2 的 `tls.default_certificate`
- 边缘加固中与证书选择相关的需求：[edge-case-hardening-plan.md](../../develop/archive/edge-case-hardening-plan.md) 的 R-03 / R-04
- 证书与 ACME 的实操说明：[docs/help/tls-certificates.md](../../help/tls-certificates.md)
