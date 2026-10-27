# Fast-Https 使用文档

本文面向部署与运维人员，只讲"怎么装、怎么配、怎么跑、怎么排错"。开发、架构与测试相关内容在仓库 `docs/` 下的其他目录，不随发行包分发。

> 文档中的路径都相对于解压后的程序根目录。配置里的相对路径按**进程工作目录**解析，因此请始终在根目录启动程序，或改用绝对路径。

## 1. 能力一览

| 能力 | 说明 | 关键配置 |
| --- | --- | --- |
| 静态文件 | 目录索引、`try_files`、304、Range、目录穿越防护 | `type: local` |
| 反向代理 | 上游连接池、请求/响应头改写、分块解码 | `type: proxy` + `proxy_pass` |
| TCP 转发 | 四层转发，不解析 HTTP | `listen: "9000 tcp"` |
| WebSocket | 升级劫持与双向管道 | `type: proxy` |
| 代理缓存 | 落盘持久化 + 内存热层 LRU | `proxy_cache` |
| 响应压缩 | gzip / brotli 内容协商 | `zip` |
| URL 重写 | 301 重定向与开发模式 | `type: rewrite`、`type: devmod` |
| Basic 认证 | location 级 | `auth` |
| 黑名单与限流 | per-IP 令牌桶、location 计数限流 | `blaklist`、`limit` |
| 应用防火墙 | GuardianWAF 引擎，命中返回 403 | `waf`、`appfirewall` |
| 配置热重载 | 校验通过后原子切换快照，零停机 | `reload` 命令 |
| HTTP/2 | 协议栈由 `golang.org/x/net/http2` 承担 | `listen: "443 ssl h2"` |

## 2. 获取与安装

### 发行包

1. 从 <https://gitee.com/ncepu-bj/fast-https/releases/> 下载对应平台的包
2. 解压到目标目录（Linux/macOS 为 `tar.gz`，Windows 为 `zip`）
3. 按需修改 `config/fast-https.json`

### RPM

```bash
rpm -ivh fast-https-*.rpm
```

RPM 把程序装到 `/usr/share/fast-https/`。

### 容器

```bash
docker build -t fast-https .
docker build -f docker/Dockerfile_ubuntu -t fast-https:ubuntu .
docker build -f docker/Dockerfile_centos7 -t fast-https:centos7 .
```

镜像基于 Alpine、Ubuntu 24.04 与 CentOS 7，暴露 8080 与 443。

## 3. 目录布局

```text
fast-https                  主程序（Windows 下另有 monitor.exe 托盘程序）
config/fast-https.json      唯一配置真源
config/conf.d/              拆分出来的站点配置（由 http.include 指向）
config/mime.json            扩展名 → Content-Type 映射
config/cert/                TLS 证书，*.pem 不入库，首次启动自动生成
httpdoc/root/               默认站点根目录
logs/                       运行日志
```

## 4. 快速开始

### 4.1 最小配置

`config/fast-https.json`：

```json
{
  "log_root": "./logs",
  "http": {
    "server": [
      {
        "listen": 8080,
        "server_name": "localhost",
        "location": [
          {
            "url": "/",
            "type": "local",
            "root": "./httpdoc/root",
            "index": ["index.html", "index.htm"]
          }
        ]
      }
    ],
    "include": ["./config/conf.d"]
  }
}
```

### 4.2 启动并验证

```bash
./fast-https start                 # 无参数等同 start
curl -I http://127.0.0.1:8080/
```

看到 `200` 与 `Server: fast-https` 即启动成功。`listen` 也可以写成字符串 `"8080"`，两种写法等价。

### 4.3 打开 HTTPS 与 HTTP/2

```json
{
  "listen": "443 ssl h2",
  "server_name": "localhost",
  "ssl_certificate": "./config/cert/localhost.pem",
  "ssl_certificate_key": "./config/cert/localhost-key.pem",
  "location": [
    { "url": "/", "type": "local", "root": "./httpdoc/root", "index": ["index.html"] }
  ]
}
```

