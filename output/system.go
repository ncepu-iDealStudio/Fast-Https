// Package output 负责输出版本横幅与系统信息。
package output

import "fmt"

func PrintInitialStart() {
	fmt.Println(`======================= System Initialization Start =====================`)
}

func PrintInitialEnd() {
	fmt.Println(`======================= System Initialization End =======================`)
}

func PrintPortsListenerStart() {
	fmt.Print("\n")
	fmt.Println(`======================= Port Listening Start ============================`)
}

func PrintPortsListenerEnd() {
	fmt.Println(`======================= Port Listening End ==============================`)
}
