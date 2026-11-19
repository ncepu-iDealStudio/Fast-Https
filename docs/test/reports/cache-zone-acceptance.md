# 缓存区（cache zone）重构 —— 验收报告

- 日期：2026-11-18
- 范围：主配置顶层 `cache` 段、站点 location `proxy_cache` 引用、CLI `cache` 命令组、桌面端缓存区管理页
- 结论：**通过**。缓存区的定义、引用、运行时缓存行为、CLI 管理与桌面端管理全部按设计工作；旧的站点级缓存对象写法已在解析期拒绝。

## 1. 验收依据

本次重构把缓存配置从「站点 location 上的对象」改为「主配置里的具名缓存区 + 站点按名字引用」：

| 能力 | 入口 | 验收标准 |
| --- | --- | --- |
| 定义缓存区 | 主配置 `cache: [{name, path, valid, max_size}]` | 名称唯一、目录必填、容量非负、`valid` 至少一条 |
| 引用缓存区 | 站点 location `proxy_cache: "<区名>"` | 必须是已定义区名；旧对象写法解析期报错 |
| 缓存运行时 | `modules/proxy` + `modules/cache` | 命中不回源、`no-cache` 回源、非 GET/HEAD 不缓存、重启可恢复 |
| CLI 管理 | `fast-https cache list/add/update/delete` | JSON 输出稳定；被引用时拒绝删除并列出引用位置 |
| CLI 站点联动 | `site add/update --cache-zone` | 未知区名在写盘前拒绝；空值清除引用 |
| 桌面端管理 | 独立「缓存区」页 + 站点弹窗下拉 | 增删改查可用，有效期用结构化控件录入 |

## 2. CLI 真实验证

环境：Windows amd64，本机 `go build -o %TEMP%\fh-cache-check\fast-https.exe .`；工作目录为临时状态目录，配置为最小主配置 + 一个 `conf.d` 站点文件；服务未运行（写操作只落盘，提示下次 reload/启动生效）。

| # | 命令 | 退出码 | 结果 |
| --- | --- | --- | --- |
| 1 | `site add --domain a.local --root <www> --listen 18080 --proxy-pass http://127.0.0.1:9000 --yes --json` | 0 | 站点写入 `conf.d/a.local.json`，事务版本 v002 |
| 2 | `cache list --json` | 0 | `{"zones":[]}` |
| 3 | `cache add --name edge --path ./cache --valid 200:10m --valid any:30s --max-size 1048576 --json` | 0 | 写入成功；stderr 告警「缓存区已定义但没有站点使用」 |
| 4 | `site update a.local --cache-zone edge --yes --json` | 0 | diff：`location[0].proxy_cache` added = `edge` |
| 5 | `cache delete edge --json` | **10** | `CONFLICT`：`still referenced by: ...conf.d\a.local.json location "/"` |
| 6 | `site update a.local --cache-zone nope --yes --json` | **2** | `USAGE_ERROR`：`cache zone "nope" is not defined ...`，未进入事务 |
| 7 | `site update a.local --cache-zone= --yes --json` | 0 | diff：`proxy_cache` removed（old=`edge`），附 unused-zone 告警 |
| 8 | `cache delete edge --json` | 0 | 引用已解除，删除成功 |
| 9 | `cache list --json` | 0 | `{"zones":[]}` |
| 10 | `config validate --json` | 0 | `{"valid":true}` |

关键输出摘录（路径已省略前缀）：

```json
{"ok":false,"command":"cache.delete","error":{"code":"CONFLICT",
 "message":"cache zone \"edge\" is still referenced by: ...\\conf.d\\a.local.json location \"/\""}}
```

```json
{"ok":false,"command":"site.update.a.local","error":{"code":"USAGE_ERROR",
 "message":"cache zone \"nope\" is not defined in ./config/fast-https.json; run `fast-https cache list` to see available zones"}}
```

反例：手工把站点改成 `"proxy_cache": "missing"` 后执行 `config validate --json`，退出码 **5**：

