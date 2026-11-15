# fast-https CLI `--json` 输出形状参考

> 依据 `cmd/`（`cmd/commands.go`、`cmd/internal/{cli,sitecmd,configcmd,servicecmd,inspectcmd,acmecmd}`）
> 与 `management/`（`view/txn/cfgdiff/cfgver/logquery/monview/diagnose/operation/rtclient/svcctl`）
> 的 Go 代码整理。字段名以实际序列化的 json tag / map key 为准。
> 供下游写 Go struct 与前端渲染使用。

---

## 0. 通用封套、错误与退出码

### 0.1 成功封套（`cli.EmitOK`，cmd/internal/cli/output.go）

除 `config path`（见 §2.1）与 `acme show`（见 §7.3）外，所有带 `--json` 的命令成功时向 stdout 输出**恰好一个** JSON 文档：

```json
{
  "ok": true,
  "command": "<命令标识>",
  "request_id": "<string 或 null>",
  "data": "<各命令自己的 data，见下文>",
  "warnings": ["<告警字符串>", "..."]
}
```

| 字段 | 类型 | 语义 |
|---|---|---|
| `ok` | bool | 恒为 `true` |
| `command` | string | 命令标识（点分，如 `site.list`、`config.apply`），见各命令 |
| `request_id` | string \| null | `--request-id` 传入的值；未传且命令不要求时为 `null`；写类命令会自动生成 `req_<16位hex>` |
| `data` | any | 各命令的数据体 |
| `warnings` | []string | 告警；无告警时为 `[]`（不为 null） |

### 0.2 错误封套（`cli.ReportError`，`--json` 时 stdout）

```json
{
  "ok": false,
  "command": "<命令标识>",
  "request_id": "<string 或 null>",
  "error": {
    "code": "<ERROR_CODE>",
    "message": "<人读信息>",
    "details": [
      { "code": "<string, 可省>", "path": "<string, 可省>", "message": "<string>" }
    ]
  }
}
```

文本模式下错误为 stderr 上一行 `Error: <message>`。`details` 元素是 `cli.ErrorDetail`
（json tag：`code,omitempty`、`path,omitempty`、`message`）；事务错误会带
`path:"operation_id"` / `path:"restore_point"` / `path:"config_state"` 以及端口冲突时
`code:"port_in_use", path:"port"`。

### 0.3 错误码 → 退出码（`cmd/internal/cli/errors.go`，测试锁定）

| code | 退出码 | | code | 退出码 |
|---|---|---|---|---|
| `INTERNAL_ERROR` | 1 | | `OPERATION_UNKNOWN` | 7 |
| `USAGE_ERROR` | 2 | | `TIMEOUT` | 8 |
| `PERMISSION_DENIED` | 3 | | `NOT_FOUND` | 9 |
| `SERVICE_UNREACHABLE` | 4 | | `LOG_SOURCE_UNAVAILABLE` | 9 |
| `CONFIG_INVALID` | 5 | | `CONFLICT` | 10 |
| `OPERATION_FAILED` | 6 | | `REQUEST_ID_CONFLICT` | 10 |
| `RECOVERY_REQUIRED` | 6 | | `UNMET_DEPENDENCY` | 11 |
| | | | `NOT_SUPPORTED` | 11 |

非 `*CLIError` 的普通 error（含旧文本命令）一律退出码 1。旧命令里的
`logging.ConsoleLog.Fatal` 走 zerolog 的 os.Exit(1)。

### 0.4 全局 flags（root persistent，`cli.RegisterGlobalFlags`）

| flag | 类型 | 含义 | 默认 |
|---|---|---|---|
| `--json` | bool | stdout 只输出一个 JSON 文档；控制台日志改走 stderr | `false` |
| `--timeout` | duration | 命令超时（写类命令建 ctx 用），负数 → `USAGE_ERROR` | `0`（用命令自身默认值，写事务默认 30s = `svcctl.DefaultTimeout`） |
| `--request-id` | string | 幂等键；写类命令（site 写、config apply/rollback、service reload）未传时自动生成 `req_<16hex>` | `""` |
| `--lang` | string | 帮助语言 `zh` / `en`（实际取值在解析前扫 `FASTHTTPS_LANG`） | `""`（未设置时 zh） |

### 0.5 写类命令共用的 data：事务结果（`cli.EmitTxnResult`）

`site add/update/delete/enable/disable` 与 `config apply/rollback` 的 `data` 都是
`txn.Result` 的投影（map 构造，key 固定）：

```json
{
  "dry_run": false,
  "idempotent": false,
  "identical": false,
  "changes": [
    {
      "path": "http.server[0].listen[1]",
      "op": "added",
      "value": "443 ssl h2",
      "old": "<任意 JSON 值，可省>",
      "new": "<任意 JSON 值，可省>",
      "changed": true
    }
  ],
  "restore_point": "v012",
  "version": "v013",
  "rolled_back": false,
  "config_state": "matches_candidate",
  "operation_id": "op_xxx",
  "status": "succeeded",
  "result": {
    "restore_point": "v012",
    "version": "v013",
    "rolled_back": false,
    "config_state": "matches_candidate",
    "cause": "<失败原因，可省>",
    "last_phase": "<对账时最后阶段，可省>"
  }
}
```

| 字段 | 类型 | 语义 |
|---|---|---|
| `dry_run` | bool | 是否为 `--dry-run` 预览 |
| `idempotent` | bool | 命中 `--request-id` 幂等重放（结果取自已有 operation 记录） |
| `identical` | bool | 差异为空（`changes` 长度为 0） |
| `changes` | array | 差异条目，元素为 `cfgdiff.Change`（json tag：`path`、`op`、`value,omitempty`、`old,omitempty`、`new,omitempty`、`changed,omitempty`）。`op ∈ added/removed/modified`；敏感键（口令/令牌等）只出 `changed:true`，不出新旧值；`changes` 非 null（空数组） |
| `restore_point` | string | 切换前打的版本快照号（如 `v012`）；**dry-run 时为空串**（不建恢复点） |
| `version` | string | 提交后当前 active 的配置版本号；dry-run 为空串 |
| `rolled_back` | bool | 失败后已从恢复点补偿还原 |
| `config_state` | string | `matches_candidate` \| `matches_restore_point` \| `unchanged` \| `uncertain`（dry-run 恒 `unchanged`） |
| `operation_id` | string | **仅当产生 operation 记录时出现**（dry-run 无此 key） |
| `status` | string | operation 状态：`queued/running/succeeded/failed/cancelled/unknown/recovery_required`；仅随 `operation_id` 出现 |
| `result` | object | 落盘的 operation result map（键见示例）；仅随 `operation_id` 出现 |

文本模式：dry-run 输出 `dry-run: identical` 或 2 空格缩进的差异 JSON；否则一行
`<operation_id> <status> restore=<restore_point> version=<version>`。
`changes` 条目含义同 `config diff`（§2.6）。

**幂等语义**：同一 `--request-id` + 同参数 → 重放已有记录（`idempotent:true`）；
同 request-id 不同参数 → `REQUEST_ID_CONFLICT`（退出 10）。

