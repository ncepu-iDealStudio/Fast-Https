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

### Linux 发行版包（deb / rpm / apk）

```bash
dpkg -i fast-https_*.deb                     # Debian / Ubuntu
rpm -ivh fast-https-*.rpm                    # RHEL / openEuler / CentOS
apk add --allow-untrusted fast-https_*.apk   # Alpine
```

包装好后的位置按 FHS 分开放：程序 `/usr/bin/fast-https`，配置与证书 `/etc/fast-https/`，站点 `/usr/share/fast-https/httpdoc/`，日志 `/var/log/fast-https/`，pid `/run/fast-https/`，服务单元 `/usr/lib/systemd/system/fast-https.service`，手册页 `/usr/share/man/man1/fast-https.1`，日志轮转 `/etc/logrotate.d/fast-https`。启动与查手册：

```bash
systemctl enable --now fast-https
man fast-https
```

随包默认配置同时监听 80 与 `443 ssl h2`，装完 `systemctl enable --now fast-https` 就能提供 HTTP 与 HTTPS 站点。443 的证书不需要预置文件：首次 `start` 会用本地 CA 生成 `/etc/fast-https/cert/<server_name>.pem`；在那之前 `reload` 的预检会报 `ssl_certificate not found`，先跑一次 `start` 即可。`/etc/fast-https/`、`/var/log/fast-https/` 归 root 所有，手工执行 `start` / `reload` 要用 `sudo`，普通用户会因为写不了日志而失败。运行期路径由编译期基址决定，可以从任意工作目录执行 `fast-https`；外部控制面若要知道实际路径，用 `fast-https config path --json` 读取，不要自己判断装法。

从 1.5.0 或更早的包升级时，安装脚本会把 `/usr/share/fast-https/config/` 下已有的配置与证书复制到 `/etc/fast-https/`（目标已存在则跳过，旧文件保留不动），确认无误后可手工删除旧目录。

### 容器

```bash
docker build -f build/Dockerfile -t fast-https .
docker build -f build/docker/Dockerfile_ubuntu -t fast-https:ubuntu .
docker build -f build/docker/Dockerfile_centos7 -t fast-https:centos7 .
```

镜像基于 Alpine、Ubuntu 24.04 与 CentOS 7，暴露 80 与 443，与随包默认配置一致。容器里跑 80/443 需要 root 或相应 capability，要映射到高位端口时改 `listen`。

## 3. 目录布局

```text
fast-https                  主程序
config/fast-https.json      系统级配置（include、acme、日志、WAF…）
config/conf.d/              站点配置，一个文件一个站点（由 http.include 指向）
config/mime.json            扩展名 → Content-Type 映射
config/cert/                TLS 证书，*.pem 不入库，首次启动自动生成
httpdoc/root/               默认站点根目录
logs/                       运行日志
```

## 4. 快速开始

### 4.1 默认配置

配置分成两层：`config/fast-https.json` 只放系统级设置（`http.include`、`acme`、日志、WAF、安全响应头），站点一律写在 `config/conf.d/` 下，一个文件一个站点。两个文件都刻意不带注释。

`config/fast-https.json`：

```json
{
  "log_root": "./logs",
  "security": {
    "host_check": "off",
    "headers": {
      "nosniff": true,
      "hsts": "",
      "x_frame_options": "",
      "referrer_policy": "",
      "content_security_policy": ""
    }
  },
  "waf": {
    "mode": "disabled"
  },
  "http": {
    "include": ["./config/conf.d"],
    "default_type": "application/octet-stream",
    "servers_limit": {
      "max_header_size": 4096,
      "max_body_size": 524288,
      "limit": 0,
      "burst": 0
    },
    "blaklist": [],
    "log_format": ["ip_port", "time", "type", "method", "path", "host", "status", "size", "user_agent"]
  },
  "acme": {
    "enabled": false,
    "email": ""
  }
}
```

上面每一项的取值都等于内置默认值，写全是为了在文件里直接看到有哪些系统级开关（`waf.mode: disabled` 等价于不启用，引擎不构建，零开销）。

随包的 `config/conf.d/default.json` 是开箱即用的默认站点，角色相当于 nginx 的默认站点：80 提供 HTTP，443 提供 HTTPS + HTTP/2，两个端口都指向 `./httpdoc/root`：

```json
{
  "listen": [80, "443 ssl h2"],
  "server_name": "localhost",
  "ssl_certificate": "./config/cert/localhost.pem",
  "ssl_certificate_key": "./config/cert/localhost-key.pem",
  "location": [
    {
      "url": "/",
      "type": "local",
      "root": "./httpdoc/root",
      "index": ["index.html", "index.htm"]
    }
  ]
}
```

逐项说明：

