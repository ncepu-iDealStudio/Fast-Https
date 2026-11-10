# 22. 独立 AI Agent

状态：✅ 第一阶段已实现（只读诊断与待审批）。自动摘除上游和证书续期未做。

## 功能

`agent` 是独立于 Server 的进程。它尾读 Monitor 的 `events.jsonl`，用固定参数调用本机 `fast-https` 的只读命令，并直接 `GET /runtime`。然后按事件类型写诊断和计划。

本阶段不调用模型，不执行 `config apply`、`service reload`、摘除上游、证书续期或重启。写操作在策略里是 `DENY`。需要人看的计划进入 `WAITING_APPROVAL`，可用 CLI 查询，还没有 `agent approve`。

停掉 Agent 不影响业务端口和 `/runtime`。Server 不 import `agent`。

| Monitor `type` | 结果 |
| --- | --- |
| `5xx_ratio`、`port_probe`、`stuck_suspected`、`certificate_expiring` | `WAITING_APPROVAL`，步骤为空 |
| `unreachable`、`collect_error`、无法识别的类型 | `ESCALATED` |
| `recovered` | 把同一次启动里未关闭的 `unreachable` 标为 `COMPLETED` |
| `restarted` | 只记录，`COMPLETED` |

`days_left` 低于 30 时，若事件文件里还没有这张证书的 `certificate_expiring`，Agent 会用 `cert list --json` 补一条。`cert list` 失败只记审计。

## 使用

```powershell
go run ./agent -once -monitor-dir .\logs\monitor -state-dir .\state\agent

go run ./agent -monitor-dir .\logs\monitor -state-dir .\state\agent
```

状态库默认是 `./state/agent/agent.db`，可用 `FASTHTTPS_AGENT_DIR` 或 `-state-dir` 覆盖。事件目录可用 `FASTHTTPS_MONITOR_DIR` 或 `-monitor-dir`。

查询不启动 Agent：

```powershell
fast-https agent status --json --state-dir .\state\agent
fast-https agent tasks --json
fast-https agent task get t_<event_id> --json
fast-https agent approvals --json
```

库文件不存在时 `agent status` 为 `not_running`，退出码 0。

## 实现方案

- `agent/` 只引用自己的子包、标准库和 `modernc.org/sqlite`。事件 ID 与 Monitor 的算法一致：已有 32 位十六进制就用，旧行按类型、级别、说明和快照里的 pid、`started_at` 补齐。
- 上下文写入 `agent_task.context_json`。`/runtime` 的 `schema_version` 不是 `"1"` 时记为 `unavailable`。CLI 参数必须整表命中白名单，不经过 Shell。
- `management/agentview` 用 SQLite 只读模式打开库，供 `cmd/agent.go` 查询。

## 测试

`go test ./agent/`：收件幂等、断点续读、坏行跳过、上下文白名单、证书阈值、诊断映射、注入文本不成为步骤、`recovered` 关闭对应的 `unreachable`。

`cmd/cli_agent_test.go`：库不存在、审批脱敏、任务 JSON。`tests/arch` 的 `TestAgentDoesNotImportServer` 拒绝 Agent 引用 `runtime`、`cmd`、`core`、`management`。

## 已知限制

- 不自动修改配置，不摘除上游，不续期证书。
- 不打包进 deb/rpm，也没有 systemd 单元。
- 不调用大模型。`agent/model` 只有接口，未配置时返回「模型不可用」。
- 待审批只能查看，不能在 CLI 里批准后执行。
