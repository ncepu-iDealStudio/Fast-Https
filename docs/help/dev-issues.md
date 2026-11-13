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
  - `.gitattributes` 增加 `*.sh text eol=lf`、`build/spec`、`build/docker/*`（当时还列了 `build/Dockerfile`，该文件后来已删除）
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
- 实机结论（1.5.0-next，均为 `ALL-PASS`；含产物校验和、镜像摘要与逐项结果的完整记录见 [../test/3-distro-verification.md](../test/3-distro-verification.md)）：

  | 场景 | 镜像 | 关键结论 |
  | --- | --- | --- |
  | deb | `ubuntu:24.04` | 文件树、conffile、启动 80/443、轮转、`remove` 保留、`purge` 清理全部符合预期，`dpkg --audit` 干净 |
  | 升级迁移 | `ubuntu:24.04` | 旧 `config/` 下的证书与站点片段被复制到 `/etc/fast-https`，旧文件保留，重装不覆盖现场文件，迁移后服务可启动 |
  | rpm | `almalinux:9` | 装/启/轮转正常；`%config(noreplace)` 重装保留本地修改（留 `.rpmsave`）；`rpm -e` 清掉证书、日志与 `conf.d` |
  | apk | `alpine:3.21` | 无 systemd 时目录由 postinstall 兜底；`logrotate` 随依赖装上；卸载删包内默认配置、留证书与日志 |

  两个易误判的点：官方 ubuntu 镜像带 `path-exclude=/usr/share/man/*`，man 页不会落盘但包内登记正常；`dnf install ... curl` 会与预装的 `curl-minimal` 冲突，验证脚本因此只在没有 `curl` 时才装。

## 003 http-01 端口告警漏算 include 站点，acme 没有域名时不提示

- 发现：2026-11-05，生产机（Ubuntu）上按 `site init` → `acme` → `start` 走完流程后仍然访问不了
- 症状：
  - 每次 `start` 都告警 `acme: http-01 is enabled but no http.server listens on port 80`，但 `conf.d/default.json` 明确写了 `listen: [80, "443 ssl h2"]`
  - `acme show` 的「将申请的域名」是 `(无)`，只列出一条 `跳过 localhost：not a public domain name`，用户看不出"还要给站点加 443"
  - 控制台只有"acme 配置已写入"，没有任何"这次配置不会申请证书"的信号
- 根因：
  - `warnHTTP01WithoutPort80` 只拿到 `root.HTTP.Servers`（主配置 `http.server`），`http.include` 展开的 conf.d 站点没算进去。站点通常只写在 conf.d，这条告警于是必然误报，还把真正的问题盖住
  - `certmanager.Plan` 只收 `listen` 含 `ssl` 的 server；`site init` 默认 `listen=80`，向导里没勾 TLS 时生成的站点永远进不了申请列表
  - `acme setup` 结束时只打"配置已写入"，不检查"有没有可申请的域名"
- 修复：
  - `validateIncludes` 改为返回解析后的 server 块，http-01 端口判断合并主配置与 include 的站点
  - `printACMENextSteps` 在 `Plan.Managed` 为空时直接说明原因、列出被跳过的域名、给出加 `443 ssl h2` 的具体写法
  - `ssl_certificate not found` 的错误信息补一句"首次 start 会用本地 CA 生成，先跑一次 start"
- 防回归：
  - `config/acme_warn_test.go`：include 站点监听 80 时不得告警，没有 80 时必须告警
  - `cmd/acme_test.go`：无 ssl 站点时 `acme setup` 输出必须包含"不会发出任何证书请求"与 443 写法；有 ssl 站点时不得出现该提示
- 教训：站点的主战场是 `conf.d`，任何"扫描所有 server"的判断都必须基于 include 展开后的结果，不能只看主配置。

## 004 卸载后进程仍在跑，80/443 被占

- 发现：2026-11-06，Ubuntu 24.04 实机，`apt remove fast-https` 之后 `ps aux` 仍能看到 `/usr/bin/fast-https`
- 影响：deb / rpm 的卸载路径；apk 的 `pre-deinstall` 拿不到参数，本次只保持原行为
- 症状：
  - 包已卸载、`/usr/bin/fast-https` 已删除，进程以 deleted inode 继续跑，80/443 仍在监听
  - `systemctl status fast-https` 显示 inactive/未找到，`systemctl stop` 毫无作用
  - `purge` 把 `/run/fast-https` 一起删了，pid 文件没了，事后连"怎么找到它"都成问题
  - 重装后 `start` 报端口被占用，而 `ps` 里那个进程的命令行是 `/usr/bin/fast-https`——**不带任何参数**
- 根因：
  - `preremove.sh` 只做 `systemctl stop/disable`，这只能作用于单元 cgroup 里的进程
  - 直接执行 `fast-https`（不带子命令）或 `fast-https start` 时，`cmd.daemonize()` 会 fork、父进程退出、子进程 re-exec 自身（`os.Args[1:]` 原样带过去）。`fast-https` 不带参数时子进程命令行就是 `/usr/bin/fast-https`，正是现场看到的样子；它被 init 收养，不在单元里
  - 服务端本身支持 `SIGTERM` 优雅停机，但整条卸载链路里没有任何人给它发信号
- 修复：
  - `preremove.sh` 的 `remove|purge|0` 分支先 `stop_unit`，再 `kill_leftovers`：pid 文件（`/run/fast-https/fast-https.pid`，JSON 取 `pid`）与 `/proc` 扫描两路都走
  - 动手前双重确认身份：`/proc/PID/exe`（含 ` (deleted)` 形态）或 argv[0] 精确匹配二进制路径，避免 pid 复用误杀
  - `SIGTERM` 后最多等 5 秒，不退再 `SIGKILL` 并往 stderr 留一行
  - 升级分支（deb `upgrade`、rpm `$1=1`）刻意不动进程，避免服务被停掉却没人拉起；apk 的 `pre-deinstall` 无参数，只停单元不做强杀
- 防回归：
  - `tests/packaging/preremove_test.go`：卸载分支必须同时调用 `stop_unit` 与 `kill_leftovers`、必须有 `/proc` 扫描与 `SIGKILL` 兜底、必须认 ` (deleted)`、升级分支不得出现停止动作
  - WSL 实测四条路径：pid 文件命中、二进制已删且无 pid 文件（靠 `/proc` 扫到）、无关进程不受影响、`upgrade` 参数不杀进程
- 教训：**包管理脚本里的"停服务"不等于"停 unit"**。守护进程一旦自己 daemonize 就脱离了 systemd 管辖，卸载脚本必须按可执行文件自己找进程，而且要覆盖"二进制已删、pid 文件已丢"这个中间态——否则包卸干净了，端口还锁着。

## 待跟进（尚未修复）

| 项 | 说明 |
| --- | --- |
| 归档 / Docker 布局的日志轮转 | 包布局已随包装 logrotate 配置；归档与 Docker 的日志在 `./logs`，目前要部署方自己接轮转或定期清理 |
| 真实旧版升级 | 容器里用"构造旧布局 + 装新版"覆盖了迁移逻辑，尚未用真实的 1.5.0 包做一次跨版本升级 |
| apk 卸载的残留进程 | `pre-deinstall` 拿不到参数，分不清升级与卸载，目前只停单元不做强杀。要覆盖得让 nfpm 支持独立的 upgrade 钩子（apk 的 `pre-upgrade`），或在脚本里另找判据 |
| 升级后仍在跑旧二进制 | 升级不打断服务，运行中的是 deleted inode，需要重启才换新二进制。是否让 postinstall 自动 `try-restart` 还没定 |
