# Fast-Https Desktop（Wails v3）

本机图形化管理端。**单窗口 + 后台托盘**：从 Windows 开始菜单或 Linux 应用菜单点开；点窗口关闭按钮会弹窗询问「隐藏到后台 / 直接退出 / 取消」，隐藏后程序继续在托盘运行，左键单击托盘图标可显示／隐藏主界面，托盘右键菜单里的「退出」等同于「直接退出」。桌面端始终不接管 fast-https 服务进程。

- Wails v3 CLI：`v3.1.0`（`go install github.com/wailsapp/wails/v3/cmd/wails3@latest`，`@latest` 指向周更稳定版，不跟 nightly）
- 独立 Go module `fast-https-desktop`，**不 import 主项目的 `config` 包**，主项目 `go.mod` 不受影响
- 前端：Vanilla JS + Vite，产物编译期 embed 进单文件可执行程序
- 视觉：亮色是文档式「运行报告」，暗色是深石墨「值班台」；导航只保留左侧一列，按钮为克制圆角，选中态用中性淡灰。侧边栏／顶栏与工作区靠底色差区分（无边框线），滚动条弱化为几乎不可见，日志列表占满剩余高度

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

## 数据从哪来（转交 CLI）

桌面端是 CLI 的图形前端：**所有状态读取与管理动作都转交同目录的主程序** `fast-https <cmd> --json`，由主程序的配置事务负责校验、恢复点、reload 与回滚，桌面端与命令行看到的永远是同一套语义。点击站点页刷出站点列表，本质就是执行 `site list`。

| 界面动作 | CLI 命令 |
| --- | --- |
| 站点页列表 / 状态矩阵 | `site list`（站点按监听拆行展示） |
| 站点详情弹窗 | `site show` |
| 添加 / 编辑 / 删除 / 启停站点 | `site add` / `site update` / `site delete` / `site enable` / `site disable` |
| 设置页表单读取 / 保存 | `config show`（脱敏）/ `config apply`（候选配置树 + 事务） |
| 设置页校验 / 版本 / 对比 / 回滚 | `config validate` / `config versions` / `config diff` / `config rollback` |
| 运行概况 / 状态栏 / 启停重载 | `service status` / `start` / `stop` / `service reload` / `status` |
| 运行快照卡片 | `runtime status` / `runtime stats` |
| 健康检查弹窗 | `diagnose` |
| Monitor 卡片 | `monitor status` / `monitor events` |
| 证书列表 / 检查 / ACME 设置 | `cert list` / `cert check` / `acme show` |
| 日志页 / 矩阵访问日志栏 | `logs query` / `logs access` |
| 最近操作时间线 | `operation list` / `operation get` |
| 命令面板 | 全部 35 条命令（通用表单 + 原样结果） |

命令的 `data` JSON 形状见 [docs/cli/cli-json-shapes.md](../docs/cli/cli-json-shapes.md)；`internal/cli` 解析封套，`internal/model` 做界面视图模型（例如合并 listen 拆行），不复制命令结构体。运行期路径仍由 `config path --json` 提供（归档 / deb / rpm / apk 布局由主程序按编译期基址算好，桌面端不拼路径）。本地保留的文件读取只有两处：搭建候选配置树（保存表单时）与文件元数据（大小/修改时间）。

**已知边界**：设置页只做 JSON/JSONC 语法校验，不判断配置语义；权威校验由 `config apply` 事务负责。桌面端需要同目录有主程序 `fast-https`（开发态可退回 PATH）。站点列表是 `site list` 的口径：只列 `http.include` 目录下的站点文件，主配置内联站点在设置页里看。

## 站点页：添加站点

站点页右上角「添加站点」弹出表单，字段与 `site add` 的 flag 一一对应：`server_name`、`listen`（逗号或换行分隔，支持 `80` 与 `443 ssl h2` 混写）、类型（`local` 静态 / `proxy` 反代）、`root` 或 `proxy_pass`。证书路径、首页（index）与文件名不在命令契约里，由配置事务自动处理：TLS 站点证书默认写 `./cert/<主域名>.pem`（即 `init.CertInit` 生成的本地证书），文件名即主域名。`listen` 里带 `ssl` 即启用 TLS。

