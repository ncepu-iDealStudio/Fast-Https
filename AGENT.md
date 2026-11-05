# AGENT.md

给 AI 编码助手与新加入的开发者：先读这一页，再动代码。

Fast-Https 是 Go 写的 web server 与反向代理，自带 ACME 证书签发。同一份代码要产出五种形态：源码直跑、tar.gz/zip 归档、Docker 镜像、Windows NSIS 安装包、Linux deb/rpm/apk。**这五种形态的运行期路径不一样**，是这个仓库最容易踩坑的地方。

## 硬规则

违反下面任何一条，都会产出"能编译、装完却不能运行"的包。

### 1. 打包相关文件必须是 LF 行尾

`build/` 下的 shell 脚本、`build/spec`、`build/Dockerfile*` 会被 dpkg/rpm 直接 `exec`。CRLF 会让 shebang 变成 `#!/bin/sh\r`，内核去找名为 `/bin/sh\r` 的解释器，报 `ENOENT`——表现是**装得上、卸载不掉**，且错误信息说"无法执行 prerm：没有那个文件或目录"，而文件明明在。

Windows 上 `core.autocrlf=true` 会自动把工作区转成 CRLF，所以：

- `.gitattributes` 已声明 `*.sh text eol=lf`、`build/spec`、`build/Dockerfile`、`build/docker/*`
- `go test ./tests/packaging` 会扫所有会进包的文件，发现 `\r` 就失败
- 新增打包脚本后，确认规则覆盖到它

### 2. 两套目录布局由 build tag 决定，不是运行时判断

| 构建 | tag | 配置/证书 | 站点 | 日志 | pid |
| --- | --- | --- | --- | --- | --- |
| 归档 / Docker / Windows | 无 | `./config` | `.` | `./logs` | `.` |
| deb / rpm / apk | `-tags=rpm` | `/etc/fast-https` | `/usr/share/fast-https` | `/var/log/fast-https` | `/run/fast-https` |

- 基址常量在 `config/base_dir.go`（`!rpm`）与 `config/base_dir_rpm.go`（`rpm`），改一处必须两处一起改。
- 该 tag 名叫 `rpm`，但 deb 和 apk 也用（goreleaser 的 `linux-pkg` 构建共用）。
- 改完必须跑：`go test ./config/...` + `go test -tags=rpm -run TestPackageLayoutPaths ./config/`。
- 归档布局的路径**一个字都不能变**，`config/paths_layout_test.go` 逐字锁住。

### 3. 生成物不能手改

| 生成物 | 由谁生成 | 校验 |
| --- | --- | --- |
| `build/packaging/man/fast-https.1` | `go run ./tools/manpage` | `go test ./tools/manpage` |
| `docs/usage.html` | `go run ./tools/docgen` | `go test ./tools/docgen` |

改了命令树（`cmd.RootCmd()`）或 `docs/usage.md`，必须重新生成，否则测试直接失败。

### 4. 版本号三处同步

`config/consts.go` 的 `CURRENT_VERSION`、`build/goreleaser.yaml` 的 `snapshot.version_template`、Windows NSIS 的 `APP_VERSION` 默认值。漏一处会出现"文件名与徽章各说各的"。

## 常用命令

```bash
go build ./... && go vet ./...            # 两种布局都要过
go vet -tags=rpm ./...
go test ./...                              # 全量单元与 E2E
go test -race ./...                        # 提交前
go test ./tests/packaging                  # 行尾、打包清单、轮转配置守卫
sh tests/packaging/distro/run.sh all        # 容器里真装一遍（deb/upgrade/rpm/apk，需要 docker）
go run . check                             # 归档布局下校验配置
go run . config path --json                # 打印运行期四个绝对路径

go run ./tools/manpage                     # 重新生成 man 页
go run ./tools/docgen                      # 重新生成 usage.html

goreleaser check -f build/goreleaser.yaml
goreleaser release -f build/goreleaser.yaml --snapshot --clean
```

Windows 上并行跑测试偶发 `Access is denied`，用 `go test -p 1 ./...`。

## 目录地图

| 位置 | 改这里意味着什么 |
| --- | --- |
| `config/` | 配置解析、路径基准、校验。改路径先看 `base_dir*.go` 与 `consts.go` |
| `cmd/` | 命令行。新增子命令要重新生成 man 页 |
| `runtime/` | 监听、连接、请求/响应、reload |
| `modules/` | 各功能模块（proxy/static/cache/compress/auth/waf/certmanager…） |
| `init/` | 启动编排：证书初始化、自签 CA |
| `build/packaging/` | 打包资源：systemd 单元、安装脚本、man 页、仓库脚本 |
| `build/goreleaser.yaml` | 产物定义：构建、归档、nfpms |
| `tests/packaging/` | 打包产物守卫：行尾、logrotate 覆盖、卸载语义、distro 容器脚本结构 |

## 提交前自检

1. `go build ./... && go vet ./...` 干净（两种 tag）
2. `go test ./...` 全绿
3. 改了命令树/usage.md → 重新生成对应产物
4. 改了打包脚本或安装布局 → `go test ./tests/packaging`，再 `sh tests/packaging/distro/run.sh all` 确认真能装上
5. 改了运行期路径 → 两种布局各自 `check` 一遍
6. 行为变化同步 `docs/feature/`，结构变化同步 `docs/architecture-map.md`

## 文档去哪找

| 需要什么 | 位置 |
| --- | --- |
| 安装、构建、卸载、排障 | [docs/help/](docs/help/README.md) |
| 环境、测试、规范、发布 | [docs/develop/](docs/develop/README.md) |
| 功能怎么用、怎么实现 | [docs/feature/](docs/feature/README.md) |
| 历史踩坑记录 | [docs/help/dev-issues.md](docs/help/dev-issues.md) |
| 装包实机验证的结果与是否要重跑 | [docs/test/3-distro-verification.md](docs/test/3-distro-verification.md) |
| 文档总索引 | [docs/README.md](docs/README.md) |
