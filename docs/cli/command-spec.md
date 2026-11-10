# fast-https CLI 命令契约

本文是管理类命令的稳定契约索引：命令标识、优先级、JSON 封套、错误码与退出码。人类可读说明见 [usage.md](../usage.md) 第 10 节；实现细节见 [feature/18-cli-service.md](../feature/18-cli-service.md)。

## JSON 封套

成功时 stdout 为单个 JSON 对象：

```json
{
  "ok": true,
  "command": "<stable.id>",
  "request_id": null,
  "data": {},
  "warnings": []
}
```

失败时：

```json
{
  "ok": false,
  "command": "<stable.id>",
  "request_id": null,
  "error": {
    "code": "<CODE>",
    "message": "...",
    "details": []
  }
}
```

全局标志：`--json`（机器输出）、`--timeout`、修改类命令的 `--request-id`；破坏性写操作需 `--yes`（`--confirm` 同义），`--dry-run` 不写生产状态。

## 退出码（管理类命令）

| 码 | 典型 `error.code` | 含义 |
|---:|---|---|
| 0 | — | 成功 |
| 1 | `INTERNAL` 等 | 未分类内部错误 |
| 2 | `USAGE` | 参数或用法错误 |
| 3 | `PERMISSION_DENIED` | 权限不足 |
| 4 | `SERVICE_UNREACHABLE` | 服务未运行或端点不可达 |
| 5 | `CONFIG_INVALID` | 配置校验失败 |
| 6 | `OPERATION_FAILED` | 操作执行失败 |
| 7 | `OPERATION_UNKNOWN` | 状态未知，应查 operation |
| 8 | `TIMEOUT` | 超时 |
| 9 | `NOT_FOUND`、`LOG_SOURCE_UNAVAILABLE` | 资源不存在 / 日志不可用 |
| 10 | `CONFLICT`、`REQUEST_ID_CONFLICT` | 冲突或幂等键冲突 |
| 11 | `UNMET_DEPENDENCY`、`NOT_SUPPORTED` | 依赖不满足 / 未实现 |

旧顶层 `start`、`stop`、`reload`、`status` 仍使用原文案与非表内退出码；见 `cmd/compat_test.go`。

## P0 命令

| 命令 | `command` 字段 | 状态 |
|---|---|---|
| `service status` | `service.status` | 已实现 |
| `service version` | `service.version` | 已实现 |
| `runtime status` | `runtime.status` | 已实现（需本机 `/runtime`） |
| `runtime stats` | `runtime.stats` | 已实现；`--since`/`--domain` → `NOT_SUPPORTED` |
| `config show` | `config.show` | 已实现 |
| `config validate` | `config.validate` | 已实现 |
| `config diff` | `config.diff` | 已实现 |
| `config apply` | `config.apply` | 已实现（事务 + operation） |
| `config versions` | `config.versions` | 已实现 |
| `config rollback` | `config.rollback` | 已实现 |
| `operation list` | `operation.list` | 已实现 |
| `operation get` | `operation.get` | 已实现 |
| `logs error` | `logs.error` | 已实现 |
| `diagnose` | `diagnose` | 已实现 |

## P1 命令

| 命令 | `command` 字段 | 状态 |
|---|---|---|
| `site list` … `site disable` | `site.*` | 已实现（写操作走事务） |
| `cert list` / `cert check` | `cert.list` / `cert.check` | 已实现 |
| `cert renew` | `cert.renew` | 已实现。本机 `127.0.0.1:9089`，令牌文件 `renew.token`，必须 `--yes` |
| `logs access` / `logs query` | `logs.*` | 按规范范围实现情况见 usage §10 |
| `monitor status` | `monitor.status` | 已实现（只读文件；无 Monitor 时为 `not_installed`） |
| `monitor events` | `monitor.events` | 已实现 |

## 环境与路径

| 变量 | 作用 |
|---|---|
| `FASTHTTPS_STATE_DIR` | 状态目录（锁、配置版本、operation） |
| `FASTHTTPS_RUNTIME_URL` | 覆盖默认 `http://127.0.0.1:9088/runtime` |
| `FASTHTTPS_RENEW_URL` | 覆盖默认 `http://127.0.0.1:9089/renew` |
| `FASTHTTPS_MONITOR_DIR` | 覆盖 Monitor 状态目录（默认 `<log_root>/monitor`） |
| `FAST_HTTPS_LANG` / `--lang` | `en` / `zh` 帮助与部分文案 |

包布局下状态目录为 `config.STATE_DIR`（`/var/lib/fast-https`）；归档布局为 `./state`。

## operation 与 `--request-id`

适用：`service reload`、`config apply`、`config rollback`、`site add/update/delete/enable/disable`、`upstream drain`、`upstream undrain`、`cert renew`。`cert renew` 的幂等记录写在 Server 进程里：同一个 `request_id` 不会向 CA 发第二次。

- 相同 `request_id` + 相同参数：返回已有记录，不重复副作用。
- 相同 `request_id` + 不同参数：退出码 10，`REQUEST_ID_CONFLICT`。
- 未传时 CLI 生成并在 JSON 中回显。
- Unix 事务期间忽略 `SIGHUP`/`SIGPIPE`，便于 SSH 断线后 `operation get` 对账。

## 安全与数据边界

- 输出经 `management/redact` 脱敏；站点 JSON 非 `--json` 模式可能含明文口令。
- 日志查询只返回数据，不执行日志内文本；见 `tests/arch` 与 `cli_s2_test.go`、`cli_s12_test.go`。
- `management/` 不依赖 `runtime/` 或 `core`；`/runtime` 经 `management/rtclient`。

## 测试映射（节选）

| 验收主题 | 测试 |
|---|---|
| JSON 封套与退出码 | `cmd/cli_test.go` |
| 旧命令兼容 | `cmd/compat_test.go` |
| 只读与脱敏 | `cmd/cli_s2_test.go` |
| 事务与 apply | `cmd/cli_s8_test.go`、`management/txn` |
| runtime / reload 验证 | `cmd/cli_s11_test.go`、`management/svcctl`、`management/rtclient` |
| monitor 文件视图 | `cmd/cli_s12_test.go`、`management/monview` |
| 无 exec | `tests/arch` `TestNoExecInProduction` |

完整列表见 [18-cli-service.md](../feature/18-cli-service.md#测试)。
