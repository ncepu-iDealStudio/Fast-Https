# 1. 测试体系

## 基线命令

```bash
go test ./...                 # 全量（Windows 上用 -p 1 规避资源竞争）
go test -race ./...           # 提交前底线：29 个包全绿、零竞争报告（需 gcc）
go vet ./...                  # 静态检查
```

任何提交前三条必须全绿；涉及 reload 的改动另跑 reload E2E（下文）。

## 测试位置约定

| 位置 | 放什么 | 示例 |
| --- | --- | --- |
| 包内 `*_test.go` | 白盒：未导出逻辑、并发竞争、基准 | `runtime/listener/hot_update_race_test.go`、`runtime/request/parse_bench_test.go` |
| `tests/unit_test/...` | 黑盒：导出 API、配置校验、格式 | `tests/unit_test/runtime/request` |
| `tests/client_test/` | E2E：子进程起真实服务走网络 | proxy/h2/reload 三篇 |
| `tests/arch/` | 架构守卫：import 分层规则 | layering_test.go |
| `tests/packaging/` | 打包守卫：行尾、logrotate 覆盖、卸载语义、脚本结构 | `lf_test.go`、`logrotate_test.go` |
| `tests/packaging/distro/` | 容器内实机验证：真装一遍 deb/rpm/apk | `deb.sh`、`rpm.sh`、`apk.sh`、`upgrade.sh` |

## E2E（环境变量门控）

| 变量 | 测试 | 说明 |
| --- | --- | --- |
| `FASTHTTPS_E2E_RELOAD=1` | TestReloadPortSwitchE2E | 真实进程切端口热重载 |
| （内部 helper 变量） | TestProxyHTTPIntegration / TestH2Integration | 默认随 `go test` 运行 |

涉及 reload 或防火墙的改动必须跑对应 E2E；它们使用子进程 + 临时运行目录，不改仓库内 `config/fast-https.json`（铁律）。

## 基准

```bash
go test ./runtime/request/ -bench . -benchmem -benchtime=2s -run xxx
```

现有基准：解析器 vs net/http（`parse_bench_test.go`）。写基准的教训：流式喂入用固定内存的 cycleReader，`bytes.Repeat(…, b.N)` 会随迭代数膨胀到数百 MB 引入 GC 噪声。

## 架构守卫

`tests/arch/layering_test.go` 用 import 扫描锁定分层方向：infra → config → core → runtime，modules 可依赖 core/runtime 但不可被 runtime 反向依赖，组合根（cmd）唯一装配。新增跨层引用（如经 core.Services 钩子注入功能包）先看该文件的 allow 清单，违反会在 `go test ./tests/arch` 报出违规包与 import 路径。

## 安装包实机验证（需要 docker）

`tests/packaging/*_test.go` 只在文本层守约定（LF、清单、轮转配置覆盖），装包后到底能不能跑要真装一遍：

```bash
goreleaser release -f build/goreleaser.yaml --snapshot --clean --skip=archive,publish,announce
sh tests/packaging/distro/run.sh all      # deb → upgrade → rpm → apk
```

四个场景分别在 `ubuntu:24.04`、`almalinux:9`、`alpine:3.21` 里装包、启动、轮转、卸载，逐项打印 `PASS` / `FAIL`，退出码即结果。

**跑之前先看 [test/3-distro-verification.md](../test/3-distro-verification.md)**：那里有上一次实测的产物校验和、镜像摘要、逐项结论，以及"看着像缺陷其实是镜像策略"的四条环境差异（man `path-exclude`、`curl-minimal` 冲突、容器内 `start` 不 daemonize、rpm `.rpmsave`）。同一份产物不重复跑。

## 新功能测试清单

1. 行为测试（含边界与畸形输入）
2. 涉及共享状态的用 `-race` 专项并发用例（参考 `hot_update_race_test.go`）
3. E2E 覆盖（若走网络路径）
4. 样本集表驱动（参考 `modules/appfirewall/waf_test.go`）
5. 改了打包清单、安装布局或维护脚本：`go test ./tests/packaging` + `sh tests/packaging/distro/run.sh all`
