# CLI 运行示例

以下均在仓库根目录 `e:\Open_source_project\fast-https\fast-https-src`（归档布局）于 **2026-11-08** 实际执行：`go build` 后运行二进制，**服务未启动**（除注明外）。stdout 为完整输出；exit code 单独列出。

环境：Windows 10，`config/fast-https.json` 存在且校验通过。

## `diagnose --json`

```text
{"ok":true,"command":"diagnose","request_id":null,"data":{"observed_at":"2026-11-08T12:24:29Z","overall":"warning","checks":[{"name":"service","status":"warning","message":"service is not running"},{"name":"config","status":"ok","message":"configuration is valid"},{"name":"config_drift","status":"unavailable","message":"no running process to compare against"},{"name":"certs","status":"ok","message":"1 certificate file(s) valid"},{"name":"ports","status":"warning","message":"none of 2 configured port(s) reachable"},{"name":"logs","status":"ok","message":"0 error record(s) in the last hour"},{"name":"runtime","status":"warning","message":"runtime snapshot unreachable"},{"name":"monitor","status":"ok","message":"monitor is not installed"},{"name":"state","status":"warning","message":"state directory does not exist yet"}],"report_id":"diag_20261108T122429Z_7062401a"},"warnings":[]}
```

退出码：`0`

## `runtime status --json`（服务未运行）

```text
{"ok":false,"command":"runtime.status","request_id":null,"error":{"code":"SERVICE_UNREACHABLE","message":"runtime snapshot unreachable: Get \"http://127.0.0.1:9088/runtime\": dial tcp 127.0.0.1:9088: connectex: No connection could be made because the target machine actively refused it.; the runtime endpoint binds 127.0.0.1:9088 and is skipped when that address is occupied","details":[]}}
```

退出码：`4`

## `monitor status --json`（Monitor 未安装）

```text
{"ok":true,"command":"monitor.status","request_id":null,"data":{"status":"not_installed","observed_at":"2026-11-08T20:24:29+08:00","source":"monitor_status_file"},"warnings":[]}
```

退出码：`0`

## `config validate --json`

```text
{"ok":true,"command":"config.validate","request_id":null,"data":{"valid":true,"file":"./config/fast-https.json","candidate":false},"warnings":[]}
```

退出码：`0`

## `operation list --json`

```text
{"ok":true,"command":"operation.list","request_id":null,"data":{"operations":[]},"warnings":[]}
```

退出码：`0`

## `cert list --json`

```text
{"ok":true,"command":"cert.list","request_id":null,"data":{"renew_before_days":30,"certificates":[{"domains":["localhost"],"file":"E:\\Open_source_project\\fast-https\\fast-https-src\\config\\cert\\localhost.pem","status":"valid","subject":"localhost","issuer":"fast-https fast-https@ncepu.edu.cn","names":["localhost"],"not_before":"2026-06-23T04:53:10Z","not_after":"2028-09-23T04:53:10Z","days_left":684,"self_signed":false,"local_ca":true,"acme_managed":false,"deployed_by":["default"]}]},"warnings":[]}
```

退出码：`0`

## `logs error --json --limit 3`

```text
{"ok":true,"command":"logs.error","request_id":null,"data":{"records":[],"source":{"file":"error.log","scanned_bytes":0},"time_range":{},"range_complete":true,"scan_truncated":false,"output_truncated":false,"truncated":false,"limit":3},"warnings":[]}
```

退出码：`0`

## `service status --json`

```text
{"ok":true,"command":"service.status","request_id":null,"data":{"status":"stopped","stale_pid":true,"observed_at":"2026-11-08T20:25:22+08:00","source":"pid_file+port_probe","ports":[{"port":80,"reachable":false},{"port":443,"reachable":false}]},"warnings":[]}
```

退出码：`0`。`stale_pid` 来自工作目录里残留的 pid 文件。

## `monitor status --json`（已采集，Server 未监听 9088）

2026-11-08：临时目录里放一份 `monitor.json`（`pid_file` 指向不存在的文件），执行 `go run ./monitor -once -config <该文件> -dir <临时目录>`。采集失败，Monitor 退出码 1。再把 `FASTHTTPS_MONITOR_DIR` 指到该目录，执行 `go run . monitor status --json`。

```text
{"ok":true,"command":"monitor.status","request_id":null,"data":{"status":"unhealthy","observed_at":"2026-11-08T20:45:54+08:00","source":"monitor_status_file","version":"dev","pid":2708,"healthy":false,"last_collect_at":"2026-11-08T20:45:54.011+08:00","last_event_at":"2026-11-08T20:45:54.011+08:00","last_error":"unreachable: Get \"http://127.0.0.1:9088/runtime\": dial tcp 127.0.0.1:9088: connectex: No connection could be made because the target machine actively refused it.; server not started (no pid file)"},"warnings":[]}
```

退出码：`0`。`unhealthy` 表示状态文件在、最近一次采集失败；上面的 `not_installed` 表示没有状态文件。`version` 为 `dev` 是因为这次没有用 `-ldflags` 注入版本。
