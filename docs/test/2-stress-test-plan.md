# 2. 压力测试计划（4C4G / 3Mbps 出带宽）

目标环境：单台云服务器（4 vCPU / 4GB 内存 / 公网带宽 3Mbps），fast-https 与上游/静态站点同机部署。压测客户端尽量从另一台机器发起；若只有一台，用本机回环压测（结论只对 CPU/内存有效，带宽结论以 3Mbps 折算）。

> 3Mbps ≈ 375KB/s 出流量——带宽是本环境第一瓶颈。计划因此分为
> **本机回环（测 CPU/内存/连接处理）** 与 **跨机（测真实吞吐）** 两层。

## 2.1 环境准备

### 服务端配置

```json
{
  "http": {
    "server": [
      { "listen": "18080", "server_name": "static.local",
        "location": [{ "url": "/", "type": "local", "root": "./bench/root",
                       "index": ["index.html"], "zip": ["gzip", "br"] }] },
      { "listen": "18081", "server_name": "proxy.local",
        "location": [{ "url": "/", "type": "proxy",
                       "proxy_pass": "http://127.0.0.1:19000" }] }
    ]
  }
}
```

### 静态资源矩阵（bench/root/）

| 文件 | 大小 | 类型 | 用途 |
| --- | --- | --- | --- |
| `small.txt` | 1KB | text | 内建开销占比（响应头 > body） |
| `index.html` | 10KB | html | 典型页面 |
| `app.js` | 100KB | js（高重复） | gzip/br 压缩收益 |
| `img.jpg` | 500KB | 二进制（随机字节不可压） | sendfile/零压缩路径、带宽瓶颈 |
| `big.bin` | 5MB | 随机 | Range/大文件传输 |

生成：`head -c 1048576 /dev/urandom > big.bin` 等；`app.js` 用重复文本块。

### 上游（反代目标）

同机起 Go 静态服务（模拟真实上游延迟可选 5ms）：

```go
// upstream.go — 19000 端口，serve 同一 bench/root
```

或直接复用一个 fast-https 实例当上游（两端口互压）。

### 工具

- 压测：`wrk`（首选）或 `hey`；连接数压测：`wrk -c`/`--latency`
- 监控：`vmstat 1`、`pidstat -p <pid> 1`（CPU/内存）、`ss -s`（连接）、`curl -w`（TTFB）
- 服务端：dev 模式跑（pprof 在 10000 端口：`go tool pprof http://127.0.0.1:10000/debug/pprof/profile?seconds=30`）

## 2.2 测试用例

### A. 静态资源（18080）

| # | 用例 | 命令（示例） | 关注指标 |
| --- | --- | --- | --- |
| A1 | 小文件高并发 | `wrk -t4 -c200 -d60s http://IP:18080/small.txt` | RPS、p99、CPU% |
| A2 | 典型页面 | 同上 `/index.html` | RPS、p99 |
| A3 | 可压大文件+gzip | 同上 `/app.js`（wrk 默认带 Accept-Encoding: gzip） | RPS、压缩 CPU 占比、传输字节 |
| A4 | 不可压二进制 | `/img.jpg` | 带宽饱和点（3Mbps→~45 req/s 理论上限） |
| A5 | 304 命中 | 先取 Last-Modified，wrk 脚本注入 If-Modified-Since | RPS 提升、无正文 |
| A6 | Range 抽样 | 脚本随机 `bytes=X-Y` | 正确性（206）+ 吞吐 |
| A7 | keep-alive vs close | wrk 默认 keep-alive；`Connection: close` 对照 | 复用收益 |

### B. 反向代理（18081）

| # | 用例 | 命令/要点 | 关注指标 |
| --- | --- | --- | --- |
| B1 | 小响应代理 | `wrk -t4 -c200 -d60s http://IP:18081/small.txt` | RPS、与 A1 差值（代理税） |
| B2 | 页面代理 | `/index.html` | p99、上游池命中（连接数稳定） |
| B3 | 大文件代理 | `/img.jpg` | 吞吐（预期同 A4 带宽顶） |
| B4 | 连接池验证 | B1 进行时 `ss -tn | grep 19000` | 上游连接数 ≪ 客户端并发数（复用生效） |
| B5 | 上游 5ms 延迟 | 上游注入 sleep | p99 劣化、并发承载 |
| B6 | 混合读写 | 50% GET 页面 + 50% POST 1KB | POST 路径（CL 读）稳定性 |

### C. 稳定性与边界

| # | 用例 | 方法 | 期望 |
| --- | --- | --- | --- |
| C1 | 长稳 | A1+B1 各 30 分钟 | 无内存增长（RSS 稳定）、无 FD 泄漏（`ls /proc/<pid>/fd \| wc -l` 平稳） |
| C2 | 慢客户端 | `curl --limit-rate 1k /img.jpg` 并发 100 | 不拖垮其他请求（p99 对比基线） |
| C3 | 连接风暴 | `wrk -c2000 -d10s`（短连接 `-H "Connection: close"`） | accept 不丢、限流器桶回收（Snapshot().Buckets 回落） |
| C4 | reload 中压测 | B1 持续中执行 5 次 reload | 无失败突刺（快照切换）、端口不丢 |
| C5 | 恶意边界 | `wrk` 脚本发畸形头/超大头/注入载荷 | 全部 400、进程存活、WAF 命中计入 Snapshot |
| C6 | 并发下载 Range | 100 并发分片拉 `big.bin` | 完整拼回原文件（正确性） |

## 2.3 执行顺序与判定

1. **基线**：A1/A2/B1/B2 各 3 轮取中位数（首轮预热丢弃）。
2. **能力扫描**：并发阶梯 50→100→200→500→1000，找 RPS 拐点与错误率抬头点。
3. **专项**：A3-A6、B3-B6、C1-C6。
4. **判定参考**（4C4G 同机回环，校验用，非硬指标）：
   - 小文件静态 RPS ≥ 20k（回环）；代理 ≥ 静态的 60%
   - p99 < 50ms（回环）；错误率 = 0（B5/C2 允许 <0.1% 超时）
   - 30 分钟 RSS 增长 < 10%；FD 稳定
   - 跨机 3Mbps：4xx/5xx = 0，img.jpg 单流 ≈ 45 req/s 或单连接跑满 375KB/s
5. **产出**：每用例记录 RPS/p50/p99/p999/error%/RSS/CPU，汇总表 +
   拐点结论 +（发现瓶颈时）pprof 火焰图快照。

## 2.4 已知预期（写进结论用）

- 3Mbps 带宽下大文件吞吐测试的瓶颈在网卡不在服务；CPU 结论以回环为准。
- gzip(br) 压缩在 100KB+ 文件上是 CPU 换带宽：观察 A3 的 CPU% 与 A4 对比。
- 连接池在 B1 下应把上游 TCP 连接稳定在个位数；若观察到大量新建（TIME_WAIT 激增）即为回归。
- C2 慢客户端受限于每连接 30s 总 deadline 与 goroutine-per-conn 模型，100 个慢客户端预期占用 100 goroutine、内存线性可预期。