- `listen: [80, "443 ssl h2"]`：一个站点文件可以写多个监听，加载时按每个监听展开成独立的运行期 server 块，`server_name`、`location` 与证书共用；只想留一个端口就从数组里删掉另一个。写成单个值时也可以直接写 `"listen": 80`。
- `listen: 80`：HTTP 默认端口，绑定它需要 root；普通用户手工运行时改成 `8080` 之类的高位端口。写成字符串 `"80"` 等价。
- `listen: "443 ssl h2"`：HTTPS + HTTP/2，`ssl` 开 TLS，`h2` 才向客户端通告 HTTP/2。
- `ssl_certificate` / `ssl_certificate_key`：文件不存在时，首次 `start` 会用本地 CA 生成 `config/cert/localhost.pem` 与 `localhost-key.pem`，可以直接 `curl -k https://127.0.0.1/` 验证；浏览器会提示不受信任。正式环境换成权威 CA 的 PEM，或者打开 7.1 的 ACME 自动签发。
- 不需要 HTTPS 时把 `"443 ssl h2"` 从 `listen` 里去掉；443 被别的程序占用（Windows 上常见于 IIS）时换端口或去掉这一项。
- `server_name`：按请求的 `Host` 头匹配。支持多域名写法：空格分隔多个域名（如 `"server_name": "example.com www.example.com"`），它们共享同一组 location 和证书。没命中任何 `server_name` 时回落到该端口的第一个 server 块（相当于 nginx 的 `default_server`），所以用 IP 或域名别名访问也能进到本站点，不会整站 404。
- `root: ./httpdoc/root`：站点根目录，相对路径按站点根基址解析（包布局是 `/usr/share/fast-https`，归档布局是当前目录），与二进制所在目录无关。
- `http.include: ["./config/conf.d"]`：该目录下每个 `*.json` 会被追加为一个 server 块，站点一多就拆出去，主配置不必跟着变长。主配置可以完全不写 `http.server`，只要 include 里至少有一个站点文件；`fast-https site` 的子命令就是按这个约定增删站点文件的。
- `acme.enabled: false`：ACME 自动签发默认关闭，行为与老版本一致。需要受信任证书时按 7.1 手工配置，或直接运行 `fast-https acme` 向导。

配置本身允许 `//`、`/* */` 注释与末尾多余逗号（见第 5 节），需要给自己留备注时随时加。随包默认文件不带注释，是为了让「用脚本或向导改配置」这件事不用处理注释：文档负责解释字段，文件保持最小可读 JSON。

### 4.2 启动并验证

```bash
sudo ./fast-https start            # 无参数等同 start；80 端口需要 root
curl -I http://127.0.0.1/          # 换成高位端口时写成 http://127.0.0.1:8080/
```

看到 `200` 与 `Server: fast-https` 即启动成功。`listen` 也可以写成字符串 `"80"`，两种写法等价。

### 4.3 HTTPS 与 HTTP/2

默认配置里的第二个 server 块已经打开 HTTPS，不需要额外准备证书文件：

```bash
curl -kI https://127.0.0.1/          # 自签证书用 -k 跳过校验
curl -I --http2 -k https://127.0.0.1/  # 确认协商到 HTTP/2
```

要点：

- `ssl` 必须配 `ssl_certificate` 与 `ssl_certificate_key`，缺一个启动期就报错。
- 证书文件不存在时，`start` 的预检阶段会用本地 CA 生成自签证书（`config/cert/<server_name>.pem`）；`reload` 的预检不生成证书，所以在第一次 `start` 之前会报 `ssl_certificate not found`。
- 只有 `listen` 里写了 `h2` 的端口才通告 HTTP/2，纯 `ssl` 端口只走 HTTP/1.1。
- 换权威证书有两种方式：把这两个路径改成自己签发的 PEM 后 `reload`；或者用 7.1 的 ACME 自动签发，签发成功后证书会就地热替换。
- 端口冲突时（例如 Windows 上 443 被 IIS 占用）改 `listen` 里的端口号，或整段删掉这个 server 块。

### 4.4 用 site 命令管理站点文件

站点多了不必都堆在主配置里：`config/conf.d/*.json` 每个文件就是一个 server 块（字段与 `http.server[]` 元素完全一致），主配置的 `http.include` 指向该目录即可。

新增、更新、删除、启用、停用请用 `site add/update/delete/enable/disable`：复制当前配置树到候选目录、改站点文件，再走 `config apply` 同一套事务（`--dry-run`、`--request-id` 幂等、失败回滚、自动 reload）。`site add` 不带 `--domain` 且标准输入是终端时进向导，交互入口与批量调用共用同一套校验与事务。