```json
{"ok":false,"command":"config.validate","error":{"code":"CONFIG_INVALID",
 "message":"location \"/\" references unknown cache zone \"missing\""}}
```

结论：CLI 的四个写入口都在写盘前完成前置校验，错误码与文案符合 `docs/cli/cli-json-shapes.md` 的约定；删除仍被引用的缓存区不会走事务回滚，而是直接返回 CONFLICT 并指出引用文件与 location。

### 2.1 `site add`：监听、H2 与缓存区

H2 不是独立 flag，而是 listen 串里的标记：`ssl` 表示 TLS，`h2` 表示 HTTP/2，两者互不蕴含。因此：

- `--listen "18443 ssl"`：TLS，但**不开** H2；
- `--listen "18444 ssl h2"`：TLS + H2；
- `--listen 18445 --tls`：只给端口时按旧约定补成 `18445 ssl h2`；
- `--cache-zone edge`：与监听一起写进同一个站点文件。

真实验证（新临时目录 `%TEMP%\fh-site-h2-check`，同一二进制）：

| # | 命令 | 退出码 | 生成的站点文件 |
| --- | --- | --- | --- |
| 1 | `site add --domain plain.local --root <www> --listen "18443 ssl" --yes --json` | 0 | `listen=18443 ssl`，无 `h2` |
| 2 | `site add --domain h2.local --root <www> --listen "18444 ssl h2" --yes --json` | 0 | `listen=18444 ssl h2` |
| 3 | `site add --domain auto.local --root <www> --listen 18445 --tls --yes --json` | 0 | `listen=18445 ssl h2`（`--tls` 自动补） |
| 4 | `cache add --name edge --path ./cache --valid any:30s --json` | 0 | 缓存区写入主配置 |
| 5 | `site add --domain cached.local --root <www> --listen "18446 ssl h2" --cache-zone edge --yes --json` | 0 | `listen=18446 ssl h2`，`proxy_cache=edge` |
| 6 | `config validate --json` | 0 | `{"valid":true}` |

生成的四个站点文件（`listen` / `proxy_cache`）：

```text
plain.local:  listen=18443 ssl      proxy_cache=
h2.local:     listen=18444 ssl h2   proxy_cache=
auto.local:   listen=18445 ssl h2   proxy_cache=
cached.local: listen=18446 ssl h2   proxy_cache=edge
```

对应回归用例：

- `TestSiteAddH2ToggleWithCacheZone`（`cmd/internal/sitecmd`）：真实执行 `site add`，逐字断言站点文件里的 `listen`、是否含 `h2`、以及 `proxy_cache`。
- `TestResolveListens`：新增「`--tls` + 显式 `ssl`（无 h2）」两组，确认不会被强行补成 `ssl h2`。

## 3. 真实进程 E2E（p2p 链路）

用例：`tests/client_test/cache_zone_e2e_test.go` → `TestProxyCacheZoneE2E`

拓扑：客户端 → 真实 Fast-Https 子进程（临时目录中 `cache` 段 + `proxy_cache`）→ 上游 stub。上游每次被访问都返回不同的 `hit-N`，因此「正文不变 + 上游命中次数不增加」即可证明响应来自缓存。

| 场景 | 断言 |
| --- | --- |
| 首次请求 | 回源，正文 `hit-1`，上游命中 1 |
| 二次请求 | 正文仍 `hit-1`，上游命中次数保持 1（命中缓存） |
| `Cache-Control: no-cache` | 回源拿到 `hit-2` |
| POST 同路径 | 回源拿到 `hit-3`，不读不写缓存 |
| 杀掉进程再启动 | GET 仍返回 `hit-2`，上游命中次数仍为 3（从缓存区目录恢复索引） |

执行记录：

```text
=== RUN   TestProxyCacheZoneE2E
--- PASS: TestProxyCacheZoneE2E (1.47s)
PASS
ok  	fast-https/tests/client_test	3.090s
```

`tests/client_test` 全量（含代理基础链路、连接复用、拒绝日志等既有 E2E）：

```text
ok  	fast-https/tests/client_test	6.221s
```

