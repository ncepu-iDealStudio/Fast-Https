# 1. 边界测试用例集

按输入域分类的边界用例。`✅` 已有自动化测试锁定，`📋` 计划补测。
每类给出用例、期望行为与测试位置。

## 1.1 请求行与头部（runtime/request）

| 用例 | 输入 | 期望 | 状态 |
| --- | --- | --- | --- |
| 空请求 | `""` | None（连接静默关闭） | ✅ parse_test |
| 无终止 CRLF | `"GET / HTTP/1.1"` | UnknowInvalid → 400 | ✅ |
| 字段数 ≠3 | `"GET  HTTP/1.1"` | FirstLineInvalid → 400 | ✅ |
| 非法方法 | `"HIT / HTTP/1.1"` | MethodInvalid → 400 | ✅ |
| 非法协议 | `"GET / HTTP/9.9"` | ProtocolInvalid → 400 | ✅ |
| 头值裸 LF 注入 | `"Host: a\nEvil: x"` | InvalidHeaders → 400 | ✅ injection_test |
| 头名裸 CR 注入 | `"X-E\rFoo: bar"` | InvalidHeaders → 400 | ✅ |
| **请求行裸 LF** | `"GET /a\nHTTP/1.1: 1 HTTP/1.1"` | FirstLineInvalid → 400 | ✅ |
| 整包裸 LF 分行 | `"GET / HTTP/1.1\nHost: a\n\n"` | UnknowInvalid → 400（走私变体） | ✅ |
| 头名大小写 | `host:/`小写 | canonical 化后可读 | ✅ |
| 非法 CL | `Content-Length: abc / -5` | InvalidHeaders → 400 | ✅ |
| 头分片到达 | 头被拆成多个 TCP 段 | 循环读满再处理 | 📋（见覆盖评估 P1） |
| 超长头 | > max_header_size | 400，连接关闭 | 📋 |
| 无名头行 | `": value"` | InvalidHeaders | ✅ |

## 1.2 请求体（chunked / multipart / CL）

| 用例 | 输入 | 期望 | 状态 |
| --- | --- | --- | --- |
| CL 精确读 | body=N | 恰读 N 字节 | ✅ events_test |
| chunked 单读/跨读 | 标准 chunk 流 | 解码为原始体 | ✅ chunked_test |
| chunk 坏长度/超限 | `zz\r\n` / 超 max | ErrChunkedBody → 关闭 | ✅ |
| chunked 截断 | 流中途 EOF | 错误 → 500 路径 | ✅ proxy_read_test |
| multipart 终止边界 | 分块到达 | 增量扫描判完成 | ✅（Flush 清进度已锁） |
| CL 与实际不符 | 少发/多发 | RequestBodyValid false → 继续读/超时关 | 📋 |

## 1.3 响应与序列化（runtime/response）

| 用例 | 输入 | 期望 | 状态 |
| --- | --- | --- | --- |
| 上游响应头值裸 LF | `Set-Cookie: a\nEvil: x` | InvalidHeaders（响应注入防护） | ✅ response_test |
| 上游响应无终止空行 | 只有头 | 头合法解析、Body 零值 | ✅ |
| 首行 <3 字段 | `HTTP/1.1 200` | FirstLineInvalid → 509 路径 | ✅ |
| Date 格式 | 任意响应 | RFC 7231、秒缓存 | ✅ date_test |
| HEAD | HEAD 请求 | 保留 CL/CT、无 body | ✅ static body_test |

## 1.4 静态文件（modules/static）

| 用例 | 输入 | 期望 | 状态 |
| --- | --- | --- | --- |
| 路径穿越 | `/../../etc/passwd` | 404（Clean + root 包含校验） | ✅ traversal_test |
| 多层穿越 | `/a/../../x` | 404 | ✅ |
| 目录索引（Clean 后） | `GET /` | 命中 index.html | ✅ |
| 304 | If-Modified-Since ≥ mtime | 无正文 | ✅ conditional_test |
| Range 单段/开区间/后缀 | `0-4` / `5-` / `-3` | 206 + 正确切片 | ✅ |
| Range 越界/畸形 | `100-200` / 无横线 | 416 + `bytes */size` | ✅ |
| Range 多段 | `0-1,3-4` | 200 全量 | ✅ |
| Range×压缩 | Range + Accept-Encoding | 不压缩 | ✅ |
| Vary | 配置压缩 | `Vary: Accept-Encoding` | ✅ |

## 1.5 代理（modules/proxy）

| 用例 | 场景 | 期望 | 状态 |
| --- | --- | --- | --- |
| 连接池复用 | 同上游连续请求 | 同一条连接（不打日志验证则经 E2E） | ✅ pool_test |
| 池中过期连接 | 空闲 > TTL | 丢弃重拨 | ✅ |
| 复用连接首写失败 | 上游已关 | 换新连接重试一次 | ✅（逻辑单测） |
| 上游建连失败 | 端口不通 | 500 + error_page.proxy | ✅ E2E |
| chunked 上游响应 | TE: chunked | 解码 + 改写 CL | ✅ proxy_read_test |
| 📋 上游半响应 | 连接中途重置 | 500 / 关闭，不悬挂 | 📋 |
| 📋 上游慢响应 | >20s deadline | 超时关闭 | 📋 |

## 1.6 数值与解析边界

| 用例 | 输入 | 期望 | 状态 |
| --- | --- | --- | --- |
| 黑名单 IP 段反序 | `10.0.0.5-10.0.0.1` | 报错（不死循环） | ✅ counts_sweep_test |
| 黑名单超大段 | `0.0.0.0-255.255.255.255` | 报错（不 OOM） | ✅ |
| IPv6 地址匹配 | `[::1]:8080` | 正确取 host | ✅ |
| Range `bytes=0-` 空文件 | size=0 | 416（start≥size） | ✅ parseRange 语义 |
| 限流 rate=0 | 配置缺省 | 不限流（Inf） | ✅ everyFromRate |
| Content-Length 巨值 | `999999999999` | Atoi 溢出报错 → 400 | ✅（atoi 失败路径） |

## 1.7 并发与生命周期边界

| 用例 | 场景 | 期望 | 状态 |
| --- | --- | --- | --- |
| reload 与在途读并发 | 反复热更新 × 并发路由 | `-race` 干净 | ✅ hot_update_race |
| WAF 引擎重建并发 | BuildWAF × Allowed | `-race` 干净 | ✅ waf_test |
| 配置快照切换撕裂 | 50 次 reload × 并发读 | 永远完整新/旧 | ✅ reload_window |
| 双重优雅停机 | stopOnce | 只执行一次 | ✅ server 单测 |
| 控制面变更后 reload | overlay 存活 | 运行时变更不丢 | ✅ control_test |

## 补测方法约定

- 注入/协议类：直接喂字节流（参考 `injection_test.go` 的写法），断言错误码而非实现细节。
- 网络分片类：用 `net.Pipe` + 手工分段写（参考 `chunked_test.go`）。
- 上游异常类：`httptest.Server` 注入 `Flush` 后挂起/主动断连。
