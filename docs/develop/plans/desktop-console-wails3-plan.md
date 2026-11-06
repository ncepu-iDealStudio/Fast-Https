# 桌面管理端（Wails v3）方案与实施记录

状态：**实施中**。更新：2026-11-06。分支：`feature_desktop`。

目标一句话：用 Wails v3 做一个**单窗口 + 后台托盘**的本机图形化管理端，看状态、看站点、看证书、点一下重载；Windows 用 NSIS 装、Linux 多打一个 desktop deb。

> 早期版本里的「monitor 与 desktop 过渡期并存」「pin v3.1.0」等假设已作废。托盘需求在 2026-11-06 后续反馈里恢复为**后台托盘**（关闭窗口隐藏到托盘，不是 monitor 那种常驻控制托盘）。

## 1. 用户已拍板的边界

| 项 | 决定 |
| --- | --- |
| 托盘 | **后台托盘**：点窗口 X 隐藏到托盘、程序继续运行；左键单击托盘显示／隐藏主界面，右键菜单「退出」真正结束。桌面端仍不接管 fast-https 服务进程 |
| monitor | **不要版本管理、不要过渡期**。`monitor/` 已从仓库删除 |
| Wails 版本 | **用最新版**，不 pin 具体版本 |
| Windows 安装包 | 继续用 NSIS，支持**快捷方式创建选择**与**开始菜单选择** |
| Linux 安装包 | 额外多打一个 **desktop deb**，支持图形化安装 |
| 视觉 | 亮色＝文档式运行报告，暗色＝深石墨值班台；选中态中性淡灰、不要蓝色；语义状态色独立 |
| 布局 | 顶部通栏只放 logo 与窗口按钮（页面标题只留在主区，避免重复）；工作区是浅底 + 白卡片（运行概况 / 告警条 / 常用操作 / 最近事件）；导航只留左侧一列 |
| 导航选中态 | 去掉左侧竖指示条，改成中性色圆角块 |
| 设计预览稿 | `design-demos/` 已删除，不再维护；方向与调整全部落到 `desktop/` 源码 |
| 分支 | 用户自建 `feature_desktop`，会随时改动 |
| PR / push | 用户自己来 |

用户原话留痕：「dir-1.png 这个是亮色主题」「dir-3.png 这个是暗色主题」「对于状态选择，不要是蓝色，就是亮色是那种淡淡的灰色就行」「搭建 wails 环境吧，就是开始开发实现就行了」「design-demos 这个就删了吧，不要改这个了，直接改源码就行了」「这个页面左边的条给去了吧，不需要，还有就是 logo 什么的，给我换一下回来，还有实现一下退出后台托盘的实现」「页面布局改造成这个样子吧」。

## 2. 事实核实（2026-11-06）

| 事实 | 结论 | 证据 |
| --- | --- | --- |
| `@latest` 解析到哪 | `v3.1.0`（周更稳定版，2026-11-01） | `go install .../cmd/wails3@latest` 实测，`wails3 version` 输出 `v3.1.0` |
| 是否有更新的版本 | 有 `v3.2.0-nightly.20261101/02/03`，但 **nightly 不被 `@latest` 选中** | `go list -m -versions github.com/wailsapp/wails/v3`；Wails 发布策略页 |
| 托盘 API | `app.SystemTray.New()` → `SetIcon([]byte)` / `SetMenu` / `SetTooltip` / `AttachWindow`；`SystemTrayManager.New()` 在 `app.Run()` 时自动启动托盘 | `v3@v3.1.0/pkg/application/systemtray.go`、`system_tray_manager.go` |
| 关闭拦截 | `window.RegisterHook(events.Common.WindowClosing, func(e *WindowEvent){ e.Cancel(); window.Hide() })` | `webview_window_close_test.go` 的 `TestCancelledWindowClosingAllowsLaterClose` |
| 托盘图标格式 | PNG / ICO 都可；Windows `w32.CreateSmallHIconFromImage` 明确支持 PNG，Linux 走 IconPixmap，macOS 走 NSImage | `pkg/w32/icon.go:185`、`systemtray_windows.go:388` |
| Windows 运行依赖 | WebView2 Runtime；NSIS 模板带 bootstrapper 宏 | `desktop/build/windows/nsis/project.nsi` |
| Linux 运行依赖 | GTK4 + WebKitGTK 6.0（v3.1 已移除 gtk3 后端） | `desktop/build/linux/nfpm/nfpm.yaml` |
| 本机工具链 | Go 1.27.1 / Node v24.19.0 / npm 12.0.2 | 实测 |

