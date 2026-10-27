// Package banner 负责控制台横幅与初始化阶段提示的输出。
//
// 属于 infra 展示原语（与 color 同类）：只依赖标准库与 fatih/color，
// 版本号由调用方注入，不反向依赖 config。
// 颜色经 fatih/color 输出，非 TTY / NO_COLOR / dumb 终端自动降级为纯文本。
package banner

import (
	"fmt"
	"strings"

	"github.com/fatih/color"
)

var fastHttpsLogo = `

███████╗ █████╗ ███████╗████████╗    ██╗  ██╗████████╗████████╗██████╗ ███████╗
██╔════╝██╔══██╗██╔════╝╚══██╔══╝    ██║  ██║╚══██╔══╝╚══██╔══╝██╔══██╗██╔════╝
█████╗  ███████║███████╗   ██║       ███████║   ██║      ██║   ██████╔╝███████╗
██╔══╝  ██╔══██║╚════██║   ██║       ██╔══██║   ██║      ██║   ██╔═══╝ ╚════██║
██║     ██║  ██║███████║   ██║       ██║  ██║   ██║      ██║   ██║     ███████║
╚═╝     ╚═╝  ╚═╝╚══════╝   ╚═╝       ╚═╝  ╚═╝   ╚═╝      ╚═╝   ╚═╝     ╚══════╝
`

var topLine = `┌──────────────────────────────────────────────────────┐`
var borderLine = `│`
var bottomLine = `└──────────────────────────────────────────────────────┘`

// logoGradient 是横幅各行自上而下的 256 色渐变：青 → 蓝。
var logoGradient = []int{51, 45, 39, 33, 27, 21}

// fg256 返回带 256 色前景码的着色器；经 fatih/color 的多属性序列
// 生成 SGR（38;5;N），写出走 colorable，Windows 传统控制台也能渲染。
// NoColor（非 TTY / NO_COLOR / dumb 终端）时保持纯文本。
func fg256(code int) *color.Color {
	return color.New(color.Attribute(38), color.Attribute(5), color.Attribute(code))
}

func gray(s string) string {
	return fg256(240).Sprint(s)
}

// PrintLogo 打印彩色横幅与项目链接框。版本号由调用方注入
// （banner 属于 infra 展示原语，不反向依赖 config）。
func PrintLogo(version string) {
	lines := strings.Split(strings.Trim(fastHttpsLogo, "\n"), "\n")
	for i, line := range lines {
		fmt.Fprintln(color.Output, fg256(logoGradient[i%len(logoGradient)]).Sprint(line))
	}
	fmt.Fprintln(color.Output, "\t"+fg256(39).Sprint(version))
	fmt.Fprintln(color.Output, gray(topLine))
	fmt.Fprintln(color.Output, gray(borderLine)+" [Github] https://gitee.com/ncepu-bj/fast-https       "+gray(borderLine))
	fmt.Fprintln(color.Output, gray(borderLine)+" [tutorial] https://gitee.com/ncepu-bj/fast-https     "+gray(borderLine))
	fmt.Fprintln(color.Output, gray(borderLine)+" [document] https://gitee.com/ncepu-bj/fast-https     "+gray(borderLine))
	fmt.Fprintln(color.Output, gray(bottomLine))
}
