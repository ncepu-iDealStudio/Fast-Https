# Fast-Https Desktop（Wails v3）

本机图形化管理端。**单窗口 + 后台托盘**：从 Windows 开始菜单或 Linux 应用菜单点开；点窗口关闭按钮会弹窗询问「隐藏到后台 / 直接退出 / 取消」，隐藏后程序继续在托盘运行，左键单击托盘图标可显示／隐藏主界面，托盘右键菜单里的「退出」等同于「直接退出」。桌面端始终不接管 fast-https 服务进程。

- Wails v3 CLI：`v3.1.0`（`go install github.com/wailsapp/wails/v3/cmd/wails3@latest`，`@latest` 指向周更稳定版，不跟 nightly）
- 独立 Go module `fast-https-desktop`，**不 import 主项目的 `config` 包**，主项目 `go.mod` 不受影响
- 前端：Vanilla JS + Vite，产物编译期 embed 进单文件可执行程序
- 视觉：亮色是文档式「运行报告」，暗色是深石墨「值班台」；导航只保留左侧一列，按钮为克制圆角，选中态用中性淡灰

## 环境

| 依赖 | 版本（本机实测） |
| --- | --- |
| Go | 1.27.1 |
| Node / npm | v24.19.0 / 12.0.2 |
| Wails v3 CLI | 3.1.0 |
| Windows 运行期 | WebView2 Runtime（安装包带引导器） |
| Linux 运行期 | GTK4 + WebKitGTK 6.0（Ubuntu 24.04+ / Debian 13+） |

## 开发模式

`wails3 dev`（或 `wails3 task dev`）会启动 Vite dev server 并打开应用窗口，前端热更新、Go 代码改动自动重新构建，不需要打包。前端服务端口默认 `9245`，由 `WAILS_VITE_PORT` 控制；**同一时间只能跑一个 dev 实例**，重复启动会报 `bind: Only one usage of each socket address...`。

如果窗口里看不到最新改动，先确认只有一份 Vite 在 9245，关掉旧窗口再重跑 `wails3 dev`。加 `-port 9246` 可以换端口另起一个实例。

## 构建

```powershell
cd desktop
npm --prefix frontend install
# 生成 Go → JS bindings（改过 service.go / state.go 的类型后必须重跑）
wails3 generate bindings -clean=true -time-type=Date
npm --prefix frontend run build
go build -tags production -trimpath -ldflags="-w -s -H windowsgui" -o bin/fast-https-desktop.exe .
```

也可以用官方 Taskfile（需要 `task`）：`wails3 build`、`wails3 dev`。

开发时 `wails3 dev` 会起 Vite dev server；`bin/fast-https-desktop.exe` 需要放在**主程序同目录**才能读到配置与调用 CLI。

## 打包

| 平台 | 命令 | 产物 |
| --- | --- | --- |
| Windows NSIS | `wails3 task windows:create:nsis:installer` | `bin/fast-https-desktop-amd64-installer.exe` |
| Linux desktop deb | `wails3 task linux:create:deb` | `bin/fast-https-desktop_<version>_<arch>.deb` |