```bash
fast-https site add --domain demo.example --root ./httpdoc/root --listen 8081 --dry-run --json
fast-https site add --domain demo.example --root ./httpdoc/root --listen 8081 --yes --request-id site-1
fast-https site update demo.example --listen 8082 --yes
fast-https site disable demo.example --yes
fast-https site enable demo.example --yes
fast-https site delete demo.example --yes   # 只删配置文件，不删内容目录与证书
fast-https site list --json                 # 含 enabled、cert_status
fast-https site add --domain blog.example --listen 80 --root ./httpdoc/root --yes
fast-https site show blog
fast-https site delete blog --yes
```

- **site add**：`--domain` 必填，站点名由域名规范化得到；`--root` 或 `--proxy-pass` 二选一。同名或同 `server_name` 已存在时退出码 10。`--root` 必须是已存在的目录，不能含 `..`，也不能指向配置、证书、状态或日志目录。非 dry-run 必须 `--yes`。
- **site update**：只允许 `--listen`、`--server-name`、`--root`、`--proxy-pass`、`--tls`。内联在主配置 `http.server[]` 里的站点返回退出码 11。注释与未建模字段由补丁保留。
- **site disable / enable**：把 `<name>.json` 改名为 `<name>.json.disabled` 或改回（`include` 只加载 `*.json`）。`site list --json` 里停用站点为 `enabled: false`。
- **site delete**：必须 `--yes`。只删站点配置文件，不删内容目录、证书、私钥；配置仍留在版本快照里，可用 `config rollback` 找回。
- **交互式**：`fast-https site add` 不带 `--domain` 且标准输入是终端时进向导，一题一屏依次问站点名（也是文件名）、监听端口、类型、是否 TLS；最后一屏按所选类型决定——静态站点问站点根目录，反向代理问上游地址（`http://127.0.0.1:8080`）；加 `--plain` 改成逐行问答。最后一屏确认即视为 `--yes`。标准输入不是终端时不会挂起，会提示改用 `--domain` 等参数。
- **反代站点（旧 init）**：`--type proxy --proxy-pass http://127.0.0.1:8080`。
- **改已有站点**：用 `site update <name>`，只改白名单字段（`--listen`、`--server-name`、`--root`、`--proxy-pass`、`--tls`），不接受 `--type`。同名站点不再隐式更新，`site add` 遇到重名直接返回冲突（退出码 10）。
- **多端口**：`--listen` 可重复，一个文件就能同时监听多个端口，站点文件里写成数组。

  ```bash
  fast-https site add --domain blog.example --listen 80 \
      --listen 80 --listen "443 ssl h2" --non-interactive
  ```

  加载时这个块会展开成"一块一个端口"的两个 server 块，`server_name`、location 与证书共用；`site list` 的 LISTEN 列显示为 `80, 443 ssl h2`。
- **TLS 站点**：`--tls` 保证至少有一个 TLS 监听，且不会把 `ssl` 贴到 80 上。不给 `--listen` 时生成 `["80", "443 ssl h2"]`；给了 `--listen` 时写什么用什么，只把非 80 的那条补成 `ssl h2`（`--listen 80 --tls` 仍是 `80` + `443 ssl h2`）。证书默认路径是 `config/cert/<server_name>.pem` 与 `-key.pem`，由首次 `start` 生成；在那之前 `reload` 的预检会报 `ssl_certificate not found`（与主配置一致）。
- **写入安全（旧 init）**：先生成 `.tmp`，用主配置同一套 schema 校验通过后才 rename；同名文件已存在时要求 `--force`。
- **生效方式**：`site add/update/delete/enable/disable` 全部走事务并自动 reload，失败回滚；不再有「只写文件、手动 reload」的老路径。

### 4.5 停止

```bash
./fast-https stop
```

Linux / macOS 上 `stop` 发送 `SIGTERM`，服务会关闭监听并等在途请求处理完（最长 `GracefulStopTimeout`）再退出，pid 文件随之删除；重复执行 `stop` 视为「本来就没在跑」，返回 0。Windows 上没有信号语义，直接终止进程。容器内、systemd（`INVOCATION_ID`）与设置了 `FASTHTTPS_FOREGROUND=1` 时 `start` 不转后台，交由进程管理器托管。

## 5. 配置文件

文件仍然是 JSON（`http.include` 只收录 `*.json`），但解析前会先去掉 `//`、`/* */` 注释与对象/数组末尾多余的逗号，所以配置可以像 nginx.conf 一样逐段写注释。字符串里的 `//`（例如 `proxy_pass: "http://..."`）不受影响，纯 JSON 文件行为完全不变。

### 5.1 顶层键