---

## 1. site 命令组

### 1.1 `site list`

- **命令标识**：`site.list`
- **flags**：
  | flag | 类型 | 含义 | 默认 |
  |---|---|---|---|
  | `--config` | string | 主配置文件 | `config.CONFIG_FILE_PATH`（编译期路径） |
- **data JSON**（`view.SiteListData`）：
  ```json
  {
    "include_dirs": ["conf.d"],
    "sites": [
      {
        "name": "blog",
        "file": "/etc/fast-https/conf.d/blog.json",
        "listen": "80, 443 ssl h2",
        "server_name": "blog.example.com",
        "locations": ["/ → local ./httpdoc/root"],
        "enabled": true,
        "cert_status": "valid"
      }
    ]
  }
  ```
  | 字段 | 类型 | 语义 |
  |---|---|---|
  | `include_dirs` | []string | 主配置 `http.include` 展开的目录；不会为 null |
  | `sites[].name` | string | 文件名主干（`.json` / `.json.disabled` 去掉后缀） |
  | `sites[].file` | string | 站点文件路径 |
  | `sites[].listen` | string | 监听合并展示（逗号+空格连接，如 `80, 443 ssl h2`） |
  | `sites[].server_name` | string | `server_name` 字段 |
  | `sites[].locations` | []string | 每个 location 的描述，格式 `"<url> → <local\|proxy> [<root\|upstream>]"`；不会为 null |
  | `sites[].enabled` | bool | `false` = `.json.disabled`（停用） |
  | `sites[].cert_status` | string? | `omitempty`；按 server_name 对上的证书状态（`valid/expiring/...`，同 cert list） |
  | `sites[].error` | string? | `omitempty`；文件解析/校验失败的原因 |
- **行为要点**：含停用文件（`site.ListAll`）。`http.include` 无可用目录 → `sites:[]`
  + warning `"http.include has no usable directory"`，退出 0。主配置解析失败 →
  `CONFIG_INVALID`（5）。

### 1.2 `site show <name>`

- **命令标识**：`site.show`
- **flags**：`--config`（同上）；`--merged`（bool，默认 `false`，打印合并后每个字段都生效的 server 块）
- **data JSON**（`view.SiteShowData`）：
  ```json
  {
    "name": "blog",
    "source": "/etc/fast-https/conf.d/blog.json",
    "merged": false,
    "server": { "<站点 server 块，脱敏后的任意 JSON 对象>": "..." }
  }
  ```
  | 字段 | 类型 | 语义 |
  |---|---|---|
  | `name` | string | 站点名（`.json` 后缀会被剥掉） |
  | `source` | string | server 块来源文件（站点文件；`--merged` 或主配置内联时为主配置或站点文件路径） |
  | `merged` | bool | 是否合并默认值后的块 |
  | `server` | object | server 块内容（`redact.Value` 脱敏；JSON 里不含注释） |
- **行为要点**：名字含 `/`、`\` 或 `..` → `USAGE_ERROR`（2）；找不到站点 →
  `NOT_FOUND`（9）；站点文件解析失败 → `CONFIG_INVALID`（5）。
  文本模式默认打站点文件原文（带 `// 来源：` 头），`--merged` 打合并后的块。

### 1.3 `site add`（写、走配置事务）

- **命令标识**：`site.add`
- **flags**：
  | flag | 类型 | 含义 | 默认 |
  |---|---|---|---|
  | `--domain` | string | 站点域名，同时用作 server_name 与文件名；省略且 stdin 是终端时进向导 | `""` |
  | `--root` | string | 本地站点文档根目录（local 类型） | `""`（向导/生成时落 `./httpdoc/root`） |
  | `--listen` | []string（可重复） | 监听值，如 `80`、`"443 ssl h2"` | 空 → `["80"]`；`--tls` 时 `["80","443 ssl h2"]` |
  | `--proxy-pass` | string | 反代上游；设置后站点类型为 proxy | `""` |
  | `--tls` | bool | 启用 TLS（给监听补 `ssl h2`、补默认证书路径） | `false` |
  | `--config` | string | 主配置文件 | 编译期默认路径 |
  | `--dry-run` | bool | 只生成差异，不应用（不写文件、不 reload、不建恢复点、不记 operation） | `false` |
  | `--yes` | bool | 确认执行 | `false` |
- **data JSON**：见 §0.5 事务结果。dry-run 时无 `operation_id/status/result`，
  `restore_point`/`version` 为空串，`config_state:"unchanged"`。
- **行为要点**：
  - **`--yes` / `--dry-run`**：带 `--domain`（纯 flag 模式）时，非 dry-run **必须 `--yes`**
    （否则 `USAGE_ERROR`：`add requires --yes; use --dry-run to preview`）；
    `--dry-run` 不需要 `--yes`。不带 `--domain` 进向导时向导末屏自己确认，不再要 `--yes`；
    stdin 非终端且无 `--domain` → `USAGE_ERROR`。
  - 纯 flag 模式 `--root` 与 `--proxy-pass` 至少一个（否则 `USAGE_ERROR`）。
  - 站点名由域名规范化（小写、trim）得到；同名（含停用）或同 server_name 已存在 →
    `CONFLICT`（10）。root 落在禁入目录（配置目录、证书目录、状态目录、日志根）→
    `USAGE_ERROR`（2），且发生在事务外（不产生 uncertain 状态）。
  - `--tls` 语义："至少一个 TLS 监听"，永不给 80 贴 ssl；给了 `--listen` 又 `--tls` 时
    443 升级成 TLS / 非 80 监听升级 / 全是 80 则补 `443 ssl h2`。
  - 事务失败错误码按 §0.5/§0.3；`request_id` 自动生成。

### 1.4 `site update <name>`

- **命令标识**：`site.update`
- **flags**：`--listen`（[]string）、`--server-name`（string）、`--root`（string）、
  `--proxy-pass`（string）、`--tls`（bool，`false`）、`--config`、`--dry-run`（`false`）、`--yes`（`false`）。
- **data JSON**：同 §0.5 事务结果。
- **行为要点**：白名单字段之外的任意 JSON 合并被拒绝（没有相应 flag）。至少要显式传一个
  变更 flag，否则 `USAGE_ERROR`。**非 dry-run 必须 `--yes`**（`update requires --yes; use --dry-run to preview`）。
  只改 URL 为 `/` 的 location，其余手写 location 保留。站点内联在主配置 `http.server[]` →
  `NOT_SUPPORTED`（11）；站点不存在 → `NOT_FOUND`（9）；重复 server_name → `CONFLICT`（10）。

### 1.5 `site delete <name>`

- **命令标识**：`site.delete`
- **flags**：`--config`、`--dry-run`（`false`）、`--yes`（`false`）
- **data JSON**：同 §0.5 事务结果。
- **行为要点**：**非 dry-run 必须 `--yes`**。只删站点配置文件，不删内容目录与证书。
  内联站点 → `NOT_SUPPORTED`（11）；不存在 → `NOT_FOUND`（9）。

### 1.6 `site enable <name>` / `site disable <name>`

