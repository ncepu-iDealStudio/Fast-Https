# 开发问题日志

记录**已定位并修复**的缺陷与踩坑，重点写清根因和防回归手段。排查中的问题先放 [troubleshooting.md](troubleshooting.md)，确认是代码或打包缺陷并修掉后再补到本文件。

编号递增，不复用。新条目加在最后——编号即时间顺序。

## 条目模板

```md
## 0NN 一句话描述

- 发现：YYYY-MM-DD（环境）
- 影响：哪些产物/场景受影响
- 症状：用户能看到什么
- 根因：为什么会这样
- 修复：改了哪些文件
- 防回归：加了什么测试或检查
```

## 001 CRLF 行尾打进包导致卸载失败

- 发现：2026-11-05，Ubuntu 24.04 实机，`apt install` 后 `apt remove` 失败
- 影响：deb 装得上、卸不掉；rpm 与 apk 的维护脚本来源相同，同样中招
- 症状：

  ```text
  dpkg (子进程)：无法执行 old fast-https package prerm maintainer script
                 (/var/lib/dpkg/info/fast-https.prerm): 没有那个文件或目录
  dpkg: 处理软件包 fast-https (--remove)时出错：
   old fast-https package prerm maintainer script subprocess failed with exit status 2
  ```

  `--force-all` 也绕不过去。文件 `/var/lib/dpkg/info/fast-https.prerm` 明明存在，所以"补一个空脚本"的判断条件 `[ -f ... ]` 会直接跳过。

- 根因：`build/packaging/linux/preremove.sh` 在工作区是 CRLF 行尾，goreleaser 从工作区读文件原样打包，deb 里的 `prerm` 首行变成 `#!/bin/sh\r`。dpkg 执行时内核去找名为 `/bin/sh\r` 的解释器，不存在 → `ENOENT` → 退出码 2。

  Windows 开发机 `core.autocrlf=true`，工作区所有文本文件都是 CRLF（`git ls-files --eol` 显示大量 `i/lf w/crlf`）。仓库当时没有任何 eol 规则，于是 CRLF 一路进了包。`postinstall.sh` 恰好刚被重写过（LF），所以只有它正常执行，问题的迷惑性也来自这里——装了包能看到 postinstall 的输出，却卸载不掉。

- 修复：
  - `.gitattributes` 增加 `*.sh text eol=lf`、`build/spec`、`build/Dockerfile`、`build/docker/*`
  - `preremove.sh`、`postremove.sh`、`postinstall.sh`、`build/spec`、两个 Dockerfile 重新检出为 LF
  - 三个维护脚本头部加注释，写明为什么必须 LF
  - 已装坏包的机器：从新 deb 里 `dpkg-deb --ctrl-tarfile | tar -x` 取出正确脚本覆盖 `/var/lib/dpkg/info/fast-https.{prerm,postrm}`，再 `dpkg --purge`

- 防回归：新增 `tests/packaging/lf_test.go`，扫描 `build/packaging/**/*.sh`、systemd 单元、`build/spec`、Dockerfile，发现 `\r` 即失败。`docs/develop/5-release.md` 的发布前清单也加了这条检查。

- 教训：**打包产物的字节级正确性要靠测试守，不能靠"在 Linux 上试一次"**。这类问题在 Windows 开发机上完全看不出来，一到 Linux 装包才暴露，且报错信息指向的路径是存在的，容易误判为 dpkg 数据库损坏。

## 002 日志没有轮转，卸载语义只在文档里

- 发现：2026-11-05，Ubuntu 24.04 / AlmaLinux 9 / Alpine 3.21 容器实装
- 影响：deb / rpm / apk；归档与 Docker 布局仍无自动轮转
- 症状：
  - `/var/log/fast-https` 四个文件只增不减，长期跑高流量站点会把 `/var` 写满
  - 文档写"apk 卸载一律保留配置与证书"，与 apk 实际行为不符：包内默认配置会被删掉
- 根因：
  - 日志目录 1.5.0 才搬到 `/var/log`，当时只改了路径，没有配套 logrotate 配置
  - 进程用 `O_APPEND` 常开四个 fd。标准的 `create` + `postrotate` 会把当前文件改名、新建同名文件，进程仍写旧 inode，新文件永远是空的，所以轮转策略必须用 `copytruncate`
  - apk 没有 purge 概念，卸载时删除包内文件（含默认配置与空目录），只有运行期生成的文件留下
- 修复：
  - 新增 `build/packaging/logrotate/fast-https`，装到 `/etc/logrotate.d/fast-https`，登记为配置文件（deb conffile、rpm `%config(noreplace)`）
  - `nfpms.dependencies` 增加 `logrotate`：没装它这条配置不会被执行，属硬依赖
  - 策略 `daily / rotate 14 / maxsize 100M / compress / delaycompress / copytruncate`
  - 修正 apk 卸载的文档与断言：包内默认配置随包删除，证书、ACME 账户与日志保留
- 防回归：
  - `tests/packaging/logrotate_test.go`：轮转规则必须覆盖 `infra/logging` 的四个文件名（改名即失败）、必须含 `copytruncate`、`goreleaser.yaml` 必须安装并以依赖形式声明 `logrotate`
  - `tests/packaging/distro/*.sh`：在真容器里装包、启动、`logrotate -f` 后验证"截断仍继续写当前文件"、再走卸载
  - `tests/packaging/distro_test.go`：每个验证脚本必须 source `common.sh`（里面 `trap stop_service EXIT`），`run.sh` 必须 `docker run --rm`
- 实机结论（1.5.0-next，均为 `ALL-PASS`）：

  | 场景 | 镜像 | 关键结论 |
  | --- | --- | --- |
  | deb | `ubuntu:24.04` | 文件树、conffile、启动 80/443、轮转、`remove` 保留、`purge` 清理全部符合预期，`dpkg --audit` 干净 |
  | 升级迁移 | `ubuntu:24.04` | 旧 `config/` 下的证书与站点片段被复制到 `/etc/fast-https`，旧文件保留，重装不覆盖现场文件，迁移后服务可启动 |
  | rpm | `almalinux:9` | 装/启/轮转正常；`%config(noreplace)` 重装保留本地修改（留 `.rpmsave`）；`rpm -e` 清掉证书、日志与 `conf.d` |
  | apk | `alpine:3.21` | 无 systemd 时目录由 postinstall 兜底；`logrotate` 随依赖装上；卸载删包内默认配置、留证书与日志 |

  两个易误判的点：官方 ubuntu 镜像带 `path-exclude=/usr/share/man/*`，man 页不会落盘但包内登记正常；`dnf install ... curl` 会与预装的 `curl-minimal` 冲突，验证脚本因此只在没有 `curl` 时才装。

## 待跟进（尚未修复）

| 项 | 说明 |
| --- | --- |
| 归档 / Docker 布局的日志轮转 | 包布局已随包装 logrotate 配置；归档与 Docker 的日志在 `./logs`，目前要部署方自己接轮转或定期清理 |
| 真实旧版升级 | 容器里用"构造旧布局 + 装新版"覆盖了迁移逻辑，尚未用真实的 1.5.0 包做一次跨版本升级 |
