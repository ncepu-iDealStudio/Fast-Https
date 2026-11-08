# 21. 独立 Monitor 进程

状态：✅ 已实现（第 6 步；规则与打包见后续步骤）

## 功能

`monitor` 是独立于 Server 的常驻进程。它按固定周期读本机 `GET 127.0.0.1:9088/runtime`，把原始快照追加到本地 JSON 行文件，并在三种情况下写一条事件：

| 事件 | 触发条件 | 级别 |
| --- | --- | --- |
| `collect_error` | 采集失败且失败类别变化（HTTP 非 200、JSON 不合 schema、`schema_version` 不认识） | warning |
| `unreachable` | 连续 3 次采集失败 | error |
| `recovered` | 写过 `unreachable` 之后第一次采集成功 | info |
| `restarted` | 快照里的 `server.pid` 或 `server.started_at` 变化 | warning |

Monitor 不进 Server 的请求路径，也不被 `fast-https start` 启动。停掉它，业务端口与 `/runtime` 都不受影响。

`/runtime` 连不上时，Monitor 还会读 PID 文件（默认 `fast-https.pid`，与组合根的 `WritePid` 同格式）区分三种状态，写进 `status.json` 的 `last_error` 与事件文案：

- `server not started (no pid file)`：从未启动；
- `server crashed (pid N is gone)`：PID 文件在、进程已退出；
- `server is starting (pid N alive, /runtime not answering)`：进程在但接口还没通，例如首次生成 CA。

## 使用

```powershell
# 单次采集，用于手工确认快照与状态文件
go run ./monitor -once

# 常驻
go run ./monitor -dir .\logs\monitor

# 指定配置文件
go run ./monitor -config .\config\monitor.json
```

配置文件是 Monitor 自己的，与站点配置 `fast-https.json` 无关。文件不存在时按默认值运行。

```json
{
  "interval": "5s",
  "timeout": "1s",
  "snapshot_url": "http://127.0.0.1:9088/runtime",
  "dir": "logs/monitor",
  "pid_file": "fast-https.pid",
  "event_max_bytes": 10485760,
  "event_keep": 3
}
```

优先级：配置文件 > 环境变量 `FASTHTTPS_MONITOR_DIR` > 默认值。字段缺失或写成空值等于「没设置」，回落默认值。`timeout` 不得大于 `interval`。

产物都在 `dir` 下：

| 文件 | 内容 |
| --- | --- |
| `snapshots.jsonl` | 每次成功采集的原始快照，一行一条 |
| `events.jsonl` | 事件，一行一条；单文件超过 `event_max_bytes` 就轮转成 `.1`、`.2`…，保留 `event_keep` 份 |
| `status.json` | Monitor 自身的状态，供 `fast-https monitor status` 只读 |

`status.json` 的 `status` 只有 `running` 与 `unhealthy`；`stale` 由读取方按时间差推导。`schema_version` 为 `"1"`。

## 实现方案

- `monitor/collector.go`：一次采集与分类。`FailKind` 把连不上、非 200、坏 JSON、版本不认识分开，避免混成一个「失败」。
- `monitor/runner.go`：跨轮次状态（连续失败数、是否已告警、上次 PID 与 `started_at`）与事件判定。`RunOnce` 是唯一入口，`Run` 只是按时钟驱动它。
- `monitor/events.go`：JSONL 追加、按大小轮转、`status.json` 原子写（临时文件 + rename）。
- `monitor/pid.go` + `proc_windows.go` / `proc_other.go`：PID 文件读取与进程存活判断。Windows 走 `OpenProcess` + 退出码，Unix 走信号 0。
- `monitor/clock.go`：可替换时钟，测试用假时钟推进，不真的等 5 秒。
- 不 import 仓库内任何其他包，`tests/arch/layering_test.go` 的 `monitor` 层只放行它自己。

## 部署（第一阶段）

只给 systemd 单元草案，随安装包发布的打包留到后续步骤。目录按包布局取 `/var/log/fast-https/monitor`，不新增基址。

```ini
[Unit]
Description=Fast-Https Monitor
After=network.target
# 不 Requires= fast-https：Server 没起时 Monitor 也要能跑，并写「不可达」

[Service]
Type=simple
ExecStart=/usr/bin/fast-https-monitor -dir /var/log/fast-https/monitor -config /etc/fast-https/monitor.json
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

Windows 与容器下由使用者自行托管；容器里 Monitor 与 Server 必须在同一网络命名空间，否则 `127.0.0.1:9088` 不可达。

## 测试

`go test ./monitor/`：采集成功与四类失败、超时、连续失败只告警一次、恢复、重启识别、PID 文件三种判断、事件与快照轮转、状态文件往返、配置与站点配置分离。

## 已知限制

- 只有「不可达 / 恢复 / 重启 / 采集异常」四类事件；5xx 比例、端口探测、疑似卡死属于后续步骤，尚未实现。
- 不读操作系统资源（内存、打开文件数、CPU）。
- 不发邮件、不调 Webhook、不自动修复，只写本地文件。
- 不区分「reload 后新增端口被占用导致进程退出」这种重启原因，一律按 `restarted` 记录。
- 快照文件也按与事件文件相同的上限轮转；需要长期留档时要额外导出。
