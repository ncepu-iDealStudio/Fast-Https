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
- **文件写入**：`syncWriter` 互斥串行化（多连接协程写同一文件不交错），`io.MultiWriter` 同时回显 stdout。
- **访问日志**（dynlog）：按 `log_format` 配置的字段函数表（atomic 整体替换，reload 安全）直接往 zerolog 事件上追加字段，零中间结构；`DynLogger` 由处理过程暂存 status/size。
- **安全日志**：限流/黑名单/WAF 类拒绝统一带 `reason` 字段。
- **reload**：先开新文件再关旧文件，新目录打不开时旧文件继续可写。

## 测试

`tests/unit_test/infra/logging`（五路格式、目录切换、失败保持旧文件）。

## 已知限制

- 文件无轮转（按天/大小切割未实现，需外部 logrotate）。
- 访问日志字段集固定 9 种可选，无自由模板语法。