- **命令标识**：`site.enable` / `site.disable`
- **flags**：`--config`、`--dry-run`（`false`）、`--yes`（`false`，**不需要**，帮助注明 disable/enable 不需要确认）
- **data JSON**：同 §0.5 事务结果（operation 记录的 args 里有 `enabled`）。
- **行为要点**：`.json` ↔ `.json.disabled` 改名 + 同一事务（校验/恢复点/reload/可回滚）。
  不要求 `--yes`（可逆）。已经处于目标状态 → `CONFLICT`（10）；不存在 → `NOT_FOUND`（9）；
  内联站点 → `NOT_SUPPORTED`（11）。

---

## 2. config 命令组

### 2.1 `config path`（**特例：不套封套**）

- **命令标识**：无（不走 `EmitOK`；保留桌面端旧契约，输出缩进 JSON）
- **flags**：无专属 flag（`--json` 是全局的）
- **`--json` 输出**（`configcmd.RuntimePaths`，2 空格缩进，**没有** `ok/command/data/warnings`）：
  ```json
  {
    "config": "/abs/path/fast-https.json",
    "cert": "/abs/path/cert",
    "log": "/abs/path/logs",
    "pid": "/abs/path/fast-https.pid",
    "root_crt": "/abs/path/ca/root.crt",
    "state": "/abs/path/state"
  }
  ```
  | 字段 | 类型 | 语义 |
  |---|---|---|
  | `config` | string | 主配置文件（绝对路径） |
  | `cert` | string | 证书目录 |
  | `log` | string | 当前布局默认日志目录（配置里 `log_root` 优先，调用方自行覆盖） |
  | `pid` | string | pid 文件 |
  | `root_crt` | string | 本地根 CA 证书 |
  | `state` | string | 管理状态目录（配置版本、operation） |
- **行为要点**：文本模式为 `key value` 对齐行（`config/cert/log/pid/root_crt/state`）。

### 2.2 `config show`

- **命令标识**：`config.show`
- **flags**：`--config`；`--section`（string，默认 `""`，可选 `http|acme|security|waf|sites`）
- **data JSON**（`map[string]any`，`view.ConfigShow`）：
  ```json
  {
    "source": "disk",
    "config_file": "/abs/fast-https.json",
    "files": ["/abs/fast-https.json", "/abs/conf.d/blog.json"],
    "config": { "<脱敏后的主配置对象；或按 --section 只含该段>": "..." },
    "sites": [
      { "name": "blog", "file": "/abs/conf.d/blog.json", "server": { "...": "..." } },
      { "name": "bad", "file": "/abs/conf.d/bad.json", "error": "parse ... failed: ..." }
    ],
    "section": "http",
    "config_dirty_hint": true
  }
  ```
  | 字段 | 类型 | 语义 |
  |---|---|---|
  | `source` | string | 恒 `"disk"`（来源是磁盘，不是运行中进程生效配置） |
  | `config_file` | string | 主配置绝对路径 |
  | `files` | []string | 主配置 + 各站点 json 的绝对路径 |
  | `config` | object | 脱敏后的主配置（无 `--section`）；有 `--section`（非 sites）时是 `{<section>: <脱敏段>}` |
  | `sites` | []object | 每个 include 目录下 `*.json`：`name`、`file`、`server`（脱敏对象）或 `error`（解析失败原因），按 name 排序 |
  | `section` | string | **仅 `--section` 时出现**，回显段名 |
  | `config_dirty_hint` | bool | **仅进程在跑时出现**：配置文件比 pid 文件新（改过未重载的提示） |
- **行为要点**：`--section` 不在白名单 → `USAGE_ERROR`（2，`ErrBadSection`）。
  配置解析失败 → `CONFIG_INVALID`（5）。敏感值永远脱敏，无明文选项。
  文本模式输出同样的数据（2 空格缩进 JSON）。无 `--section` 时 `config`+`sites` 都有；
  `--section sites` 时只有 `section:"sites"` 与 `sites`（无 `config` key）。

### 2.3 `config validate`

- **命令标识**：`config.validate`
- **flags**：`--config`；`--candidate`（string，默认 `""`；候选目录，只校验不应用）
- **data JSON**（`view.ConfigValidateData`）：
  ```json
  { "valid": true, "file": "/abs/candidate-or-config", "candidate": true }
  ```
  | 字段 | 类型 | 语义 |
  |---|---|---|
  | `valid` | bool | 校验通过（失败不返回 data，直接错误封套） |
  | `file` | string | 校验的文件/候选树根 |
  | `candidate` | bool | 是否校验的是候选目录 |
- **行为要点**：校验失败 → `CONFIG_INVALID`（5），data 里不会出现 `valid:false`。
  候选路径含 `..` 或符号链接逃出 → `USAGE_ERROR`（2）。文件不存在 → 错误封套
  （NOT_FOUND 路径，见 `FromViewError`/os.ErrNotExist → `NOT_FOUND` 9）。
  只校验不应用不重载。

### 2.4 `config versions`

- **命令标识**：`config.versions`
- **flags**：`--config`
- **data JSON**（`cfgver.ListData`）：
  ```json
  {
    "versions": [
      {
        "version": "v013",
        "created_at": "2026-01-02T03:04:05Z",
        "created_by": "alice",
        "summary": "config apply",
        "parent": "v012",
        "active": true
      }
    ],
    "drift": false
  }
  ```
  | 字段 | 类型 | 语义 |
  |---|---|---|
  | `versions` | []object | 版本条目（按版本号降序） |
  | `versions[].version` | string | 版本号（`vNNN`） |
  | `versions[].created_at` | string | 创建时间 |
  | `versions[].created_by` | string | 创建用户 |
  | `versions[].summary` | string? | `omitempty`，创建原因 |
  | `versions[].parent` | string? | `omitempty`，上一个版本号 |
  | `versions[].active` | bool | 当前磁盘配置树哈希 == 该快照树哈希（不是存储的标志位） |
  | `drift` | bool | 有版本但没有任何一个与磁盘树哈希相同 |
- **行为要点**：快照不含证书与私钥。空时 `versions: []`（非 null）。
  状态目录读失败 → `FromStateError`（一般 `INTERNAL_ERROR` 1）。

### 2.5 `config diff`

- **命令标识**：`config.diff`
- **flags**：
  | flag | 类型 | 含义 | 默认 |
  |---|---|---|---|
  | `--config` | string | 主配置文件 | 编译期默认 |
  | `--current` | string | 当前侧（文件或目录） | 空 → 与 `--config` 相同 |
  | `--candidate` | string | 候选：目录、主配置文件或版本号（如 `v001`） | `""`（必填） |