| 键 | 说明 |
| --- | --- |
| `pid` | pid 文件路径，`status` / `stop` 依赖它 |
| `log_root` | 日志目录，默认 `./logs` |
| `engine` | 预留的主从注册字段，当前不参与运行逻辑 |
| `waf` | 全局应用防火墙：`{"mode": "enforce" \| "monitor" \| "disabled"}` |
| `http` | HTTP 服务主体，见下 |
| `acme` | ACME 自动签发与续期，见 7.1。缺省即关闭，行为与老版本一致 |
| `security` | Host 校验与安全响应头，见 5.5。缺省只加 `X-Content-Type-Options: nosniff` |

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
| `server_name` | 虚拟主机名，按 `Host` 匹配，必填。多域名用空格分隔：`"example.com www.example.com"`，共享同一组 location 和证书 |
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
| `auth` | `{"type": "basic", "user": "...", "pswd": "...", "realm": "..."}`。只支持 `basic`，写了 `type` 就必须同时写 `user` 与 `pswd`（缺任何一项校验就报错）；`realm` 可选，留空时用站点名 |
| `appfirewall` | 应用防火墙开关，例如 `["xss", "sqli"]` |
| `error_page` | `{"not_found": "./httpdoc/404.html", "proxy": "./httpdoc/502.html"}` |
| `allow_methods` | 只允许列出的方法，例如 `["GET", "HEAD"]`；越界返回 405 并带 `Allow` 头。缺省不限制，`HEAD` 随 `GET` 自动放行 |

`proxy_set_header` 的 `HeaderValue` 支持变量插值：

| 变量 | 含义 |
| --- | --- |
| `$request_method` | 请求方法 |
| `$request_uri` | 原始请求 URI |
| `$host` | 请求的 Host |
| `$proxy_host` | 上游地址 |
| `$remote_addr` | 客户端 IP |
| `$proxy_add_x_forwarded_for` | 追加当前客户端 IP 后的 `X-Forwarded-For` |

### 5.5 `security` 段

```json
"security": {
  "host_check": "warn",
  "headers": {
    "nosniff": true,
    "hsts": "max-age=31536000",
    "x_frame_options": "SAMEORIGIN",
    "referrer_policy": "no-referrer",
    "content_security_policy": "default-src 'self'"
  }
}
```

| 键 | 默认 | 说明 |
| --- | --- | --- |
| `host_check` | `off` | `off` / `warn` / `enforce`。`enforce` 时 `Host` 没命中任何 `server_name` 直接返回 421，不再回落到该端口的第一个 server；`warn` 只写安全日志、行为不变 |
| `headers.nosniff` | `true` | 写入 `X-Content-Type-Options: nosniff`。这一项无兼容风险，所以默认开 |
| `headers.hsts` | 空 | 写入 `Strict-Transport-Security`，内容原样写入。只在全站 HTTPS 后开，否则 HTTP 访问会被浏览器记住 |
| `headers.x_frame_options` | 空 | 写入 `X-Frame-Options` |
| `headers.referrer_policy` | 空 | 写入 `Referrer-Policy` |
| `headers.content_security_policy` | 空 | 写入 `Content-Security-Policy` |

安全头在响应写出的唯一出口统一补齐：模块自己设置过同名头时不覆盖，重复调用幂等。

> 请求路径在解析阶段就完成规范化：解一层 `%xx`、合并连续的 `/`、拒绝二次编码（`%252f`）、反斜杠、控制字符与超过 8192 字节的目标，非法请求直接 400。location 匹配、静态文件解析与日志看到的都是规范化后的路径。

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
- **热替换**：执行 `reload` 后证书从磁盘重新加载，不需要重启进程；启用 ACME 后新证书签发成功也会就地热替换。
- **ACME 自动签发与续期**：配置 `acme` 段后向 Let's Encrypt / ZeroSSL 申请受信任证书，到期前自动续期，见 7.1。
- **外部签发**：仍可用 certbot 等外部工具签发，把 `fullchain.pem` 与私钥路径填进 `ssl_certificate` / `ssl_certificate_key`。

### 7.1 ACME 自动签发

不想手写这段 JSON 时，跑 `fast-https acme` 走一遍交互向导：一题一屏，依次问启用开关、邮箱、CA、挑战方式等，只改写顶层 `acme` 段，其余配置与注释原样保留。向导的默认值取自当前配置（邮箱、CA、挑战方式、续期天数、缓存目录、回落开关），回车即保留原值，不会退回出厂默认；确认页的摘要也是答完题之后才生成，显示的是这次要写入的内容。不喜欢全屏表单就加 `--plain` 改成逐行问答；脚本里则用 `fast-https acme setup --email you@example.com --non-interactive` 全走参数。改完执行 `fast-https reload` 即可生效，不需要重启进程。

```json
"acme": {
  "enabled": true,
  "email": "ops@example.com",
  "challenge": ["auto"],
  "renew_before_days": 30,
  "fallback_self_signed": true,
  "storage": "./config/cert/acme",
  "cas": [
    { "name": "letsencrypt", "directory": "https://acme-v02.api.letsencrypt.org/directory" }
  ]
}
```