- NSIS 的组件页里，**桌面快捷方式**与**开始菜单**是两个可勾选、可取消的 Section；`build/package.ps1` 会把对应架构的 `fast-https-desktop.exe` 一起打进安装包，快捷方式指向它。主程序 `fast-https.exe` 是 `-H=windowsgui` 的命令行服务端，双击没有窗口，图形入口只有桌面管理端。
- `MicrosoftEdgeWebview2Setup.exe` 是打包时由 `wails3 generate webview2bootstrapper` 生成的中间产物（已忽略、不入库），走 `wails3 task windows:package` 会自动重新生成；手工跑 `makensis` 前需要先生成它。
- goreleaser 产出的 desktop deb / rpm / apk 是「主程序包 + 桌面端」的完整包：除 `/usr/bin/fast-https-desktop`、图标与 `.desktop` 外，还带 `/usr/bin/fast-https`（与独立主程序包同一个二进制）、`/etc/fast-https` 配置与站点片段、`/usr/share/fast-https` 站点资源、systemd unit、logrotate 与安装维护脚本。离线只装这一个包就能 `fast-https start`，装完在应用菜单可见。它与独立主程序包声明 `Conflicts: fast-https` / `Replaces: fast-https`，避免同一文件被两个包各装一份；实机验证见 `tests/packaging/distro/desktop-deb.sh`。
- `build/package.ps1` 先准备前端与 `.desktop`，再由 goreleaser 用 `purego` 无 CGO 交叉编译 Linux 二进制，产出 `dist/fast-https-desktop_<version>_linux_{amd64,arm64}.{deb,rpm,apk}`；依赖名按格式分别用 Debian / Fedora / Alpine 的 GTK4 + WebKitGTK 6.0 包。Linux 主机上仍可跑 `wails3 task linux:create:deb` 走原生 CGO 构建。`frontend/bindings/` 是 `wails3 generate bindings` 的生成物，已加入 git 忽略——走 `wails3 build` 会自动生成，手工只跑 `npm run build` 前要先执行一次生成命令。

`build/` 已按范围清理：只保留 `appicon.png`、`config.yml`、共通与平台 Taskfile、Windows 图标/NSIS 脚本、Linux nfpm 与 `.desktop` 生成任务。iOS / Android / macOS 打包资产、MSIX、AppImage、RPM/Arch、server mode、Docker 交叉编译与模板占位配置都已删除，Taskfile 的 includes 与任务也同步精简。

## 自绘标题栏

窗口用 `Frameless: true`，没有系统标题栏（图标、标题文字与系统按钮那条都不显示）。顶部通栏通过 `--wails-draggable: drag` 承担窗口拖拽，右侧三个自绘按钮按系统原生比例做成 46px 宽、撑满通栏高度、直角，图标是统一线宽的 SVG；分别调用 Go 侧 `MinimiseWindow` / `ToggleMaximiseWindow`。点「关闭」不直接隐藏，而是弹出询问框：

- 「隐藏到后台」调 `HideWindow()`：窗口隐藏、程序留在托盘，`WindowClosing` hook 的默认行为也是隐藏（Alt+F4 同样只隐藏）。
- 「直接退出」调 `QuitApp()`：置位 `appQuitting` 后 `app.Quit()`，hook 才会放行真正的关闭；托盘菜单的「退出」走同一条路径。
- 「取消」或 Esc / 点遮罩关闭弹窗。

Win11 的圆角与阴影保留（`DisableFramelessWindowDecorations` 保持默认 `false`）。

## 图标

`build/windows/icon.ico` 与 `build/appicon.png` 都来自项目原有应用图标（原 `starting.ico`，副本已内置在 `build/` 下）：前者是原 ICO（32/48/256 三帧），供 Windows 托盘、`.syso` 窗口资源与 NSIS 安装包使用；后者是从同一个 ICO 里提取出的 256×256 PNG 帧，供 Linux 托盘与 deb 的 `hicolor` 图标。前端 logo 用 `frontend/public/appicon.png`（内容相同的副本，走 `/appicon.png`，保证 `wails3 dev` 与生产构建都能加载）。换图标时这三处一起替换，不需要再跑图标生成任务。

## 数据从哪来（进程外读取）

桌面端不接管服务进程、不给主程序加 API、不监听新端口。

| 数据 | 来源 |
| --- | --- |
| 运行时路径 | 主程序 `fast-https config path --json`（归档 / deb / rpm / apk 布局不同，由主程序按编译期基址算好） |
| 站点 | 主配置 `http.server[]` + `http.include` 目录下递归收集的 `*.json`，支持 JSONC 注释与尾逗号 |
| TLS 判定 | `listen` 里是否含 `ssl`（与主程序 `config/validate.go` 一致） |
| 证书 | 读 `ssl_certificate` 指向的 PEM，用 `crypto/x509` 取签发者与到期时间 |
| pid | pid 文件是 `{"pid":1234}`，不是纯数字 |
| 运行状态 | 对配置里声明的端口做回环 TCP 探测（避开 Windows / Unix 进程判活的差异） |
| 配置是否未重载 | 启发式：配置文件比 pid 文件新 |
| 日志 | `access.log` / `error.log` / `system.log` 尾部，只读不跟随写入句柄 |
| 操作 | 转交主程序 `start / stop / reload / status`，桌面端不自己管理进程 |