- **data JSON**（`cfgdiff.Result`）：
  ```json
  {
    "identical": false,
    "changes": [
      {
        "path": "http.server[0].listen[1]",
        "op": "modified",
        "value": "<added 时>",
        "old": "<removed/modified 时的旧值>",
        "new": "<modified 时的新值>",
        "changed": true
      }
    ]
  }
  ```
  | 字段 | 类型 | 语义 |
  |---|---|---|
  | `identical` | bool | 无差异 |
  | `changes` | []`cfgdiff.Change` | 路径级差异，按 `path` 排序 |
  | `changes[].path` | string | JSON 路径（`a.b[0].c` 形式） |
  | `changes[].op` | string | `added` \| `removed` \| `modified` |
  | `changes[].value` | any? | `added` 时的新值（敏感键省略） |
  | `changes[].old` | any? | `removed`/`modified` 时旧值（敏感键省略） |
  | `changes[].new` | any? | `modified` 时新值（敏感键省略） |
  | `changes[].changed` | bool? | 敏感键的标记：只有 `true`，不带新旧值 |
- **行为要点**：`--candidate` 缺失 → `USAGE_ERROR`（2）；候选不存在（非 v 开头）→
  `NOT_FOUND`（9）；版本号不存在 → `NOT_FOUND`（9）。解析失败是错误
  （`CONFIG_INVALID` 5），**不会用空差异冒充一致**。候选路径逃逸（`..`/符号链接）→
  `USAGE_ERROR`（2）。目录或版本快照比较整棵 JSON 树（主文件 + conf.d）。
  文本模式 `identical:true` 时输出一行 `identical: true`，否则缩进 JSON。

### 2.6 `config apply`

- **命令标识**：`config.apply`
- **flags**：`--config`、`--candidate`（string，必填）、`--dry-run`（bool，`false`）、`--yes`（bool，`false`）
- **data JSON**：§0.5 事务结果。dry-run 无 `operation_id/status/result`、
  `restore_point:""`、`version:""`、`config_state:"unchanged"`；成功提交时
  `restore_point` = 应用前的版本快照号，`version` = 新 active 版本号，
  `result` 里同样带 `restore_point/version/rolled_back:false/config_state:"matches_candidate"`。
- **行为要点**：
  - **`--yes` / `--dry-run`**：非 dry-run **必须 `--yes`**（`apply requires --yes; use --dry-run to preview`）。
    `--dry-run` 只输出校验与差异：不写正式文件、不 reload、不建恢复点、不记 operation。
  - 流程：校验候选 → 生成差异 → 保存恢复点 → 切换文件 → reload → 验证（读得到
    `/runtime` 用强验证，否则弱验证）→ 提交；失败从恢复点还原（`rolled_back:true`）。
  - 服务未运行也允许提交（配置写好、下次启动生效，stderr 提示）。
  - 错误码：候选校验失败 `CONFIG_INVALID`（5）；端口冲突 `CONFLICT`（10，details 带
    `port_in_use`）；目录不可写 `PERMISSION_DENIED`（3）；补偿还原失败
    `RECOVERY_REQUIRED`（6）；超时 `TIMEOUT`/`OPERATION_UNKNOWN`（details 带
    `operation_id`、`restore_point`、`config_state`）。
  - `--candidate` 缺失 → `USAGE_ERROR`（2）；路径逃逸 → `USAGE_ERROR`（2）。

### 2.7 `config rollback`

- **命令标识**：`config.rollback`
- **flags**：`--config`、`--version`（string，必填，如 `v001`）、`--dry-run`（bool，`false`）、`--yes`（bool，`false`）
- **data JSON**：同 §0.5 事务结果（`result` / operation args 里含 `version`）。
- **行为要点**：回滚是一次**新的 operation**（不删历史版本）：先校验目标版本
  （含引用证书仍存在），再走与 apply 相同的事务。非 dry-run **必须 `--yes`**
  （`rollback requires --yes; use --dry-run to preview`）。目标版本目录不存在 →
  `NOT_FOUND`（9）。`--version` 缺失 → `USAGE_ERROR`（2）。

---

## 3. 旧的纯文本命令：status / version / start / stop / reload

这五个命令（`cmd/commands.go`）**没有 JSON 分支**，`--json` 全局 flag 能被解析但
成功输出仍是文本（错误时若带 `--json` 则由 `ReportError` 输出错误封套）。桌面端与
systemd 依赖它们的文本输出。

- **`version`**（`newVersionCmd`）
  - flags：无专属。
  - 输出：一行 `fast-https <CURRENT_VERSION>\n`（编译期常量）。退出 0。

- **`status`**（`statusHandler`）
  - flags：无专属。
  - 输出形态：
    - 运行中：stdout `fast-https is running, pid <pid>\n`，退出 0；
    - pid 残留（进程不在）：返回 error → stderr `Error: fast-https is not running, stale pid <pid>`，退出 1；
    - 无 pid 文件：stderr `Error: fast-https is not running, no pid file`，退出 1。
  - 注意与 `service status` 的区别（见 §4.4）。

- **`start`**（`StartHandler`）
  - flags：无专属。
  - 行为：启动前 `PreCheckHandler`（配置校验、自签证书生成、已有实例、端口占用；
    任一失败 `logging.ConsoleLog.Fatal` → 输出错误并退出 1）。
    Unix 非容器/非 systemd 时后台化（重新 exec 自身），父进程等子进程报告端口就绪
    （≤10s）后输出 `fast-https started in background, pid <pid>, logs in <LogRoot>`，退出 0。
    Windows / `FASTHTTPS_FOREGROUND=1` / `INVOCATION_ID` / 容器标记（`/.dockerenv`、
    `/run/.containerenv`）保持前台：输出 banner，随后 `server.Run()` 阻塞。
  - 无 JSON 输出。

- **`stop`**（`StopHandler`）
  - flags：无专属。
  - 输出形态（stdout，退出 0，幂等）：
    - 无 pid 文件：`fast-https is not running`；
    - pid 残留：`fast-https is not running, removed stale pid file (pid <pid>)`；
    - 成功：`fast-https stopped`（Unix SIGTERM 优雅停机；Windows TerminateProcess）。
  - 停机超时/失败走 Fatal（退出 1）。

- **`reload`**（`ReloadHandler` → `reload()`）
  - flags：无专属。
  - 行为：先在**客户端校验配置**（失败 → stderr `Error: 配置未通过校验，已保留运行中的配置：<原因>`，退出 1，
    不惊动运行中进程）→ 读 pid → 预检新增端口 → 发 reload 信号。
    没在跑（含 stale pid，顺手清理）→ `fast-https is not running, nothing to reload (config ok)`
    （stale 版带 `stale pid <pid>`），退出 0。
    信号失败 → Fatal（退出 1）。**成功时无输出**，退出 0。
  - 与 `service reload` 的区别见 §4.4。

---

## 4. service 命令组

### 4.1 `service status`

