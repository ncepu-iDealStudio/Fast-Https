# 3. 发行版安装包实机验证

单测和 `tests/packaging/*_test.go` 只能证明"清单写对了、脚本是 LF"，证明不了"装上去真能跑"。这份文档记录一次完整的容器实测结论：**先看这里判断是否已经覆盖，再决定要不要重跑**。

实测日期：2026-11-05（develop 分支）。
被测产物：`1.5.0-next` 快照（`goreleaser release -f build/goreleaser.yaml --snapshot --clean --skip=archive,publish,announce`）。

> **待重跑：Monitor 未覆盖（2026-11-08 起）**
>
> 第 8 步之后，deb / rpm 包里多了 `fast-https-monitor` 二进制、`fast-https-monitor.service` 与 `/etc/fast-https/monitor.json`；`deb.sh` / `rpm.sh` 也加了对应断言（二进制与单元落位、Server 未启动时 `-once` 报「不可达」、`remove` 保留事件目录、`purge` / `rpm -e` 删配置与事件）。
>
> 这些断言**未运行**：本次开发环境没有 Docker，`sh tests/packaging/distro/run.sh all` 没有执行。下面「结果总览」仍是 Monitor 之前的实测结论，合并发布前需在有 Docker 的机器上重跑并补回结论。

## 实测环境

| 项 | 值 |
| --- | --- |
| 宿主 | Windows 11 + Docker Desktop 29.7.2（WSL2 后端） |
| 架构 | `amd64`（宿主与容器一致；`arm64` 产物本次未实装） |
| 镜像 | `ubuntu@sha256:fe815609…ef852`、`almalinux@sha256:245f8aab…57ae2`、`alpine@sha256:ee16d92f…b5bd` |
| 脚本 | [`tests/packaging/distro/`](../../tests/packaging/distro/README.md) |

产物校验和（sha256，取自 `dist/checksums.txt`）：

```text
82cab792282dea1e8574a7653989fc1f12e7c015d288cd274facda5ecee02a0b  fast-https_1.5.0-next_linux_amd64.deb
3992c2f0185851e6241372d839adbd38126b4ac2f46b57b24e3e65d0c393959b  fast-https_1.5.0-next_linux_amd64.rpm
5129ece71259457c02c401fba79dc98cc53d9de25701a47e1860984e8c47ebcd  fast-https_1.5.0-next_linux_amd64.apk
```

## 怎么复现

```bash
# 1. 出包
goreleaser release -f build/goreleaser.yaml --snapshot --clean --skip=archive,publish,announce

# 2. 跑全部场景（deb → upgrade → rpm → apk），需要 docker
sh tests/packaging/distro/run.sh all

# 3. 只跑一个 / 换架构
sh tests/packaging/distro/run.sh rpm
sh tests/packaging/distro/run.sh deb arm64
```

脚本按 `PASS` / `FAIL` 逐项打印，末尾一行 `=== RESULT: ALL-PASS | HAVE-FAILURES ===`，退出码即结果。`INFO` 是解释性输出，不判失败。

## 结果总览

| 场景 | 镜像 | 结论 | 判定项 |
| --- | --- | --- | --- |
| `deb` | `ubuntu:24.04` | ALL-PASS | 文件树、conffile、logrotate、启动监听、轮转、`remove`/`purge` |
| `upgrade` | `ubuntu:24.04` | ALL-PASS | 旧版布局迁移、不覆盖现场文件、迁移后可用 |
| `rpm` | `almalinux:9` | ALL-PASS | 同 deb，另加 `%config(noreplace)` 与 `rpm -e` |
| `apk` | `alpine:3.21` | ALL-PASS | 无 systemd 兜底、logrotate 依赖、卸载语义 |

## 分场景明细

### deb（Ubuntu 24.04）

| 检查项 | 结果 |
| --- | --- |
| `/usr/bin/fast-https`、`/etc/fast-https/{fast-https.json,mime.json,conf.d/default.json,cert}`、`/var/log/fast-https` 落位 | PASS |
| `/etc/logrotate.d/fast-https` 安装且登记为 conffile | PASS |
| `/run` 下无包内文件（pid 由运行期写） | PASS |
| man 页在包内登记（官方镜像 `path-exclude` 导致不落盘，见下） | PASS |
| `logrotate -d` 能解析配置 | PASS |
| `fast-https config path --json` 返回四个 `/etc`、`/var/log`、`/run` 绝对路径 | PASS |
| 启动后 80 与 443 都在听，HTTPS 返回页面 | PASS |
| 首启生成 `/etc/fast-https/cert/localhost.pem`，pid 写入 `/run/fast-https` | PASS |
| `logrotate -f` 产生 `access.log.1`，当前文件被截断，截断后进程仍写当前文件 | PASS |
| `apt remove`：配置、证书、日志、logrotate conffile 全部保留，二进制删除 | PASS |
| `apt purge`：`cert`、`conf.d`、主配置、日志、`run`、logrotate 配置、孤儿 `root.crt` 全部清掉 | PASS |
| `purge` 保留用户自建网页 `/usr/share/fast-https/httpdoc/root/user.html` | PASS |
| `dpkg --audit` 无输出 | PASS |

