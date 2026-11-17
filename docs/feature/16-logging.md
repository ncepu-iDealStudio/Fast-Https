# 16. 日志系统

状态：✅ 已实现

## 功能

控制台一路 + 文件四路（system/access/error/safe）全 JSON 结构化日志；访问日志字段按配置定制；reload 时日志文件原子重开。

## 使用

```json
{ "log_root": "./logs", "log_format": ["ip_port","time","type","method","path","host","status","size","user_agent"] }
```

文件：`logs/system.log`、`access.log`、`error.log`、`safe.log`。DEBUG/TRACE 走 stderr，其余 stdout。

## 实现方案（infra/logging/、runtime/dynlog/）

- **引擎**：zerolog（2026 年基准仍为最快 JSON 日志库）。五路 Logger；四路文件 Logger 以 `atomic.Pointer` 持有，`Open`（启动与 reload 均调用）整体换入，读取端经 `System()/Access()/Error()/Safe()` 拿到完整的新旧某一个。
- **控制台镜像**：`ConsoleLog` 的 Warn 及以上在写出控制台的同时写入 `error.log`，行内 `kind:"error"`、`source:"console"`。Info/Debug 不镜像。`Open` 之前文件目标不存在，直接跳过、不缓冲。
- **文件写入**：`syncWriter` 互斥串行化（多连接协程写同一文件不交错），`io.MultiWriter` 同时回显 stdout。
- **访问日志**（dynlog）：按 `log_format` 配置的字段函数表（atomic 整体替换，reload 安全）直接往 zerolog 事件上追加字段，零中间结构；`DynLogger` 由处理过程暂存 status/size。
- **安全日志**：限流/黑名单/WAF 类拒绝统一带 `reason` 字段。
- **拒绝日志（R-10）**：所有拒绝统一带 `reject_reason`（枚举）、`rule`（命中的配置项/规则名）、`remote_ip`（客户端 IP，不含端口）。安全类保留既有 `reason` 字段：`blacklist`、`rate_limit`、`location_limit`、`waf`、`host_check`（warn 与 enforce 都记）；数据面拒绝：路径穿越（`path_traversal` / `static_root`；Windows 保留设备名与尾点/尾空格规范化拒绝用 `invalid_path` / `windows_reserved_name`、`windows_path_normalization`）、隐藏路径（`hidden_path` / `hidden_path_segment`）、符号链接策略（`symlink_rejected` / `symlink_policy`）、超限（`header_too_large` / `max_header_size`、`body_too_large` / `max_body_size`，h1 与 h2 共用）、未知 SNI（`unknown_sni` / `tls.sni`，同时写安全日志与控制台/`error.log`）。压缩跳过用 debug：`compression_skipped`，rule 为 `zip_min_length`、`zip_skip_types` 或 `accept_encoding`（客户端没要），静态文件与代理响应共用同一口径。日志不记录原始请求路径；`logs query --reject-reason` 可按枚举过滤。
- **reload**：先开新文件再关旧文件，新目录打不开时旧文件继续可写。

## 测试

`tests/unit_test/infra/logging`（五路格式、目录切换、失败保持旧文件、控制台 Warn/Error 镜像到 error.log）。

## 已知限制

- 文件无轮转（按天/大小切割未实现，需外部 logrotate）。
- 访问日志字段集固定 9 种可选，无自由模板语法。
- 控制台 Warn/Error 会同时出现在 stdout（或 daemon 模式下的 `daemon.log` / journald）与 `error.log`。采集两侧时同一条错误会有两份。
