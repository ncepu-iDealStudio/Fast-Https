# 0. 开发环境与构建

## 环境

- Go **1.27.x**（go.mod 声明 1.27.0，toolchain 1.27.1；官方支持期内）
- Windows 开发需 gcc（`-race` 与 CGO 依赖）；Linux CI 同版本 Go
- 无 vendored 依赖：`go mod download` 拉取（直接依赖见根 README 依赖表）

## 获取与构建

```bash
git clone <repo> && cd fast-https
go build ./...          # 全量编译（含 monitor）
go vet ./...            # 静态检查，提交前必须干净
```

## 本地运行

```bash
# 开发模式：前台运行 + trace 控制台 + pprof(0.0.0.0:10000)
go run . dev

# 后台服务
go run . start

# 只校验配置不启动
go run . check
```

配置读取 `config/fast-https.json`（相对工作目录），站点配置可用 `http.include` 展开 conf.d 目录。日志落 `logs/`（JSON）。

## 冒烟验证

```bash
curl http://127.0.0.1:<port>/              # 静态或代理（按配置）
curl -v http://127.0.0.1:<port>/not-exist  # 404 → error_page
```

## Docker

根目录 `Dockerfile` 基于 `golang:1.27-alpine` 多阶段构建；运行时需要挂载 config/ 与 httpdoc/。

## 发布

见 [5-release.md](5-release.md)（GoReleaser snapshot、RPM、版本号规则）。

## 相关

测试基线见 [1-testing.md](1-testing.md)；目录语义见 [../architecture-map.md](../architecture-map.md)。
