# 21. 独立 Monitor 进程

状态：✅ 已实现（第 6、7 步；第 8 步已进 deb / rpm / apk 与 tar.gz / Windows zip）

## 功能

`monitor` 是独立于 Server 的常驻进程。它按固定周期读本机 `GET 127.0.0.1:9088/runtime`，把原始快照追加到本地 JSON 行文件，并用几条确定性规则写事件。规则全是算术与阈值判断，不用模型，不做统计推断。

| 事件（`type` / `rule`） | 触发条件 | 级别 |
| --- | --- | --- |
| `collect_error` | 采集失败且失败类别变化（HTTP 非 200、JSON 不合 schema、`schema_version` 不认识） | warning |
| `unreachable` | 连续 3 次采集失败 | error |
| `recovered` | 写过 `unreachable` 之后第一次采集成功 | info |
| `restarted` | 快照里的 `server.pid` 或 `server.started_at` 变化 | warning |
| `5xx_ratio` | 最近 1 分钟的 5xx 增量 ÷ 请求增量超过阈值，且请求增量不少于下限 | warning |
| `port_probe`（可关） | `/runtime` 正常，但快照里的业务端口连不上 | warning |
| `stuck_suspected`（可关） | `requests.active` 长时间不降且 `requests.total` 不再增长 | info（只提示） |
| `certificate_expiring` | `cert_files` 里某张证书剩余天数低于 `cert_warn_days`（默认 30） | warning |
| `certificate_read_error` | `cert_files` 里的文件打不开或不是证书 PEM | warning |

