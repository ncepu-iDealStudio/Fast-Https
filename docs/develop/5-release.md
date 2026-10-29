# 5. 发布流程

## 版本号

`config/consts.go` 的 `CURRENT_VERSION`（当前 `V1.4.0`）；GoReleaser 快照版本号 `1.4.0-next`。发布正式版：改 CURRENT_VERSION → 打 git tag → 跑 GoReleaser。

## 本地快照构建

一条命令出全部产物（Windows 上执行；需要 goreleaser 与 NSIS）：

```powershell
.\scripts\package.ps1            # 快照构建：压缩包 + Windows 安装包，dist 只留产物
.\scripts\package.ps1 -Release   # 正式发版（先打 tag）
.\scripts\package.ps1 -SkipBuild -NoInstaller   # 只整理 dist / 只出安装包
```

等价的手工命令：

```bash
goreleaser release -f .goreleaser.yaml --snapshot --clean
```

产物在 `dist/`；压缩包内打入 `config/fast-https.json`、`config/mime.json`、`config/fastcgi.conf`、`docs/usage.html`、`httpdoc/root/`、`README.md`，并用 `.keep` 保留 `config/cert/`、`config/conf.d/`、`logs/` 空目录。

`dist/` 里除了最终产物还会出现 GoReleaser 的中间目录（`unix_linux_amd64_v1/` 等）、`config.yaml`、`metadata.json`、`artifacts.json`。`scripts/package.ps1` 收尾会把它们删掉，只留下：

```text
fast-https_{Windows_x86_64,Windows_arm64}.zip
fast-https_{Linux,Darwin}_{x86_64,arm64}.tar.gz
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

脚本在 `installer/windows/fast-https.nsi`，由 `scripts/package.ps1` 逐个 Windows zip 生成，产出 `dist/fast-https-<version>-windows-<arch>-setup.exe`。

- 安装目录 `%ProgramFiles%\Fast-Https`，写入卸载信息与开始菜单快捷方式（使用文档、安装目录、托盘程序、卸载）
- 升级时不覆盖用户改过的 `config/fast-https.json`；安装前会先 `taskkill` 掉在跑的 `monitor.exe` / `fast-https.exe`
- 可选组件「开机自动启动托盘」写 `HKCU\...\Run`，仅当安装包内含 `monitor.exe` 时生效
- 卸载停止进程、清注册表、删快捷方式与安装目录

手工编译（makensis 需在 PATH 或指定路径）：

```powershell
makensis /DAPP_VERSION=1.4.0 /DAPP_VERSION4=1.4.0.0 /DAPP_ARCH=amd64 `
         /DSTAGE_DIR=dist\.stage\amd64 `
         /DOUT_FILE=dist\fast-https-1.4.0-windows-amd64-setup.exe `
         installer\windows\fast-https.nsi
```

`STAGE_DIR` 是解压后的发行目录（zip 内容 + `LICENSE`）。脚本必须带 UTF-8 BOM，否则 NSIS 按 ANSI 解析会报 `Bad text encoding`；`package.ps1` 会自动补 BOM。

发行包只带使用文档 `docs/usage.html`，其余 `docs/` 内容属于开发与设计资料，不进包。生成命令：

```bash
go run ./tools/docgen
```

## RPM

`spec/` 下的构建脚本自带 Go 1.27.1 下载；注意 `config/base_dir_rpm.go`（`-tags=rpm`）会改变资源定位路径——RPM 构建必须带该 tag 验证。

## Docker

根目录 `Dockerfile`（`golang:1.27-alpine` 多阶段）；运行时挂载 config/ 与 httpdoc/。

## 发布前清单

0. `go run ./tools/docgen` 重新生成使用文档（`go test ./tools/docgen` 校验 md 与 html 是否一致）
1. `go build ./... && go vet ./...` 干净
2. `go test -race ./...` 全绿（29 包）
3. 三个 E2E：`FASTHTTPS_E2E_RELOAD=1` reload、proxy、h2
4. 根 README 依赖表与 `go.mod` 一致
5. `docs/feature/` 与 `develop/plans/iteration-plan.md` 状态同步
6. 快照产物冒烟：安装目录跑 `check`/`start`/`status`/`reload`/`stop`
7. Windows 与 Linux 各自编译验证（不以单平台代替）；Windows 再跑一次安装包：安装 → 托盘启动 → 卸载，确认 `dist` 里只剩产物
