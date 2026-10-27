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

## 新功能测试清单

1. 行为测试（含边界与畸形输入）
2. 涉及共享状态的用 `-race` 专项并发用例（参考 `hot_update_race_test.go`）
3. E2E 覆盖（若走网络路径）
4. 样本集表驱动（参考 `modules/appfirewall/waf_test.go`）
