# 5. 发布流程

## 版本号

`config/consts.go` 的 `CURRENT_VERSION`（当前 `V1.4.0`）；GoReleaser 快照版本号 `1.4.0-next`。发布正式版：改 CURRENT_VERSION → 打 git tag → 跑 GoReleaser。

## 本地快照构建

```bash
goreleaser release -f .goreleaser.yaml --snapshot --clean
```

产物在 `dist/`；压缩包内打入 `config/fast-https.json`、`config/mime.json`、`config/fastcgi.conf`、`docs/usage.html`、`httpdoc/root/`、`README.md`，并用 `.keep` 保留 `config/cert/`、`config/conf.d/`、`logs/` 空目录。

发行包只带使用文档 `docs/usage.html`，其余 `docs/` 内容属于开发与设计资料，不进包。生成命令：

```bash
go run ./tools/docgen
```

## RPM

`spec/` 下的构建脚本自带 Go 1.27.1 下载；注意 `config/base_dir_rpm.go`（`-tags=rpm`）会改变资源定位路径——RPM 构建必须带该 tag 验证。

## Docker

根目录 `Dockerfile`（`golang:1.27-alpine` 多阶段）；运行时挂载 config/ 与 httpdoc/。

## 发布前清单

0. `go run ./tools/docgen` 重新生成使用文档（`go test ./tools/docgen` 校验 md 与 html 是否一致）
1. `go build ./... && go vet ./...` 干净
2. `go test -race ./...` 全绿（29 包）
3. 三个 E2E：`FASTHTTPS_E2E_RELOAD=1` reload、proxy、h2
4. 根 README 依赖表与 `go.mod` 一致
5. `docs/feature/` 与 `develop/plans/iteration-plan.md` 状态同步
6. 快照产物冒烟：安装目录跑 `check`/`start`/`status`/`reload`/`stop`
7. Windows 与 Linux 各自编译验证（不以单平台代替）
