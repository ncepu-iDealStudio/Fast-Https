# 19. 桌面管理端

状态：✅ 已实现（Wails v3，1.6.0 起随发行版提供）

## 功能

本机图形化管理端，用来查看 fast-https 的运行状态、管理站点与主配置、看证书与日志，并启停服务。它是**单窗口 + 后台托盘**的独立进程：不接管服务进程、不给主程序加 API、不监听新端口。

**桌面端是 CLI 的图形前端**：所有状态读取与管理动作都转交同目录的 `fast-https <cmd> --json`，由主程序的配置事务负责校验、恢复点、reload 与回滚，桌面端与命令行看到的永远是同一套语义。点击站点页刷出站点列表，本质就是执行 `site list`；每个界面动作都有对应的 CLI 命令。

窗口没有系统标题栏，顶部通栏负责拖拽和窗口按钮；左下角「本机管理」菜单放外观设置、设置页入口、隐藏到托盘与帮助文档。点「关闭」会弹窗询问「隐藏到后台 / 直接退出 / 取消」，不会默默结束进程。

| 工作区 | 内容 |
| --- | --- |
| 运行概况 | 服务状态、监听端口数、站点数、证书数四项指标（`service status` + `site list` + `cert list`）；配置状态与告警条（`config show` 的 config_dirty_hint）；常用操作跳转；「最近事件」双 tab：操作记录（`operation list`，点击看 `operation get` 详情）与本机动态；「健康检查」弹窗（`diagnose`） |
| 状态矩阵 | 运行态条（`service status` 四态 + pid、端口、站点数）、站点矩阵（`site list`）、访问日志栏（`logs access`）；「运行快照」卡片（`runtime status` / `runtime stats`）与「Monitor」卡片（`monitor status` / `monitor events`） |
| 站点 | `site list` 的站点按监听逐端口展开，每行给出可点击的完整访问地址；添加/详情弹窗对应 `site add` / `site show` / `site update`，启停与删除对应 `site enable/disable` / `site delete`，全部走配置事务 |
| 证书 | `cert list` 显示主题、签发者、到期时间与剩余天数；行内「检查」对应 `cert check`（存在性/有效期/域名覆盖/密钥配对）；「ACME 设置」展示 `acme show`；「信任本地 CA…」给出导入系统信任库的引导 |
| 日志 | `logs query` 按白名单字段筛选（级别、状态码、Host、方法、路径前缀、原因、时间），倒序滚动分页（`--until` 游标），支持 access / error / system / safe 四路 |
| 设置 | 主配置表单的字段值来自 `config show`（脱敏）；保存写候选配置树后转交 `config apply` 事务（校验、恢复点、切换、reload、失败回滚）；「校验配置」对应 `config validate`；「配置版本」卡片对应 `config versions` / `config diff` / `config rollback`；只读展示 `config path` 的运行时路径 |

「启动／停止服务」（大按钮）与「重载配置」（`service reload` 受控重载）收在概览页「运行概况」卡片右上；底部状态栏固定在窗口下沿，只放运行状态、pid、监听探测结果、刷新时间与两个图标按钮（启停、重载）。状态每 2 秒刷新一次（`service status` + `site list`，证书与配置提示有短 TTL 缓存）。版本号只在侧边栏显示一次。站点／证书／日志页不重复页面标题，说明文字直接铺在卡片里；窗口不显示页面大标题，内容直接从顶部铺满。

## 使用

从 Windows 开始菜单或 Linux 应用菜单启动 `fast-https-desktop`。它必须与 `fast-https` 主程序在同一目录（或由发行版包安装到 `/usr/bin`），否则调不到 CLI，所有页面都会显示读取失败。