| 字段 | 默认 | 说明 |
| --- | --- | --- |
| `enabled` | `false` | 总开关。关闭时进程行为与未配置 `acme` 完全一致 |
| `email` | 空 | 账户邮箱，`enabled` 为真时必填，否则配置校验直接报错 |
| `staging` | `false` | 为真时把 Let's Encrypt 目录换成测试环境，便于联调；自定义目录不受影响 |
| `challenge` | `["auto"]` | 可选 `auto`、`http-01`、`tls-alpn-01`。`auto` 同时启用后两者，由 ACME 客户端按成功率自动选择 |
| `renew_before_days` | `30` | 到期前的续期提前量，按 90 天有效期换算成续期窗口比例 |
| `fallback_self_signed` | `true` | 签发失败时是否继续用自签证书提供服务 |
| `storage` | `config/cert/acme` | 账户与订单缓存目录，相对路径按配置基址解析（包布局落在 `/etc/fast-https/cert/acme`）；只有真正发起过申请才有内容 |
| `cas` | Let's Encrypt、ZeroSSL | CA 降级链，按顺序尝试；ZeroSSL 必须提供 `eab_kid` 与 `eab_hmac_key`，否则该 CA 被跳过 |

行为：

- **启动不被阻塞**：监听建立后才会在后台申请，申请期间继续用自签证书，首次握手不会卡住。
- **两种挑战**：`http-01` 由数据面在 `/.well-known/acme-challenge/<token>` 上应答（不依赖站点根目录，token 只存在内存里、不落盘），`tls-alpn-01` 在同一个 443 端口上用临时证书应答（ALPN 通告 `acme-tls/1`）。两者都不额外占用端口。
- **落盘位置不变**：签发成功后写回该 server 块配置的 `ssl_certificate` / `ssl_certificate_key`（默认 `config/cert/<server_name>.pem`），写盘用临时文件 + rename，随后热替换证书，监听 socket 不变。
- **失败回落**：内网、域名未解析、速率限制等情况下继续使用自签证书，日志里每个域名一条告警，按退避重试，不会刷屏。
- **申请范围**：只有 `listen` 里带 `ssl` 的 server 块会被申请证书。只监听 80 的站点不会触发任何申请——先给它加 `"443 ssl h2"` 与 `ssl_certificate` / `ssl_certificate_key`，再用 `fast-https acme show` 确认"将申请的域名"里出现了它。
- **跳过不可用的名字**：IP、`localhost`、不含点的名字与通配符（只能 `dns-01`）不会发起申请，启动时各留一条告警。
- **续期**：由后台任务在到期前完成，续期成功后同样热替换；不需要 `reload`，也不需要重启。

> 账户私钥、订单与证书缓存都在 `storage` 目录下，备份时与 `config/cert` 一起打包即可。`http-01` 需要外部能访问本进程的 80 端口（也可以是前置反代转发 `/.well-known/acme-challenge/`），`tls-alpn-01` 需要外部能直连 443。

> 本地 CA 的证书 `root.crt` 放在 `httpdoc/root/`（首页可直接下载安装），私钥 `root.key` 放在 `config/cert/`，不在对外服务的站点根下。旧版本把两者都放在站点根，升级后首次启动会把私钥搬到 `config/cert/`。如果你自己改过站点根，确认新目录里不含私钥。

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

Linux 包会安装 `/etc/logrotate.d/fast-https`：每天轮转、保留 14 份、单个文件超过 100MB 也提前轮转、gzip 压缩。四个日志文件在进程里常开 fd，所以用 `copytruncate`——先复制再截断，不需要重启或 `reload`。该文件是包管理器登记的配置文件（deb conffile、rpm `%config(noreplace)`），本地修改在升级时保留。归档与 Docker 部署的日志在 `./logs`，不在这个配置管辖范围，需要自己接 logrotate 或按天清理。

## 10. 命令行