## 3. 架构

```
desktop/                    独立 Go module（fast-https-desktop），主项目 go.mod 不变
├── main.go                 单窗口 1280×800；关窗不退出（Mac 选项置 false）
├── tray.go                 后台托盘：WindowClosing hook + 托盘菜单（显示/退出）
├── service.go              前端可调的 11 个 Go 方法
├── state.go                进程外读文件 + 调主程序 CLI
├── frontend/               Vanilla JS + Vite，卡片式布局/主题/交互/bindings
└── build/                  Wails 官方打包资产（NSIS / nfpm / .desktop / 图标）
```

**为什么不 import 主项目 `config` 包**：主配置 schema 未导出；路径解析依赖编译期 build tag 与进程 CWD；`LoadFile` 会写全局快照并强制 `mime.json`；主 module 无版本语义且会拉入整棵重依赖树。

| 数据 | 来源 |
| --- | --- |
| 路径 | `fast-https config path --json`：config / cert / log / pid 四个绝对路径 |
| 站点 | 主配置 `http.server[]` + `http.include` 递归收集 `*.json`，支持 JSONC |
| TLS | `listen` 是否含 `ssl` |
| 证书 | `ssl_certificate` 指向的 PEM，`crypto/x509` 读签发者与到期 |
| pid | `{"pid":1234}`，**不是纯数字** |
| 运行状态 | 配置声明端口的回环 TCP 探测 |
| 日志 | `access/error/system.log` 尾部 256KB |
| 操作 | 转交主程序 `start / stop / reload / status` |

桌面端**不加 API、不开端口、不接管服务进程**；托盘只负责窗口显隐与退出。

## 4. 已实施

| 项 | 内容 |
| --- | --- |
| 环境 | `go install .../wails3@latest` → `v3.1.0`；`wails3 init -t vanilla-js` |
| Go 侧 | `state.go` + `service.go`（11 方法 / 4 模型）+ `main.go` + `tray.go`，bindings 已生成 |
| 前端 | 顶部通栏（logo + 窗口按钮）、左侧单列导航（中性圆角选中块）、卡片式工作区：运行概况 4 指标 / 告警条 / 常用操作 / 最近事件；亮暗双主题 + 五档中性色 |
| 主配置表单 | 设置页按字段编辑 `config/fast-https.json`：`ReadConfigForm` / `SaveConfigForm(values)`，字段清单数据驱动、只提交改动项、保留 `http.server[]` / `acme.cas` 与未知键；`.bak` 备份 + 原子替换，保存后需手动重载；配套 `config_editor_test.go` |
| 托盘 | 关窗隐藏到托盘、左键切换窗口、右键菜单退出；`quitting` 标志保证退出不被 hook 拦下 |
| 窗口 | `Frameless: true` 去掉系统标题栏；顶部通栏用 `--wails-draggable: drag` 拖拽，自绘最小化/最大化/关闭（关闭＝隐藏到托盘） |
| 账户菜单 | 左下角「本机管理」弹出菜单：外观、设置页、隐藏到托盘、帮助文档；顶部工具按钮与左侧设置项已移除。按用户意见移除「校验配置」「关于／版本信息」——主程序没有独立校验能力，也不做版本弹层 |
| 构建 | `npm run build` → `frontend/dist`；`go build` 单文件 exe |
| 开发模式 | `wails3 dev` / `wails3 task dev`：Vite dev server（默认 9245）＋真 Go 后端窗口，不打包即可测试；端口只容一个实例 |
| 打包 | NSIS（Wails 与主项目两份）都有可选「桌面快捷方式」「开始菜单」；`nfpm.yaml` 产出带 `.desktop` + 图标的 Linux deb |
| 发布链 | `build/goreleaser.yaml` 移除 `monitor.exe` |

