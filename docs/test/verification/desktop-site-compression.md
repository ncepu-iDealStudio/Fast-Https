# 桌面端新增站点压缩默认值验证

记录日期：2026-11-17
范围：桌面端「站点 → 新增」弹窗提交的压缩字段，经 `siteops.AddSite` 翻译成
`site add --zip / --zip-min-length / --zip-skip-types` 后的落盘结果与回读结果。
对应配置项：`location.zip`、`location.zip_min_length`、`location.zip_skip_types`。

桌面端不直接改站点文件：新增走 `siteops.AddSite` → `planAdd` 生成 CLI 参数 →
`site add` 配置事务（校验、恢复点、切换、reload）。本次验证直接调用
`AddSite` / `ReadSiteDetail` 这两个后端入口，参数与弹窗提交的字段一致；
**未点击 GUI 窗口**，弹窗默认值的渲染由前端构建与 `planAdd` 单测覆盖。

## 环境

| 项 | 值 |
| --- | --- |
| 系统 | Windows 11 |
| 源码 | 提交 `77ce5af`（site add 默认压缩 + 整类通配） |
| CLI | 从当前源码构建的 `fast-https.exe`，与配置放在同一临时工作目录 |
| 工作目录 | `%TEMP%\fh-site-cmp-verify`，避免改动仓库内实际配置 |
| 配置基线 | 最小主配置 `{"http":{"include":["conf.d"]}}`，`conf.d` 初始为空 |
| 操作方式 | 一次性驱动脚本调用 `AddSite` / `ReadSiteDetail` + 落盘内容比对 + CLI 只读复核 |

## 用例与结论

| 编号 | 场景 | 期望 | 结果 |
| --- | --- | --- | --- |
| D-CMP-01 | 使用弹窗默认值新增 local 站点 | 写入 br+gzip、1 KB 阈值、整类跳过清单 | ✅ |
| D-CMP-02 | 只留 gzip，阈值 0，跳过 `image/png` | 只写 `zip: ["gzip"]` 与精确跳过项，不写阈值键 | ✅ |
| D-CMP-03 | 取消两个压缩勾选 | 站点文件不含任何 `zip*` 键 | ✅ |
| D-CMP-04 | 新增 proxy 站点 | 只写 `proxy_pass`，不带压缩参数 | ✅ |
| D-CMP-05 | 跳过类型填 `png` | `USAGE_ERROR`，站点文件未创建 | ✅ |

### D-CMP-01 默认值

- 提交字段：`zip=[gzip, br]`、`zipMinLength=1024`、
  `zipSkipTypes=[image/*, video/*, audio/*, font/woff2, application/zip, application/gzip]`。
- 界面等价提示：

  ```text
  已创建站点 auto.example（操作 op_20261117_001，配置版本 v002）
  ```

- 落盘内容（`config/conf.d/auto.example.json`）：

  ```json
  {
    "listen": 18081,
    "server_name": "auto.example",
    "location": [
      {
        "url": "/",
        "type": "local",
        "root": "C:\\Users\\...\\fh-site-cmp-verify\\httpdoc\\root",
        "index": ["index.html", "index.htm"],
        "zip": ["gzip", "br"],
        "zip_min_length": 1024,
        "zip_skip_types": ["image/*", "video/*", "audio/*", "font/woff2",
                            "application/zip", "application/gzip"]
      }
    ]
  }
  ```

- 回读 `ReadSiteDetail`：

  ```text
  zip=[gzip br] min=1024
  skip=[image/* video/* audio/* font/woff2 application/zip application/gzip]
  ```

- CLI 复核：

  ```text
  $ fast-https.exe site show auto.example --json
  {"server":{"listen":18081,"location":[{"zip":["gzip","br"],
   "zip_min_length":1024,
   "zip_skip_types":["image/*","video/*","audio/*","font/woff2",
                     "application/zip","application/gzip"], ...}]}}
  ```