### 3.1 H2 开关的运行时验证

用例：`tests/client_test/h2_toggle_e2e_test.go` → `TestH2ToggleE2E`

两个真实进程分别按 `"<port> ssl"` 与 `"<port> ssl h2"` 启动，客户端在 ALPN 里同时提供 `h2` 与 `http/1.1`：

| 监听写法 | 协商结果 | 请求协议 |
| --- | --- | --- |
| `<port> ssl` | 空（不通告 h2） | HTTP/1.1，200，正文正确 |
| `<port> ssl h2` | `h2` | HTTP/2，200，正文正确 |

```text
=== RUN   TestH2ToggleE2E
=== RUN   TestH2ToggleE2E/ssl_only_keeps_http/1.1
=== RUN   TestH2ToggleE2E/ssl_h2_negotiates_h2
--- PASS: TestH2ToggleE2E (1.28s)
PASS
ok  	fast-https/tests/client_test	1.805s
```

## 4. 测试与覆盖率

### 4.1 主仓库

| 命令 | 结果 |
| --- | --- |
| `go test ./...` | 全部通过（退出码 0） |
| `go test -count=1 ./tests/client_test` | `ok` |
| `go test -race -count=1 ./modules/proxy ./modules/cache ./config/internal/cacheconf ./cmd/internal/cachecmd` | 四个包全部 `ok` |

本次改动相关包的语句覆盖率：

| 包 | 覆盖率 | 说明 |
| --- | --- | --- |
| `config/internal/cacheconf` | 81.3% | 缓存区读写、局部替换、空列表规范化 |
| `config/internal/validate` | 87.2% | 重名、目录、`valid`、未知引用校验 |
| `config/internal/loader` | 81.9% | 主配置与站点文件合并 |
| `config/internal/sitefile` | 77.5% | 站点文件读写与 patch |
| `cmd/internal/cachecmd` | 70.9% | list/add/update/delete + dry-run + 冲突 |
| `cmd/internal/sitecmd` | 60.6% | `--cache-zone` 前置校验、`--tls`/显式 `ssl` 的 listen 推导与清除 |
| `modules/proxy` | 85.2% | 缓存键、TTL 选择、命中/写入/绕过 |
| `modules/cache` | 94.2% | 落盘、索引、热层、容量记账、启动恢复 |
| `management/siteops` | 71.5% | 站点写操作的统一入口 |

补充用例（本次新增或扩充）：

- `config/internal/validate/cache_zone_test.go`：重名、缺目录、负数容量、空 `valid`、未知引用。
- `modules/proxy/cache_key_test.go`、`cache_flow_test.go`、`cache_test.go`：区名前缀隔离、`status:duration`/`any` 选择、方法限制。
- `modules/cache/load_cache_test.go`：启动扫描、过期条目丢弃、损坏文件跳过。
- `cmd/internal/sitecmd/site_test.go`：`--cache-zone` 的未知区名拒绝与空值清除；`site add` 同时验证「`ssl` 不开 H2 / `ssl h2` 开 H2」与缓存区落盘（`TestSiteAddH2ToggleWithCacheZone`）。
- `tests/client_test/h2_toggle_e2e_test.go`：真实进程分别以 `ssl` 与 `ssl h2` 启动，断言 ALPN 协商结果与请求协议。

### 4.2 桌面端

```text
ok  	fast-https-desktop	2.568s	coverage: 16.5% of statements
ok  	fast-https-desktop/internal/cacheops	1.373s	coverage: 43.3% of statements
ok  	fast-https-desktop/internal/cli	1.383s	coverage: 41.0% of statements
ok  	fast-https-desktop/internal/configops	1.461s	coverage: 52.6% of statements
ok  	fast-https-desktop/internal/siteops	1.469s	coverage: 55.9% of statements
ok  	fast-https-desktop/internal/view	1.562s	coverage: 39.7% of statements
```

`cacheops` 新增 `editor_test.go`，锁定三件事：`cache list` JSON → 桌面模型的映射、写操作前的快速校验文案、`--valid` 参数拼装（空列表显式传 `--valid=`，保证 update 能清空）。

