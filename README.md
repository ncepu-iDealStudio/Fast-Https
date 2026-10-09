# Fast-Https Web服务器

## 介绍

Fast-Https 是一款用 Go 开发的多任务、高并发 Web 服务器。当前实现覆盖 HTTP/1.1、HTTP/2、HTTPS、静态站点、HTTP/HTTPS 反向代理、TCP 代理和 WebSocket 升级。测试环境下，服务启动时可以为带 `ssl` 的站点自动生成自签名证书。

目前提供 Windows 与 Linux 安装包，其它平台陆续补充。

Fast-Https 已加入 openEuler 社区（<https://gitee.com/src-openeuler/fast-https>），欢迎更多开发者一起完善。

## 技术栈及软件版本和相关重要依赖

当前产品版本是 `V1.3.2`（`config/consts.go` 的 `CURRENT_VERSION`）。
模块声明的 Go 版本是 **1.27.0**，工具链固定为 **go1.27.1**，见 `go.mod`。这是 2026 年 10 月仍在官方支持期内的当前主流版本，同时受支持的还有 1.26。根目录 `Dockerfile` 使用 `golang:1.27-alpine` 编译；`spec` 里的 RPM 构建脚本下载 Go 1.27.1。


服务自己解析 HTTP/1.1 和 HTTP/2。标准库提供 TCP、`crypto/tls` 和 `gzip`。开发模式在 `0.0.0.0:10000` 使用 `net/http` 的默认多路复用器提供 pprof。

运行时直接依赖如下，版本以 `go.mod` 为准：

| 依赖 | 版本 | 用途 |
| --- | --- | --- |
| `github.com/spf13/cobra` | v1.8.1 | 命令行：`start`、`dev`、`stop`、`reload`、`install`、`uninstall`、`status` |
| `github.com/spf13/viper` | v1.19.0 | 读取 `config/fast-https.json` 和 `http.include` |
| `github.com/kardianos/service` | v1.2.2 | 安装、卸载系统服务 |
| `github.com/sirupsen/logrus` | v1.9.3 | `logs/` 下的系统、访问、错误、安全日志 |
| `github.com/fufuok/chanx` | v1.2.2 | 日志消息队列 |
| `golang.org/x/time` | v0.5.0 | 连接限流 |
| `github.com/microcosm-cc/bluemonday` | v1.0.26 | `xss` 规则清理 JSON 中的 HTML |
| `github.com/andybalholm/brotli` | v1.1.0 | Brotli 压缩 |
| `github.com/Jxck/hpack` | 2015-07-13 快照 | HTTP/2 HPACK 整数编码与 Huffman |
| `github.com/emirpasic/gods` | v1.18.1 | 缓存使用的红黑树 |
| `github.com/panjf2000/ants/v2` | v2.9.1 | 协程池 |
| `github.com/chenhg5/collection` | 2020-09-25 快照 | 监听端口去重 |
| `github.com/getlantern/systray` | v1.2.2 | Windows 托盘程序 `monitor/` |
| `golang.org/x/sys` | v0.21.0 | Windows 控制台 Ctrl+C |
| `github.com/fatih/color` | v1.17.0 | 控制台着色 |

测试依赖 `github.com/stretchr/testify` v1.9.0，不参与服务器二进制。发布包用 GoReleaser，配置是 `.goreleaser.yaml`。快照版本号是 `1.3.2-next`。

## 软件架构

服务自己完成监听、请求解析和响应写出。入口在 `fast-https.go`，命令由 `cmd` 分发，配置由 `config` 加载，初始化在 `init`，连接与协议处理在 `modules/core`。每个监听端口由独立协程 `Accept`，再按连接类型进入 HTTP/1.1 事件循环或 HTTP/2 处理。

`location.type` 决定请求交给哪个处理函数。`modules/core/server` 通过空白导入注册静态文件、反向代理、重写和开发模块。

