# Fast-Https 测试框架说明

## 一、单元测试框架

### 1.1 目录结构

```text
test/
├── readme.md
├── fast-https_test.go
├── scripts/                         # 开发模式启动脚本
│   ├── dev-test.sh
│   └── dev-test.ps1
├── single/
│   └── workloop.go
├── client_test/                     # 子进程集成测试
│   ├── proxy_e2e_test.go
│   ├── reload_e2e_test.go
│   └── http_https_change_test.go
└── unit_test/
    ├── helpers/
    │   ├── mock_system.go           # 进程和信号处理模拟
    │   └── mock_io.go               # 文件和输出模拟
    ├── cmd/commands_test.go
    ├── config/check_config_test.go
    ├── compress/compress_test.go
    ├── safe/safe_test.go
    ├── modules/
    │   ├── module_test.go
    │   ├── module_registration_consistency_test.go
    │   └── core/
    │       ├── server/server_test.go
    │       ├── listener/reload_common_test.go
    │       ├── request/parse_test.go
    │       ├── response/response_test.go
    │       └── h2/conn/
    └── utils/
        ├── logger/logger_test.go
        └── security/rsa_test.go
```

### 1.2 测试辅助工具

#### MockProcessManager (mock_system.go)

用于模拟进程管理和信号处理：

```go
mockProc := helpers.NewMockProcessManager()
mockProc.CreateProcess(1234)           // 创建模拟进程
proc, _ := mockProc.FindProcess(1234)  // 查找进程
proc.Signal(os.Interrupt)              // 发送信号
```

#### MockIO (mock_io.go)

用于模拟文件操作和输出捕获：

```go
mockIO := helpers.NewMockIO()
cleanup := mockIO.CaptureOutput()      // 开始捕获输出
defer cleanup()                        // 确保清理

mockIO.WriteFile("test.txt", []byte("data"), 0644)  // 写文件
data, _ := mockIO.ReadFile("test.txt")              // 读文件
output := mockIO.Stdout.String()                     // 获取标准输出
```

### 1.3 测试执行

#### 运行测试命令

在仓库根目录执行。`go.mod` 在根目录，下面的包路径相对仓库根目录。

```bash
go test ./test/unit_test/...
```

#### 特定模块测试

```bash
go test ./test/unit_test/cmd/...
go test ./test/unit_test/modules/core/server/...
go test ./test/unit_test/utils/logger/...
```

#### Reload 端到端回归（可选）

该测试默认跳过，仅在需要验证 reload 端口切换时启用。

Windows PowerShell:

```powershell
$env:FASTHTTPS_E2E_RELOAD = "1"
go test ./test/client_test -run TestReloadPortSwitchE2E -v
```

完成后可清理环境变量：

```powershell
Remove-Item Env:FASTHTTPS_E2E_RELOAD
```

#### 反向代理集成测试

该测试会在临时工作目录启动 Fast-Https 子进程，并使用本地 HTTP 服务模拟动态 HTML 页面、JSON API 和静态资源上游。测试覆盖 GET 查询参数、POST 请求体、自定义请求头、响应状态码、响应头和响应体，不需要安装 JSP 或 Python Web 运行时，也不会修改仓库配置。

```bash
go test ./test/client_test -run TestProxyHTTPIntegration -v
```

#### 测试覆盖率

在仓库根目录执行：

```bash
go test -cover ./...

go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### 1.4 模块测试说明

#### 命令模块 (commands_test.go)

测试范围：

- 根命令初始化
- 服务启动/停止/重载
- 服务安装/卸载

示例：

```go
func TestRootCmd(t *testing.T) {
    rootCmd := cmd.RootCmd()
    assert.NotNil(t, rootCmd)
    // ... 更多断言
}
```

#### 服务器核心 (server_test.go)

测试范围：

- 服务器初始化
- 信号处理
- 端口扫描
- 重载功能
- 连接处理

示例：

```go
func TestServerInit(t *testing.T) {
    s := server.ServerInit()
    assert.NotNil(t, s)
    // ... 更多断言
}
```

#### 日志模块 (logger_test.go)

测试范围：

- 日志级别设置
- 各类型日志输出
- 日志格式化
- Fatal日志处理

示例：

```go
func TestLoggerLevel(t *testing.T) {
    logger.Level(4)
    // ... 测试日志输出
}
```

### 1.5 测试开发指南

#### 测试文件模板

```go
package xxx_test

