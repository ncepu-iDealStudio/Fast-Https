# Fast-Https 运行期目录布局改造计划（FHS 分离布局）

修订日期：2026-10-29，实施日期 2026-11-05。状态：**阶段 0、1 已实施**；阶段 2 的迁移脚本已写进 postinstall，尚未在真实 deb/rpm 升级上验证；阶段 3（专用用户与 sandbox）未启动。

两阶段的落地方式：

- **阶段 0（先拆结构、不改值）**：`config/base_dir.go` 与 `config/base_dir_rpm.go` 各定义 ETC / DATA / LOG / RUN 四个基址，`consts.go` 的路径常量改为拼对应基址，两套基址的值仍等于改造前的路径，对外零行为变化。
- **阶段 1（改值 + 改打包）**：包布局的四个值改为 `/etc/fast-https`、`/usr/share/fast-https`、`/var/log/fast-https`、`/run/fast-https`；`nfpms.contents`、`build/spec`、systemd 单元、postinstall、man 页 `files` 表同步；归档布局一个字都没动。

两套布局各有一条测试锁住字面量：`config/paths_layout_test.go`、`config/paths_layout_rpm_test.go`。

本文记录一项打包与运行期布局改造：把"所有运行资源都放在 `/usr/share/fast-https/` 下"改成发行版仓库能接受的 FHS 分离布局。

触发原因：改造前的 deb / rpm / apk 已经能安装、能 `systemctl enable --now` 常驻、也带 man 手册页（见 [5-release.md](../5-release.md)），但日志和配置文件落在 `/usr/share` 下，被发行版仓库评审拦截只是时间问题。本文只处理**路径与权限**，不改功能、不改配置格式、不改 CLI。

## 1. 改造前的现状

| 内容 | 当前路径 | 评价 |
| --- | --- | --- |
| 二进制 | `/usr/bin/fast-https` | 符合 |
| 主配置 `config/fast-https.json` | `/usr/share/fast-https/config/fast-https.json` | 应在 `/etc`；Debian 的 conffiles 机制按 `/etc` 识别，现在靠显式声明 `config\|noreplace` 兜着 |
| `mime.json` | `/usr/share/fast-https/config/` | 同上 |
| 证书与私钥 `config/cert/` | `/usr/share/fast-https/config/cert/` | 私钥放在世界可读的数据目录，权限模型不理想 |
| 站点根 `httpdoc/root/` | `/usr/share/fast-https/httpdoc/root/` | 符合（架构无关的只读数据） |
| 日志 `logs/` | `/usr/share/fast-https/logs/` | **违反 FHS**；`/usr` 只读（容器 `ProtectSystem=strict`、不可变发行版）时启动即失败 |
| pid `fast-https.pid` | `/usr/share/fast-https/fast-https.pid` | 应在 `/run` |
| systemd 单元 | `/usr/lib/systemd/system/fast-https.service` | 符合 |
| 使用文档 | `/usr/share/doc/fast-https/usage.html` | 符合 |
| man 手册页 | `/usr/share/man/man1/fast-https.1` | 符合 |

代码侧根因（阶段 0 之前）：`config/base_dir_rpm.go` 里只有一个 `FAST_HTTPS_BASE_DIR = "/usr/share/fast-https/"`，`config/consts.go` 的 `PID_FILE`、`CONFIG_FILE_PATH`、`MIME_FILE_PATH`、`DEFAULT_LOG_ROOT`、`ROOT_CRT_DIR`、`CERT_DIR` 全部由它拼出；非包构建走 `config/base_dir.go` 的 `"./"`。

实施后的现状：两个文件各提供 `FAST_HTTPS_ETC_DIR`、`FAST_HTTPS_DATA_DIR`、`FAST_HTTPS_LOG_DIR`、`FAST_HTTPS_RUN_DIR` 四个基址，`consts.go` 按内容类型分别拼接。`FAST_HTTPS_BASE_DIR` 已删除，`absConfigPath` 拆成 `absETCPath` / `absDataPath` / `absLogPath` 三个按类型选基准的函数。1.5.0 配置里的 `./config/xxx` 前缀由 `absPathIn` 在目标不存在时自动剥掉，升级不需要手工改配置。

## 2. 实施后的布局

| 基址常量 | 包内（`-tags=rpm`） | 开发 / 归档构建 | 内容 |
| --- | --- | --- | --- |
| `FAST_HTTPS_ETC_DIR` | `/etc/fast-https` | `./config` | 主配置、`mime.json`、`cert/`（含 `acme/` 缓存）、`conf.d/` |
| `FAST_HTTPS_DATA_DIR` | `/usr/share/fast-https` | `.` | 站点根 `httpdoc/root/` |
| `FAST_HTTPS_LOG_DIR` | `/var/log/fast-https` | `./logs` | 运行日志 |
| `FAST_HTTPS_RUN_DIR` | `/run/fast-https` | `.` | pid 文件 |

开发与归档构建（`build/Dockerfile`、`build/docker/Dockerfile_*`、tar.gz 包）保持 `./config`、`./logs`、`.` 不变，因此只有 `rpm` tag 下的实现改路径。

两个决策值得记下来：

- **`/etc` 下不再保留 `config/` 这一层**（`/etc/fast-https/fast-https.json` 而不是 `/etc/fast-https/config/fast-https.json`）。代价是配置里的相对路径要去掉 `config/` 前缀，老配置靠前缀回退兼容。
- **ACME 账户与证书缓存随证书留在 `/etc/fast-https/cert/acme/`**，没有进 `/var/lib`。理由是跨平台搬运与备份只需要一个目录；代价是 `/etc` 的备份里会包含机器生成的密钥与缓存，`cert/` 目录权限收紧到 `0700`。