- **命令标识**：`service.status`
- **flags**：`--config`
- **data JSON**（`view.ServiceStatus`）：
  ```json
  {
    "status": "running",
    "pid": 1234,
    "stale_pid": false,
    "observed_at": "2026-01-02T03:04:05Z",
    "source": "pid_file+port_probe+runtime",
    "ports": [ { "port": 80, "reachable": true } ],
    "uptime_seconds": 3600,
    "started_at": "2026-01-02T02:04:05Z",
    "config_version": "v013",
    "version": "1.2.3"
  }
  ```
  | 字段 | 类型 | 语义 |
  |---|---|---|
  | `status` | string | `running` \| `degraded`（进程在但端口全连不上）\| `stopped` \| `unknown`（证据矛盾） |
  | `pid` | int? | `omitempty`，进程 pid |
  | `stale_pid` | bool? | `omitempty`，pid 文件是残留 |
  | `observed_at` | string | RFC3339 观测时刻 |
  | `source` | string | `pid_file+port_probe`；读得到 /runtime 时为 `pid_file+port_probe+runtime` |
  | `ports` | []object | 配置端口逐个回环探测：`port`(int)、`reachable`(bool) |
  | `uptime_seconds` | int64? | `omitempty`，由 started_at 推算 |
  | `started_at` | string? | `omitempty` |
  | `config_version` | string? | `omitempty`，状态目录里当前 active 的版本号 |
  | `version` | string? | `omitempty`，服务版本（/runtime） |
- **行为要点**：**只读查询，服务没在跑也是成功（退出码 0）**。/runtime 连不上时
  warning `"uptime, started_at and version unavailable: runtime snapshot unreachable"`。

### 4.2 `service version`

- **命令标识**：`service.version`
- **flags**：无专属。
- **data JSON**：
  ```json
  { "cli_version": "1.2.3", "server_version": "1.2.3" }
  ```
  | 字段 | 类型 | 语义 |
  |---|---|---|
  | `cli_version` | string | 编译期版本 |
  | `server_version` | string \| null | 服务版本（/runtime 快照）；拿不到为 `null` + warning |
- **行为要点**：文本模式只打 `fast-https <ver>`（不含 server 版本）。
  快照不可达时 warning `"server_version unavailable: runtime snapshot unreachable"`。

### 4.3 `service reload`

- **命令标识**：`service.reload`
- **flags**：`--config`；`--dry-run`（bool，`false`，只校验与预检，不发信号）
- **data JSON**（`cli.EmitReloadResult`）：
  ```json
  {
    "dry_run": false,
    "idempotent": false,
    "verification": {
      "level": "runtime",
      "reload_confirmed": true,
      "process_alive": true,
      "ports_reachable": true
    },
    "operation_id": "op_xxx",
    "status": "succeeded",
    "result": { "verification": { "...": "..." }, "pid": 1234 },
    "pid": 1234
  }
  ```
  | 字段 | 类型 | 语义 |
  |---|---|---|
  | `dry_run` | bool | 是否 dry-run |
  | `idempotent` | bool | request-id 幂等重放 |
  | `verification` | object | `svcctl.Verification`：`level`(`runtime`\|`basic`)、`reload_confirmed`(bool；runtime 级表示 config_generation 加一)、`process_alive`(bool?)、`ports_reachable`(bool?)，后两个 `omitempty` |
  | `operation_id` | string | 仅非 dry-run 且登记了 operation 时出现 |
  | `status` | string | operation 状态，同 §0.5 |
  | `result` | object | operation result（`verification`、`pid`；失败时 `config_state:"uncertain"`） |
  | `pid` | int | `res.PID != 0` 时出现 |
- **行为要点**：不改配置文件；服务没在跑 → `SERVICE_UNREACHABLE`（4，`service is not running`）。
  `--dry-run` 只校验+端口预检，不发信号、不记 operation。request-id 冲突 →
  `REQUEST_ID_CONFLICT`（10）。强验证（/runtime 可达）要求 pid/started_at 不变且
  config_generation 加一；否则弱验证（进程仍在、端口可连）。超时 → operation 标
  `unknown`、`config_state:"uncertain"`，退出码 8（`TIMEOUT`）。

### 4.4 与旧命令的区别（问题 7）

- **`service status` vs 顶层 `status`**：顶层 `status` 是一行文本、"没在跑"以非 0
  退出（脚本判活用）；`service status` 是只读查询，退出码恒 0，给出四态
  （running/degraded/stopped/unknown）、逐端口探测、uptime/version/config_version，
  支持 `--json`。
- **`service reload` vs 顶层 `reload`**：顶层 `reload` 客户端校验后发信号即完
  （成功无输出、无验证、无 operation 记录、无 dry-run）；`service reload` 走受控
  流程——校验、新增端口预检、operation 记录、request-id 幂等、reload 后强/弱验证、
  `--dry-run` 预览，结果带 `verification`。

---

## 5. runtime / diagnose

### 5.1 `runtime status`

- **命令标识**：`runtime.status`
- **flags**：无专属。
- **data JSON**（`inspectcmd.runtimeStatusData`，map 构造）：
  ```json
  {
    "observed_at": "2026-01-02T03:04:05Z",
    "source": "runtime",
    "state": "running",
    "pid": 1234,
    "started_at": "2026-01-02T02:04:05Z",
    "uptime_seconds": 3600,
    "connections": {
      "active": 3,
      "accepted_total": 1000,
      "closed_total": 997,
      "rejected_total": 0
    },
    "requests": {
      "active": 1,
      "total": 5000
    }
  }
  ```
  | 字段 | 类型 | 语义 |
  |---|---|---|
  | `observed_at` | string | 快照观测时间（服务端 RFC3339） |
  | `source` | string | 恒 `"runtime"`（127.0.0.1:9088/runtime） |
  | `state` | string | 服务状态（快照 `server.state`） |
  | `pid` | int? | 快照 `server.pid`，为 0 时省略 |
  | `started_at` | string? | 省略当空 |
  | `uptime_seconds` | int64? | 由 started_at 推算，解析失败省略 |
  | `connections` | object | `active`(int64)、`accepted_total`/`closed_total`/`rejected_total`(uint64) |
  | `requests` | object | `active`(int64)、`total`(uint64) |
- **行为要点**：固定 warnings：
  `"connections.reading, writing and waiting are not collected"`、
  `"resources.cpu_percent, memory_bytes and open files are not collected"`。
  快照连不上 → `SERVICE_UNREACHABLE`（4，消息尾附 9088 端点说明）；schema 不符 →
  `UNMET_DEPENDENCY`（11）。文本模式只打 state/pid/uptime/connections.active/requests 摘要。

### 5.2 `runtime stats`

- **命令标识**：`runtime.stats`
- **flags**：`--since`（string，`""`，尚未支持）、`--domain`（string，`""`，尚未支持）
- **data JSON**（`inspectcmd.runtimeStatsData`）：
  ```json
  {
    "observed_at": "2026-01-02T03:04:05Z",
    "source": "runtime",
    "window": "since_process_start",
    "requests": {
      "active": 1,
      "total": 5000,
      "parse_errors_total": 0,
      "aborted_total": 0
    },
    "responses": {
      "2xx_total": 4800,
      "3xx_total": 100,
      "4xx_total": 90,
      "5xx_total": 10,
      "unknown_total": 0
    },
    "request_duration": {
      "unit": "ms",
      "count": 5000,
      "buckets": [ { "le": 10, "count": 4000 }, { "le": "+Inf", "count": 5000 } ]
    },
    "upstream": {
      "dial_fail_total": 0,
      "send_timeout_total": 0,
      "read_timeout_total": 0,
      "idle_conns": 2
    }
  }
  ```
  | 字段 | 类型 | 语义 |
  |---|---|---|
  | `observed_at` / `source` | string | 同上 |
  | `window` | string | 恒 `"since_process_start"` |
  | `requests` | object | `active`、`total`、`parse_errors_total`、`aborted_total` |
  | `responses` | object | `2xx_total`、`3xx_total`、`4xx_total`、`5xx_total`、`unknown_total` |
  | `request_duration` | object? | 省略当缺失；`unit`(string)、`count`(uint64)、`buckets`(`[{le: number\|"+Inf", count: uint64}]`) |
  | `upstream` | object? | 省略当缺失；`dial_fail_total`、`send_timeout_total`、`read_timeout_total`、`idle_conns` |
