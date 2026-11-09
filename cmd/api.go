package cmd

import (
	"fast-https/cmd/internal/procctl"
	"fast-https/cmd/internal/servicecmd"
)

// 这些是 cmd 包对外的公共 API：main 与 tests/ 下的集成测试会直接调用，
// 实现本身在 internal 子包里，这里只做最小的对外暴露。

// WritePid 把当前进程的 pid 写入指定文件。
func WritePid(path string) error { return procctl.WritePid(path) }

// RegisterHandlers 注册服务器事件处理器。
func RegisterHandlers() { servicecmd.RegisterHandlers() }

// RegisterServices 注册所有自定义服务。
func RegisterServices() { servicecmd.RegisterServices() }

// StartRuntimeEndpoint 启动运行时查询端点。
func StartRuntimeEndpoint() error { return servicecmd.StartRuntimeEndpoint() }

// StopRuntimeEndpoint 关闭运行时查询端点。
func StopRuntimeEndpoint() { servicecmd.StopRuntimeEndpoint() }
