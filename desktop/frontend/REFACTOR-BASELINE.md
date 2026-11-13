# 前端重构功能基线（Vue 3 迁移用）

本文件是「渐进式重构、功能零丢失」的验收清单。重构期间**逐项迁移、逐项勾掉**；
任何时候都能拿它对照现有 vanilla 版本，确认没有功能被悄悄丢掉。

基线提交：`df4af32`（develop）。重构分支：`refactor-vue3-frontend`。

## 基线状态（迁移前实测）

- devtest：`node devtest/run.mjs` → **32/32 通过**
- i18n：`node scripts/i18n-check.mjs` → 两份字典一致、**554 键**，代码引用 **513 键**
- 构建：`npm run build` → JS 178.87 kB / gzip 54.58 kB，CSS 36.65 kB
- Go 侧：`go test ./...`（desktop 模块）全绿

## 一、Wails Service 契约（28 个导出方法，Go 侧不动）

bindings 由 `wails3 generate bindings` 生成到 `frontend/bindings/`。
重构后所有方法必须仍被调用到，签名保持不变。

### 窗口与外壳
- [ ] `MinimiseWindow()` — 顶栏最小化
- [ ] `ToggleMaximiseWindow()` — 顶栏最大化/还原
- [ ] `HideWindow()` — 关闭按钮走「隐藏到托盘」确认弹窗
- [ ] `QuitApp()` — 托盘菜单真正退出
- [ ] `OpenHelp()` — 帮助文档
- [ ] `OpenPath(path)` — 打开配置目录 / 本地路径
- [ ] `PickPath(kind)` — 文件/目录选择器（表单里的「浏览…」）

### 配置
- [ ] `ReadConfigForm()` — 系统级配置表单
- [ ] `SaveConfigForm(values)` — 保存配置
- [ ] `Paths()` — 运行期路径（配置/证书/日志/pid/state）

### 站点
- [ ] `ListSites()` — 站点列表
- [ ] `ReadSiteDetail(id)` — 站点详情（含高级配置回填）
- [ ] `AddSite(in)` — 新建站点（含 zip/limit/auth/cache 补全）
- [ ] `SaveSiteDetail(in)` — 编辑站点（差分更新）
- [ ] `DeleteSite(id)` — 删除站点
- [ ] `SetSiteEnabled(id, enabled)` — 启用/停用

### 证书
- [ ] `ListCerts()` — 证书列表
- [ ] `ImportLocalCA()` — 导入本地 CA
- [ ] `CertTrustGuide()` — CA 信任指引

### 服务与状态
- [ ] `GetOverview()` — 概览（2 秒轮询）
- [ ] `StartServer()` / `StopServer()` / `ReloadServer()` — 启停与重载
- [ ] `ServerStatus()` — 服务状态
- [ ] `TailLog(name, lines)` — 日志尾部
- [ ] `QueryLogs(q)` — 日志检索分页
- [ ] `RunCommand(args)`（`commands.go`）— 任意 CLI 透传，GUI 的
      `monitor status` / `monitor events` / `diagnose` / `certcheck` 都靠它

## 二、页面（7 个 pane）

- [ ] `overview` 概览：运行概况、常用操作、最近事件、状态卡
- [ ] `matrix` 状态矩阵：多面板（含 Monitor 面板 `monitor status` + `monitor events`）
- [ ] `sites` 站点：列表、添加/编辑/删除、启停、详情抽屉
- [ ] `certs` 证书：列表、导入 CA、信任指引、检查弹窗
- [ ] `logs` 日志：日志切换、检索、自动刷新、分页
- [ ] `runtime` 运行快照
- [ ] `operations` 操作记录
- [ ] `settings` 设置：配置表单（含 ACME 分组、路径列表、枚举下拉、浏览按钮）、配置版本

## 三、弹窗与浮层（7 个）

- [ ] `site` 添加/编辑站点：**高级配置四项（响应压缩 gzip/br、限流 rate/burst/mem/nodelay、Basic 认证、反向代理缓存 path/valid/max_size）**，新建与编辑都要显示并能提交
- [ ] `close` 关闭确认（隐藏到托盘 / 退出）
- [ ] `confirm` 通用确认
- [ ] `trust` CA 信任指引
- [ ] `diagnose` 健康检查
- [ ] `certcheck` 证书检查
- [ ] `help` 帮助目录（左侧目录 + 右侧正文、底部内容）
- [ ] `account-menu` 账户菜单（语言切换、主题切换）

## 四、横切能力

- [ ] **i18n**：zh-CN / en 双语言，554 键；切换语言不整窗 reload，原地重绘
- [ ] **主题**：亮/暗双套 CSS 变量（`html[data-mode]`），持久化
- [ ] **图标**：`data-ic` 线性 SVG 水合
- [ ] **loading**：按钮 spinner、立即加载（不走 140ms 延迟）
- [ ] **toast**：操作结果提示
- [ ] **轮询**：overview 2s / details 15s，切到页面才拉数据
- [ ] **窗口**：Frameless 自绘顶栏、F5/Ctrl+R 强制重载、禁用浏览器右键菜单（输入框保留）
- [ ] **错误展示**：`cli.FriendlyError` / `msgOf` 人话错误
- [ ] **缓存**：命令结果短 TTL 缓存，合并同周期重复调用
- [ ] **devtest**：浏览器端回归（截图 + 断言），迁移后需等价覆盖
- [ ] **i18n-check**：静态校验两份字典一致、界面 key 全覆盖

## 五、设计系统（保留，不重做）

- `base.css` 的 15 个 CSS 变量 + 暗色覆盖原样保留
- 视觉方向：亮色「文档式运行报告」/ 暗色「深石墨值班台」，中性色导航，无侧边色块
- 新增 `tokens.css` 收敛散落的字号（11/12/12.5/13px）与间距（6/8/10/14px）

## 六、迁移策略

1. **保留 `src/` 原样**，新建 `src-vue/` 并行开发；Vite 入口先指向 `src-vue/`，
   `src/` 作为随时可回退的参照。
2. 按 pane 逐个迁移，每迁一个就用 devtest 等价断言验证该页功能。
3. Go 侧与 `frontend/bindings/` 完全不动。
4. 全部勾完、且新旧两套 devtest 都绿之后，才删除 `src/`。
5. 期间 `develop` 保持可用；本分支合入前不影响任何已发布行为。
