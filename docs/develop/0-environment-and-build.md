# 0. 开发环境与构建

## 环境

- Go **1.27.x**（go.mod 声明 1.27.0，toolchain 1.27.1；官方支持期内）
- Windows 开发需 gcc（`-race` 与 CGO 依赖）；Linux CI 同版本 Go
- 无 vendored 依赖：`go mod download` 拉取（直接依赖见根 README 依赖表）
- 只在 Windows 打安装包时需要 **NSIS 3.x**（见下文「NSIS（Windows 安装包）」）

## 获取与构建

```bash
git clone <repo> && cd fast-https
go build ./...          # 全量编译（含 monitor）
go vet ./...            # 静态检查，提交前必须干净
```

## 本地运行

```bash
# 开发模式：前台运行 + trace 控制台 + pprof(0.0.0.0:10000)
go run . dev

# 后台服务
go run . start

# 只校验配置不启动
go run . check
```

配置读取 `config/fast-https.json`（相对工作目录），站点配置可用 `http.include` 展开 conf.d 目录。日志落 `logs/`（JSON）。

## 冒烟验证

```bash
curl http://127.0.0.1:<port>/              # 静态或代理（按配置）
curl -v http://127.0.0.1:<port>/not-exist  # 404 → error_page
```

## Docker

`build/Dockerfile` 基于 `golang:1.27-alpine` 多阶段构建；运行时需要挂载 config/ 与 httpdoc/。

## NSIS（Windows 安装包）

只有生成 `dist\fast-https-<version>-windows-<arch>-setup.exe` 这一步需要 NSIS；GoReleaser 的 zip/tar.gz 与 Linux 的 deb/rpm/apk 都不依赖它。

**必须 3.x**：`build/installer/windows/fast-https.nsi` 用了 `Unicode true` 和 `MUI2.nsh`，NSIS 2 直接编译失败。

**下载安装**

1. 打开 <https://nsis.sourceforge.io/Download>，下载 `nsis-3.xx-setup.exe`（官网最新稳定版即可，实测 `v3.13` 可用）。
2. 安装时保持默认组件：需要 `NSIS Language Files`（脚本 `MUI_LANGUAGE "SimpChinese"` 依赖 `SimpChinese.nlf`）和自带的 LZMA 压缩器（脚本 `SetCompressor /SOLID lzma`）。
3. 记住安装目录，默认是 `C:\Program Files (x86)\NSIS\`。

**验证**

```powershell
& "C:\Program Files (x86)\NSIS\makensis.exe" /VERSION
```

输出形如 `v3.13` 即就绪。

**脚本如何找 makensis**

`build/package.ps1` 的 `Resolve-Makensis` 按以下顺序探测，命中即用：

1. 命令行参数 `-Makensis <path>`
2. `D:\NSIS\makensis.exe`（仓库作者的常用安装路径）
3. `%ProgramFiles(x86)%\NSIS\makensis.exe`
4. `%ProgramFiles%\NSIS\makensis.exe`
5. `PATH` 里的 `makensis.exe`

都没找到会报 `makensis.exe not found, pass -Makensis <path>`，此时改用参数指定：

```powershell
.\build\package.ps1 -Makensis "D:\Tools\NSIS\makensis.exe"
```

只出归档、不需要安装包时用 `-NoInstaller` 跳过这一步。

**常见问题**

| 现象 | 原因与处理 |
| --- | --- |
| `makensis.exe not found` | 上面 5 条探测路径都没命中；用 `-Makensis` 指定 |
| `Bad text encoding` | `.nsi` 不是 UTF-8 BOM 编码。`package.ps1` 会自动补 BOM，手工调 `makensis` 时要自己保证 |
| `Invalid command: Unicode` / `MUI2` 报错 | 用的是 NSIS 2；换 3.x |
| 编译时报找不到 `SimpChinese` 语言文件 | 安装时取消了语言文件组件（`SimpChinese.nlf`），重装并勾选 `NSIS Language Files` |

## 发布

见 [5-release.md](5-release.md)（GoReleaser snapshot、RPM、版本号规则）。

## 相关

测试基线见 [1-testing.md](1-testing.md)；目录语义见 [../architecture-map.md](../architecture-map.md)。
