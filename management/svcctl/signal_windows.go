//go:build windows

package svcctl

import (
	"fmt"
	"os"
)

// SignalReload 让服务进程重载配置。
//
// Windows 的 GenerateConsoleCtrlEvent 有个硬限制：它只能发给**整个控制台
// 进程组**，不能定向到单个进程。具体后果：
//
//   - CTRL_BREAK 发给目标 pid 时，若该 pid 恰好是一个进程组的组长，信号
//     会波及组内所有进程；若服务进程没注册 handler，直接被终止。
//   - CTRL_C 更危险：它发给调用者所在的进程组，会连发起方一起打断
//     （桌面端/CLI 自己收到 Ctrl+C，退出码 0xc000013a）。
//
// 实测故障：桌面端点「禁用站点」触发配置事务，事务的 reload 阶段打断
// 了服务进程，用户看到的现象是「点禁用，服务停了」。
//
// 所以这里不再尝试 Ctrl 事件，改为返回明确错误，由上层降级成
// 「停止 + 启动」——这是桌面端 ReloadServer 已有的兜底路径。
// 将来若要恢复热重载，需要服务端用 CREATE_NEW_PROCESS_GROUP 启动并
// 自行注册 SetConsoleCtrlHandler，那时再放开下面的实现。
func SignalReload(pid int) error {
	if pid <= 0 {
		return os.ErrInvalid
	}
	// 目标就是自己：发 Ctrl 事件会打断当前进程，绝不允许。
	if pid == os.Getpid() {
		return fmt.Errorf("不能向自身进程发送 reload 信号")
	}
	if _, err := os.FindProcess(pid); err != nil {
		return fmt.Errorf("找不到服务进程 %d：%w", pid, err)
	}
	// Windows 上无法安全地定向投递 reload 信号：让上层走 stop+start。
	return fmt.Errorf("not supported by windows: 无法定向投递 reload 信号，请用重启代替")
}