**已知边界**：设置页只做 JSON/JSONC 语法校验，不判断配置语义；权威校验由主程序 `reload` 负责。配置 schema 若在主项目变更，这里需要同步 `state.go` 的字段。

## 站点页：添加站点

站点页右上角「添加站点」弹出表单，字段有 `server_name`、`listen`（逗号或换行分隔，支持 `80` 与 `443 ssl h2` 混写）、类型（`local` 静态 / `proxy` 反代）、`root` 或 `proxy_pass`、可选的 `index`、`ssl_certificate` / `ssl_certificate_key` 与文件名。

`server_name` **一个站点只填一个名称**，可以是域名、`localhost` 或本机 IP（如 `192.168.1.10`）——运行时按 `server_name:端口` 精确匹配 Host，没有多域名分割。用 IP 或别名访问时，默认的 `security.host_check=off` 会回落到该端口的站点；改成 `enforce` 才严格校验 Host。

「站点根目录」「证书路径」「私钥路径」右侧有「浏览…」按钮，调 Wails 的原生文件/目录选择框（`PickPath`）：目录用于 root，文件用于证书与私钥（过滤 PEM/CRT/CER）；选中的绝对路径若在 `config` 内会转成 `./cert/x.pem` 这类相对写法，否则保留绝对路径。取消选择不会改动输入框。

新站点写成 `conf.d/<名称>.json`（文件名默认按域名生成，非 ASCII 会回落到 `site`），**不改主配置、不覆盖已有文件**。Go 侧 `AddSite` 会先校验：端口合法且未被现有站点占用、`local` 必须有 root、`proxy_pass` 必须带 http(s) scheme、文件名不能路径穿越、同名文件直接拒绝。写入是同目录临时文件 + rename。创建后需要点「重载配置」才生效。

## 设置页：主配置表单

设置页**按字段编辑** `fast-https config path --json` 给出的主配置文件（通常是 `config/fast-https.json`）：分组表单覆盖 `security` / `waf` / `acme` / `http` / 基础等标量字段，枚举用下拉、布尔用勾选、数组用多行输入。字段清单在 `desktop/config_editor.go` 维护，前端通用渲染，新增字段只改 Go 一侧。`engine`（集群占位）当前没有模块读取、项目也未启用集群，不放进表单；配置里若仍带该段会原样保留。

保存只提交**被改动的字段**，合并回原始 JSON 后写回；未纳入表单的 `http.server[]`、`acme.cas` 与未知顶层键原样保留。保存会重新格式化文件（JSONC 注释会丢失），原文件完整备份为 `fast-https.json.bak`，再用临时文件原子替换。配置语义仍由主程序 `reload` 校验，保存后需点「重载配置」。主程序不在同目录时会显示读取失败并禁用表单。

## 主题

左下角「本机管理」点击弹出账户菜单，里面包含：亮色／暗色、五档中性色（石墨、暖灰、松绿、赭石、靛灰，默认亮色 + 石墨）、设置页、隐藏到后台托盘、帮助文档。主题选择保存在本机 `localStorage`。

「帮助文档」打开发行包里的 `docs/usage.html`（开发时 exe 在 `desktop/bin/`，会回退到仓库根）。顶部工具按钮与左侧「设置」导航项已移除，统一从这个菜单进入。

**选中／激活态一律用中性色**：亮色是淡淡的灰，暗色是略亮的灰。正常、告警、危险三种语义色独立，不随主题变化。