`ssl_certificate` 指向的文件不存在时，启动阶段会用它自己签发的本地 CA 生成一张自签证书，可以直接用 `curl -k` 验证。浏览器不信任这张证书，正式环境请替换为权威 CA 签发的证书文件。

### 4.4 停止

```bash
./fast-https stop
```

Linux / macOS 上 `stop` 发送 `SIGTERM`，服务会关闭监听并等在途请求处理完（最长 `GracefulStopTimeout`）再退出，pid 文件随之删除；重复执行 `stop` 视为「本来就没在跑」，返回 0。Windows 上没有信号语义，直接终止进程。容器内、systemd（`INVOCATION_ID`）与设置了 `FASTHTTPS_FOREGROUND=1` 时 `start` 不转后台，交由进程管理器托管。

## 5. 配置文件

### 5.1 顶层键

| 键 | 说明 |
| --- | --- |
| `pid` | pid 文件路径，`status` / `stop` 依赖它 |
| `log_root` | 日志目录，默认 `./logs` |
| `engine` | 预留的主从注册字段，当前不参与运行逻辑 |
| `waf` | 全局应用防火墙：`{"mode": "enforce" \| "monitor" \| "disabled"}` |
| `http` | HTTP 服务主体，见下 |

### 5.2 `http` 段

| 键 | 说明 |
| --- | --- |
| `server` | 站点数组，每一项是一个虚拟主机 |
| `include` | 额外加载的目录或 json 文件列表，默认 `config/conf.d` |
| `default_type` | 未知扩展名的默认 Content-Type |
| `servers_limit` | `max_body_size`、`max_header_size`、`limit`、`burst` |
| `blaklist` | IP 黑名单（键名是历史拼写，保持兼容） |
| `log_format` | 访问日志字段顺序，见第 9 节 |

### 5.3 `server` 段

| 键 | 说明 |
| --- | --- |
| `listen` | 端口，可带 `ssl`、`h2`、`tcp`，例如 `8080`、`443 ssl`、`443 ssl h2`、`9000 tcp` |
| `server_name` | 虚拟主机名，按 `Host` 匹配，必填 |
| `ssl_certificate` | 证书链 PEM 路径，`listen` 含 `ssl` 时必填 |
| `ssl_certificate_key` | 私钥 PEM 路径，`listen` 含 `ssl` 时必填 |
| `location` | 路由数组，见 5.4 |

### 5.4 `location` 段

| 键 | 说明 |
| --- | --- |
| `url` | 匹配规则，支持正则，例如 `/`、`/api/`、`\.php$` |
| `type` | `local`（静态）、`proxy`（反向代理）、`rewrite`（重定向）、`devmod`（开发模式） |
| `root` | `local` 的站点根目录 |
| `index` | 默认索引文件名列表 |
| `try` | `try_files` 语义：`[{"uri": "/", "file": ["index.html"], "next": "@backend"}]` |
| `rewrite` | `type: rewrite` 时的目标地址 |
| `proxy_pass` | `type: proxy` 时的上游地址，必须带 `http://` 或 `https://` |
| `proxy_set_header` | 请求头改写，格式 `[{"HeaderKey": "X-Real-IP", "HeaderValue": "$remote_addr"}]` |
| `proxy_cache` | 代理缓存，见 6.5 |
| `zip` | 响应压缩，取值 `gzip`、`br`，可同时配置 |
| `limit` | `mem`、`rate`、`burst`、`nodelay` |
| `auth` | `{"type": "basic", "user": "...", "pswd": "..."}` |
| `appfirewall` | 应用防火墙开关，例如 `["xss", "sqli"]` |
| `error_page` | `{"not_found": "./httpdoc/404.html", "proxy": "./httpdoc/502.html"}` |

`proxy_set_header` 的 `HeaderValue` 支持变量插值：

