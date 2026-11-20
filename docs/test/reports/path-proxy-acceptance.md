# 路径代理（多 location）—— 覆盖与端到端验收报告

- 日期：2026-11-19
- 分支：`feature/path-proxy`
- 范围：`proxy_pass` 的 URI 解析与 nginx 式路径改写、location 前缀匹配、额外 location 的 siteops/CLI/桌面端入口
- 结论：**通过**。单元、CLI、桌面端辅助函数与真实进程 E2E 全部通过；`/` 静态 + `/api` 反代 + 去前缀反代四条链路按 nginx 语义工作。

## 1. 验收依据

需求是「同一个站点里 `/` 提供静态文件，`/api` 转发到后端」，等价于 Vite 的 `server.proxy`。实现拆成四层，逐层验收：

| 能力 | 入口 | 验收标准 |
| --- | --- | --- |
| 解析上游 URI | `config/internal/loader` 的 `splitProxyPass` | `proxy_pass` 拆成 host 与 URI 两段；无 URI、`/`、`/v1`、IPv6、空串都不丢信息 |
| 路径匹配 | `runtime/filters` 的 `matchLocation` | 前缀匹配，起点命中优先，同起点更长优先 |
| 上游改写 | `modules/proxy` 的 `upstreamTarget` | 不带 URI 原样转发；带 URI 按 nginx 规则替换命中的 location 前缀；query 保留 |
| 不污染观测面 | `runtime/request` 的 `ByteRowTo` | 只替换请求行目标，`Req.Path` 不变，日志与缓存键仍是原始请求 |
| 配置写入 | `management/siteops` 的 location CRUD | 主 `/` 不可增删改；额外路径校验前缀、重复、类型与上游/目录 |
| CLI | `site location list/add/update/delete/set` | 写操作走配置事务；`delete` 必须 `--yes`；非法 JSON 在事务前拒绝 |
| 桌面端 | 站点弹窗「路径代理」列表 | 保存时整体替换额外 location，主 location 仍由站点表单管理 |

## 2. 用例矩阵

| 测试 | 位置 | 锁定的行为 |
| --- | --- | --- |
| `TestUpstreamTarget` | `modules/proxy/upstream_path_test.go` | 8 组：无 URI 保留完整路径、`/` 替换前缀、`/v1`、`/v1/`、精确匹配、精确命中带 query、只有 query、非起点命中不替换 |
| `TestSplitProxyPass` | `config/internal/loader/loader_test.go` | 无 URI、`/`、`/v1`、IPv6 字面量、空串 |
| `TestByteRowToKeepsOriginalPath` | `runtime/request/injection_test.go` | 请求行目标被替换、头部保留、`Req.Path` 不被改写 |
| `TestLocationCRUD` | `management/siteops/location_test.go` | add（proxy/local）、重复与主 location 拒绝、非法类型与非法正则拒绝、update、整体替换（set）、清空、delete、删除 `/` 拒绝 |
| `TestLocationCLI` | `cmd/internal/sitecmd/location_test.go` | add / 重复拒绝 / update / delete 缺 `--yes` 拒绝 / 成功删除 / 主 `/` 拒绝 / list 输出 / set 整体替换 / set 空列表清空 / 非法 JSON 拒绝 |
| `TestExtraLocationHelpers` | `desktop/internal/siteops/editor_test.go` | `site location set` 命令拼装、空列表清空、前置校验（空 URL、`/`、无前导斜杠、空格、上游缺协议、静态缺目录、目录不存在、未知类型、重复）、变更判定 |
| `TestSetExtraLocationsKeepsAdvancedFields` | `management/siteops/location_test.go` | 整体替换只带基础字段时，同 URL 上已有的 `zip` / `index` 与未建模的 `allow_methods` 都必须保留 |
| `TestSetExtraLocationsInPlaceEdits` | `management/siteops/location_test.go` | local ↔ proxy 切换走原地补丁：写入新类型目标，清掉 `root` / `index` / `proxy_pass` 等另一类型的专属键 |
| `TestValidateLocationPattern` | `config/internal/validate/validate_test.go` | `location.url` 必须能编译为正则，非法模式在配置校验阶段拒绝，避免 reload 时 `MustCompile` panic |
| `TestDeleteArrayElement` | `infra/jsonc/patch_test.go` | 删除数组元素（多行/单行/多行文档里的单行数组）不破坏 JSONC 结构，支撑按 URL 删除 location |
| `TestPathProxyE2E` | `tests/client_test/path_proxy_e2e_test.go` | 真实进程：静态根、静态子目录 location、保留前缀反代、去前缀反代 |

