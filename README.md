<h1 align="center">Fast-Https</h1>

<p align="center">用 Go 写的多任务、高并发 Web 服务器与反向代理。<br />
一份 JSON 描述监听端口与 location，单文件二进制直接跑。</p>

<p align="center">
  <img src="https://img.shields.io/badge/version-1.4.0-0E7490?style=flat-square" alt="version" />
  <img src="https://img.shields.io/badge/Go-1.27-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go" />
  <img src="https://img.shields.io/badge/HTTP%2F2-x%2Fnet%2Fhttp2-0E7490?style=flat-square" alt="HTTP/2" />
  <img src="https://img.shields.io/badge/platform-Linux%20%7C%20macOS%20%7C%20Windows-334155?style=flat-square" alt="platform" />
  <img src="https://img.shields.io/badge/license-GPL--2.0-334155?style=flat-square" alt="license" />
  <img src="https://img.shields.io/badge/Gitee-ncepu--bj%2Ffast--https-C71D23?style=flat-square&logo=gitee&logoColor=white" alt="gitee" />
</p>

<p align="center">
  <img src="https://skillicons.dev/icons?i=go,docker,linux,windows" height="34" alt="tech stack" />
</p>

<p align="center">简体中文 | <a href="README.en.md">English</a></p>

## 介绍

Fast-Https 自己完成监听、HTTP/1.1 解析和响应写出，不依赖 nginx / Caddy 这类现成服务器；HTTP/2 的分帧与 HPACK 交给 `golang.org/x/net/http2`，TLS 用标准库。配置是纯 JSON，`location` 的写法与 nginx 的心智接近。

一条请求的处理链路：

```text
连接 → 限流 / IP 黑名单 → location 路由 → 应用防火墙 → 静态文件 / 反向代理 / TCP / WebSocket → 响应
```

- **静态站点**：`Last-Modified` 条件请求返回 304，单段 `Range` 返回 206/416，按 `Accept-Encoding` 协商 gzip / brotli；目录缺结尾斜杠时 301 到带斜杠的地址。
- **反向代理**：HTTP/HTTPS 上游连接池复用、响应缓存（map 索引 + LRU）、WebSocket 升级、TCP 双向转发。
- **安全**：per-IP 令牌桶限流、IP 黑名单（含 IPv6 网段）、Basic 认证、GuardianWAF 应用防火墙（sqli/xss/lfi/cmdi/xxe/ssrf，可只记录不阻断）。
- **运维**：`start` / `stop` / `reload` / `status` / `check` / `dev`，配置热重载不断连接；Windows 有托盘程序，支持安装为系统服务。
- **交付**：Linux / macOS / Windows 安装包、RPM、三套 Docker 基础镜像。

核心依赖（版本以 `go.mod` 为准，其余均为标准库）：

| 依赖 | 用途 |
| --- | --- |
| `spf13/cobra` | 命令行入口与子命令 |
| `rs/zerolog` | 控制台 + 系统/访问/错误/安全四路 JSON 日志 |
| `golang.org/x/net` | HTTP/2 协议栈 |
| `guardianwaf/guardianwaf` | 应用防火墙引擎 |
| `klauspost/compress`、`andybalholm/brotli` | gzip 与 brotli 压缩 |
| `kardianos/service`、`fyne.io/systray` | 系统服务安装、Windows 托盘 |

## 快速开始

```bash
# 方式一：安装包
# Windows 用 dist 里的 fast-https-<version>-windows-<arch>-setup.exe 安装；
# 其他平台到 https://gitee.com/ncepu-bj/fast-https/releases 下载压缩包，解压后进入目录
./fast-https start

# 方式二：源码构建（需要 Go 1.27）
go build -o fast-https . && ./fast-https start
```

随包的 `config/fast-https.json` 已经是一份可直接服务的默认配置（等价于 nginx 的默认站点）：80 端口对外提供 `./httpdoc/root`，非 root 运行时把它改成 `8080` 之类的高位端口。

```json
{
  "http": {
    "server": [
      {
        "listen": 80,
        "server_name": "localhost",
        "location": [
          { "url": "/", "type": "local", "root": "./httpdoc/root", "index": ["index.html", "index.htm"] }
        ]
      }
    ],
    "include": ["./config/conf.d"]
  }
}
```

