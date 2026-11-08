# CLI 与 Monitor 遗留改进计划

修订日期：2026-11-08。状态：**待排期**。不新开功能线，只收两份已基本落地的改造计划里还没闭环的项。

对照原文（逐步设计仍保留，文首已标明执行状态，不再把「没有 Monitor」当成现状）：

- [fast-https-CLI改造开发计划.md](../../fast-https-CLI改造开发计划.md)（S0–S14）
- [fast-https-Monitor改造开发计划.md](../../fast-https-Monitor改造开发计划.md)（第 0–8 步）

现行行为以功能文档为准：[18-cli-service.md](../../feature/18-cli-service.md)、[20-runtime.md](../../feature/20-runtime.md)、[21-monitor.md](../../feature/21-monitor.md)。收口当时的记录见 [S14-delivery-summary.md](../../cli/S14-delivery-summary.md)（其中「Monitor 第 6–7 步未做」已标明过时）。

## 1. 落地结论

代码主路径已经在仓库里，对应关系如下。

| 计划 | 步骤 | 结论 |
| --- | --- | --- |
| CLI | S0–S12、S14 文档与安全自动化 | 已落地。`management/`、`cmd/` 的 service / runtime / config / site / logs / diagnose / operation / monitor 只读查询均有测试 |
| CLI | S13 `cert renew` | **未做**。计划默认是返回 `NOT_SUPPORTED` 的命令壳；仓库里没有该子命令 |
| CLI | S14 全量回归与 DSH 手工 | **部分**。见第 2 节 |
| Monitor | 第 0–7 步 | 已落地。`monitor/` 单测覆盖采集、PID 区分、轮转、规则（含端口探测与疑似卡死） |
| Monitor | 第 8 步打包 | 代码与 `tests/packaging` 守卫已在（`fast-https-monitor` 二进制、systemd 单元、`monitor.json`）。发行版容器实装 **未在本环境跑过** |
| Monitor | 第 4 节隔离环境核对、第 6/7 步手工、里程碑 A 的「压测已记录」 | 压测数字写在 [20-runtime.md](../../feature/20-runtime.md)（Windows 回环，噪声内）。隔离环境清单 **没有单独的核对记录** |

P0 命令均已实现。P1 里除 `cert renew` 外，`site *`、`cert list/check`、`logs access`、`logs query`、`monitor status/events` 均有命令入口。

## 2. 还没跑完的验收（优先）

这些是计划写明要做、但没有留下「已通过」记录的检查。做完只更新本文件的状态列，不改两份原计划。

| ID | 检查 | 计划出处 | 现状 |
| --- | --- | --- | --- |
| V1 | `go test -p 1 ./...` 全绿 | CLI S14；Monitor 里程碑 A | 2026-11-08 本机失败于 `infra/banner.TestPrintLogoColored`，与 CLI/Monitor 无关。Linux 上需再跑一次并记下结果 |
| V2 | `go test -race ./...` | 同上；热路径计数 | 只跑过 `management/svcctl`。全量未跑 |
| V3 | `sh tests/packaging/distro/run.sh all` | CLI S14；Monitor 第 8 步 | 未跑（需 Docker）。要确认 deb/rpm/apk 能装、升、卸，且 Monitor 单元可启动 |
| V4 | `goreleaser check -f build/goreleaser.yaml` | 同上 | 本机未安装 `goreleaser`，未跑 |
| V5 | `desktop/` 构建 + `go test` | CLI S14（旧命令兼容） | 未跑。`go test` 在没有 `frontend/dist` 时直接 setup 失败 |
| V6 | DSH 只读 SSH：`service status`、`config validate`、`logs error`、`diagnose`、`operation get`，stdout 只有一个 JSON | CLI S14 第 3 步；规范 §8.5 第 9 条 | 未做。自动化测的是本机进程，不是 SSH |
| V7 | `FASTHTTPS_E2E_RUNTIME=1` 下 `tests/client_test` 的 runtime 用例 | CLI S11 | 未跑。默认测试不启真实 Server 打 `/runtime` |
| V8 | Monitor 计划第 4 节七条隔离核对 | Monitor §4 | 无记录。含：9088 绑失败不影响业务口、启动期 `state:starting`、拉快照不拖垮请求、reload 后 SIGTERM 清 PID、活动连接归零、停 Monitor 不影响 Server |
| V9 | 手工：停 Server 见 `unreachable`，再起见 `restarted`；制造 5xx 升高见事件且冷却期内不重复；只起 Monitor 不起 Server 写不可达且不崩溃 | Monitor 第 6、7、8 步手工 | 无记录 |
| V10 | Linux 上补一轮与 [20-runtime.md](../../feature/20-runtime.md) 同口径的压测 | Monitor 里程碑 A；该文写明 Linux 对照留到隔离环境 | 只有 Windows 回环数据 |

