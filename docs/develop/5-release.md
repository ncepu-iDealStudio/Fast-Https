# 5. 发布流程

## 版本号

版本信息统一由 `.\build\bump-version.ps1 -Version <x.y.z>` 维护：主程序 `config/consts.go` 的 `CURRENT_VERSION`、`build/goreleaser.yaml` 的 `snapshot.version_template`、Windows NSIS 的 `APP_VERSION` 默认值、桌面端各版本字段与前端兜底文本、README 版本徽章，以及生成器产出的 man 页与 `docs/usage.html`。脚本可重复执行，匹配任意 `x.y.z` 而非某个具体旧版本。

发布正式版：**先跑脚本同步版本 → 提交 → 再打 git tag → 跑 GoReleaser**。只打 tag 不改代码里的版本号，会出现「tag 已是新版本、快照产物与二进制 `version` 仍打印旧版本」的不一致。`version_template` 只影响 `--snapshot`（本地打包默认走快照，用 `-Release` 才取 tag 的版本）；`go test ./tools/manpage` 与 `go test ./tools/docgen` 会在生成物与版本常量不一致时失败。

## 本地快照构建

一条命令出全部产物（Windows 上执行；需要 goreleaser 与 NSIS）：

```powershell
.\build\package.ps1            # 快照构建：压缩包 + Windows 安装包，dist 只留产物
.\build\package.ps1 -Release   # 正式发版（先打 tag）
.\build\package.ps1 -SkipBuild -NoInstaller   # 只整理 dist / 只出安装包
```

只想出压缩包与 Linux 包（不出 Windows 安装包）时可以手工执行：

```bash
goreleaser release -f build/goreleaser.yaml --snapshot --clean
```

这条命令**不会**产出 `setup.exe`：`build/goreleaser.yaml` 里没有 `nsis:` 段，Windows 安装包是 `build/package.ps1` 在 goreleaser 跑完之后调用 `makensis` 生成的（`package.ps1` 还会读 `dist/metadata.json` 取版本号、给 `.nsi` 补 UTF-8 BOM）。已经跑过 goreleaser、只想补安装包时用：

```powershell
.\build\package.ps1 -SkipBuild
```

产物在 `dist/`；压缩包内打入 `config/fast-https.json`、`config/mime.json`、`docs/usage.html`、`httpdoc/root/`、`README.md`，并用 `.keep` 保留 `config/cert/`、`config/conf.d/`、`logs/` 空目录。

`dist/` 里除了最终产物还会出现 GoReleaser 的中间目录（`unix_linux_amd64_v1/` 等）、`config.yaml`、`metadata.json`、`artifacts.json`。`build/package.ps1` 收尾会把它们删掉，只留下：

```text
fast-https_{Windows_x86_64,Windows_arm64}.zip
fast-https_{Linux,Darwin}_{x86_64,arm64}.tar.gz
fast-https_<version>_linux_{amd64,arm64}.{deb,rpm,apk}
fast-https-<version>-windows-<arch>-setup.exe
checksums.txt
```

### 目标平台

| OS | 架构 | 归档 | 备注 |
| --- | --- | --- | --- |
| Linux | amd64、arm64 | `tar.gz` | |
| Darwin | amd64、arm64 | `tar.gz` | |
| Windows | amd64 | `zip` | |
| Windows | arm64 | `zip` | |

## Linux 桌面端包

`build/package.ps1` 先准备桌面端前端与 `.desktop`，再由 goreleaser 用 `purego` 无 CGO 交叉编译 Linux 二进制，产出 `dist/fast-https-desktop_<version>_linux_{amd64,arm64}.{deb,rpm,apk}`，都带 `.desktop` 与图标，依赖按格式使用 Debian / Fedora / Alpine 的 GTK4 + WebKitGTK 6.0 包名。

桌面端包是「主程序包 + 桌面端」：同时带 `/usr/bin/fast-https`（与独立主程序包同一二进制）、`/etc/fast-https` 配置与站点、systemd unit、logrotate 与维护脚本，因此离线只装这一个包也能启动服务；它与独立主程序包声明 `Conflicts` / `Replaces`，最终 `/usr/bin/fast-https` 只有一份。实机验证走 `sh tests/packaging/distro/run.sh desktop`。Linux 二进制不在 Windows 上执行，GUI 行为仍需在带桌面的 Linux 实机验证。

## Windows 安装包（NSIS）

脚本在 `build/installer/windows/fast-https.nsi`，由 `build/package.ps1` 逐个 Windows zip 生成，产出 `dist/fast-https-<version>-windows-<arch>-setup.exe`。

- 安装目录 `%ProgramFiles%\Fast-Https`，写入卸载信息与开始菜单快捷方式（使用文档、安装目录、卸载）
- 安装包会带上对应架构的 `fast-https-desktop.exe`（桌面管理端，图形入口），桌面与开始菜单快捷方式指向它；`fast-https.exe` 是 `-H=windowsgui` 编译的命令行服务端，双击不会出现窗口，日常从桌面管理端启停
- 升级时不覆盖用户改过的 `config/fast-https.json`；安装前会先 `taskkill` 掉在跑的 `fast-https-desktop.exe` / `fast-https.exe`
- 卸载停止进程、清注册表、删快捷方式与安装目录