```bash
curl -I http://127.0.0.1/    # 期望 200 与 Server: fast-https
```

配置支持 `//` 与 `/* */` 注释和尾随逗号，随包文件里每段配置都带注释说明。

| 命令 | 作用 |
| --- | --- |
| `./fast-https start` | 启动（无参数等同 `start`）。Linux / macOS 转后台，容器与 systemd 托管时保持前台 |
| `./fast-https dev` | 前台启动，日志级别 6，并在 `0.0.0.0:10000` 打开 pprof |
| `./fast-https stop` | 停止。Unix 发 `SIGTERM` 优雅停机，重复执行视为已停止 |
| `./fast-https reload` | 校验配置后热重载，失败保留旧配置 |
| `./fast-https status` | 读 pid 文件判断是否在运行 |
| `./fast-https check` | 只校验配置，不监听端口、不写 pid |
| `./fast-https install` | 安装为系统服务 |

## 使用文档

| 文档 | 内容 |
| --- | --- |
| [Fast-Https 产品说明（中文）](https://idealstudio-ncepu.yuque.com/kgagg7/lbeklg) | 面向部署运维：配置项、站点、证书、代理、缓存、WAF |
| [Fast-Https User Guide (English)](https://idealstudio-ncepu.yuque.com/kgagg7/rstqtx) | 同上，英文版 |
| [docs/usage.md](docs/usage.md) | 仓库内使用文档源文件，生成 `docs/usage.html` 随发行包分发 |
| [docs/feature/](docs/feature/README.md) | 按功能分篇的说明，每篇含配置示例与实现方案 |
| [docs/develop/](docs/develop/README.md) | 环境构建、测试体系、代码规范、新增配置项、调试、发布 |
| [docs/architecture-map.md](docs/architecture-map.md) | package 盘点与依赖方向 |
| [docs/nginx-core-gap.md](docs/nginx-core-gap.md) | 与 nginx 核心功能的差异对比 |

文档总索引见 [docs/README.md](docs/README.md)。

## 项目结构

```text
fast-https/
├── fast-https.go        入口，调用 cmd.RootCmd()
├── cmd/                 命令行：start / dev / stop / reload / status / check / install
│                        daemon_unix.go 后台化（exec 自身），daemon_windows.go 为空实现
├── config/              配置解析、校验与热重载，fast-https.json 是唯一真源
├── init/                启动初始化：配置、日志、证书、缓存
├── core/                契约层：Event、处理器注册表、生命周期钩子
├── runtime/             数据面：监听、连接、请求/响应、过滤、热重载
├── modules/             功能包：static / proxy / proxy_tcp / websocket / cache /
│                        compress / auth / safe / appfirewall / rewrite
├── infra/               基础设施：logging / color / timeutil / banner
├── monitor/             Windows 托盘程序（独立 main）
├── httpdoc/root/        默认站点与自签发根证书目录
├── docs/                功能、开发、测试文档
├── tests/               单元测试与端到端测试
└── build/               构建与打包：goreleaser、Dockerfile、RPM spec、NSIS、systemd unit
```

分层约定：`core` 只放契约，`runtime` 不认识 `modules`，功能包统一由 `cmd.RegisterHandlers` / `cmd.RegisterServices` 在启动时装配。细节见 [docs/develop/2-code-standards.md](docs/develop/2-code-standards.md)。

## 更多

- 打包与发布（二进制、RPM、Docker、GoReleaser）：[docs/develop/5-release.md](docs/develop/5-release.md)
- 测试布局与运行方式：[tests/readme.md](tests/readme.md)；覆盖率与压测：[docs/test/](docs/test/README.md)
- 启动流程与信号处理：[docs/about.md](docs/about.md)
- 参与贡献：Fork 仓库 → 新建 `Feat_xxx` 分支 → 提交 → 发起 Pull Request
- 许可证 [GPL-2.0](LICENSE)；项目已加入 openEuler 社区（<https://gitee.com/src-openeuler/fast-https>）