| 命令 | 作用 |
| --- | --- |
| `start` | 以守护进程方式启动（无参数等同此命令）；容器与 systemd 托管、或设了 `FASTHTTPS_FOREGROUND=1` 时保持前台 |
| `stop` | 停止服务 |
| `reload` | 先校验配置，通过才发信号热重载；校验失败直接返回原因，运行中的配置保持不动 |
| `status` | 读 pid 文件，进程存在返回 0，否则非 0 |
| `version` | 打印版本号，输出一行 `fast-https <版本>` |
| `service version` | 查询 CLI 版本；加 `--json` 输出机器可读的封套。运行中进程的 `server_version` 来自 `/runtime`，连不上时为 `null` 并带 warning |
| `service status` | 只读查询：读 pid 文件判断进程，再对配置里的端口做一次回环连接，给出 `running`、`degraded`、`stopped`、`unknown`；读得到 `/runtime` 时补 `uptime_seconds`、`started_at`、`version`。服务没在跑也是成功的查询，退出码 0 |
| `service reload` | 受控 reload：校验、预检新增端口、发信号，再用 `/runtime` 做强验证（pid/`started_at` 不变且 `config_generation` 加一）；读不到快照时退回弱验证。不改配置文件。`--dry-run` 只校验与预检。服务没在跑退出码 4。记 operation，支持 `--request-id` 幂等 |
| `runtime status` | 读取 `127.0.0.1:9088/runtime` 的瞬时连接与请求状态。连不上退出码 4；`schema_version` 不认识退出码 11。没有的字段（如 CPU、reading/writing/waiting）省略并在 `warnings` 说明 |
| `runtime stats` | 进程启动以来的累计计数，响应里 `window` 为 `since_process_start`。`--since`、`--domain` 尚未支持，传入即退出码 11（`NOT_SUPPORTED`）。Monitor 只消费这份累计快照，不另算时间窗 |
| `config show` | 显示磁盘上的主配置与 `http.include` 站点，口令、令牌等敏感值一律脱敏；`--section` 可选 `http`、`acme`、`security`、`waf`、`sites` |
| `config validate` | 校验当前配置，或用 `--candidate` 校验一棵候选目录（`fast-https.json` + `conf.d`）；只校验，不应用；失败退出码 5；路径含 `..` 或符号链接逃出目录时退出码 2 |
| `config versions` | 列出状态目录里的配置版本快照。`active` 由当前磁盘配置树哈希决定，都不匹配则 `drift:true`。快照不含证书与私钥 |
| `config diff --candidate` | 比较当前配置与候选目录、文件或版本号。候选是目录时比较整棵 JSON 树。敏感键只显示 `changed` |
| `config apply --candidate` | 将候选目录受控应用到线上（校验、差异、恢复点、切换、reload、强验证；读不到 `/runtime` 时退回弱验证；失败则还原）。必须 `--yes`。`--dry-run` 不写正式文件、不 reload |
| `config rollback --version` | 回滚到历史版本，本身是新 operation；先校验目标版本（含证书仍存在），不删除历史版本。必须 `--yes` |
| `operation list` / `operation get` | 查询落盘的管理操作。`running` 且持有者进程已不在时对账为 `unknown` |
| `cert list` | 按证书文件列出配置里各 ssl 站点声明的证书：域名、状态（`valid`、`expiring`、`expired`、`not_yet_valid`、`missing`、`unreadable`、`unknown`）、到期天数、颜发者、是否由 ACME 管理、被哪些站点使用；只读，不输出私钥内容与路径 |
| `cert check --domain <域名>` | 检查该域名使用的证书：文件是否存在、有效期、是否覆盖该域名、证书与私钥是否配对。证书有问题也是成功的查询（退出码 0），域名不在任何 ssl 站点里退出码 9 |
| `logs error` | 只读 `error.log`。`--since`、`--limit`（默认 100，超过 1000 钳到 1000）、`--domain`。含 `PrintErr` 以及控制台 Warn/Error 的镜像（`source:"console"`，`kind:"error"`） |
| `logs access` | 只读 `access.log`，过滤条件同上，另有 `--status`（`500`、`4xx` 或 `400-499`） |
| `logs query` | `--kind` 必填（`access`、`error`、`system`、`safe`），一次只查一个文件。另可按 `--level`、`--method`、`--path-prefix`、`--reason`、`--until` 过滤。没有 `--file`，不读已轮转的文件，日志内容只作为数据返回 |
| `diagnose` | 只读汇总健康检查。失败与不可用分开：`runtime` 读 `/runtime`（连不上为 warning）；`monitor` 读状态文件（未安装为 ok，过期为 warning）。配置有问题也退出码 0 |
| `monitor status` | 只读 Monitor 的 `status.json`。文件不存在时 `status` 为 `not_installed`（退出码 0）；采集时间早于 30 秒为 `stale`。不连接 Monitor 进程，也不负责启停 |
| `monitor events` | 只读 `events.jsonl`。`--since`、`--limit`（默认 100，上限 1000）、`--severity`（`info`、`warning`、`error`、`critical`）。坏行跳过并计数，事件文本只作为数据返回 |
| `site add` | 受控新增站点（`--domain`、`--root` 或 `--proxy-pass`、`--listen`、`--tls`）。走配置事务，非 dry-run 必须 `--yes`。同名或同 `server_name` 退出码 10 |
| `site update <name>` | 受控更新白名单字段（`--listen`、`--server-name`、`--root`、`--proxy-pass`、`--tls`）。内联主配置站点退出码 11 |
| `site delete <name>` | 受控删除站点配置（不删内容目录与证书）。必须 `--yes` |
| `site enable <name>` / `site disable <name>` | 把 `<name>.json` 与 `<name>.json.disabled` 互改并 reload。必须 `--yes` |
| `site list` | 列出站点文件、监听、`server_name` 与 location 类型；加 `--json` 输出封套（含 `enabled`、`cert_status`） |
| `site show <name>` | 打印站点文件原文；加 `--merged` 输出合并后的完整 server 块；加 `--json` 输出脱敏后的封套（文本模式仍打印原文，包含写在文件里的口令） |
| `acme` | 交互式配置 ACME：问邮箱、CA、挑战方式，写回 `acme` 段 |
| `acme setup` | 同 `acme`，可用 `--email`、`--ca`、`--challenge` 等 flags 非交互执行 |
| `acme show` | 打印生效的 ACME 配置（EAB 私钥脱敏）与会申请证书的域名 |