| `location.type` / `listen` | 处理位置 | 行为 |
| --- | --- | --- |
| `local` | `modules/static` | 按 `root` / `index` 返回本地文件 |
| `proxy`，地址以 `http` 开头 | `modules/proxy` | HTTP 反向代理 |
| `proxy`，地址以 `https` 开头 | `modules/proxy` | HTTPS 反向代理 |
| `rewrite` | `modules/rewrite` | 返回 301，`Location` 为配置中的重写地址 |
| `devmod` | `modules/dev_mod` | 返回固定的开发响应 |
| `listen` 含 `tcp` | `modules/proxy_tcp` | 按 TCP 双向转发，不走 HTTP 解析 |
| `listen` 含 `ssl` | `modules/core/listener` | TLS |
| `listen` 含 `h2` | `modules/core/h2` | HTTP/2 |

同一条请求还会经过连接过滤（黑名单、限流）、可选的 Basic 认证（`modules/auth`）和访问计数（`modules/safe`）。静态与代理路径会执行应用防火墙：`xss` 在 JSON 对象或数组里的 `<script` 被去掉后返回 403；`sql` 已注册，不会拦截请求。

`listen` 含 `h2` 时，未知帧类型丢掉载荷后继续读下一帧。HPACK 索引非法时关闭这条流。对端流量窗口不够时，停止写出 DATA 并关闭连接。

### 项目结构

```text
fast-https/
├── fast-https.go                 # 程序入口，调用 cmd.RootCmd()
├── cmd/                          # 命令行：start / dev / stop / reload / install / uninstall / status
│   ├── commands.go
│   ├── unix_linux_amd64.go       # Linux amd64 守护进程
│   ├── unix_other.go             # 其它平台的守护进程空实现
│   ├── reload_windows.go         # Windows 重载信号
│   └── reload_other.go           # 非 Windows 重载信号
├── config/                       # 运行时配置
│   ├── fast-https.json           # 配置真源
│   ├── conf.d/                   # 由 http.include 继续加载的 json
│   ├── mime.json                 # Content-Type 映射
│   ├── cert/                     # 站点证书；缺失时由 init 生成
│   ├── dev.yaml                  # 开发参数示例，运行时不会加载
│   ├── config.go                 # 解析、校验、重载
│   ├── consts.go                 # 路径、版本、默认值
│   ├── base_dir.go               # 默认工作目录 ./
│   └── base_dir_rpm.go           # -tags=rpm 时安装到 /usr/share/fast-https/
├── init/
│   ├── init.go                   # 配置、日志、证书、缓存、安全模块初始化
│   └── autocert.go               # 自签名根证书与站点证书
├── modules/
│   ├── module.go                 # 模块注册表
│   ├── core/                     # 监听、事件、HTTP/1.1、HTTP/2、请求与响应
│   │   ├── server/               # 启停、信号、热重载
│   │   ├── listener/             # 按 listen 建监听器
│   │   ├── events/               # 连接与请求分发
│   │   ├── request/  response/   # 报文解析与默认响应
│   │   ├── filters/              # 连接、监听、解析、路由过滤
│   │   ├── h2/                   # HTTP/2 帧、流、HPACK
│   │   ├── dynlog/               # 访问日志注册
│   │   └── engine/               # master/slave 注册与心跳（当前启动路径未调用）
│   ├── static/  proxy/  proxy_tcp/  rewrite/  websocket/
│   ├── auth/  safe/  cache/  compress/  appfirewall/  logging/
│   ├── dev_mod/                  # location type = devmod
│   └── workchain/example/        # 模块注册示例，服务启动时不会加载
├── output/                       # 启动 Logo 与初始化提示
├── utils/                        # 日志、消息、文件、颜色、RSA
├── monitor/                      # Windows 托盘程序（独立 main）
├── httpdoc/root/                 # 默认静态站点与自签名根证书存放目录
├── test/                         # 单元测试、客户端集成测试、开发脚本
├── docker/                       # CentOS 7、Ubuntu 镜像
├── Dockerfile                    # 多阶段构建，暴露 8080 与 443
├── spec                          # RPM spec，构建时使用 -tags=rpm
├── shell/.acme.sh/               # 随仓库附带的 acme.sh，Go 启动流程不会调用
├── engine.sh                     # 替换引擎标识后分别编译 master/slave
├── .goreleaser.yaml              # Linux、Darwin、Windows 发布
└── .workflow/go-test.yml         # Gitee Go：推送和合并请求上执行 go test ./...
```

