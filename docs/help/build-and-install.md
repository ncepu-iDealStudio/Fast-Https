# 构建与安装

从源码到装进系统，中间只有一条链路。这里按顺序列出每一步和验收方式。

## 环境

| 工具 | 版本 | 用途 |
| --- | --- | --- |
| Go | 1.27.1 | 编译 |
| GoReleaser | v2.x | 出归归档与三种 Linux 包 |
| NSIS | 3.x | 只做 Windows 安装包时用 |
| dpkg-dev / createrepo_c | 可选 | 只在生成 apt/yum 源时需要 |

环境细节见 [../develop/0-environment-and-build.md](../develop/0-environment-and-build.md)。

## 直接跑源码

```bash
go build -o fast-https .
./fast-https start      # 会先校验配置，失败直接报原因；前台/后台取决于环境
```

源码构建走 `!rpm` 布局：配置在 `./config`，日志在 `./logs`，站点在 `./httpdoc/root`。

## 出全部产物

Windows 上一条命令：

```powershell
.\build\package.ps1            # 快照构建
.\build\package.ps1 -Release   # 正式发版（先打 tag）
```

只想出 Linux 包与归档：

```bash
goreleaser release -f build/goreleaser.yaml --snapshot --clean
```

这条命令**不会**产出 `setup.exe`，Windows 安装包由 `build/package.ps1` 调 `makensis` 生成。

产物在 `dist/`：

```text
fast-https_{Windows_x86_64,Windows_arm64}.zip
fast-https_{Linux,Darwin}_{x86_64,arm64}.tar.gz
fast-https_<version>_linux_{amd64,arm64}.{deb,rpm,apk}
fast-https-<version>-windows-<arch>-setup.exe
checksums.txt
```

## 安装 Linux 包

```bash
sudo apt install ./fast-https_1.5.0-next_linux_amd64.deb   # Debian / Ubuntu
sudo dnf install ./fast-https-1.5.0-next-1.x86_64.rpm      # RHEL / openEuler
sudo apk add --allow-untrusted fast-https_1.5.0-next_linux_amd64.apk
```

装完文件落在哪，见 [package-layout.md](package-layout.md)。

## 装完验收

```bash
dpkg -L fast-https | grep -E '^/(etc|var|run)'
fast-https config path --json
sudo systemctl enable --now fast-https
systemctl status fast-https --no-pager
sudo ss -ltnp | grep -E ':(80|443)'
```

期望：配置在 `/etc/fast-https`、日志目录 `/var/log/fast-https`、pid 在 `/run/fast-https`、服务 `active (running)`、80 与 443 都在听。

首次启动会用本地 CA 生成证书；在那之前 `fast-https reload` 报 `ssl_certificate not found` 是正常的，先跑一次 `start` 即可。

## 日志轮转

包会装 `/etc/logrotate.d/fast-https`：每天轮转、保留 14 份、单文件超过 100MB 也提前轮转、gzip 压缩。`logrotate` 是包的硬依赖，装包时一并装上。

为什么用 `copytruncate`：四个日志文件在进程里常开 fd 并以 `O_APPEND` 追加。`create` 会改名当前文件、新建同名文件，进程却继续写改名后的旧 inode，新文件永远是空的——除非同时让进程重新打开文件。`copytruncate` 先复制再截断，不用重启、也不用 `reload`，代价是复制与截断之间几毫秒的写入会丢。

```bash
sudo logrotate -d /etc/logrotate.d/fast-https   # 干跑：只打印会做什么
sudo logrotate -f /etc/logrotate.d/fast-https   # 立刻轮转一次
ls -l /var/log/fast-https                        # access.log.1 / .2.gz ...
```

该文件是包管理器登记的配置文件（deb conffile、rpm `%config(noreplace)`），本地修改在升级时保留。归档与 Docker 布局的日志在 `./logs`，不在这个配置管辖范围，需要自己接 logrotate。

## 卸载

三档语义，选你需要的：

| 操作 | 配置与证书 | 日志 |
| --- | --- | --- |
| `apt remove` / `dnf remove` | 保留 | 保留 |
| `apt purge` / `rpm -e` | 删除 | 删除 |
| 升级 | 不动 | 不动 |

```bash
sudo apt remove fast-https   # 以后还要重装、想接着用现有证书，选这个
sudo apt purge  fast-https   # 确定不要了，连证书、ACME 缓存、日志一起清
sudo dpkg --audit            # 无输出即数据库干净
```

`remove` / `purge` 时 `prerm` 会先停 systemd 单元，再把单元之外的残留进程一并停掉（先 `SIGTERM`，5 秒不退补 `SIGKILL`）。这一步不能省：直接执行 `fast-https`（不带子命令）时进程会自己 daemonize 到 init 下面，`systemctl stop` 管不到它，不强杀就会变成"包删了、进程还在占 80/443"。