手工编译（makensis 的下载安装与路径探测见 [0-environment-and-build.md](0-environment-and-build.md) 的「NSIS（Windows 安装包）」）：

```powershell
makensis /DAPP_VERSION=<version> /DAPP_VERSION4=<version>.0 /DAPP_ARCH=amd64 `
         /DSTAGE_DIR=dist\.stage\amd64 `
         /DOUT_FILE=dist\fast-https-<version>-windows-amd64-setup.exe `
         build\installer\windows\fast-https.nsi
```

`STAGE_DIR` 是解压后的发行目录（zip 内容 + `LICENSE`）。脚本必须带 UTF-8 BOM，否则 NSIS 按 ANSI 解析会报 `Bad text encoding`；`package.ps1` 会自动补 BOM。

发行包只带使用文档 `docs/usage.html`，其余 `docs/` 内容属于开发与设计资料，不进包。生成命令：

```bash
go run ./tools/docgen
```

## RPM

`build/spec` 下的构建脚本自带 Go 1.27.1 下载；注意 `config/base_dir_rpm.go`（`-tags=rpm`）会改变资源定位路径——RPM 构建必须带该 tag 验证。

## Linux 发行版包（deb / rpm / apk）

`build/goreleaser.yaml` 的 `nfpms` 段用 `linux-pkg` 构建（带 `-tags=rpm`）产出三种包：

- 程序装到 `/usr/bin/fast-https`；配置与证书 `/etc/fast-https/`、站点 `/usr/share/fast-https/httpdoc/`、日志 `/var/log/fast-https/`、pid `/run/fast-https/`
- 单元文件 `build/packaging/systemd/fast-https.service` 装到 `/usr/lib/systemd/system/`
- 手册页中英各一份：`build/packaging/man/fast-https.1` 装到 `/usr/share/man/man1/`，`fast-https.en.1` 装到 `/usr/share/man/en/man1/`；两份都由 `go run ./tools/manpage` 从命令树生成，改子命令后必须重新生成
- 日志轮转 `build/packaging/logrotate/fast-https` 装到 `/etc/logrotate.d/fast-https`，登记为配置文件（deb conffile、rpm `%config(noreplace)`）。四个日志文件在进程里常开 fd 且 `O_APPEND` 追加，所以配置用 `copytruncate`，不走 `create` + `postrotate`；`nfpms.dependencies` 因此包含 `logrotate`，没有它日志会无上限增长
- 安装/卸载脚本在 `build/packaging/linux/`：安装后 `systemctl daemon-reload` 并打印启动提示，不自动 `enable`（容器与 CI 里没有 systemd）；卸载分两档——`remove` 保留配置与证书，`purge` 清掉 `/etc/fast-https` 下的证书、`conf.d` 与 `/var/log/fast-https`，升级两档都不触发
- 装完执行 `systemctl enable --now fast-https` 即可常驻；`Type=simple` 配合 `FASTHTTPS_FOREGROUND=1`，服务不会自行转后台

`/var/log/fast-https` 在包里用 `type: dir` 声明（配 `.keep` 占位），`/run/fast-https` 不写进包——rpm 不接受包内 `/run` 内容，改由 systemd 的 `RuntimeDirectory=` 与 postinstall 脚本创建，程序写 pid 前也会自建父目录。细节与后续阶段见 [plans/fhs-layout-plan.md](plans/fhs-layout-plan.md)。

归档（tar.gz / Docker / Windows）布局不受影响，始终相对工作目录（`./config`、`./logs`、`./httpdoc`），由 `!rpm` 构建的基址常量保证。两种布局由同一个二进制通过 build tag 区分，不做运行时判断。

外部控制面读取运行期路径用 `fast-https config path --json`，输出 `config` / `cert` / `log` / `pid` 四个绝对路径。

```bash
goreleaser release -f build/goreleaser.yaml --snapshot --clean
```

### 包元数据

仓库客户端解析依赖、分类、升级关系时读的是包元数据，改动 `nfpms` 段时别丢掉这些字段：

| 字段 | 值 | 作用 |
| --- | --- | --- |
| `section` / `priority` | `web` / `optional` | apt 的软件分类与优先级，缺省会被算进 `unknown` |
| `depends` | `ca-certificates` | ACME 校验与 HTTPS 上游都要系统 CA；精简容器里默认没有 |
| `release` | `"1"` | Debian revision，同一上游版本重新打包时靠它区分 |
| `deb.compression` | `xz` | 体积更小，`dpkg` 1.17+ 都支持 |
| `rpm.group` / `rpm.summary` / `rpm.packager` | `Applications/Internet` 等 | 没有 `summary` 的 rpm 在 `dnf info` 里是空的 |

## 仓库发布（apt / dnf / apk 源）

`build/packaging/repo/publish-repo.sh` 把 `dist/` 里的包组装成可直接静态托管的仓库树：

```bash
# 在 Debian 系环境里跑（需要 dpkg-scanpackages、apt-ftparchive；yum 源另需 createrepo_c）
bash build/packaging/repo/publish-repo.sh \
     dist repo-out \
     <GPG_KEY_ID> \
     https://repo.example.com/fast-https