### 服务启动流程

1. `fast-https.go` 将日志级别设为 4，执行 `cmd.RootCmd()`。没有子命令时按 `start` 处理。
2. `start` / `dev` 先做预检：校验 `config/fast-https.json` 及其 `include`、扫描端口、确认没有已在运行的实例。
3. 打印 Logo 后调用 `init.InitSystem()`：加载配置与 `mime.json`、启动消息日志、生成或加载证书、从磁盘恢复缓存并启动过期清理、初始化安全模块。
4. `start` 在容器外的 Linux amd64 上会 fork，并保持当前工作目录，再把子进程 pid 写入 `fast-https.pid`。Windows，以及设置了 `FASTHTTPS_FOREGROUND=1` 或检测到容器标记文件时，进程保持前台并写同一 pid 文件。`dev` 不进入守护进程，日志级别改为 6，并在 `0.0.0.0:10000` 打开 pprof。
5. `server.ServerInit()` 注册信号处理，按配置创建监听器，再注册动态日志。
6. 每个端口一个协程接受连接。`listen` 含 `h2` 时走 HTTP/2，其余走 HTTP/1.1 事件循环。

信号处理：

- `SIGTERM`、`SIGQUIT`：结束服务
- `SIGINT`：Windows 下结束服务；其它平台下重载配置
- `reload` 命令读取 pid 后发信号：Windows 发送控制台 Ctrl+C，其它平台发送 `SIGINT`

重载会重新读取配置，热更新仍在监听的端口，关闭已删除的端口，并为新增端口启动协程。

## 安装教程

1. 在 <https://gitee.com/ncepu-bj/fast-https/releases/> 获取对应版本的安装包
2. 解压到目标目录
3. 修改 `config/fast-https.json`，需要拆分的站点配置放到 `http.include` 指向的目录（默认 `config/conf.d`）

## 开发模式说明

### 配置真源说明

- 运行时唯一配置真源是 `config/fast-https.json`
- 启动时读取该文件，再按 `http.include` 加载其中列出的 json（默认包含 `config/conf.d`）
- `config/mime.json` 提供扩展名到 Content-Type 的映射
- `config/dev.yaml` 只是开发参数示例，运行时不会加载

### 启动常用命令

```bash
# 前台或守护进程启动（无参数等同 start）
go run fast-https.go start

# 开发模式：日志级别 6，并打开 10000 端口的 pprof
go run fast-https.go dev

# 停止、重载、安装为系统服务、卸载
go run fast-https.go stop
go run fast-https.go reload
go run fast-https.go status
go run fast-https.go install
go run fast-https.go uninstall
```

`status` 读取 `fast-https.pid`。进程在运行时打印 pid 并以 0 退出；没有 pid 文件，或 pid 对应的进程已经不在时，以非 0 退出。

`reload` 成功后会按当前 `log_root` 重新打开 `logs` 下的四份日志。新目录打不开时，继续写原来的文件。

### 开发模式特性

1. **详细日志**
   - 日志级别设为 6
   - 保留初始化阶段的控制台输出

2. **调试入口**
   - `0.0.0.0:10000` 使用默认 HTTP 多路复用器，可接 pprof
   - 示例：`go tool pprof http://localhost:10000/debug/pprof/profile`

3. **进程行为**
   - 不进入 Linux 守护进程
   - 证书文件不存在时，`init.CertInit` 会为带 `ssl` 的 `server_name` 生成自签名证书