## 3. 覆盖率

测量方式：`go test -count=1 -cover <pkg>`，函数级用 `-coverprofile` + `go tool cover -func`（每次使用独立的临时 profile 文件）。

| 包 | 语句覆盖率 | 说明 |
| --- | --- | --- |
| `modules/proxy` | 85.6% | 代理主流程 + 本次新增的 `upstreamTarget` |
| `runtime/request` | 74.8% | 请求解析与报文重建 |
| `runtime/filters` | 77.5% | location 匹配入口 |
| `config/internal/loader` | 82.0% | `proxy_pass` 拆分与站点文件转换 |
| `infra/jsonc` | 88.0% | 按路径补丁，含数组元素删除 |
| `management/siteops` | 72.0% | 站点写操作统一入口（含既有站点逻辑） |
| `cmd/internal/sitecmd` | 67.5% | `site` 命令组（含既有子命令） |
| `desktop/internal/siteops`（模块 `fast-https-desktop`） | 52.8% | 桌面端表单 → CLI 参数翻译 |

本次改动直接相关的函数：

| 函数 | 覆盖率 |
| --- | --- |
| `upstreamTarget` | 94.1% |
| `matchLocation` | 100.0% |
| `convertPath` | 100.0% |
| `splitProxyPass` | 83.3% |
| `ByteRow` / `ByteRowTo` / `byteRow` | 100.0% / 100.0% / 100.0% |
| `PatchPath` / `deleteElem` | 100.0% / 91.7% |
| `normalizeLocationURL` / `validateLocationPattern` / `buildLocation` | 75.0% / 100.0% / 80.0% |
| `AddLocation` / `UpdateLocation` / `DeleteLocation` | 90.0% / 51.2% / 63.3% |
| `SetExtraLocations` / `locationBasicEdits` / `ListLocations` | 76.6% / 89.3% / 70.0% |
| `runLocationAdd` / `runLocationUpdate` / `runLocationDelete` | 100.0% / 76.5% / 100.0% |
| `runLocationSet` / `runLocationList` | 100.0% / 77.8% |
| `locationSetArgs` / `validateExtraLocations` / `sameLocations` | 85.7% / 100.0% / 83.3% |

`UpdateLocation` 的 51.2% 主要来自 `apply` 回调里每个可选字段的分支组合；CLI 与桌面端路径均已覆盖，未覆盖的是直接在 Go 层调用 API 的少见组合。

## 4. 真实进程 E2E（p2p）

用例：`tests/client_test/path_proxy_e2e_test.go` → `TestPathProxyE2E`

拓扑：

```
curl → 真实 Fast-Https 子进程（临时目录，4 个 location）
         ├─ /        → local ./httpdoc/root
         ├─ /assets  → local ./httpdoc/assets
         ├─ /api     → proxy <上游>（proxy_pass 不带 URI）
         └─ /strip   → proxy <上游>/（带 URI，替换 /strip 前缀）
                                     ↓
                              回显请求行的上游 stub
```

站点配置（测试内联，等价于 `conf.d` 写法）：

```json
{
  "listen": "<port>",
  "server_name": "127.0.0.1",
  "location": [
    {"url": "/", "type": "local", "root": "./httpdoc/root"},
    {"url": "/assets", "type": "local", "root": "./httpdoc/assets"},
    {"url": "/api", "type": "proxy", "proxy_pass": "http://127.0.0.1:<up>"},
    {"url": "/strip", "type": "proxy", "proxy_pass": "http://127.0.0.1:<up>/"}
  ]
}
```

| 场景 | 请求 | 断言 |
| --- | --- | --- |
| 静态根 | `GET /hello.txt` | 正文 `static-root`，来自 `./httpdoc/root` |
| 静态子目录 location | `GET /assets/app.js` | 正文 `static-assets`，来自 `./httpdoc/assets`，未被 `/` 抢走 |
| 反代保留完整路径 | `GET /api/users?x=1` | 上游收到的请求目标是 `/api/users?x=1` |
| 反代去前缀 | `GET /strip/users?x=1` | 上游收到的请求目标是 `/users?x=1` |

执行记录：