### upgrade（Ubuntu 24.04，旧版单前缀布局 → FHS）

| 检查项 | 结果 |
| --- | --- |
| 旧 `/usr/share/fast-https/config/cert/*.pem`、`conf.d/*.json`、`cert/acme/account.key` 复制到 `/etc/fast-https` | PASS |
| 包内没有的文件（`mysite.pem`）也被迁移 | PASS |
| 旧目录只复制不删除，`fast-https.json` 等原件仍在 | PASS |
| 主配置用包内默认值，不被旧文件盲目覆盖（旧配置需管理员自己合并） | PASS |
| 重装时"目标已存在则跳过"，不覆盖已改过的 `/etc` 现场文件 | PASS |
| 迁移后 `check` 只缺首启才会生成的自签证书（预期），`start` 能监听 443 并写访问日志 | PASS |
| `purge` 不动旧布局目录（需管理员手工清理） | PASS |

### rpm（AlmaLinux 9）

| 检查项 | 结果 |
| --- | --- |
| `dnf install` 成功，依赖 `ca-certificates`、`logrotate` 从仓库解析 | PASS |
| 文件树、`rpm -qc` 登记（含 `/etc/logrotate.d/fast-https`）、man 页落盘 | PASS |
| 启动监听 80/443、生成自签证书、写 `/run` 与 `/var/log` | PASS |
| `logrotate -f` 轮转后仍写当前文件 | PASS |
| `rpm -U --replacepkgs` 重装保留被改过的 `fast-https.json`（`%config(noreplace)`） | PASS |
| 重装未产生 `.rpmnew`（包内默认值与本机相同，属正常） | PASS |
| `rpm -e` 清掉 cert、`conf.d`、日志、`run`、logrotate 配置、孤儿 `root.crt`、二进制 | PASS |
| 卸载保留用户自建网页，rpm 数据库清除 | PASS |
| `/etc/fast-https` 残留 `fast-https.json.rpmsave`（rpm 对被修改的 `%config` 的正常行为） | INFO |

### apk（Alpine 3.21）

| 检查项 | 结果 |
| --- | --- |
| `apk add --allow-untrusted` 成功，`logrotate` 作为依赖装上 | PASS |
| 无 systemd 时目录由 postinstall 兜底建好 | PASS |
| 启动监听 80/443、生成自签证书、写 `/run` 与 `/var/log` | PASS |
| `logrotate -f` 轮转后仍写当前文件 | PASS |
| 卸载删除包内默认配置与 `/etc/logrotate.d/fast-https` | PASS |
| 卸载保留运行期生成的 `cert/localhost.pem` 与 `/var/log/fast-https`（apk 无 purge，`post-deinstall` 也拿不到参数） | PASS |

## 环境差异（看着像缺陷，其实不是）

| 现象 | 原因 | 处理 |
| --- | --- | --- |
| 装完 `/usr/share/man/man1/fast-https.1` 不存在 | 官方 `ubuntu` 镜像带 `path-exclude=/usr/share/man/*` | 以 `dpkg -L` / `rpm -ql` / `apk info -L` 的登记为准，脚本已分开判定 |
| `dnf install … curl` 报 `curl-minimal` 冲突 | AlmaLinux 预装 `curl-minimal`，与 `curl` 互斥 | 脚本只在没有 `curl` 时才装 |
| 容器里 `fast-https start` 不返回、一直占前台 | `runInForeground()` 在容器内为真，`start` 故意不 daemonize | `common.sh` 的 `start_server` 自己转后台并轮询端口，`trap stop_service EXIT` 兜底 stop |
| `rpm -e` 后 `/etc/fast-https` 剩 `.rpmsave` | rpm 对被修改过的 `%config` 的标准行为 | 视为正常，脚本打印 INFO |

## 覆盖边界

**未覆盖**：

- `arm64` 产物（脚本支持 `run.sh deb arm64`，本次未跑，宿主是 amd64）
- 真实 systemd 托管（容器无 init，`systemctl enable --now` 与 `RuntimeDirectory=` 未实测）
- 真实跨版本升级（本次是"构造旧布局 + 装新版"，不是拿 1.5.0 包升级）
- 归档 / Docker 布局的日志轮转（`./logs` 不由包配置管辖）
- 签名与仓库安装路径（`publish-repo.sh` 产物、GPG 校验未在容器内验证）

**必须重跑的场景**（改了这些文件就跑一遍 `sh tests/packaging/distro/run.sh all`）：

| 改动 | 原因 |
| --- | --- |
| `build/goreleaser.yaml` | 文件落位、依赖、scripts 挂载全靠它 |
| `build/packaging/**` | 维护脚本、unit、logrotate 配置 |
| `config/base_dir*.go`、`config/consts.go` | 运行期路径基址 |
| `cmd/` 里的启动/停止/pid 逻辑 | 容器内前后台行为、`stop` 是否真能停 |

## 与其它文档的关系

- 缺陷与根因：[help/dev-issues.md 002](../help/dev-issues.md)（本文是验证记录，那里是问题记录）
- 打包与 FHS 布局：[help/build-and-install.md](../help/build-and-install.md)、[help/package-layout.md](../help/package-layout.md)
- 发布前清单：[develop/5-release.md](../develop/5-release.md)