| 变量 | 含义 |
| --- | --- |
| `$request_method` | 请求方法 |
| `$request_uri` | 原始请求 URI |
| `$host` | 请求的 Host |
| `$proxy_host` | 上游地址 |
| `$remote_addr` | 客户端 IP |
| `$proxy_add_x_forwarded_for` | 追加当前客户端 IP 后的 `X-Forwarded-For` |

## 6. 典型配置

### 6.1 静态站点 + try_files

```json
{
  "url": "/",
  "type": "local",
  "root": "./httpdoc/root",
  "index": ["index.html"],
  "try": [{ "uri": "/", "file": ["index.html"], "next": "" }]
}
```

### 6.2 反向代理

```json
{
  "url": "/api/",
  "type": "proxy",
  "proxy_pass": "http://127.0.0.1:9000",
  "proxy_set_header": [
    { "HeaderKey": "X-Real-IP", "HeaderValue": "$remote_addr" },
    { "HeaderKey": "X-Forwarded-For", "HeaderValue": "$proxy_add_x_forwarded_for" }
  ]
}
```

`proxy_pass` 缺少 `http://` / `https://` 前缀时会按 `local` 处理，不会报错，这是最容易踩的坑。

### 6.3 TCP 转发

```json
{
  "listen": "9000 tcp",
  "server_name": "db",
  "location": [{ "url": "/", "type": "proxy", "proxy_pass": "http://10.0.0.9:3306" }]
}
```

`listen` 带 `tcp` 时整条连接按四层转发，不解析 HTTP。

### 6.4 压缩

```json
{ "url": "/", "type": "local", "root": "./httpdoc/root", "zip": ["gzip", "br"] }
```

按 `Accept-Encoding` 协商，客户端同时支持时优先给 brotli。

### 6.5 代理缓存

```json
"proxy_cache": {
  "path": "./cache",
  "key": "$request_uri",
  "valid": ["200:1h", "404:1m", "any:5m"],
  "methods": ["GET", "HEAD"],
  "max_size": 1073741824
}
```

`valid` 的格式是 `状态码:时长`，`any` 匹配其余状态码；命中会顺延一个周期（读即续期）。`key` 支持变量插值。

### 6.6 Basic 认证与限流

```json
{
  "url": "/admin/",
  "type": "local",
  "root": "./httpdoc/admin",
  "auth": { "type": "basic", "user": "admin", "pswd": "change-me" },
  "limit": { "rate": 10, "burst": 20, "nodelay": true }
}
```

## 7. TLS 与证书

- **SNI 多证书**：同一端口上的多个 `server_name` 各自持有证书，握手时按 SNI 选择，匹配不到用该端口的第一张。
- **自动生成**：首次启动时，若 `config/cert/<server_name>.pem` 不存在，会用一个本地自签 CA 生成证书。CA 有效期 10 年，域名证书 2 年 3 个月。
- **热替换**：执行 `reload` 后证书从磁盘重新加载，不需要重启进程。
- **不自动续期**：程序只判断文件是否存在，不检查有效期。证书过期前需要替换文件并 `reload`。
- **外部签发**：用 acme.sh、certbot 等外部工具签发后，把 `fullchain.pem` 与私钥路径填进 `ssl_certificate` / `ssl_certificate_key` 即可。

> 安全提示：本地 CA 的私钥生成在 `httpdoc/root/root.key`，而 `httpdoc/root` 默认是对外服务的站点根目录。上线前请确认该文件不可通过 HTTP 直接访问，或把 CA 私钥移到不对外服务的目录。

## 8. HTTP/2

`listen` 加上 `h2` 即可，例如 `"listen": "443 ssl h2"`。

- ALPN 只在含 `h2` 的端口通告 `h2`，纯 `ssl` 端口只走 HTTP/1.1，避免浏览器协商到 HTTP/2 后被 HTTP/1.1 解析器处理。
- 同一端口同时通告了 `http/1.1`，不支持 HTTP/2 的客户端会自动降级。
- `reload` 或停止时通过 `http.Server.Shutdown` 发送 GOAWAY 并等待在途请求，最长 5 秒。