- **启停与重载**：点顶部按钮，桌面端转交 `start` / `stop` / `status`；「重载配置」走 `service reload`（受控重载：校验、operation 记录、reload 后验证）。Windows 的热重载信号（CTRL_BREAK）到不了别的控制台，`service reload` 会失败；此时按钮自动降级为「停止 + 启动」重启服务，并如实提示。
- **添加站点**：站点页「+ 添加站点」按最省事的方式填——协议（仅 HTTP / 仅 HTTPS / HTTP+HTTPS）与端口用下拉和数字框选，`listen` 自动生成（如 `80, 443 ssl h2`）且可再手动微调；反代地址的协议（`http://` / `https://`）单独下拉选择，也支持粘贴完整地址自动识别。表单与 `site add` 的 flag 一一对应：证书路径、首页与文件名由配置事务自动处理（HTTPS 站点证书默认 `./cert/<主域名>.pem`，文件名即主域名）。多域名创建 = `site add` + `site update --server-name` 两条命令。
- **路径选择**：站点根目录右侧的「浏览…」调系统原生选择框；选中路径在 `config` 目录内会转成 `./xxx` 相对写法，否则保留绝对路径。
- **详情编辑**：行尾 `⋯` 打开详情（`site show`），可编辑字段即 `site update` 的白名单（server_name / listen / root / proxy_pass）；证书、首页与原始 JSON 只读展示。
- **写操作即生效**：添加/修改/删除/启停站点、保存主配置都走配置事务（校验、恢复点、切换、reload、失败自动回滚），提交后**无需手动点「重载配置」**，结果里带操作 ID 与配置版本号（`operation list` 可查）。Windows 上热重载可能没打进运行中的服务，文案如实提示「若未生效点『重载配置』（会自动重启服务）」。
- **关闭**：弹窗里的「隐藏到后台」把窗口收进托盘，托盘图标可恢复；「直接退出」或托盘右键「退出」才结束桌面端进程。`fast-https` 服务由主程序独立管理，不受桌面端退出影响。
- **外观与语言**：亮色／暗色、五档中性色、界面语言（简体中文 / English，可扩展），偏好存在本机 `localStorage`；「帮助文档」在应用内弹窗展示（中英双语、命令输出实测），弹窗底部才用浏览器打开完整命令手册 `docs/usage.html`。

## 实现方案（desktop/）

- **独立 module**：`fast-https-desktop` 不 import 主项目的 `config` 包，主项目 `go.mod` 不受影响。Wails v3 把 Go 方法暴露为 `DesktopService` bindings，前端是 Vanilla JS + Vite，产物编译期 embed 进单文件可执行程序。
- **CLI 转交层**（`internal/cli`）：`Binary / Run / RunJSON[T] / RunBareJSON[T]` 统一执行主程序、补 `--json`、分离 stdout 与 stderr、解析 `{ok,data,warnings,error}` 封套。命令的 data 形状由主程序定义（契约索引见 [docs/cli/cli-json-shapes.md](../cli/cli-json-shapes.md)），桌面端用 map 取值消费，不复制一份命令结构体；界面视图模型（`internal/model`）只做展示归一化（如合并 listen 拆行）。
- **操作 → 命令对照**：

  | 界面动作 | CLI 命令 |
  | --- | --- |
  | 站点页列表 / 矩阵 | `site list` |
  | 站点详情弹窗 | `site show` |
  | 添加 / 编辑 / 删除 / 启停站点 | `site add` / `site update` / `site delete` / `site enable` / `site disable` |
  | 设置页表单读取 / 保存 | `config show` / `config apply`（候选树 + 事务） |
  | 设置页校验 / 版本 / 对比 / 回滚 | `config validate` / `config versions` / `config diff` / `config rollback` |
  | 运行概况 / 状态栏 | `service status` |
  | 启动 / 停止 / 重载 / 查询状态 | `start` / `stop` / `service reload` / `status` |
  | 运行快照卡片 | `runtime status` / `runtime stats` |
  | 健康检查弹窗 | `diagnose` |
  | Monitor 卡片 | `monitor status` / `monitor events` |
  | 证书列表 / 检查 / ACME | `cert list` / `cert check` / `acme show` |
  | 日志页 / 矩阵访问日志栏 | `logs query` / `logs access` |
  | 最近操作时间线 | `operation list` / `operation get` |