## 3. 改造清单

### 3.1 代码（`config` 包）

- 新增四个基址常量，`base_dir_rpm.go` / `base_dir.go` 各一套；`consts.go` 的 7 个路径常量改为拼接对应基址，不再穿透单一前缀。
- `LoadFile` 转绝对路径的基准要分开：站点 `root` 用 DATA 基址，证书、私钥、`http.include` 的 `conf.d` 用 ETC 基址，日志目录用 LOG 基址。现有行为记录在 [about.md](../../about.md) 3.3 节，改造后同步。
- 目录创建分三处兜住，都有 systemd 与手工启动两条路径：`infra/logging.mkdirLogDir` 建日志目录、`init/autocert.go` 建 `CERT_DIR`、`cmd.writePidTo` 建 pid 父目录（`/run` 是 tmpfs，重启后必须能自建）。没有在程序入口加统一的"启动早期建目录"，因为这三处已经覆盖真实写入点。
- `config/paths_test.go` 的路径断言同步更新，新增 `paths_layout_test.go`、`paths_layout_rpm_test.go`、`abs_path_test.go` 三条测试。

### 3.2 systemd 单元（`build/packaging/systemd/fast-https.service`）

- 已增加 `LogsDirectory=fast-https`、`RuntimeDirectory=fast-https`、`ConfigurationDirectory=fast-https`。
- `WorkingDirectory=/usr/share/fast-https` 保留：只影响读取相对路径，不涉及写入。
- 若后续启用专用用户：`User=fast-https`、`Group=fast-https`、`AmbientCapabilities=CAP_NET_BIND_SERVICE`、`NoNewPrivileges=yes`、`ProtectSystem=strict` + `ReadWritePaths=/var/log/fast-https`，届时安装脚本还要建用户。

### 3.3 打包

- `build/goreleaser.yaml` 的 `nfpms.contents` 已按第 2 节改写；配置文件保留 `config|noreplace`；`/etc/fast-https/cert`、`/var/log/fast-https` 用 `type: dir` 声明并配 `.keep` 占位。`/run/fast-https` 不进包。
- `build/spec` 的 `%install` 与 `%files` 已同步：配置进 `%{_sysconfdir}`，日志用 `%dir %{_localstatedir}/log/%{name}`，配置文件带 `%config(noreplace)`。
- `tools/manpage/main.go` 的 `files` 表已同步，并重新生成 `build/packaging/man/fast-https.1`。

### 3.4 文档

- `docs/usage.md` 的"获取与安装"小节（已改，`go run ./tools/docgen` 重新生成 html）。
- [5-release.md](../5-release.md) 的 Linux 发行版包小节。
- [about.md](../../about.md) 3.3 节"配置来源和路径"。
- 新增：外部控制面用 `fast-https config path --json` 取路径，不再自行判断安装方式。

## 4. 分阶段与验收

| 阶段 | 内容 | 验收 |
| --- | --- | --- |
| 0 | `config` 基址常量与绝对路径基准拆分，非 rpm 布局保持不变 **（已完成 2026-11-05）** | `go build ./... && go vet ./...` 干净；`go test ./config/...` 全绿；未打 tag 构建在现有目录布局下 `check` 通过 |
| 1 | unit 目录指令 + 包内容路径 **（已完成 2026-11-05）** | `goreleaser release --snapshot` 产出的 deb/rpm/apk 文件树符合第 2 节表格；`systemd-analyze verify` 无告警。实测：`dpkg-deb -c` 与 `tar -tf`（bsdtar 读 rpm、tar 读 apk）三份文件的路径与表格一致；`dpkg-deb -I` 显示 `Section: web`、`Priority: optional`、`Depends: ca-certificates`、`Version: 1.5.0~next-1`；`systemd-analyze verify` 只剩「ExecStart 不存在」与文件权限两条预期告警 |
| 2 | 升级迁移 **（脚本已写入 postinstall，待真实升级验证）** | deb 升级、rpm 升级各跑一次：旧 `/usr/share/fast-https/config/*` 迁移到 `/etc/fast-https/`，配置不丢，服务能起来 |
| 3 | 可选：专用用户与非 root 运行 | `ProtectSystem=strict` 下能写日志、能绑 443 |

## 5. 风险

- **升级丢配置**：迁移脚本必须先备份、只复制不删除、失败时保留旧文件并打日志；`/etc` 下已有同名文件时不覆盖。
- **破坏镜像与归档用户**：Docker 镜像和 tar.gz 依赖 `./config`、`./logs` 相对工作目录，非 rpm 构建必须保持现状。
- **常量漏改即启动失败**：`-tags=rpm` 与包布局强绑定，任一常量漏改表现为装好后读不到配置，阶段 0 之后必须做一次真实安装冒烟。
- **发行版策略差异**：`/run/fast-https` 不进包（rpm 不接受包内 `/run` 内容），由 systemd `RuntimeDirectory=` 与 `cmd.writePidTo` 兜住；`/var/log/fast-https` 在 deb/rpm 里是 `%dir`，卸载后若仍有日志则保留。
- **日志轮转缺失**：`runtime/dynlog` 没有大小上限，进 `/var/log` 之后应补一份 `/etc/logrotate.d/fast-https`，否则日志会无限增长（本阶段未做）。

## 6. 不在本计划内

- 配置格式、CLI 行为、监听逻辑的任何变化。
- 已完成的 deb/rpm/apk 打包、man 手册页、安装脚本（见 [5-release.md](../5-release.md)）。
- 专用用户与 sandbox 加固以外的安全加固项（阶段 3 仅作为可选项列出）。