## 9. 日志

运行日志都写成 JSON，位于 `log_root` 指向的目录：

| 文件 | 内容 |
| --- | --- |
| `system.log` | 启动、配置加载、监听建立等系统事件 |
| `access.log` | 访问日志，字段由 `log_format` 决定 |
| `error.log` | 运行时错误 |
| `safe.log` | 黑名单、限流、应用防火墙命中 |

`log_format` 可按任意顺序组合以下字段：

| 字段 | 含义 |
| --- | --- |
| `ip_port` | 客户端地址与端口 |
| `time` | 请求时间 |
| `type` | 命中的 location 类型 |
| `method` | 请求方法 |
| `path` | 请求路径 |
| `host` | 请求的 Host |
| `status` | 响应状态码 |
| `size` | 响应字节数 |
| `user_agent` | User-Agent |

修改 `log_root` 后执行 `reload` 会重新打开日志文件；新目录打不开时继续写原文件。

## 10. 命令行

| 命令 | 作用 |
| --- | --- |
| `start` | 以守护进程方式启动（无参数等同此命令） |
| `dev` | 前台启动，日志级别 6，并在 `0.0.0.0:10000` 开 pprof |
| `stop` | 停止服务 |
| `reload` | 校验配置后热重载，失败时保留旧配置 |
| `status` | 读 pid 文件，进程存在返回 0，否则非 0 |
| `check` | 只校验配置，不监听端口、不写 pid |
| `install` | 安装为系统服务 |
| `uninstall` | 卸载系统服务 |

两点行为需要留意：

- `dev` 模式下 `Ctrl+C` 是**停止服务**，不是重载。
- `check` 通过时打印 `config ok` 并以 0 退出；`start` 的预检会做同一项校验。

## 11. 热重载与停机

`reload` 的流程是：解析并校验新配置 → 校验失败则继续用旧快照 → 校验通过则原子替换快照 → 按差异增删改监听器 → 重新打开证书表与日志文件。整个过程中已有连接不受影响。

守护进程模式下的等价操作是向主进程发送 `SIGINT`。停止时先停止接受新连接，再等待在途请求完成。

## 12. 排错

| 现象 | 原因与处理 |
| --- | --- |
| 启动报 `ssl_certificate not found` | `listen` 含 `ssl` 时证书文件必须存在。首次启动会自动生成 `config/cert/<server_name>.pem`，请确认路径与 `server_name` 一致，或改用绝对路径 |
| 启动报 `listen: port already in use` | 端口被占用，换端口或先释放 |
| 配置校验通过但访问 404 | 检查 `root` 是相对路径时的实际解析位置，确认 `server_name` 与请求 `Host` 一致 |
| 反向代理返回 404 或落到静态分支 | `proxy_pass` 缺少 `http://` / `https://` 前缀，被当作 `local` 处理 |
| HTTP/2 不生效 | `listen` 没写 `h2`；或客户端不支持 ALPN。可用 `curl -I --http2 https://...` 验证协议版本 |
| 修改配置后不生效 | 守护进程模式需要执行 `reload`；`dev` 模式需要重启 |
| 日志没有写到新目录 | 新 `log_root` 打不开时程序继续写原文件，检查目录权限 |

## 13. 已知限制

| 项 | 说明 |
| --- | --- |
| ACME 自动签发 | 未接入。仓库内的 `acme.sh` 不参与启动流程，证书需外部工具签发后填入路径 |
| 证书自动续期 | 不支持，需人工替换后 `reload` |
| `engine` 配置段 | 主从注册的预留字段，当前不参与运行逻辑 |
| 上游故障转移 | 未实现，单个上游不可用时直接返回错误页 |
| 配置文件自动重载 | 暂不支持，监听文件变化后自动 reload 仍未排期 |

## 14. 版本与许可

版本号取自 `config/consts.go` 的 `CURRENT_VERSION`，启动横幅会打印。许可证见发行包内的 `LICENSE`。
