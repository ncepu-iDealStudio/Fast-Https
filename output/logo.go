package output

import (
	"fast-https/config"
	"fmt"
)

var fastHttpsLogo = `                                        

███████╗ █████╗ ███████╗████████╗    ██╗  ██╗████████╗████████╗██████╗ ███████╗
██╔════╝██╔══██╗██╔════╝╚══██╔══╝    ██║  ██║╚══██╔══╝╚══██╔══╝██╔══██╗██╔════╝
█████╗  ███████║███████╗   ██║       ███████║   ██║      ██║   ██████╔╝███████╗
██╔══╝  ██╔══██║╚════██║   ██║       ██╔══██║   ██║      ██║   ██╔═══╝ ╚════██║
██║     ██║  ██║███████║   ██║       ██║  ██║   ██║      ██║   ██║     ███████║
╚═╝     ╚═╝  ╚═╝╚══════╝   ╚═╝       ╚═╝  ╚═╝   ╚═╝      ╚═╝   ╚═╝     ╚══════╝
                                        ` + "\n\t" + config.CURRENT_VERSION

var topLine = `┌──────────────────────────────────────────────────────┐`
var borderLine = `│`
var bottomLine = `└──────────────────────────────────────────────────────┘`

func PrintLogo() {
	fmt.Println(fastHttpsLogo)
	fmt.Println(topLine)
	fmt.Println(fmt.Sprintf("%s [Github] https://gitee.com/ncepu-bj/fast-https       %s", borderLine, borderLine))
	fmt.Println(fmt.Sprintf("%s [tutorial] https://gitee.com/ncepu-bj/fast-https     %s", borderLine, borderLine))
	fmt.Println(fmt.Sprintf("%s [document] https://gitee.com/ncepu-bj/fast-https     %s", borderLine, borderLine))
	fmt.Println(bottomLine)
}
