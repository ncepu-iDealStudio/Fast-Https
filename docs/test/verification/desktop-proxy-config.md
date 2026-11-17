# 桌面端代理来源配置验证

记录日期：2026-11-17
范围：Windows 桌面端「设置 → 主配置 → 代理」分组，对应配置项 `proxy.trusted_proxies`、
`proxy.forwarded_for.max_hops`、`proxy.preserve_host`，以及同一能力的 CLI 入口
`config proxy show/set` 与 `config show --section proxy`。

桌面端保存动作走 `configops.SaveConfigForm`：按字段搭建候选配置树 → 规范化（全默认时
`pruneEmptyProxy` 删除整段）→ 转交 `config apply` 事务（校验、恢复点、切换、reload，失败回滚）。
本次验证直接调用这个后端入口，参数与设置页提交的字段值完全一致；**未点击 GUI 窗口**，
GUI 的字段渲染与事件绑定由 `configSpecs` 单测与前端构建覆盖。

## 环境

| 项 | 值 |
| --- | --- |
| 系统 | Windows 11 |
| 源码 | 提交 `5d46353`（R-05） |
| CLI | 从当前源码构建的 `fast-https.exe`，与配置放在同一临时工作目录 |
| 配置基线 | 复制自 `desktop/bin/config/fast-https.json`（735 B，无 `proxy` 段） |
| 工作目录 | `%TEMP%\fh-proxy-verify`，避免改动仓库内实际配置 |
| 操作方式 | 调用 `configops.SaveConfigForm` + 文件 stat/hash 比对 + CLI 只读命令复核 |

## 用例与结论

| 编号 | 场景 | 期望 | 结果 |
| --- | --- | --- | --- |
| D-PXY-01 | 三项都填，保存 | 事务成功，写入 `proxy` 段，列表多行拆分 | ✅ |
| D-PXY-02 | 三项都改回默认值，保存 | 删掉整段 `proxy`，回到「未配置」 | ✅ |
| D-PXY-03 | `max_hops` 填 `-1` | 校验失败，配置不被改写 | ✅ |
| D-PXY-04 | `trusted_proxies` 填非法网段 | 校验失败并定位到下标，配置不被改写 | ✅ |

### D-PXY-01 写入

- 提交字段：
  - 可信代理：`10.0.0.0/8`、`192.168.0.0/16`（两行）
  - 链长上限：`8`
  - 透传客户端 Host：关闭
- 界面等价提示：`已保存 3 个字段并应用（配置事务完成）（操作 op_20261117_002，配置版本 v002，恢复点 v003）`。
- 配置文件：735 B → 905 B，新增：

  ```json
  "proxy": {
    "forwarded_for": {
      "max_hops": 8
    },
    "preserve_host": false,
    "trusted_proxies": [
      "10.0.0.0/8",
      "192.168.0.0/16"
    ]
  }
  ```

- CLI 复核：

  ```text
  $ fast-https.exe config proxy show --config <配置>
  trusted_proxies: 10.0.0.0/8, 192.168.0.0/16
  max_hops:         8
  preserve_host:    false
  ```

  `config show --section proxy` 返回同一段 JSON，两个入口语义一致。

### D-PXY-02 恢复默认

- 提交字段：可信代理清空、链长上限 `0`、透传 Host 打开。
- 界面等价提示：`已保存 3 个字段并应用（配置事务完成）（操作 op_20261117_003，配置版本 v003，恢复点 v004）`。
- 配置文件：905 B → 735 B，`proxy` 键整体消失（全文检索无 `proxy`）。
- CLI 复核：

  ```text
  $ fast-https.exe config proxy show --config <配置>
  proxy 未配置：只追加真实客户端地址，链长不限，Host 原样透传
  ```

### D-PXY-03 链长上限为负

- 提交：可信代理 `10.0.0.0/33`、链长上限 `-1`、透传 Host 打开（一次提交两项非法值，先报链长）。
- 错误：

  ```text
  CONFIG_INVALID: proxy.forwarded_for.max_hops must be >= 0: -1
  ```

- 配置未被改写：SHA256 前后均为 `1E1E9CFEC8612A11D5AD5712BF6E5F973C3BBAD80F36164AC75562B0463C790A`，
  修改时间不变，事务未提交。

### D-PXY-04 非法可信代理网段

- 提交：可信代理 `10.0.0.0/33`、链长上限 `8`、透传 Host 打开。
- 错误：

  ```text
  CONFIG_INVALID: proxy.trusted_proxies[0]: invalid IP or CIDR: "10.0.0.0/33"
  ```

- 配置未被改写（SHA256 不变），错误信息带下标，可直接定位到输入的哪一行。

## 结论

- 三项字段在桌面端与 CLI 之间是同一份配置语义：保存走同一个配置事务，桌面端没有第二套写盘逻辑。
- `pruneEmptyProxy` 保证「全默认」不会留下空段；只改一项时也会补齐其余默认值再判断，
  因此清空与部分修改都不会产生 `"proxy": {}` 这类无效段。
- 非法值由 `config/internal/validate` 拦截，桌面端只回显事务错误原文；拦截发生在写盘前，
  配置与恢复点均不受影响。

## 复跑方式

手工（GUI）：

1. 构建并启动桌面端（与 [desktop-tls-config.md](desktop-tls-config.md) 相同）：
   `cd desktop\frontend && npm run build`，再 `cd .. && go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui" -o bin\fast-https-desktop.exe .`
2. 在「主配置 → 代理」按 D-PXY-01 / 02 / 03 / 04 操作，每次保存后用下列命令复核：

   ```powershell
   Get-Item .\bin\config\fast-https.json | Select-Object Length,LastWriteTime
   Get-FileHash .\bin\config\fast-https.json -Algorithm SHA256
   .\bin\fast-https.exe config proxy show --config .\bin\config\fast-https.json
   ```

3. 结束前把三项改回默认并保存，使配置回到无 `proxy` 段的干净状态。

自动化回归（不需要 GUI）：

```powershell
cd desktop
go test ./internal/configops -run "Proxy|Prune" -v
cd frontend
npm run build
```

## 相关文档

- 配置项与 CLI 行为：[docs/usage.md](../../usage.md) 的 5.6 节与 `config proxy` 命令
- 边缘加固需求与实现说明：[edge-case-hardening-plan.md](../../develop/plans/edge-case-hardening-plan.md) 的 R-05
- 运行期验收用例：`modules/proxy/testdata/forwarded-chain.jsonl`