建议顺序：V1、V2 放进现有 CI；V3、V4 跟打包流水线；V6、V8、V9 用一台测试机一次做完并在本文件附录贴命令与退出码。V5 确认桌面端仍只依赖旧的 `start` / `stop` / `reload` / `status` 文本（`cmd/compat_test.go` 已锁输出，桌面构建是额外确认）。

## 3. 文档对齐（2026-11-08 已改）

| ID | 当时的问题 | 现在 |
| --- | --- | --- |
| D1 | CLI 计划 S12 写「没有 Monitor 源码，本步阻塞」 | 计划文首加了执行状态；S12 改为已落地，并指向 `monitor/` |
| D2 | `examples.md` 只有「未安装」 | 已补一条实际跑出的 `unhealthy`（Server 未监听 9088） |
| D3 | feature/18 只指向 `usage.md` 第 10 节 | 字段与退出码以 `docs/cli/command-spec.md` 为准，usage 作操作说明 |
| D4 | `--since` / `--domain` 的口径容易理解成 Monitor 会补窗口 | `usage.md` 写明窗口固定为进程启动以来的累计值，Monitor 不另算时间窗 |

## 4. 明确没做、以后可以单独立项的能力

这些不是验收漏项。原计划要么标成可选，要么写在「不做」里。

| ID | 能力 | 为什么还没做 | 若要做的边界 |
| --- | --- | --- | --- |
| F1 | `cert renew` | CLI S13：续期在 Server 进程内，需要回环写通道和令牌。用户当时明确推迟 | 先安全评审。无令牌、非回环、`Plan` 外域名必须拒绝；私钥不进响应与 operation。在此之前不要加一个假装成功的子命令 |
| F2 | `NOT_SUPPORTED` 命令壳 | S13 的默认方案（退出码 11）也未加 | 若短期仍不做续期，补这个壳比留一个「未知命令」更符合规范。工作量小，可与 D3 一起做 |
| F3 | `/runtime` 端口可配置 | feature/20：写死 `9088` | 只改绑定地址与文档，不改 JSON 字段含义（`schema_version` 仍为 `"1"`） |
| F4 | Monitor 告警外送、资源曲线 | 计划第 5、7 步明确不做：无邮件、Webhook、内存/FD/CPU 规则 | 保持文件事件。外送通道另案，且不要让 Monitor import Server |
| F5 | Windows / 容器里把 Monitor 做成服务 | 计划第 8 步：归档只带可执行文件，由使用者自行托管 | 不在本跟进里做服务注册 |
| F6 | reload 时新增端口被占用导致进程退出 | Monitor 计划第 5 节：这是数据面既有行为，不借监控去改 | CLI 的 `service reload` 已做新增端口预检。旧顶层 `reload` 是否仍会 `Fatal`，在 V8 里核对并写进 feature/4 或 feature/18，不要在跟进里顺手改热路径 |

## 5. 已知限制（记录在案，本计划不排期）

来自 feature/18、feature/20、feature/21，避免下次排查时当成回归：

- `config show` 读磁盘，不读进程内快照；`config_dirty_hint` 只比修改时间。
- 强验证依赖 `/runtime`；连不上时 `service reload` / `config apply` 退回 basic。
- `cert list/check` 不遍历目录、不做链验证；`days_left` 向零取整。
- 非 `--json` 的 `site show` 会打出文件里的口令。
- Monitor 的端口探测只拨本机回环；`stuck_suspected` 只提示。
- 快照与事件按大小自轮转，不进 logrotate；要长期留档需另行导出。
- Windows 上 `stop` 直接杀进程，不跑 Unix 的 `AfterStop`；SSH 断线忽略 `SIGHUP` 只在 Unix 实现。

## 6. 建议的执行批次

一批一个提交，做完把上表状态改成「已核对（日期、环境）」。

1. **批次 A（CI）**：V1、V2。若 banner 测试只在 Windows 失败，把原因写进 `infra/banner` 的测试注释或本文件，不要为了全绿去改 Logo。
2. **批次 B（打包机）**：V3、V4、V10。V3 必须看到 Monitor 单元，不能只看主程序包。
3. **批次 C（测试机手工）**：V6、V7、V8、V9，并按 D2 补示例。
4. **批次 D（可选产品）**：F2；F1 仅在安全评审通过后开工。F3–F6 不进这四批。

## 7. 完成标准

- 第 2 节 V1–V10 每一行都有「通过 / 失败及原因 / 明确不做」之一，不留空白。
- 失败项若属于环境缺失，写清缺什么，不写成产品缺陷。
- F1 在未做之前，文档继续写「未实现」，不出现「已支持续期」。