```text
=== RUN   TestPathProxyE2E
=== RUN   TestPathProxyE2E/static_root
=== RUN   TestPathProxyE2E/static_subdir_location
=== RUN   TestPathProxyE2E/proxy_keeps_full_path
=== RUN   TestPathProxyE2E/proxy_strips_prefix
--- PASS: TestPathProxyE2E (0.41s)
    --- PASS: TestPathProxyE2E/static_root (0.00s)
    --- PASS: TestPathProxyE2E/static_subdir_location (0.00s)
    --- PASS: TestPathProxyE2E/proxy_keeps_full_path (0.00s)
    --- PASS: TestPathProxyE2E/proxy_strips_prefix (0.01s)
PASS
ok  	fast-https/tests/client_test	0.919s
```

全量回归：

```text
go test -count=1 ./...          → 全部 ok（含 tests/arch 分层守卫、tests/client_test）
go test -count=1 ./tests/arch   → ok
cd desktop && go test -count=1 ./internal/siteops   → ok
cd desktop/frontend && npm run typecheck            → 通过（无类型错误）
```

## 5. 语义核对

| 规则 | 单元证据 | E2E 证据 |
| --- | --- | --- |
| 前缀匹配，同起点更长优先：`/assets` 命中后不再落到 `/` | `matchLocation` 100% | `static_subdir_location` |
| `proxy_pass` 不带 URI：完整路径原样转发 | `no uri keeps path` | `proxy_keeps_full_path` |
| `proxy_pass` 带 URI：用 URI 替换命中的 location 前缀 | `slash replaces prefix`、`uri without trailing slash`、`uri with trailing slash` | `proxy_strips_prefix` |
| 精确命中且 URI 以 `/` 结尾：结果保留末尾 `/` | `exact match` | — |
| 精确命中且 URI 不带尾部 `/`：原样使用 URI，不多补斜杠 | `exact match with query` | — |
| query 原样保留 | `query only` | `/api/users?x=1`、`/strip/users?x=1` |
| 非起点命中不改写 | `not at start keeps path` | — |
| 改写只作用于发往上游的报文，不改 `Req.Path` | `TestByteRowToKeepsOriginalPath` | 日志与缓存键仍按原路径记录 |

## 6. 已知边界

1. **只做前缀匹配**：不支持正则 location，也不支持 `^~` / `=` 之类的修饰符；匹配规则就是「起点命中优先、同起点更长优先」。
2. **额外 location 只暴露基础字段，但不会丢已有配置**：`site location` 与桌面端列表只管理 `url` / `type` / `root` / `proxy_pass`；`index`、`auth`、`zip`、`limit`、`proxy_cache`、`try`、`error_page`、`proxy_set_header`、`allow_methods` 等仍需手写站点文件。整体替换时，已存在的 URL 走原地补丁，只改基础字段，其余键与注释原样保留；URL 改名或删除视为新增，只写基础字段。
3. **主 location `/` 不在这里管理**：`site location` 拒绝 `/` 的增删，主 location 继续由 `site add` / `site update` 维护，避免两套入口互相覆盖。
4. **桌面端保存可能产生两次事务**：站点主字段与额外 location 都有变化时，先 `site update` 再 `site location set`，配置版本会前进两次；只改其中一项时只产生一次事务。
5. **`proxy_pass` 带 URI 时的替换范围**：与 nginx 一致，只替换请求路径开头的命中前缀；命中位置不在开头（如 `/x/api`）时保留完整路径。
6. **路径规范化**：额外 location 必须以 `/` 开头、不能含空格、不能是 `/`，并且必须能编译为正则（运行期按正则匹配），否则写入前即报 `USAGE_ERROR`，配置校验也会拦住手写坏值；静态类型的 `root` 必须是已存在的绝对路径（与站点主 location 的约定一致）。

## 7. 复跑命令

```bash
# 相关包单测与覆盖率
go test -count=1 -cover ./modules/proxy ./runtime/request ./runtime/filters \
  ./config/internal/loader ./infra/jsonc ./management/siteops ./cmd/internal/sitecmd

# 函数级覆盖率
go test -count=1 -coverprofile=/tmp/proxy.out ./modules/proxy
go tool cover -func=/tmp/proxy.out | grep -E 'upstreamTarget|^total'

# 真实进程 E2E
go test -count=1 -v -run TestPathProxyE2E ./tests/client_test

# 全量回归与分层守卫
go test -count=1 ./...
go test -count=1 ./tests/arch

# 桌面端
cd desktop && go test -count=1 -cover ./internal/siteops
cd desktop/frontend && npm run typecheck
```