三点行为需要留意：

- `SIGINT`（`Ctrl+C`）在各平台都是**重载配置**，前台运行也一样；停止用 `SIGTERM`、`SIGQUIT` 或 `stop` 命令。
- `start` 与 `reload` 都会先校验配置：失败直接停下并说明原因，`reload` 此时不动运行中的配置。
- 安装与卸载不走二进制：deb / rpm / apk 负责文件落位，`systemctl enable --now fast-https` 负责常驻。1.5.0 起不再提供 `install` / `uninstall` 子命令，老用户改用包管理器即可。

### 10.1 帮助信息的语言

帮助、用法与 flag 说明默认是中文。需要英文时用 `--lang en`，或者设置环境变量 `FAST_HTTPS_LANG=en`：

```bash
fast-https site add --help             # 中文
fast-https --lang en site add --help   # 英文
FAST_HTTPS_LANG=en fast-https -h       # 英文
```

`--lang` 优先于环境变量；两者都没给时按中文输出。语言只在进程启动时读一次，不影响运行期日志。

手册页也是两份：`man fast-https` 读到的是中文页，`LANG=en_US.UTF-8 man fast-https` 读英文页（装在 `/usr/share/man/en/man1/`）。

### 10.2 机器可读输出与退出码

根命令有三个全局选项：`--json`、`--timeout`、`--request-id`。目前 `--json` 在下列命令上有效：`service version`、`service status`、`service reload`、`runtime status`、`runtime stats`、`monitor status`、`monitor events`、`config show`、`config validate`、`config versions`、`config diff`、`config apply`、`config rollback`、`site list`、`site show`、`site add`、`site update`、`site delete`、`site enable`、`site disable`、`cert list`、`cert check`、`logs error`、`logs access`、`logs query`、`diagnose`、`operation list`、`operation get`，以及保持原格式的 `config path`。`--request-id` 用于修改类命令的幂等。`config path --json` 现有六个键，其中 `state` 是新增的管理状态目录。

`--json` 下标准输出只有一个 JSON 文档，控制台日志改到标准错误。采用封套的命令输出如下格式：

```json
{"ok":true,"command":"service.version","request_id":null,"data":{},"warnings":[]}
{"ok":false,"command":"service.version","request_id":null,"error":{"code":"USAGE_ERROR","message":"...","details":[]}}
```

`config path --json` 保持原有格式（不套封套）。原有五个键不变，S6 增加 `state`；桌面端按键解析，多一个键不影响。

退出码与错误码一一对应：

| 退出码 | 错误码 | 含义 |
| ---: | --- | --- |
| 0 | （成功） | |
| 1 | `INTERNAL_ERROR` | 未分类错误；旧命令的一般错误也是 1，文本仍在标准错误输出 `Error: ...` |
| 2 | `USAGE_ERROR` | 参数、flag 或命令使用错误（未知 flag、多余的位置参数、未知命令） |
| 3 | `PERMISSION_DENIED` | 权限不足 |
| 4 | `SERVICE_UNREACHABLE` | 目标服务不可达或未运行 |
| 5 | `CONFIG_INVALID` | 配置校验失败 |
| 6 | `OPERATION_FAILED`、`RECOVERY_REQUIRED` | 操作失败；恢复失败需人工处理 |
| 7 | `OPERATION_UNKNOWN` | 状态未知，需查询 operation |
| 8 | `TIMEOUT` | 超时 |
| 9 | `NOT_FOUND`、`LOG_SOURCE_UNAVAILABLE` | 资源不存在；日志文件不存在或是符号链接 |
| 10 | `CONFLICT`、`REQUEST_ID_CONFLICT` | 冲突 / 幂等键冲突 |
| 11 | `UNMET_DEPENDENCY`、`NOT_SUPPORTED` | 依赖或环境不满足 / 功能未实现 |

管理类命令按这张表退出。例如配置校验失败是 5，站点或日志文件不存在是 9。旧的 `start` / `stop` / `reload` / `status` 仍用原来的文本和退出码。

### 10.3 `--request-id`、operation 与 SSH 断线

`service reload`、`config apply`、`config rollback`、`site add`、`site update`、`site delete`、`site enable`、`site disable` 会在状态目录写下一条 operation。`--request-id` 由调用方提供：