`server_name` 支持**空格分隔多个域名**（nginx 风格），可以是域名、`localhost` 或本机 IP（如 `192.168.1.10`）；运行时把每个名称都注册到 `HostMap`，按 `server_name:端口` 匹配 Host，自签证书 SAN 与 ACME 申请覆盖全部名称。用 IP 或别名访问时，默认的 `security.host_check=off` 会回落到该端口的站点；改成 `enforce` 才严格校验 Host。多域名创建会执行两条命令：`site add`（主域名即文件名）+ `site update --server-name`（补全全部域名）。

「站点根目录」右侧有「浏览…」按钮，调 Wails 的原生目录选择框（`PickPath`）；选中的绝对路径若在 `config` 内会转成 `./xxx` 相对写法，否则保留绝对路径。取消选择不会改动输入框。

这里填的是**站点证书**（由本地根 CA 签发，TLS 握手时发给浏览器，配置里 `ssl_certificate` 指向它）；`httpdoc/root/root.crt` 是**本地根 CA 证书**，负责签发站点证书、放在站点根目录供下载，也是导入系统信任库要用的那一张。两者都由 `CertInit` 首次启动时生成。

增删改与启停全部走 `site` 命令的配置事务（校验、恢复点、切换、reload、失败自动回滚），提交后**立即生效，无需手动点「重载配置」**；结果里带操作 ID 与配置版本号，在概览页「最近操作」里可查。详情编辑（行尾 `⋯`）对应 `site show` 读取与 `site update` 的白名单字段（server_name / listen / root / proxy_pass），证书与原始 JSON 只读展示；主配置 `http.server[]` 内联的站点不在列表里，需到设置页修改。

## 日志页：筛选与滚动分页

日志页转交 `logs query`（`QueryLogs`）：可切 `access` / `error` / `system` / `safe` 四路（`--kind`），按白名单字段筛选——级别（`--level`）、状态码（`--status`，支持 `500` / `4xx` / `400-499`）、Host（`--domain`）、方法（`--method`）、路径前缀（`--path-prefix`）、拒绝原因（`--reason`）与起止时间（`--since` / `--until`，接受时长 `1h` 或 RFC3339，纯日期自动补全）。结果**由新到旧**，每页 30 条，滚到列表底部自动加载更早的一页，也有「加载更多」按钮兜底；切换日志类型、改筛选或点「刷新」会重置分页。

`logs query` **没有 offset 分页**：「加载更早」把上一页最早一条的时间当下一页的 `--until` 游标（`LogPage.NextUntil`），与游标同刻的记录自动剔重。每条记录是白名单字段（`LogEntry`：time / level / kind / message / method / path / status / size / host / remote_addr），**不含原始行**（悬浮提示展示记录的紧凑 JSON），message / path 由主程序脱敏并截断。矩阵页的访问日志栏走 `logs access` 尾部。

本地根 CA 证书路径由主程序 `config path --json` 的 `root_crt` 提供，桌面端不拼接数据目录、不写死安装布局。

## 设置页：主配置表单

设置页**按字段编辑**主配置（通常是 `config/fast-https.json`）：分组表单覆盖 `security` / `waf` / `acme` / `http` / 基础等标量字段，枚举用下拉、布尔用勾选、数组用多行输入。字段清单在 `desktop/internal/configops/editor.go` 维护（纯 UI 元数据），字段值来自 `config show`（敏感值由主程序脱敏）。`engine`（集群占位）当前没有模块读取、项目也未启用集群，不放进表单；配置里若仍带该段会原样保留。

保存只提交**被改动的字段**（脱敏值不触碰就不会被写回）：先搭一棵候选配置树（主配置副本 + `conf.d` 站点文件副本，复刻主程序 `siteops.CloneLiveTree`），再转交 `config apply --candidate --yes` 的配置事务——校验、生成差异、保存恢复点、切换文件、reload、失败自动回滚。保存即生效，**无需手动点「重载配置」**，结果里带操作 ID、配置版本号与恢复点。未纳入表单的 `http.server[]`、`acme.cas` 与未知顶层键原样保留。配置语义由 `config apply` 校验。

「校验配置」按钮转交 `config validate`；「配置版本」卡片转交 `config versions`，每行可「对比」（`config diff --candidate <版本>`）或「回滚」（`config rollback --version <版本> --yes`，是一次新的事务操作，可再次回滚）。「运行时路径」只读展示 `config path` 的输出。主程序不在同目录时会显示读取失败并禁用表单。

