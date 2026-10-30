# Fast-Https 运行期目录布局改造计划（FHS 分离布局）

修订日期：2026-10-29。状态：**仅规划，未实施**，等后续迭代再启动。

本文记录一项推迟的打包与运行期布局改造：把"所有运行资源都放在 `/usr/share/fast-https/` 下"改成发行版仓库能接受的 FHS 分离布局。

触发原因：当前的 deb / rpm / apk 已经能安装、能 `systemctl enable --now` 常驻、也带 man 手册页（见 [5-release.md](../5-release.md)），但日志和配置文件落在 `/usr/share` 下，被发行版仓库评审拦截只是时间问题。本文只处理**路径与权限**，不改功能、不改配置格式、不改 CLI。

## 1. 现状

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

代码侧根因：`config/base_dir_rpm.go` 里只有一个 `FAST_HTTPS_BASE_DIR = "/usr/share/fast-https/"`，`config/consts.go` 的 `PID_FILE`、`CONFIG_FILE_PATH`、`MIME_FILE_PATH`、`DEFAULT_LOG_ROOT`、`ROOT_CRT_DIR`、`CERT_DIR` 全部由它拼出；非包构建走 `config/base_dir.go` 的 `"./"`。

## 2. 目标布局

| 基址常量 | 包内（`-tags=rpm`） | 开发 / 归档构建 | 内容 |
| --- | --- | --- | --- |
| `FAST_HTTPS_ETC_DIR` | `/etc/fast-https/` | `./config/` | 主配置、`mime.json`、`cert/`、`conf.d/` |
| `FAST_HTTPS_DATA_DIR` | `/usr/share/fast-https/` | `./` | 站点根 `httpdoc/root/` |
| `FAST_HTTPS_LOG_DIR` | `/var/log/fast-https/` | `./logs/` | 运行日志 |
| `FAST_HTTPS_RUN_DIR` | `/run/fast-https/` | `./` | pid 文件 |

开发与归档构建（`build/Dockerfile`、`build/docker/Dockerfile_*`、tar.gz 包）必须保持 `./config`、`./logs`、`./` 不变，因此只有 `rpm` tag 下的实现改路径。

## 3. 改造清单

### 3.1 代码（`config` 包）

- 新增四个基址常量，`base_dir_rpm.go` / `base_dir.go` 各一套；`consts.go` 的 7 个路径常量改为拼接对应基址，不再穿透单一前缀。
- `LoadFile` 转绝对路径的基准要分开：站点 `root` 用 DATA 基址，证书、私钥、`http.include` 的 `conf.d` 用 ETC 基址，日志目录用 LOG 基址。现有行为记录在 [about.md](../../about.md) 3.3 节，改造后同步。
- 启动早期创建缺失目录：`/var/log/fast-https/`、`/run/fast-https/`、`/etc/fast-https/cert/`。现有代码对 `logs/` 是否已自动创建，实施时先确认再决定新增范围。
- `config/paths_test.go` 的路径断言同步更新，并补一条"四个基址各自拼出预期绝对路径"的单测。

### 3.2 systemd 单元（`build/packaging/systemd/fast-https.service`）

- 增加 `LogsDirectory=fast-https`、`RuntimeDirectory=fast-https`：systemd 在启动前建目录，并在 unit 停止时清理 `/run` 下的目录。
- 可选 `ConfigurationDirectory=fast-https`（仅建目录，不覆盖已有文件）。
- 若后续启用专用用户：`User=fast-https`、`Group=fast-https`、`AmbientCapabilities=CAP_NET_BIND_SERVICE`、`NoNewPrivileges=yes`、`ProtectSystem=strict` + `ReadWritePaths=/var/log/fast-https`，届时安装脚本还要建用户。

### 3.3 打包

- `build/goreleaser.yaml` 的 `nfpms.contents`：所有 `dst` 按第 2 节表格改；配置文件保留 `config|noreplace`；补 `/etc/fast-https/cert`、`/var/log/fast-https` 目录项，保证不经过 systemd 手工启动时日志目录也已存在。
- `build/spec`：`%install` 与 `%files` 同步；`/etc`、`/var/log`、`/run` 下的条目用 `%dir` / `%ghost` 表达，实施前对照 openEuler 打包规范确认写法。
- `tools/manpage/main.go` 里的 `files` 表同步，否则生成的手册页仍然告诉用户去 `/usr/share/fast-https/config`。

### 3.4 文档

- `docs/usage.md` 的"获取与安装"与"目录布局"两节（改完执行 `go run ./tools/docgen` 重新生成 html）。
- [5-release.md](../5-release.md) 的 Linux 发行版包小节。
- [architecture-map.md](../../architecture-map.md) 的 RPM、Linux 包两行。
- [about.md](../../about.md) 3.3 节"配置来源和路径"。

## 4. 分阶段与验收

| 阶段 | 内容 | 验收 |
| --- | --- | --- |
| 0 | `config` 基址常量与绝对路径基准拆分，非 rpm 布局保持不变 | `go build ./... && go vet ./...` 干净；`go test ./config/...` 全绿；未打 tag 构建在现有目录布局下 `check` 通过 |
| 1 | unit 目录指令 + 包内容路径 | `goreleaser release --snapshot` 产出的 deb/rpm/apk 文件树符合第 2 节表格；`systemd-analyze verify` 无告警 |
| 2 | 升级迁移 | deb 升级、rpm 升级各跑一次：旧 `/usr/share/fast-https/config/*` 迁移到 `/etc/fast-https/`，配置不丢，服务能起来 |
| 3 | 可选：专用用户与非 root 运行 | `ProtectSystem=strict` 下能写日志、能绑 443 |

## 5. 风险

- **升级丢配置**：迁移脚本必须先备份、只复制不删除、失败时保留旧文件并打日志；`/etc` 下已有同名文件时不覆盖。
- **破坏镜像与归档用户**：Docker 镜像和 tar.gz 依赖 `./config`、`./logs` 相对工作目录，非 rpm 构建必须保持现状。
- **常量漏改即启动失败**：`-tags=rpm` 与包布局强绑定，任一常量漏改表现为装好后读不到配置，阶段 0 之后必须做一次真实安装冒烟。
- **发行版策略差异**：rpm 不建议打包 `/run`、`/var/log` 下的内容，需要用 `%ghost` 或交给 systemd 目录指令，写法待实施时确认。

## 6. 不在本计划内

- 配置格式、CLI 行为、监听逻辑的任何变化。
- 已完成的 deb/rpm/apk 打包、man 手册页、安装脚本（见 [5-release.md](../5-release.md)）。
- 专用用户与 sandbox 加固以外的安全加固项（阶段 3 仅作为可选项列出）。