### D-CMP-02 自定义

- 提交字段：`zip=[gzip]`、`zipMinLength=0`、`zipSkipTypes=[image/png]`。
- 落盘：`"zip": ["gzip"]`、`"zip_skip_types": ["image/png"]`；
  **没有** `zip_min_length` 键——0 表示不限制，不写冗余配置。
- 提示：`已创建站点 custom.example（操作 op_20261117_002，配置版本 v004）`。

### D-CMP-03 关闭压缩

- 提交字段：两个编码都不勾选，阈值 `0`，跳过列表清空。
- 落盘：`location` 里只有 `url` / `type` / `root` / `index`，没有任何 `zip*` 键。
- 提示：`已创建站点 plain.example（操作 op_20261117_003，配置版本 v006）`。
- 这与 `--zip=` 的语义一致：显式关闭压缩，后续仍可用 `site update --zip gzip` 打开。

### D-CMP-04 代理站点

- 提交字段：类型 `proxy`，上游 `http://127.0.0.1:9000`；压缩字段不参与。
- 落盘：`"type": "proxy"`、`"proxy_pass": "http://127.0.0.1:9000"`，无 `zip*`。
- `planAdd` 对 proxy 站点不生成压缩参数，`siteops.Add` 也会拒绝显式传入的压缩参数。

### D-CMP-05 非法跳过类型

- 提交字段：`zip=[gzip]`、`zipSkipTypes=[png]`（缺少 `type/subtype`）。
- 错误（写盘前的输入校验，不是配置校验）：

  ```text
  USAGE_ERROR: invalid site input: --zip-skip-types wants MIME types like image/png or image/*, got "png"
  ```

- 站点文件不存在；事务没有产生新站点。通配形式 `image/*` 由 D-CMP-01 正向覆盖。

## 结论

- 默认值有三层来源，最终结果一致：弹窗新增模式预填 → `planAdd` 在 `Zip == nil`
  时兜底 → `siteops.Add` 在参数未显式给出时补 `DefaultZip*`。任何一层拿到默认值，
  新建 local 站点都会带压缩。
- 只有显式提交空列表才关闭压缩；阈值 0 会被省略而不是写成 0，配置保持最小。
- 代理站点与压缩互斥：桌面端不生成参数，主程序侧也拒绝。
- 非法 MIME 在生产站点文件之前被拦下，错误信息带具体取值，配置与恢复点不受影响。

## 复跑方式

手工（GUI）：

1. 构建主程序与桌面端，与 [desktop-tls-config.md](desktop-tls-config.md) 相同。
2. 启动桌面端，「站点 → 新增」，展开「高级配置 → 压缩」，按 D-CMP-01 ~ 04 操作；
   每次保存后用下列命令复核：

   ```powershell
   Get-Content .\config\conf.d\<站点名>.json
   .\fast-https.exe site show <站点名> --json
   .\fast-https.exe config validate
   ```

3. D-CMP-05 在「跳过压缩的类型」里填 `png` 再保存，应看到 `USAGE_ERROR` 且文件未生成。

自动化回归（不需要 GUI）：

```powershell
cd desktop
go test ./internal/siteops -run "PlanAdd|Add" -v
cd ..
go test ./management/siteops -run "Add|Compression" -v
go test ./modules/static -run TestAcceptEncodingCases -v
```

本次的取值核对由一次性驱动脚本完成，脚本未入库；长期回归依赖上面的
`siteops` / `management/siteops` 单测与前端 `npm run typecheck`、`npm run build`。

## 相关文档

- 压缩配置与 `site add` 默认值：[docs/usage.md](../../usage.md) 的 6.4 节
- 通配匹配实现：`modules/static/compression.go` 的 `matchMediaType`
- 默认值定义：`management/siteops/siteops.go` 的 `DefaultZip*`
- 相关提交：`77ce5af`（site add 默认压缩 + 整类通配）
