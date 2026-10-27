package banner

import (
	"fmt"

	"github.com/fatih/color"
)

// 初始化阶段分隔线：给启动日志在控制台上一个可扫读的节奏。
// 统一写 color.Output（colorable），与横幅一致。
func PrintInitialStart() {
	fmt.Fprintln(color.Output, `======================= System Initialization Start =====================`)
}

func PrintInitialEnd() {
	fmt.Fprintln(color.Output, `======================= System Initialization End =======================`)
}

func PrintPortsListenerStart() {
	fmt.Fprintln(color.Output)
	fmt.Fprintln(color.Output, `======================= Port Listening Start ============================`)
}

func PrintPortsListenerEnd() {
	fmt.Fprintln(color.Output, `======================= Port Listening End ==============================`)
}