import (
    "testing"
    "fast-https/test/unit_test/helpers"
    "github.com/stretchr/testify/assert"
)

var (
    mockIO *helpers.MockIO
)

func TestMain(m *testing.M) {
    mockIO = helpers.NewMockIO()
    m.Run()
}

func TestNewFeature(t *testing.T) {
    cleanup := mockIO.CaptureOutput()
    defer cleanup()
    
    // 测试代码
    // 断言结果
}
```

#### 测试最佳实践

1. 使用 `TestMain` 进行测试设置和清理
2. 合理使用 mock 对象避免真实系统调用
3. 每个测试函数专注于一个功能点
4. 使用有意义的测试用例名称
5. 添加清晰的测试注释
6. 确保资源在测试后正确清理

### 1.6 问题处理

#### 测试注意事项

1. 运行测试前确保已安装依赖：

```bash
go get github.com/stretchr/testify/assert
```

1. 避免在测试中使用真实的系统资源
2. 使用 defer 确保资源清理
3. 测试失败时提供有意义的错误信息
4. 定期运行完整的测试套件

#### 常见问题解决

1. 测试超时
   - 检查异步操作的超时设置
   - 确保正确清理资源

2. 测试干扰
   - 确保测试之间相互独立
   - 适当使用 `TestMain` 重置状态

3. Mock 对象使用
   - 参考 helpers 包中的示例
   - 确保正确初始化 mock 对象

## 二、开发测试模式

### 2.1 特性说明

1. 日志级别设为 6
2. 在 `0.0.0.0:10000` 打开 pprof
3. 不进入 Linux 守护进程
4. 与 `start` 相同，证书缺失时由 `init.CertInit` 生成自签名证书
5. 配置变更通过 `reload` 子命令或非 Windows 上的 `SIGINT` 生效，启动流程不会监视配置文件

### 2.2 使用方法

#### 环境准备

1. 确保已安装 `go.mod` 声明的 Go 版本（当前为 1.21.5）
2. 克隆项目到本地
3. 安装依赖：

```bash
go mod download
```

#### 启动命令

Linux/Mac:

```bash
# 在仓库根目录使用开发测试脚本
./test/scripts/dev-test.sh

# 或直接运行
go run fast-https.go dev
```

Windows:

```powershell
# 使用开发测试脚本
.\test\scripts\dev-test.ps1

# 或直接运行
go run fast-https.go dev
```

#### 配置说明

开发模式与普通模式都使用 `config/fast-https.json` 作为运行时配置文件。
`config/dev.yaml` 仅作为开发参数示例，不会被运行时自动加载。

`go run fast-https.go dev` 与 `start` 读取同一份 JSON。`config/dev.yaml` 里的端口、日志和代理片段不会进入运行时。

### 2.3 调试指南

#### 实时监控

1. 日志查看：

```bash
tail -f logs/system.log logs/access.log logs/error.log logs/safe.log
```

1. 进程监控：

```bash
ps aux | grep fast-https
```

#### 配置变更流程

1. 修改 `config/fast-https.json` 或 `config/conf.d` 下的 json 配置
2. 执行 `go run fast-https.go reload` 触发配置重载

#### 性能分析

1. CPU 分析：

```bash
go tool pprof http://localhost:10000/debug/pprof/profile
```

1. 内存分析：

```bash
go tool pprof http://localhost:10000/debug/pprof/heap
```

1. 协程分析：

```bash
go tool pprof http://localhost:10000/debug/pprof/goroutine
```

### 2.4 使用须知

#### 安全注意事项

1. 开发模式仅用于本地测试，不要在生产环境使用
2. 开发证书仅供测试使用
3. 避免使用生产环境的敏感数据

#### 运行注意事项

1. 确保测试端口未被占用
2. 使用 Ctrl+C 可以直接停止服务
3. 定期清理日志文件
4. 保持配置文件的正确格式

#### 调试建议

1. 使用 IDE 的调试功能进行断点调试
2. 适当调整日志级别获取更多信息
3. 使用性能分析工具定位性能问题
4. 定期检查资源使用情况