升级不走这段逻辑，服务不被打断；但此时运行中的仍是旧二进制（deleted inode），要让新二进制生效跑一次 `sudo systemctl restart fast-https`。

purge 删的是本程序生成的东西：

| 路径 | 内容 |
| --- | --- |
| `/etc/fast-https/cert` | CA 私钥、站点证书、ACME 账户与缓存 |
| `/etc/fast-https/conf.d` | 站点片段 |
| `/etc/fast-https/{fast-https,mime}.json` | 主配置与 MIME 表（conffile） |
| `/var/log/fast-https` | 四路日志 |
| `/run/fast-https` | pid |
| `/usr/share/fast-https/httpdoc/root/root.crt` | 本地自签 CA 证书；私钥在 `cert/` 里，不删会留下孤儿证书 |

每个目录最后用 `rmdir` 收尾——只在变空时才删。所以 `/etc/fast-https` 里若还有你自己放的文件（例如手工加的 `fastcgi.conf`），或站点根下有你自己的网页，这些目录会保留下来。dpkg 不替你决定别人的文件该不该删。

`/usr/share/fast-https/httpdoc` 下真正的网站内容一律不动。确认不要了手工删：

```bash
sudo rm -rf /usr/share/fast-https
```

apk 是另一套语义：它没有 purge，`post-deinstall` 也拿不到参数（分不清升级与卸载），所以

- 包内的默认配置随包删除：`/etc/fast-https/fast-https.json`、`mime.json`、`conf.d/default.json`、`/etc/logrotate.d/fast-https`
- 运行期生成的东西保留：`/etc/fast-https/cert/`（含 `localhost.pem` 与 ACME 账户）、`/var/log/fast-https/`
- 想要彻底清干净，卸载后手工 `sudo rm -rf /etc/fast-https /var/log/fast-https`

实测记录见 `docs/help/dev-issues.md` 的 002。

## 从 1.5.0 或更早升级

旧版本把配置放在 `/usr/share/fast-https/config/`。升级时 postinstall 会把文件**复制**到 `/etc/fast-https`（目标已存在则跳过，旧文件保留不删）。

核对无误后清理旧目录：

```bash
sudo diff -r /usr/share/fast-https/config /etc/fast-https
sudo rm -rf /usr/share/fast-https/config /usr/share/fast-https/logs
```

## 构建侧三个注意点

1. **打包脚本必须 LF**。Windows 的 `core.autocrlf` 会把 `preremove.sh` 变成 CRLF，打出来的包能装不能卸。`go test ./tests/packaging` 会拦。
2. **goreleaser 的 nfpm 字段名是 `dependencies` 不是 `depends`**。写错 `goreleaser check` 直接报 `field depends not found`。
3. **`dist/` 里的中间目录**（`unix_linux_amd64_v1/`、`metadata.json` 等）由 `package.ps1` 收尾清理；手工跑 goreleaser 时它们会留在原处。

## 生成发行源

把 `dist/` 里的包组装成可直接托管的 apt/yum/apk 仓库树，见 [../develop/5-release.md](../develop/5-release.md) 的「仓库发布」一节，脚本是 `build/packaging/repo/publish-repo.sh`。

## 实机验证

装包这件事没法只靠单测守，`tests/packaging/distro/` 会在容器里真装一遍：

```bash
goreleaser release -f build/goreleaser.yaml --snapshot --clean --skip=archive,publish,announce
sh tests/packaging/distro/run.sh all      # deb → upgrade → rpm → apk
```

| 目标 | 镜像 | 覆盖 |
| --- | --- | --- |
| `deb` | `ubuntu:24.04` | 文件树、conffile 登记、logrotate、启动监听 80/443、轮转、`remove` 保留、`purge` 清理 |
| `upgrade` | `ubuntu:24.04` | 旧版 `/usr/share/fast-https/config` 迁移到 `/etc/fast-https`、不覆盖现场文件、迁移后服务可用 |
| `rpm` | `almalinux:9` | 同 deb，另加 `%config(noreplace)` 重装保留与 `rpm -e` 清理 |
| `apk` | `alpine:3.21` | 无 systemd 时的目录兜底、logrotate 依赖、卸载语义 |

脚本依赖容器内的 `fast-https start` 自己转后台（容器里 `runInForeground()` 为真，`start` 不 daemonize），退出时统一 `stop`，否则后台进程会一直占着 80/443。

上次实测的产物校验和、镜像摘要、逐项结论与环境差异记在 [../test/3-distro-verification.md](../test/3-distro-verification.md)——同一份产物不用重复跑；缺陷背景见 [dev-issues.md](dev-issues.md) 的 002。