## 5. 打包

| 平台 | 命令 | 产物 |
| --- | --- | --- |
| Windows | `wails3 task windows:create:nsis:installer` | `bin/fast-https-desktop-amd64-installer.exe` |
| Linux | `wails3 task linux:create:deb` | `bin/fast-https-desktop_1.6.0_<arch>.deb` |

`build/package.ps1` 先准备桌面端前端与 `.desktop`，再由 goreleaser 用 `CGO_ENABLED=0 -tags production,purego` 交叉编译 Linux 二进制，产出 amd64 / arm64 的 deb、rpm、apk（依赖名按格式区分 Debian / Fedora / Alpine 的 GTK4 + WebKitGTK 6.0 包）；Linux 主机上仍可用 `wails3 task linux:create:deb` 走原生 CGO 构建。GUI 行为仍需 Linux 实机验证。详见 `desktop/README.md`。

**构建资产已按范围清理**：`desktop/build/` 只保留 `appicon.png`、`config.yml`、共通/平台 Taskfile、`windows/`（图标、info.json、manifest、nsis 脚本）、`linux/`（Taskfile、nfpm 配置与 postinstall）。Android / iOS / macOS 资产、MSIX、AppImage、RPM/Arch、server mode、Docker 交叉编译、图标合成输入与模板占位文件全部删除；`frontend/bindings/` 作为生成物已加入 git 忽略；`Taskfile.yml` 的 includes 与任务也同步精简，`wails3 task --list` 只剩 Windows / Linux 构建打包链。

## 6. 待办与风险

| 项 | 说明 |
| --- | --- |
| Linux 实机验证 | desktop deb 已由 `package.ps1` 产出，`dpkg-deb` 核实架构、依赖与 `.desktop` / 图标内容；GTK4/WebKitGTK 与 GUI 启动、应用菜单入口仍需在 Ubuntu 24.04+ 实机确认 |
| 托盘实机行为 | 已通过 `go build` 编译验证，但未在 GUI 会话里点过 X / 托盘菜单；需在 Windows 实机确认隐藏与退出 |
| frameless 拖拽 | 顶部通栏用官方 `--wails-draggable: drag`，headless 只能验证 CSS 属性与按钮存在；需在 Windows 实机确认可拖动、三个自绘按钮可用 |
| 图标尺寸 | Windows 用 `starting.ico` 的 32/48/256 三帧，Linux 用同文件提取的 256×256 PNG；若某些桌面环境显示模糊，可再补一张 128×128 |
| 配置 schema 漂移 | `state.go` 自己解析 JSON，主项目改 schema 时需同步 |
| `configDirty` 是启发式 | 用 mtime 比较，不同于主程序真实重载状态 |
| 端口探测 | 服务只绑 IPv6 或被防火墙拦时会误判「已停止」 |
| 单实例 | 未加锁；托盘常驻后开多个实例风险更高，可考虑 Wails `single_instance` |

## 7. 验收

- `wails3 generate bindings` 与实际 Go 方法一致，改类型后必须重跑
- `go build -tags production` 成功，单文件 exe 不依赖外部前端目录
- 点窗口 X 后窗口隐藏、托盘图标仍在；托盘「显示主界面」能恢复；托盘「退出」能真正结束进程
- 亮/暗切换后正文对比度 ≥4.5:1，选中态为中性色
- NSIS 组件页两个可选组件可勾选/取消，卸载后不留快捷方式
- deb 装完在应用菜单可见
