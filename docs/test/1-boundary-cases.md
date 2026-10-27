# 1. 边界测试用例集

按输入域分类的边界用例。`✅` 已有自动化测试锁定，`📋` 计划补测。
每类给出用例、期望行为与测试位置。

> 2026-10-25：P1/P2/P3 三大补测缺口（头分片到达、上游异常矩阵、缓存磁盘恢复）
> 已全部落地，且用真实公开语料（httparse 测试向量、payload-box 走私 payload）
> 完成了一轮覆盖性回打——详见 [1.8 公开语料覆盖](#18-公开语料覆盖真实用例)。
> 本轮语料回打共修掉 5 个真实缺陷，见各节 ✅(本轮) 标注。

## 1.1 请求行与头部（runtime/request）

| 用例 | 输入 | 期望 | 状态 |
| --- | --- | --- | --- |
| 空请求 | `""` | None（连接静默关闭） | ✅ parse_test |
| 无终止 CRLF | `"GET / HTTP/1.1"` | NeedReadMore → 继续读（修复前误 400） | ✅(本轮) corpus_test |
| 字段数 ≠3 | `"GET  HTTP/1.1"` | PathInvalid → 400 | ✅ |
| 非法方法 | `"HIT / HTTP/1.1"` | MethodInvalid → 400 | ✅ |
| 非法协议 | `"GET / HTTP/9.9"` | ProtocolInvalid → 400 | ✅ |
| 头值裸 LF 注入 | `"Host: a\nEvil: x"` | InvalidHeaders → 400 | ✅ injection_test |
| 头名裸 CR 注入 | `"X-E\rFoo: bar"` | InvalidHeaders → 400 | ✅ |
| **请求行裸 LF** | `"GET /a\nHTTP/1.1: 1 HTTP/1.1"` | FirstLineInvalid → 400 | ✅ |
| 整包裸 LF 分行 | `"GET / HTTP/1.1\nHost: a\n\n"` | UnknowInvalid → 400（走私变体） | ✅ |
| 孤立 CR 分行 | `"GET / HTTP/1.1\r\rHost: a\r\r"` | UnknowInvalid → 400 | ✅ |
| 头名大小写 | `host:/`小写 | canonical 化后可读 | ✅ |
| 非法 CL | `Content-Length: abc / -5` | InvalidHeaders → 400 | ✅ |
| **头分片到达** | 头拆成 8 个 TCP 段（含 CRLF 跨段切断） | 循环读满再处理，不误 400 | ✅(本轮) events/fragment_test |
| 分片前缀排列 | 合法请求的任意前缀 | 全部 NeedReadMore | ✅(本轮) corpus_test（partial_permutations） |
| **请求行前空行** | `\r\n\r\nGET / ...` | 跳过后正常解析（RFC 7230 §3.5） | ✅(本轮) corpus_test |
| **冒号前空白** | `"Name : value"` | InvalidHeaders（RFC 7230 §3.2.4 MUST） | ✅(本轮) corpus_test |
| obs-fold 折行残迹 | `"Name\r\n: value"` / 行首空格头行 | InvalidHeaders | ✅(本轮) corpus_test |
| 超长头 | > max_header_size | 400，连接关闭 | 📋 |

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
| **上游裸 LF 状态行** | `HTTP/1.0 403 Forbidden\nServer: x\n\n` | FirstLineInvalid（响应拆分通道封死） | ✅(本轮) response_corpus_test |
| 首行 <2 字段 | `HTTP/1.1`（无状态码） | FirstLineInvalid → 509 路径 | ✅(本轮) |
| **无 reason 的状态行** | `HTTP/1.1 200\r\n\r\n` | 接受，reason 为空（修复前误拒） | ✅(本轮) |
| **多词 reason 截断** | `HTTP/1.1 503 Service Unavailable` | reason 整体保留（修复前截成 "Service"） | ✅(本轮) 往返断言 |
| **冒号前空白（响应侧）** | `Access-Control-Allow-Credentials : true` | InvalidHeaders（与请求侧一致） | ✅(本轮) |
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
| **上游半响应（keep-alive）** | CL=100 只给 10 字节后关连接 | 错误上抛 → 500，不交付截断 200（修复前 ReadBytes 自旋） | ✅(本轮) upstream_matrix_test M1 |
| 上游头未到齐即关闭 | 连接建立后立即关 | 错误上抛 | ✅(本轮) M2 |
| **上游真实 RST** | SO_LINGER=0 重置 | 错误上抛，不悬挂 | ✅(本轮) M3 |
| **上游慢响应** | body 超读 deadline | ProxyErrorReadFromUpstreamTimeout（修复前交付半截 body） | ✅(本轮) M4 |
| 对照组：健康分片响应 | 头/体多段到达 | 正常拼回 | ✅(本轮) M5 |
| Connection:close EOF | 读到上游关闭 | 收齐数据正常返回 | ✅(本轮) M6/M7 |

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
- 网络分片类：用 `net.Pipe` + 手工分段写（参考 `chunked_test.go`、`events/fragment_test.go`）。
- 上游异常类：`net.Pipe` 模拟 EOF/挂起，真实 RST 用回环 `SO_LINGER=0`
  （参考 `modules/proxy/upstream_matrix_test.go`），看门狗协程兜底防自旋回归。
- 缓存磁盘类：手工构造 gob 文件（合法/损坏/旧格式/过期）后 `LoadCache`
  （参考 `modules/cache/load_cache_test.go`）。

## 1.8 公开语料覆盖（真实用例）

用例不来自模型内部知识，全部取自公开权威语料库（2026-10 抓取）：

| 语料库 | 内容 | 测试位置 |
| --- | --- | --- |
| [seanmonstar/httparse](https://github.com/seanmonstar/httparse) `src/lib.rs` 内嵌向量（继承 picohttpparser） | 请求向量 30 条（含 Partial/Err 全谱）、响应向量 14 条、chunk 尺寸向量 8 条、前缀排列性质 | `tests/unit_test/runtime/request/corpus_test.go`、`.../response/response_corpus_test.go` |
| [payload-box/http-request-smuggling-payloads](https://github.com/payload-box/http-request-smuggling-payloads) `Intruder/Detection-Payloads.txt` + `Intruder/TE-TE-Payloads.txt` | CL.TE/TE.CL 检测型、TE.TE 混淆变体族（冒号前空白、obs-fold 折行、大小写、引号、空格断词、HTTP/2 降级文本、伪头） | 同上 `smugglingCases` / `TestSmuggledPrefixNeverParsed` |

与 httparse 的两处**刻意分歧**（防御性收紧，非缺陷）：

1. 裸 LF 分行的请求/响应一律拒绝（httparse 接受）：CRLF 是唯一合法行终止，
   拒绝裸 LF 消灭整类走私/响应拆分向量。
2. 行未结束时的字段错误返回 NeedReadMore（= httparse 的 Partial 语义），
   行结束后才给出 ProtocolInvalid 等终态判定。

反走私立场：语法合法的 CL+TE/TE+TE 混淆不靠"识别恶意"防御——
解析层拒绝 RFC 明确要求拒绝的形式（§3.2.4 冒号前空白、obs-fold），
重复头折叠为单值后经 `ByteRow` 重建报文，上游看到的就是我们解析到的，
前后端不存在第二种解读。

### 本轮语料回打修掉的缺陷

| # | 缺陷 | 位置 | 后果（修复前） |
| --- | --- | --- | --- |
| 1 | 无 CRLF 缓冲一律判 UnknowInvalid | `runtime/request/parse.go` | 头分片到达（首段不含完整行）被误 400 |
| 2 | 头名两侧空白被 TrimSpace 放行 | 同上 / `runtime/response/response.go` | `Name : value` 混淆走私形态放行（违反 RFC 7230 §3.2.4 MUST 拒绝） |
| 3 | 响应 reason 只取 `parts[2]`；无 reason 无空格的行误拒 | `runtime/response/response.go` | `Service Unavailable` 截断成 `Service`；`HTTP/1.1 200\r\n` 误 509 |
| 4 | `ReadBytes` 非超时错误不退出循环 | `modules/proxy/proxy_read.go` | keep-alive 半响应/重置 → 无限自旋吃满 CPU；超时交付半截 body（缓存投毒面） |
| 5 | `LoadCache` 恢复过期条目进索引 | `modules/cache/cache.go` | Touch 读即续期把过期条目刷成"永不过期"，陈旧内容无限回放 |

### 全模块补测（第三轮）修掉的缺陷

| # | 缺陷 | 位置 | 后果（修复前） |
| --- | --- | --- | --- |
| 6 | **上游写错误非 net.Error 时走 Fatal（os.Exit）** | `modules/proxy/event_proxy.go` sendToUpstreamServer | P0：`io.ErrClosedPipe` 等一条上游连接的异常写直接打死整个进程 |
| 7 | `DecompressBytesGzip` 死代码残留 | `modules/compress/compress.go` | 无调用方，误导维护者 |

### 全模块补测（第三轮）新增用例位置

| 模块 | 测试文件 | 覆盖点 |
| --- | --- | --- |
| modules/auth | `auth_test.go` | Basic 解析畸形矩阵、常数时间比较、401 写出、放行 |
| modules/rewrite | `rewrite_test.go` | 301 + Location + 关闭 |
| modules/dev_mod | `dev_mod_test.go` | dev 应答主路径 |
| modules/safe | `counts_test.go` | per-IP 计数限流全语义（rate 精确边界/窗口重置/超额直通/清扫节流/表构建/429 写出） |
| modules/appfirewall | `adapter_test.go` 追加 | isKnownDetector 边界 |
| modules/proxy | `flow_test.go`、`cache_flow_test.go` | 构造/ChangeHeader(200/509)/ChangeHead($变量)/DialFailureResponse/明文与 TLS 读写闭包/池借还与复用/websocket 整条持有/ProxyInit/缓存 Serve·Store·逐跳头剔除·no-cache 指令 |
| modules/proxy_tcp | `event_proxy_tcp_test.go` 追加 | 拨号失败关连接不 panic |
| modules/websocket | `websocket_test.go` 追加 | 坏 CircleData 保护分支、上游断开收尾 |
| modules/static | `flow_test.go` | HandelSlash 301、StaticEvent keep-alive/close/404、event301 |
| runtime/events | `h2_helpers_test.go`、`eventhandler_test.go` | h2 助手全套、EventHandler 全链路分发、GFilterHostPath 回退 DEFAULT_PORT |
| runtime/dynlog | `dynlog_test.go` | pathTypeName 全枚举 |
| runtime/request | `parse_test.go` 追加 | query 变体/ParseHost 端口/RequestBodyValid 四分支/取值器/TryFixBody |
| runtime/response | `response_corpus_test.go` 追加 | DefaultTest/DefaultServerHeaderError/GetBody |
| runtime/listener | `sort_cert_test.go` | SortByPort 去重与 LisType、certMatches 通配/预解析/坏 DER |
| config | `internal_parse_test.go` | include 遍历五分支、parsePathType、parseZip、Reload 失败保护 |
| core | `services_test.go` | 钩子注入/nil 安全/撤销注册、RegisteredTypes、DefaultParseCommandHandler/XFF |
| infra/color | `color_test.go` | 16 色包装器往返 |
| init | `autocert_serial_test.go` | randomSerialNumber 范围与唯一性 |