同一条规则触发后进入冷却（默认 5 分钟），冷却期内不重复写。`recovered` 与 `restarted` 是状态跳变，每次跳变都记一条，不受冷却影响。

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
  "event_keep": 3,
  "cooldown": "5m",
  "five_xx_ratio": 0.05,
  "five_xx_min_requests": 20,
  "port_probe": true,
  "stuck_hint": true,
  "cert_files": [],
  "cert_warn_days": 30
}
```

优先级：配置文件 > 环境变量 `FASTHTTPS_MONITOR_DIR` > 默认值。字段缺失或写成空值等于「没设置」，回落默认值。`timeout` 不得大于 `interval`；`cooldown` 省略或写 `0` 时用默认的 5 分钟。

`port_probe` 与 `stuck_hint` **默认开启**：前者发现「接口活着但业务端口不通」，后者是 info 级别的提示。嫌噪声多就在配置里显式写 `false` 关掉——只写 `false` 能覆盖默认值，不会回落成开。

`cert_files` 默认是空列表，空列表不检查证书，也不读 `fast-https.json`。写了路径之后，剩余整天数低于 `cert_warn_days`（默认 30，与 `cert list` 的提醒天数相同）就写 `certificate_expiring`。消息里只有文件名和 `days_left`，没有 PEM 正文，也没有目录。同一文件、同一到期时刻在 `cooldown` 内只写一次。文件打不开或解析失败写 `certificate_read_error`，同样不带路径和文件内容。

## 规则口径

- **不可达 / 恢复**：连续 3 次采集失败写 `unreachable`；之后第一次成功写 `recovered`。失败类别变化时先写 `collect_error`，不必等三次。采集一断，5xx 统计窗口跟着作废，恢复后不拿断档前的样本算增量。
- **5xx 升高**：取最近 1 分钟内最早样本与本条样本的增量，`Δ5xx / Δ请求` 超过 `five_xx_ratio` 且 `Δ请求 ≥ five_xx_min_requests` 才告警。请求增量不足时不告警，避免除以一个很小的数。
- **进程重启**：`server.pid` 或 `server.started_at` 变化即记 `restarted`。计数回落不算负流量，窗口直接换新基线。
- **业务端口探测**（可关）：对快照 `listeners` 里的端口做本机 TCP 探测，`0.0.0.0` / 空主机按 `127.0.0.1` 处理，解析不出的端口跳过。只在端口从通变不通时写一条，端口表里不再存在的地址清掉状态。并发探测，单次超时 300ms。
- **疑似卡死**（可关）：`requests.active > 0` 且 `requests.total` 连续 1 分钟不增长时写一条提示，只提示不下结论。`active` 归零或 `total` 增长即复位。

事件字段：`time`、`type`、`rule`、`severity`、`message`，加一个 `snapshot` 摘要（状态、PID、`started_at`、累计请求、累计 5xx；窗口规则额外带 `requests_delta` 与 `responses_5xx_delta`）。不含请求 URI、上游地址、证书路径、黑名单。

## 产物

都在 `dir` 下：

| 文件 | 内容 |
| --- | --- |
| `snapshots.jsonl` | 每次成功采集的原始快照，一行一条 |
| `events.jsonl` | 事件，一行一条；单文件超过 `event_max_bytes` 就轮转成 `.1`、`.2`…，保留 `event_keep` 份 |
| `status.json` | Monitor 自身的状态，供 `fast-https monitor status` 只读 |

`status.json` 的 `status` 只有 `running` 与 `unhealthy`；`stale` 由读取方按时间差推导。`schema_version` 为 `"1"`。

## 实现方案

- `monitor/collector.go`：一次采集与分类。`FailKind` 把连不上、非 200、坏 JSON、版本不认识分开。
- `monitor/rules.go`：规则引擎 `Engine`。`Observe(obs)` 是唯一入口，接收一次采集结果、返回事件，时间取 `obs.At`，所以测试可以直接喂假时间序列。端口探测的 dialer 可替换。
- `monitor/runner.go`：驱动采集与落盘，不含任何规则判断。
- `monitor/events.go`：JSONL 追加、按大小轮转、`status.json` 原子写（临时文件 + rename）。
- `monitor/pid.go` + `proc_windows.go` / `proc_other.go`：PID 文件读取与进程存活判断。Windows 走 `OpenProcess` + 退出码，Unix 走信号 0。
- `monitor/clock.go`：可替换时钟，测试用假时钟推进，不真的等 5 秒。
- 不 import 仓库内任何其他包，`tests/arch/layering_test.go` 的 `monitor` 层只放行它自己。

## 部署

### 发行版包（deb / rpm / apk）

包内随主程序一起装四处：

| 路径 | 内容 |
| --- | --- |
| `/usr/bin/fast-https-monitor` | Monitor 二进制（`build/goreleaser.yaml` 的 `monitor-linux-pkg` 构建） |
| `/usr/lib/systemd/system/fast-https-monitor.service` | 独立单元 |
| `/etc/fast-https/monitor.json` | 阈值与采集周期，登记为 `config|noreplace` |
| `/var/log/fast-https/monitor/` | 快照、事件与状态，首次落盘时自建 |

桌面端包（`fast-https-desktop`）与主程序包内容一致，同样带这四处。`build/spec`（OpenEuler / COPR 路线）装二进制与 `monitor.json`；该 spec 目前不装任何 systemd 单元，与 Server 的现状一致。

单元内容以 `build/packaging/systemd/fast-https-monitor.service` 为准，要点：

- 不写 `Requires=` / `After=fast-https.service`：Server 没起时 Monitor 也要能跑，并把「不可达」如实写进事件。Server 单元也不引用 Monitor。
- `ExecStart` 用 `-config /etc/fast-https/monitor.json -dir /var/log/fast-https/monitor` 显式给出路径；Monitor 自己不做发行版判断。
- `Restart=on-failure`、`RestartSec=5s`；只声明 `LogsDirectory=fast-https`，事件子目录由 Monitor 自建。
- 不接 logrotate：快照与事件由 Monitor 自己按大小轮转。

卸载语义跟主程序一致：`apt remove` / `dnf remove` 保留配置、证书与事件；`apt purge` / `rpm -e` 清掉 `/etc/fast-https/monitor.json` 与 `/var/log/fast-https/monitor/`；升什么都不动。`prerm` 先停 Monitor 再停 Server，避免卸载本身在事件里留下一条假的「不可达」。

### 归档与 Windows

tar.gz 与 Windows zip 里只是多一个 `fast-https-monitor` 可执行文件（Windows 下为 `fast-https-monitor.exe`），默认值按工作目录解析（`./logs/monitor`、`./fast-https.pid`），不需要配置文件：

```bash
./fast-https-monitor -once                    # 单次采集
./fast-https-monitor -dir ./logs/monitor      # 常驻
```

Windows 安装包（`build/installer/windows/fast-https.nsi`）把解压好的发行目录整体复制进去，所以 `fast-https-monitor.exe` 随 zip 一起进 `setup.exe`，没有单独的组件或快捷方式。安装/升级与卸载前都会 `taskkill /F /IM fast-https-monitor.exe`：它由使用者手工启动、不在任何服务管理器里，不停掉会占着文件，表现为「升级后还是旧版本」或「卸载后目录还在」。Windows 下没有 systemd，也不打包 `monitor.json`，需要改阈值时按上表命令行参数或自建配置文件。

Windows 与容器下由使用者自行托管；容器里 Monitor 与 Server 必须在同一网络命名空间，否则 `127.0.0.1:9088` 不可达。

## 测试

`go test ./monitor/`：采集成功与四类失败、超时、连续失败只告警一次、恢复、重启识别、PID 文件三种判断、事件与快照轮转、状态文件往返、配置与站点配置分离；规则侧单独覆盖不可达/恢复、5xx 比例（含请求增量下限）、冷却、计数重置、端口探测（拒绝/可用/通配地址/非法端口/显式关闭）、疑似卡死（卡住/有进展/空闲）、事件字段白名单。

## 已知限制

- 不做耗时升高、内存增长这类窗口规则：内存、打开文件数与 CPU 不读，Server 侧也不采集。
- 不发邮件、不调 Webhook、不自动重试、不自动修复，只写本地文件。
- 端口探测只拨本机回环，不覆盖「跨机器访问不通」这类故障；被防火墙丢弃的连接会等到 300ms 超时才判失败。
- 不区分「reload 后新增端口被占用导致进程退出」这种重启原因，一律按 `restarted` 记录。
- 快照文件也按与事件文件相同的上限轮转；需要长期留档时要额外导出。