```

参数：`DIST_DIR OUT_DIR [GPG_KEY_ID] [BASE_URL]`。`GPG_KEY_ID` 留空则跳过签名（只够本地测试或配 `trusted=yes` / `repo_gpgcheck=0`），`BASE_URL` 决定生成的 `.list` / `.repo` 里的地址。

产出结构：

```text
repo-out/
├── apt/  pool/main/f/fast-https/*.deb
│         dists/stable/main/binary-{amd64,arm64}/Packages(.gz)
│         dists/stable/{Release,InRelease,Release.gpg}
│         fast-https.list              # 下发给用户
├── yum/  {x86_64,aarch64}/*.rpm + repodata/（含 repomd.xml.asc）
│         fast-https.repo              # 下发给用户
├── apk/  {x86_64,aarch64}/*.apk + APKINDEX.tar.gz
├── fast-https.asc / fast-https.gpg    # 签名时导出的公钥（dnf / apt 各用一份）
└── SHA256SUMS
```

要点：

- 架构映射：Go 的 `amd64`/`arm64` 对应 rpm 与 apk 的 `x86_64`/`aarch64`，脚本用文件名里的 `_linux_<arch>` 判断，未知架构告警跳过
- 签名：apt 用 `gpg --clearsign` 出 `InRelease`、`-abs` 出 `Release.gpg`；yum 用 `-abs` 签 `repodata/repomd.xml`；apk 用 `abuild-sign`（可选）
- rpm 包本身没有 GPG 签名（需要 rpm 工具链），所以 `.repo` 模板里是 `gpgcheck=0` + `repo_gpgcheck=1`，只校验仓库元数据
- 模板在 `build/packaging/repo/{apt,yum}/`，占位符 `@BASE_URL@`、`@SIGNED_BY@`、`@REPO_GPGCHECK@` 由脚本替换；换公钥路径或镜像地址改模板即可
- 整棵树（含 `fast-https.asc`/`fast-https.gpg`）原样传到 `BASE_URL` 对应的静态目录，客户端按脚本末尾打印的步骤导入

## Docker

`build/Dockerfile`（`golang:1.27-alpine` 多阶段）；运行时挂载 config/ 与 httpdoc/。

## 发布前清单

0. `go run ./tools/docgen && go run ./tools/manpage` 重新生成使用文档与手册页（`go test ./tools/docgen ./tools/manpage` 校验生成物是否与源一致）；`go test ./tests/packaging` 确认打包脚本是 LF 行尾——Windows 上 `core.autocrlf` 会把 `preremove.sh` 变成 CRLF，装进包里就是"能装上、卸载不掉、报 prerm ENOENT"；`sh tests/packaging/distro/run.sh all` 在 ubuntu / almalinux / alpine 容器里实装一遍（装、启动、轮转、卸载），需要 docker
1. `go build ./... && go vet ./...` 干净
2. `go test -race ./...` 全绿（29 包）
3. 三个 E2E：`FASTHTTPS_E2E_RELOAD=1` reload、proxy、h2
4. 根 README 依赖表与 `go.mod` 一致
5. `docs/feature/` 与 `develop/archive/iteration-plan.md` 状态同步
6. 快照产物冒烟：安装目录跑 `check`/`start`/`status`/`reload`/`stop`
7. Windows 与 Linux 各自编译验证（不以单平台代替）；Windows 再跑一次安装包：安装 → 托盘启动 → 卸载，确认 `dist` 里只剩产物
8. 二进制体积对比：`go build -trimpath -o /tmp/fast-https .` 记录本次改动前后的大小差。1.5.0 实测（Windows/amd64）：基线 15,084,544 → 引入 certmagic 18,152,448（+3.07MB）→ 再引入 huh 配置向导 20,212,224（+2.06MB），累计 +5.13MB（+34%）。`go tool nm` 里没有 `go-acme/lego` 符号：`certmagic/dnsprovider.go` 只在启用 DNS-01 时可达，未启用时被链接器裁掉。huh 带来 17 个传递依赖（bubbletea、lipgloss、x/ansi 等），它们只在 `acme` 向导里可达，若后续嫌大可换回手写提示层
9. ACME 联调冒烟：`acme.staging=true` + `email` 填真实地址，公网域名跑一次 `start`，确认日志出现 `cert_obtained`、`config/cert/<name>.pem` 被替换、`curl https://<name>` 不再告警；再把域名解析指向无效地址，确认回落自签且只有一条告警
10. 发行包内确认存在 `config/cert/acme/`（`.keep` 占位）：非 systemd 手工启动时账户与证书缓存需要有可写目录
