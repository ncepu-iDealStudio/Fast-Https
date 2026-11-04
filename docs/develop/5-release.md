# 5. 发布流程

## 版本号

`config/consts.go` 的 `CURRENT_VERSION`（当前 `V1.5.0`）；GoReleaser 快照版本号 `1.5.0-next`（`build/goreleaser.yaml` 的 `version_template`）。发布正式版：改 CURRENT_VERSION → 打 git tag → 跑 GoReleaser。三处版本号（`consts.go`、`version_template`、Windows NSIS 的 `APP_VERSION` 默认值）要一起改，否则安装包文件名与徽章会各说各的。

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
| Windows | amd64 | `zip` | 含托盘程序 `monitor.exe` |
| Windows | arm64 | `zip` | 不含托盘程序（`monitor` 只出 amd64；Win11 on ARM 可用 x64 模拟运行 amd64 版） |

## Windows 安装包（NSIS）

脚本在 `build/installer/windows/fast-https.nsi`，由 `build/package.ps1` 逐个 Windows zip 生成，产出 `dist/fast-https-<version>-windows-<arch>-setup.exe`。

- 安装目录 `%ProgramFiles%\Fast-Https`，写入卸载信息与开始菜单快捷方式（使用文档、安装目录、托盘程序、卸载）
- 升级时不覆盖用户改过的 `config/fast-https.json`；安装前会先 `taskkill` 掉在跑的 `monitor.exe` / `fast-https.exe`
- 可选组件「开机自动启动托盘」写 `HKCU\...\Run`，仅当安装包内含 `monitor.exe` 时生效
- 卸载停止进程、清注册表、删快捷方式与安装目录

手工编译（makensis 的下载安装与路径探测见 [0-environment-and-build.md](0-environment-and-build.md) 的「NSIS（Windows 安装包）」）：

```powershell
makensis /DAPP_VERSION=1.5.0 /DAPP_VERSION4=1.5.0.0 /DAPP_ARCH=amd64 `
         /DSTAGE_DIR=dist\.stage\amd64 `
         /DOUT_FILE=dist\fast-https-1.5.0-windows-amd64-setup.exe `
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

- 程序装到 `/usr/bin/fast-https`；配置、站点、日志装到 `/usr/share/fast-https/`
- 单元文件 `build/packaging/systemd/fast-https.service` 装到 `/usr/lib/systemd/system/`
- 手册页 `build/packaging/man/fast-https.1` 装到 `/usr/share/man/man1/`；内容由 `go run ./tools/manpage` 从 `cmd.RootCmd()` 的命令树生成，改子命令后必须重新生成
- 安装/卸载脚本在 `build/packaging/linux/`：安装后 `systemctl daemon-reload` 并打印启动提示，不自动 `enable`（容器与 CI 里没有 systemd）
- 装完执行 `systemctl enable --now fast-https` 即可常驻；`Type=simple` 配合 `FASTHTTPS_FOREGROUND=1`，服务不会自行转后台

当前配置、站点、日志共用 `/usr/share/fast-https/` 一个前缀，日志与配置因此不在 FHS 推荐位置。拆成 `/etc/fast-https`、`/usr/share/fast-https`、`/var/log/fast-https`、`/run/fast-https` 的改造排在 [plans/fhs-layout-plan.md](plans/fhs-layout-plan.md)，尚未实施。

```bash
goreleaser release -f build/goreleaser.yaml --snapshot --clean
```

## Docker

`build/Dockerfile`（`golang:1.27-alpine` 多阶段）；运行时挂载 config/ 与 httpdoc/。

## 发布前清单

0. `go run ./tools/docgen && go run ./tools/manpage` 重新生成使用文档与手册页（`go test ./tools/docgen ./tools/manpage` 校验生成物是否与源一致）
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