- **候选配置树**（`internal/configops`）：保存表单时把主配置（改过字段）与 `conf.d` 站点文件副本搭成候选树（复刻主程序 `siteops.CloneLiveTree`），再 `config apply --candidate --yes`；桌面端只准备候选，不直接改线上配置。
- **多语言**（`src/i18n.js` + `src/locales/`）：界面文案全部走 `t('key')`（静态 HTML 挂 `data-i18n`，动态渲染调 `t()`），中英字典各 437 key 一一对应，`npm run i18n:check` 自查一致性与覆盖率；加一种语言只加一个字典文件 + 注册表一行，缺 key 回落中文。语言切换整窗重载。
- **内嵌帮助**（`src/help-content.js` + `src/dialogs/help.js`）：初学者向帮助（上手、常见操作、平台差异、命令行备忘、FAQ）在应用内弹窗展示，中英双语；文中命令都在实机跑过，Windows 的 `start` 前台运行、`reload` 跨窗口失败等差异如实写明。
- **前端点击测试台**（`frontend/devtest/`）：bindings 换成 mock 后用 Playwright（无头 Edge）遍历所有页面、弹窗与关键交互，逐屏截图并收集控制台错误，中英界面各跑一遍（`node run.mjs [en]`）。
- **窗口与托盘**（`main.go`、`tray.go`、`service.go`）：`Frameless: true`，`WindowClosing` hook 默认取消关闭并隐藏；`appQuitting` 标记由弹窗「直接退出」或托盘「退出」置位后才放行真正的退出。
- **交付形态**：Windows NSIS 安装包带对应架构的 `fast-https-desktop.exe`，桌面与开始菜单快捷方式指向它（`fast-https.exe` 是无控制台窗口的命令行服务端，双击不会出现界面）；Linux 的 `deb` / `rpm` / `apk` 是「主程序包 + 桌面端」的完整包，离线只装一个包即可启动服务，与独立主程序包声明 `Conflicts` / `Replaces` 避免同一路径被两个包各装一份。

## 测试

- `desktop/`：CLI 封套解析（`internal/cli`）、弹窗字段与 `site add/update` 参数翻译（`internal/siteops`）、表单字段解析与候选树搭建（`internal/configops`）、`site list` / `cert list` / `logs query` 数据映射（`internal/view`）、纯文本命令判定（`commands_test.go`）（`go test ./...`、`go vet ./...`）。
- `tests/packaging/distro/desktop-deb.sh`：Ubuntu 24.04 容器里离线单包装 → 文件与二进制一致性 → 运行期资源 → `start` 监听 80/443 → 卸载清理，以及「已装独立主程序包再装桌面端包」的替换场景，实机 `ALL-PASS`。
- 窗口交互、表单渲染与主题用 `frontend/devtest/` 的点击测试台遍历截图（中英双语界面）；Wails GUI 本身需要在桌面实机点一次。

## 已知限制

- 只管理本机同目录的实例，不接管服务进程；**运行桌面端需要同目录有主程序 `fast-https`**（开发态可退回 PATH），没有主程序时所有页面都会报「未找到主程序」。
- 站点列表是 `site list` 的口径：只列 `http.include` 目录下的站点文件（含停用），主配置 `http.server[]` 内联的站点不进列表，只能在设置页的主配置表单里看与改。
- 站点弹窗的字段是 `site add` / `site update` flag 的并集：自定义证书路径、自定义首页（index）与自定义文件名不在命令契约里，由配置事务按域名自动处理；需要这些能力时用命令行直接改站点 JSON。
- 日志查询是 `logs query` 的白名单字段（不含原文、不含 user_agent、无关键词全文搜索），分页用 `--until` 时间游标而不是 offset；时间筛选接受时长（`1h`）或 RFC3339，纯日期会自动补全。
- `acme show` 没有 JSON 输出，ACME 设置卡片展示的是它的文本结果。
- 设置页表单只做 JSON/JSONC **语法**校验，不判断配置语义；权威校验在 `config apply` 事务里。保存会重新格式化配置（JSONC 注释丢失），恢复点在配置版本里可回滚。
- 配置事务的站点目录约定是 `conf.d`：`http.include` 里的绝对路径会被改写成 `["conf.d"]`（与 `site add` 事务一致）。
- Linux 运行期需要 GTK4 + WebKitGTK 6.0，Windows 需要 WebView2 Runtime。
- Alt+F4 走 `WindowClosing` hook 的默认行为，直接隐藏到托盘，不弹关闭询问框。