`siteops` 新增 `TestPlanAddTLSWithoutH2`：站点弹窗取消 H2 时，`listen` 必须原样是 `443 ssl`，不能带 `h2`；勾选 H2 的路径由 `TestPlanAddSingleDomain`（`443 ssl h2`）与 `TestPlanAddWithSharedCert` 覆盖。

前端静态检查：

| 命令 | 结果 |
| --- | --- |
| `cd desktop/frontend && npm run typecheck` | 通过（无类型错误） |
| `cd desktop/frontend && npm run i18n:check` | 两份字典各 698 键，代码引用 341 键，全部覆盖 |

## 5. 桌面端实机验证

启动真实桌面程序（`go build -tags production ... -H windowsgui`）逐项检查：

| 检查项 | 结论 |
| --- | --- |
| 侧栏新增「缓存区」独立页，与站点设置同构 | 符合 |
| 新建/编辑缓存区弹窗：名称、目录、有效期、最大容量 | 符合 |
| 有效期改为「状态码 + 数值 + 单位」结构化录入，落库仍为 `200:10m` / `any:30s` | 符合 |
| 列表把 `any:30s` 显示为「全部状态 30 秒」 | 符合 |
| 删除仍被站点引用的缓存区时显示 CLI 返回的冲突文案 | 符合 |
| 「新建缓存区」「添加站点」「添加证书」按钮位于标题栏右上角 | 符合 |
| 缓存区页无刷新按钮、无冗长说明文字 | 符合 |
| 站点弹窗「缓存区」下拉，留空表示不缓存 | 符合 |
| 站点弹窗 H2 为独立复选框：勾选 → `443 ssl h2`，取消 → `443 ssl`；HTTPS 关闭时禁用并提示「H2 仅在 TLS 上生效」 | 符合 |
| 日志页无「是否实时」开关，固定 5 秒自动刷新 | 符合 |
| 默认主题为暗色（仅默认值，不锁定主题） | 符合 |
| 监控中心健康卡新增「缓存区」，可跳转管理 | 符合 |
| 监控中心底部两栏 50/50 | 符合 |

## 6. 已知边界

1. **HTTP/2 仅 TLS**：当前实现只在 TLS 监听上协商 h2，明文 h2c 未支持；界面对此做了禁用与提示，不承诺明文 H2。
2. **只有 GET / HEAD 读写缓存**：其他方法一律回源且不写入，避免共享缓存区被非幂等响应污染。
3. **`no-cache` / `no-store` 只影响读取**：客户端要求绕过时必定回源，但回源得到的可缓存响应仍会写入缓存（因此本次 E2E 中重启后恢复的是 `hit-2`）。若需要「完全不入缓存」，应在上游侧或用后续策略项控制。
4. **`max_size` 是软上限**：超限时丢弃本次写入，不做淘汰扫描；空间由过期清理回收，因此实际占用可能短暂超过上限。
5. **命中即续期**：读取命中会把过期时间按 `valid` 顺延，短时间内持续访问的内容可能长期留在缓存里。
6. **改名不支持**：站点按名字引用缓存区，`cache update` 只能改目录/有效期/容量；改名需 delete + add。
7. **删除只删配置**：磁盘上的缓存目录不会被清理，需要手工回收空间。

## 7. 复跑命令

```bash
# 单元与包内测试
go test ./...
go test -cover ./config/internal/cacheconf ./config/internal/validate \
  ./cmd/internal/cachecmd ./cmd/internal/sitecmd ./modules/proxy ./modules/cache ./management/siteops

# 竞态
go test -race -count=1 ./modules/proxy ./modules/cache \
  ./config/internal/cacheconf ./cmd/internal/cachecmd

# 真实进程 E2E
go test ./tests/client_test -run 'TestProxyCacheZoneE2E|TestH2ToggleE2E' -count=1 -v
go test -count=1 ./tests/client_test

# 桌面端
cd desktop && go test -cover ./...
cd desktop/frontend && npm run typecheck && npm run i18n:check
```