- **行为要点**：传了 `--since` 或 `--domain` → `NOT_SUPPORTED`（11，退出码 11）。
  `request_duration` 缺失 → warning；`upstream` 缺失 → warning
  `"upstream stats are omitted until the proxy section is registered"`。快照错误同 5.1。

### 5.3 `diagnose`

- **命令标识**：`diagnose`
- **flags**：`--config`
- **data JSON**（`diagnose.Report`）：
  ```json
  {
    "observed_at": "2026-01-02T03:04:05Z",
    "overall": "warning",
    "checks": [
      { "name": "service", "status": "ok", "message": "..." },
      { "name": "config", "status": "ok", "message": "..." },
      { "name": "config_drift", "status": "unavailable", "message": "no pid file to compare against" },
      { "name": "certs", "status": "warning", "message": "1 certificate file(s) expiring" },
      { "name": "ports", "status": "ok", "message": "..." },
      { "name": "logs", "status": "ok", "message": "..." },
      { "name": "runtime", "status": "warning", "message": "runtime snapshot unreachable" },
      { "name": "monitor", "status": "ok", "message": "..." },
      { "name": "state", "status": "ok", "message": "..." }
    ],
    "report_id": "<string>"
  }
  ```
  | 字段 | 类型 | 语义 |
  |---|---|---|
  | `observed_at` | string | UTC RFC3339 |
  | `overall` | string | `ok` \| `warning` \| `error`（有 error 即 error，否则有 warning 即 warning；`unavailable` 不拉低） |
  | `checks` | []object | 固定顺序：`service`、`config`、`config_drift`、`certs`、`ports`、`logs`、`runtime`、`monitor`、`state` |
  | `checks[].name` | string | 检查名 |
  | `checks[].status` | string | `ok` \| `warning` \| `error` \| `unavailable`（无数据源/超时） |
  | `checks[].message` | string | 人读说明（不含秘密） |
  | `report_id` | string | 本次诊断标识 |
- **行为要点**：**诊断本身是一次成功的查询，退出码 0**（配置有问题也是 0）。
  只读、不修复、不写报告文件。单检查超时 3s → 该检查 `unavailable`，其余照常。

### 5.4 `monitor status`

- **命令标识**：`monitor.status`
- **flags**：`--config`
- **data JSON**（`monview.Status`）：
  ```json
  {
    "status": "running",
    "observed_at": "2026-01-02T03:04:05Z",
    "source": "monitor_status_file",
    "version": "1.0.0",
    "pid": 2345,
    "healthy": true,
    "last_collect_at": "2026-01-02T03:04:00Z",
    "last_event_at": "2026-01-02T03:03:50Z"
  }
  ```
  | 字段 | 类型 | 语义 |
  |---|---|---|
  | `status` | string | `not_installed`（状态文件不存在）\| `running` \| `stale`（最近采集 >30s 或缺 last_collect_at）\| `unhealthy`（healthy=false）\| `unknown`（文件不是合法 JSON） |
  | `observed_at` | string | RFC3339 |
  | `source` | string | 恒 `"monitor_status_file"`（只读文件，不连 Monitor 进程） |
  | `version` | string? | Monitor 版本 |
  | `pid` | int? | >0 才出现 |
  | `healthy` | bool? | 文件里的健康位 |
  | `last_collect_at` / `last_event_at` | string? | 时间戳 |
  | `last_error` | string? | 脱敏后的最近错误 |
- **行为要点**：文件不存在 → `status:"not_installed"`，**仍是成功查询（退出 0）**。
  stale → warning。schema 版本不符 → `UNMET_DEPENDENCY`（11）；符号链接拒绝 →
  `LOG_SOURCE_UNAVAILABLE`（9）。

### 5.5 `monitor events`

- **命令标识**：`monitor.events`
- **flags**：
  | flag | 类型 | 含义 | 默认 |
  |---|---|---|---|
  | `--config` | string | 主配置（解析日志根） | 编译期默认 |
  | `--since` | string | 只看该时长之内的事件（`1h`、`30m`，也接受 RFC3339） | `""` |
  | `--limit` | int | 返回条数 | `0` → 100；>1000 钳到 1000 |
  | `--severity` | string | `info` \| `warning` \| `error` \| `critical` | `""` |
- **data JSON**（`monview.EventsResult`）：
  ```json
  {
    "events": [
      { "time": "2026-01-02T03:04:00Z", "type": "cert_renew", "severity": "warning", "message": "..." }
    ],
    "skipped": 2,
    "truncated": false
  }
  ```
  | 字段 | 类型 | 语义 |
  |---|---|---|
  | `events` | []object | 白名单字段事件，**由新到旧**；不会为 null |
  | `events[].time` | string | 事件时间 |
  | `events[].type` | string | 事件类型 |
  | `events[].severity` | string | 小写严重级别 |
  | `events[].message` | string | 脱敏、截到 1024 字节的文本（只作为数据） |
  | `skipped` | int | 跳过的非 JSON 行数（同时产生 warning） |
  | `truncated` | bool | 读取窗口被截断或命中 limit |
- **行为要点**：坏行跳过并计数。事件文件不存在 → 空 `events` + warning
  `"monitor event file is not present"`，退出 0。非法 `--severity`/`--since`/负
  `--limit` → `USAGE_ERROR`（2）；schema 不符 → `UNMET_DEPENDENCY`（11）；符号链接 →
  `LOG_SOURCE_UNAVAILABLE`（9）。

---

## 6. cert 命令组

### 6.1 `cert list`

