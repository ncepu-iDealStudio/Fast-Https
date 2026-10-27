# 13. 安全防护：黑名单与限流

状态：✅ 已实现

## 功能

连接级 IP 黑名单（单 IP 与 IP 段）、进程级 per-IP 令牌桶限流、location 级请求计数限流。

## 使用

```json
"http": {
  "blaklist": ["10.0.0.1", "192.168.1.1-192.168.1.100"],
  "servers_limit": { "limit": 100, "burst": 200 }
}
```

```json
"limit": { "mem": 16, "rate": 50 }   // location 级
```

## 实现方案（modules/safe/）

- **黑名单**（blacklist.go）：IP 段展开为单 IP 列表（上限 65536 个，防 OOM；起止反序报错防死循环）；`net.SplitHostPort` 取主机部分，IPv6 正确；拒绝路径回 403 + 安全日志。
- **全局限流**（limit.go）：**per-IP 令牌桶**（`golang.org/x/time/rate`），客户端之间互不抢占 burst；空闲 5 分钟的桶周期清扫，map 不无界增长。拒绝回 429 + 安全日志。
- **location 计数限流**（counts.go）：按 location `ID` 定位 `CountLimit`（atomic 快照整体替换，reload 安全），每 IP 秒窗计数，"先比较后自增"（rate 次就是 rate 次）；条目每秒清扫过期回收。拒绝回 429 + 安全日志。
- 拒绝语义统一：安全日志（SafeLog）带 `reason`（blacklist/rate_limit/location_limit）。

## 测试

`modules/safe/`：blacklist（段展开/反序/超大段）、counts_sweep（清扫/计数语义）、limit_ip（per-IP 独立/清扫）、policy_test（reload 重建并发）。

## 已知限制

- 黑名单匹配为线性扫描（段展开后有上限，大黑名单性能需评估，可换 radix）。
- 黑名单/限流拒绝路径会吞掉客户端已发数据（`ev.Conn.Read`）以保证响应可达，属协商副作用。