### 最小配置

`config/fast-https.json`：

```json
{
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

仓库里的默认配置还包含 `443 ssl` 的 localhost 站点，证书路径为 `config/cert/localhost.pem` 与 `config/cert/localhost-key.pem`。

### 注意事项

1. `dev` 会打开调试端口并提高日志量，适合本机开发
2. 自签名证书只用于测试访问
3. 修改配置后执行 `reload`，或在非 Windows 的运行进程上发送 `SIGINT`

## 使用说明

见文档：<https://idealstudio-ncepu.yuque.com/dkna2e/lbeklg?#> 《Fast-Https产品说明》

测试布局与运行方式见 [test/readme.md](test/readme.md)。

## 打包与发布

语言版本 **1.27.0**，工具链 **go1.27.1**，见 `go.mod`。产品版本是 **V1.3.2**。RPM 版本写成 `1.3.2`（没有 `V` 前缀）。GoReleaser 快照版本是 `1.3.2-next`。下面的命令都在仓库根目录执行。

加载配置时会把站点 root、证书、日志目录和 include 转成绝对路径。容器镜像设置 `FASTHTTPS_FOREGROUND=1`，进程保持前台，不 fork，也不切换工作目录。

### 本地二进制

```bash
go build -o fast-https .
```

产物是当前平台的 `fast-https`。运行时需要带上 `start` 或 `dev`，例如 `./fast-https start`。

### Windows 托盘

发布配置会在打包前编译 `monitor/monitor.exe`，不依赖 `windres`。托盘图标来自 `icon.go`。手动编译和菜单说明见 [monitor/README.md](monitor/README.md)。

### 安装包

一份 `.goreleaser.yaml` 同时打 Linux、Darwin 和 Windows。`CGO_ENABLED=0`。Linux / Darwin 的目标架构是 amd64 和 arm64，归档为 `tar.gz`。Windows 只打 amd64，链接参数为 `-s -w -H=windowsgui`，归档为 `zip`，根目录包含 `monitor.exe`。输出都在 `dist/`。

```bash
goreleaser release -f .goreleaser.yaml --snapshot --clean
```

`--snapshot` 使用版本 `1.3.2-next`，不依赖 git tag。`--clean` 会清空上一次的 `dist/`。

压缩包打入 `config/fast-https.json`、`config/mime.json`、`config/fastcgi.conf`、`docs/`、`httpdoc/root/index.html`、`httpdoc/root/favicon.ico` 和 `README.md`。另外列出了 `config/cert/.keep`、`config/conf.d/.keep` 和 `logs/.keep`，用来在包里保留空目录。

### RPM

`spec` 的版本是 `1.3.2`。构建时下载 Go 1.27.1，并用 `-tags=rpm` 编译。这个 tag 把配置和站点根目录固定为 `/usr/share/fast-https/`。安装结果是：

- 可执行文件：`/usr/bin/fast-https`
- 配置、默认站点和日志目录：`/usr/share/fast-https/`

### 容器镜像

三份 Dockerfile 都从当前源码构建，构建阶段是 `golang:1.27-alpine`。镜像只复制配置 JSON 和默认站点文件，并新建空的 `config/cert`、`config/conf.d` 和 `logs`。本机证书和日志不会进镜像。暴露 8080 和 443。

```bash
docker build -t fast-https .
docker build -f docker/Dockerfile_ubuntu -t fast-https:ubuntu .
docker build -f docker/Dockerfile_centos7 -t fast-https:centos7 .
```

运行阶段分别是 Alpine、Ubuntu 24.04 和 CentOS 7。Ubuntu 与 CentOS 镜像把程序放在 `/usr/local/fast-https`，环境变量 `Fast-Https_VERSION` 为 `1.3.2`。

## 参与贡献

1. Fork 本仓库
2. 新建 Feat_xxx 分支
3. 提交代码
4. 新建 Pull Request