## 证书页：信任本地 CA 引导

证书页右上角「信任本地 CA…」弹出引导：显示 `root.crt` 的实际路径与是否已生成、风险提示、按平台给出的分步说明和可直接复制的导入命令（Windows `certutil`、Linux `update-ca-certificates`、macOS `security add-trusted-cert`）。另有「打开证书目录」与「尝试自动导入」两个按钮。

弹窗会标明当前平台（Windows / Linux / macOS），证书与私钥路径都由主程序 `config path --json` 动态给出，命令与确认方式按平台生成，通用说明用平台中性措辞。桌面端不会擅自改系统信任库：自动导入前会再弹一次确认，明确说明「本机所有 HTTPS 站点都会信任由这个 CA 签发的证书」，确认后才发起提权（Windows 走 UAC 的 `Start-Process -Verb RunAs`，Linux 走 `pkexec`，macOS 只给手动命令）。提权失败或工具缺失时，弹窗会把系统输出带回来并提示改用复制的命令手动执行。相关 Go 代码在 `cert_trust.go`，路径推导与各平台命令由 `cert_trust_test.go` 覆盖。

## 前端结构

`frontend/src` 按职责拆分，入口只做编排：

```
src/
  main.js           入口：注入弹窗模板 → 各模块 init → 导航/动作/日志/窗口绑定 → 定时刷新
  api.js            DesktopService 统一出口 + msgOf 错误提取
  state.js          跨模块共享状态（当前日志、日志游标、配置脏标记等）
  theme.js          亮暗主题与五档中性色
  router.js         工作区切换与按需加载（设置页拉配置版本、矩阵页刷新面板、证书页拉 ACME）
  refresh.js        概览 2s / 明细 15s 拉取编排
  actions.js        data-action 按钮统一入口
  account-menu.js   左下角账户菜单
  ui/dom.js         $ / $$ / bind / esc / toast / localStorage
  ui/events.js      概览「最近事件」：操作记录（operation list/get）+ 本机动态时间线
  pages/            overview / details / config / logs / commands / panels 六个页面模块
                    （panels.js = 运行快照、Monitor、ACME 三块卡片，手动刷新）
  dialogs/          close / confirm / site / trust / diagnose / certcheck 六个弹窗（各导出 template + init）
  styles/           base / layout / components / pages（变量、骨架、组件、页面）
```

弹窗与账户菜单的 HTML 不再写在根 `index.html` 里，而是由各自模块导出 `template`，入口在挂载时注入 `body` 末尾；事件绑定与业务逻辑仍留在对应模块。这样单文件不会随功能增加而膨胀，改某个页面或弹窗只需看一个文件。

普通页面的数据走 `DesktopService` 的专用方法（映射成对应 CLI 命令）；补充面板（运行快照、Monitor、操作记录、健康检查、证书检查、ACME、配置版本/差异/回滚）直接走 `RunCommand(args)` 通用转交，按命令渲染 CLI 的 data 原样 JSON，桌面端不再为每个命令定义一份结构体。

## 主题

左下角「本机管理」点击弹出账户菜单，里面包含：亮色／暗色、五档中性色（石墨、暖灰、松绿、赭石、靛灰，默认亮色 + 石墨）、设置页、隐藏到后台托盘、帮助文档。主题选择保存在本机 `localStorage`。

「帮助文档」打开发行包里的 `docs/usage.html`（开发时 exe 在 `desktop/bin/`，会回退到仓库根）。顶部工具按钮与左侧「设置」导航项已移除，统一从这个菜单进入。

「启动／停止服务」与「重载配置」（`service reload` 受控重载）在概览页「运行概况」卡片右上；底部状态栏固定在窗口下沿，只放运行状态、pid、监听探测结果、刷新时间与两个图标按钮。版本号只在侧边栏显示一次，不在状态栏或账户菜单里重复。站点／证书／日志页不再重复页面标题，说明文字直接铺在卡片里；窗口不显示页面大标题，内容直接从顶部铺满。站点页的启停、删除与编辑都走 `site` 命令的配置事务（可回滚、自动 reload），主配置内联站点不在列表里。

**选中／激活态一律用中性色**：亮色是淡淡的灰，暗色是略亮的灰。正常、告警、危险三种语义色独立，不随主题变化。