- 相同的 `--request-id` 加上相同参数：返回已有 operation，不重复执行。
- 相同的 `--request-id` 但参数不同：退出码 10，错误码 `REQUEST_ID_CONFLICT`。
- 没传时 CLI 自己生成一个，并写进 JSON 的 `request_id` 和 operation 记录。

`--dry-run` 不写正式配置、不 reload，默认也不建 operation。非 dry-run 的 `config apply`、`config rollback` 和受控站点写命令必须加 `--yes`（`--confirm` 同义）。

Unix 上，事务执行期间忽略 `SIGHUP` 与 `SIGPIPE`，避免 SSH 断开把正在写状态的进程杀掉。命令超时或调用方断开后，用响应里的 `operation_id` 执行 `operation get`。若记录仍是 `running` 而持有者进程已经不在，查询时会把它对账成 `unknown`。不要在状态未明时换一个新的 `--request-id` 再执行一次。

Windows 没有 `SIGHUP`。这条断线语义只在 Unix 上实现，Windows 上的断线行为未在本环境验证。

状态目录默认是 `./state`（安装包布局是 `/var/lib/fast-https`）。可用 `FASTHTTPS_STATE_DIR` 指到别处。`operation list` 与 `operation get` 只读这个目录。

## 11. 热重载与停机

`reload` 的流程是：解析并校验新配置 → 校验失败则继续用旧快照 → 校验通过则原子替换快照 → 按差异增删改监听器 → 重新打开证书表与日志文件。整个过程中已有连接不受影响。

守护进程模式下的等价操作是向主进程发送 `SIGINT`。停止时先停止接受新连接，再等待在途请求完成。

## 12. 排错

| 现象 | 原因与处理 |
| --- | --- |
| 启动或 `reload` 报 `ssl_certificate not found` | `listen` 含 `ssl` 时证书文件必须存在。`start` 会用本地 CA 自动生成 `config/cert/<server_name>.pem`，`reload` 不生成，所以新装的目录先跑一次 `start`；路径与 `server_name` 不一致时改成实际路径或绝对路径 |
| 启动报 `listen: port already in use` | 端口被占用，换端口或先释放 |
| 配置校验通过但访问 404 | 检查 `root` 是相对路径时的实际解析位置，确认 `server_name` 与请求 `Host` 一致 |
| 反向代理返回 404 或落到静态分支 | `proxy_pass` 缺少 `http://` / `https://` 前缀，被当作 `local` 处理 |
| HTTP/2 不生效 | `listen` 没写 `h2`；或客户端不支持 ALPN。可用 `curl -I --http2 https://...` 验证协议版本 |
| 启用 ACME 后仍是自签证书 | 查日志里的 `acme:` 告警：域名是否解析到本机、80/443 是否可达、CA 是否限速。失败期间服务继续用自签证书，按退避自动重试 |
| 访问返回 421 | `security.host_check` 是 `enforce`，且请求的 `Host` 没命中任何 `server_name`。核对 `server_name`，或改成 `warn` / `off` |
| 访问返回 405 | 该 location 配了 `allow_methods`。响应的 `Allow` 头列出允许的方法；`HEAD` 只在允许 `GET` 时自动放行 |
| 修改配置后不生效 | 执行 `reload`；看日志里是 `acme: manager restarted (reason: ...)` 还是 `kept previous manager, new config is unusable` |
| 日志没有写到新目录 | 新 `log_root` 打不开时程序继续写原文件，检查目录权限 |

## 13. 已知限制

| 项 | 说明 |
| --- | --- |
| ACME 通配符与 IP 证书 | 只实现 `http-01` 与 `tls-alpn-01`，通配符需要 `dns-01`，这类 `server_name` 不会发起申请，继续用自签证书 |
| ACME 管理器重建窗口 | 配置差异触发重建时，旧实例已停、新实例的挑战 token 还没写入，这个毫秒级空窗里恰好来验证会失败一次，由 certmagic 退避重试；配置没变的 `reload` 不重建，不受影响 |
| `engine` 配置段 | 主从注册的预留字段，当前不参与运行逻辑 |
| 上游故障转移 | 未实现，单个上游不可用时直接返回错误页 |
| 配置文件自动重载 | 暂不支持，监听文件变化后自动 reload 仍未排期 |
| `cert renew` | 未实现。续期需要服务端写通道，本阶段不做；读取用 `cert list` 与 `cert check` |
| Monitor 进程 | 采集进程是独立二进制 `fast-https-monitor`，随 deb / rpm / apk 与归档发布，包内另有独立单元 `fast-https-monitor.service`；CLI 不启停它，只读 `status.json` 与 `events.jsonl`，文件不存在时 `monitor status` 为 `not_installed` |

## 14. 版本与许可

版本号取自 `config/consts.go` 的 `CURRENT_VERSION`，启动横幅会打印。许可证见发行包内的 `LICENSE`。