- **命令标识**：`cert.list`
- **flags**：`--config`
- **data JSON**（`view.CertListData`）：
  ```json
  {
    "renew_before_days": 30,
    "certificates": [
      {
        "domains": ["blog.example.com"],
        "file": "/etc/fast-https/cert/blog.example.com.pem",
        "status": "valid",
        "subject": "CN=blog.example.com",
        "issuer": "CN=fast-https local CA",
        "names": ["blog.example.com"],
        "not_before": "2026-01-01T00:00:00Z",
        "not_after": "2026-04-01T00:00:00Z",
        "days_left": 80,
        "self_signed": true,
        "local_ca": true,
        "acme_managed": false,
        "deployed_by": ["blog"]
      }
    ]
  }
  ```
  | 字段 | 类型 | 语义 |
  |---|---|---|
  | `renew_before_days` | int | expiring 阈值（acme.renew_before_days，默认 30） |
  | `certificates` | []`CertEntry` | 按证书文件归并（同一文件多域名/多站点只一条），按 file 排序 |
  | `.domains` | []string | 该证书覆盖的声明域名 |
  | `.file` | string | 证书文件路径（**永不输出私钥路径/内容**） |
  | `.status` | string | `valid` \| `expiring`（剩余天数≤renew_before_days）\| `expired` \| `not_yet_valid` \| `missing` \| `unreadable` \| `unknown`（未声明证书路径） |
  | `.subject` / `.issuer` | string? | `omitempty` |
  | `.names` | []string? | SAN，`omitempty` |
  | `.not_before` / `.not_after` | string? | RFC3339，`omitempty` |
  | `.days_left` | int? | `omitempty` |
  | `.self_signed` / `.local_ca` | bool? | `omitempty` |
  | `.acme_managed` | bool | 域名会被 ACME 申请 |
  | `.deployed_by` | []string | 使用该证书的站点名 |
  | `.error` | string? | `omitempty`，missing/unreadable 等原因 |
- **行为要点**：只读配置里声明的证书路径，不遍历证书目录。没有 ssl server 块 →
  warning `"no ssl server block is configured"` + 空列表，退出 0。

### 6.2 `cert check`

- **命令标识**：`cert.check`
- **flags**：`--config`；`--domain`（string，必填，缺失 → `USAGE_ERROR` 2）
- **data JSON**（`view.CertCheckData`）：
  ```json
  {
    "domain": "blog.example.com",
    "passed": true,
    "certificate": { "<CertEntry，同 6.1>": "..." },
    "checks": {
      "exists":   { "checked": true, "ok": true },
      "validity": { "checked": true, "ok": true, "detail": "valid" },
      "hostname": { "checked": true, "ok": true },
      "key_pair": { "checked": true, "ok": true },
      "chain":    { "checked": false, "certificates_in_file": 1 }
    }
  }
  ```
  | 字段 | 类型 | 语义 |
  |---|---|---|
  | `domain` | string | 回显被检查的域名 |
  | `passed` | bool | 已执行的检查全部通过 |
  | `certificate` | object | 该域名的 CertEntry |
  | `checks.exists` | `CheckItem` | 文件存在性（`checked`、`ok`、`detail,omitempty`） |
  | `checks.validity` | `CheckItem` | 有效期（detail 为 status） |
  | `checks.hostname` | `CheckItem` | 证书是否覆盖该域名 |
  | `checks.key_pair` | `CheckItem` | 证书与私钥是否配对 |
  | `checks.chain` | `ChainCheck` | `checked`（恒 false，不做链验证）、`certificates_in_file`（文件内证书张数） |
- **行为要点**：**检查结果是数据**——证书有问题也退出 0；域名不在任何 ssl 站点 →
  `NOT_FOUND`（9）。`checked:false` 表示"没能执行"（如私钥不可读），与"执行了没过"区分。
  同名多声明只查第一个 + warning。

### 6.3 `acme show`（**特例：无 JSON 分支**）

- **命令标识**：无（`acmecmd.runAcmeShow` 只写文本，**不看 `JSONMode`**，`acme` /
  `acme setup` 同样纯文本；`--json` 全局 flag 能解析但被忽略）
- **flags**：无专属。
- **文本输出形态**（`writeACMEShow`）：
  ```
  状态：启用|关闭
  邮箱：<email 或 (空)>
  测试环境（staging）：<bool>
  挑战：<auto|http-01|tls-alpn-01>
  续期提前：<N> 天
  缓存目录：<path>
  失败回落自签：<bool>
  CA 降级链：
    - <name> <directory> (eab_kid=<kid>, eab_hmac_key=****<尾4位>)   ← ZeroSSL 时
    - <name> <directory>
  将申请的域名：<逗号分隔域名 或 (无)>
  跳过 <域名>：<原因>
  ```
  未配置 acme 段时一行提示 `未配置 acme 段，当前使用自签证书。运行 fast-https acme 生成配置。`
- **行为要点**：EAB HMAC key 脱敏（只露尾 4 位）。读配置失败返回 error（退出 1）。
  **下游若需要 JSON，当前没有可消费的 data 结构，需按文本解析或先补实现。**

---

## 7. logs 命令组

三个子命令共用同一个 data（`logquery.Result`）。无 offset/游标分页：只有 `--limit`，
从文件尾向前读（由新到旧），最多扫描 64 MiB（`MaxScanBytes`）。

- **共同 flags**（`addLogReadFlags`）：
  | flag | 类型 | 含义 | 默认 |
  |---|---|---|---|
  | `--config` | string | 主配置（解析 log_root；读不到回退默认日志目录 + warning） | 编译期默认 |
  | `--since` | string | 只看该时长之内的记录（`1h`/`30m`，也接受 RFC3339） | `""` |
  | `--limit` | int | 返回条数 | `0` → 100；>1000 钳到 1000（warning `limit capped at 1000`）；负数 → `USAGE_ERROR` |
  | `--domain` | string | 按 `host` 字段过滤（大小写不敏感精确匹配） | `""` |
  | `--status` | string | 状态码过滤：`500`、`4xx`、`400-499`（**仅 access/query 有此 flag**） | `""` |

- **data JSON**（`logquery.Result`）：
  ```json
  {
    "records": [
      {
        "time": "2026-01-02T03:04:05.123",
        "kind": "access",
        "level": "info",
        "message": "request served",
        "method": "GET",
        "path": "/index.html?token=***",
        "status": 200,
        "size": 1234,
        "host": "blog.example.com",
        "remote_addr": "10.0.0.1",
        "truncated": false
      }
    ],
    "source": { "file": "access.log", "scanned_bytes": 65536 },
    "time_range": { "since": "2026-01-02T02:04:05", "earliest_scanned": "2026-01-02T02:30:00" },
    "range_complete": true,
    "scan_truncated": false,
    "output_truncated": false,
    "truncated": false,
    "limit": 100
  }
  ```
  | 字段 | 类型 | 语义 |
  |---|---|---|
  | `records` | []`Record` | 匹配记录，**由新到旧**；不会为 null |
  | `records[].time` | string? | 日志时间（`omitempty`） |
  | `records[].kind` | string? | 日志种类（`access/error/system/safe/console`，取自行内 `kind` 字段） |
  | `records[].level` | string? | 级别 |
  | `records[].message` | string? | 消息，脱敏 + 截到 1024 字节 |
  | `records[].method` | string? | 请求方法 |
  | `records[].path` | string? | 请求路径（查询串中的秘密脱敏），截到 1024 字节 |
  | `records[].status` | int? | 状态码 |
  | `records[].size` | int? | 响应字节数 |
  | `records[].host` | string? | Host |
  | `records[].remote_addr` | string? | 客户端地址 |
  | `records[].reason` | string? | 安全日志拒绝原因（脱敏） |
  | `records[].truncated` | bool? | 本条 message/path 被截断 |
  | `source.file` | string | 固定文件名（`access.log`/`error.log`/`system.log`/`safe.log`） |
  | `source.scanned_bytes` | int64 | 实际扫描字节数 |
  | `time_range.since` | string? | 请求的时间下界（重写成日志时间格式） |
  | `time_range.earliest_scanned` | string? | 扫描到的最早记录时间 |
  | `range_complete` | bool | 扫描覆盖了请求的时间范围 |
  | `scan_truncated` | bool | 触到 64 MiB 扫描上限 |
  | `output_truncated` | bool | 触到 256 KiB 输出上限 |
  | `truncated` | bool | `scan_truncated \|\| output_truncated \|\| 任一字段被截断` |
  | `limit` | int | 归一化后的 limit（回显） |
- **不含原始行、不含 user_agent**；结果只含固定白名单字段。Bearer/Basic 凭据与
  查询串秘密脱敏。非 JSON 行跳过并 warning。日志文件不存在或为符号链接 →
  `LOG_SOURCE_UNAVAILABLE`（9）；过滤条件非法 → `USAGE_ERROR`（2）；无权限 →
  `PERMISSION_DENIED`（3）。

### 7.1 `logs access`

- **命令标识**：`logs.access`
- **flags**：共同 flags 全部（含 `--status`）。查 `access.log`。

### 7.2 `logs error`

- **命令标识**：`logs.error`
- **flags**：共同 flags 中除 `--status` 外（`--config`、`--since`、`--limit`、`--domain`）。
  查 `error.log`（含 PrintErr 与控制台 Warn/Error 镜像，行内 `kind` 可能是 `console`）。

### 7.3 `logs query`

- **命令标识**：`logs.query`
- **flags**：
  | flag | 类型 | 含义 | 默认 |
  |---|---|---|---|
  | `--kind` | string | `access` \| `error` \| `system` \| `safe`（**必填**，缺失 → `USAGE_ERROR`） | `""` |
  | `--until` | string | 只看该时长**之前**的记录（`10m` 等） | `""` |
  | `--level` | string | 按 `level` 精确过滤 | `""` |
  | `--method` | string | 按 `method` 精确过滤 | `""` |
  | `--path-prefix` | string | 按 `path` 前缀过滤 | `""` |
  | `--reason` | string | 按 `reason` 精确过滤 | `""` |
  | + 共同 flags | | `--config`、`--since`、`--limit`、`--domain`、`--status` | |

- **data JSON**：与 7. 相同（一次只查一个文件；`source.file` 即该 kind 对应文件）。

---

## 8. operation 命令组

记录条目结构 `operation.Record`（json tag 即如下）：

```json
{
  "operation_id": "op_01H...",
  "request_id": "req_ab12...",
  "command": "config.apply",
  "status": "succeeded",
  "created_at": "2026-01-02T03:04:05Z",
  "started_at": "2026-01-02T03:04:05Z",
  "finished_at": "2026-01-02T03:04:09Z",
  "owner_pid": 1234,
  "owner_start_key": "<进程启动标识>",
  "phase": "committed",
  "args_hash": "<sha256>",
  "result": {
    "restore_point": "v012",
    "version": "v013",
    "rolled_back": false,
    "config_state": "matches_candidate"
  },
  "error": { "code": "OPERATION_FAILED", "message": "..." }
}
```

| 字段 | 类型 | 语义 |
|---|---|---|
| `operation_id` | string | 操作 id |
| `request_id` | string? | 幂等键，`omitempty` |
| `command` | string | 命令标识 |
| `status` | string | `queued/running/succeeded/failed/cancelled/unknown/recovery_required` |
| `created_at` / `started_at` / `finished_at` | string | 时间戳（后两个 `omitempty`） |
| `owner_pid` / `owner_start_key` | int / string? | 持有者进程（崩溃对账用），`omitempty` |
| `phase` | string? | 最后阶段：`started/validated/restore_point_saved/files_switched/reloaded/verified/committed/compensating/restored/restore_verified` |
| `args_hash` | string? | 参数哈希（幂等冲突检测），`omitempty` |
| `result` | object? | 任意 map：`restore_point`、`version`、`rolled_back`、`config_state`，失败补偿时另有 `cause`、对账时 `last_phase`；service reload 的记录含 `verification`、`pid` |
| `error` | object \| null | `{code?, message}`（`json:"error"`，无 omitempty：成功时为 `null`） |

### 8.1 `operation list`

- **命令标识**：`operation.list`
- **flags**：
  | flag | 类型 | 含义 | 默认 |
  |---|---|---|---|
  | `--limit` | int | 返回条数 | `0` → 50；>1000 钳到 1000 |
  | `--status` | string | 按状态过滤（取值见上，非法值 → `USAGE_ERROR`） | `""` |
  | `--since` | string | 只看该时长之内的记录（`1h`，`time.ParseDuration` 语法，非法 → `USAGE_ERROR`） | `""` |
- **data JSON**：
  ```json
  { "operations": [ { "<operation.Record>": "..." } ] }
  ```
  `operations` 不会为 null（空数组）。

### 8.2 `operation get <operation-id>`

- **命令标识**：`operation.get`
- **flags**：无专属（1 个位置参数 = operation id）。
- **data JSON**：单个 `operation.Record`（**直接是记录对象**，不带 `operations` 包装）。
- **行为要点**：id 不存在 → `NOT_FOUND`（9）。状态落盘持久化，SSH 断线后可继续按
  operation_id 查询；`running` 且持有者进程已不在的记录会被对账成 `unknown`
  （或已切文件时 `recovery_required`），保留最后完成的 phase（`result.last_phase`）。

---

## 9. 速查：命令标识与例外

| 命令 | 封套 command | data 类型来源 |
|---|---|---|
| site list / show / add / update / delete / enable / disable | `site.list` / `site.show` / `site.add` / `site.update` / `site.delete` / `site.enable` / `site.disable` | `view.SiteListData` / `view.SiteShowData` / §0.5 |
| config path | **无封套** | `configcmd.RuntimePaths` 裸 JSON |
| config show / validate / versions / diff / apply / rollback | `config.show` / `config.validate` / `config.versions` / `config.diff` / `config.apply` / `config.rollback` | `map` / `view.ConfigValidateData` / `cfgver.ListData` / `cfgdiff.Result` / §0.5 / §0.5 |
| status / version / start / stop / reload | 无 JSON（纯文本；错误时 `--json` 出错误封套，command 按参数推断） | — |
| service status / version / reload | `service.status` / `service.version` / `service.reload` | `view.ServiceStatus` / `{cli_version,server_version}` / §4.3 |
| runtime status / stats | `runtime.status` / `runtime.stats` | §5.1 / §5.2 |
| diagnose | `diagnose` | `diagnose.Report` |
| monitor status / events | `monitor.status` / `monitor.events` | `monview.Status` / `monview.EventsResult` |
| cert list / check | `cert.list` / `cert.check` | `view.CertListData` / `view.CertCheckData` |
| acme show | **无 JSON（纯文本）** | — |
| logs access / error / query | `logs.access` / `logs.error` / `logs.query` | `logquery.Result` |
| operation list / get | `operation.list` / `operation.get` | `{operations:[Record]}` / `Record` |
