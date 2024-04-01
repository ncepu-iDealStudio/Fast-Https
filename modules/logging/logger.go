/**
* @Author:刘浩宇
* @Description：
* @File：logger
* @Version:1.0.0
* @Date:2023/10/22 14:56:31
 */

package logging

import (
	"fast-https/modules"
	"log"
	"os"

	"github.com/fatih/color"
)

// DefaultLogger 是一个日志插件的默认实现，它将日志输出到标准输出。
// 如果配置文件中没有指明该功能的实现，则默认使用该插件。
type DefaultLogger struct {
	logger *log.Logger
}

// init 用于注册模块，通过导包的方式实现注册模块
// func init() {
// 	modules.RegisterModule(&DefaultLogger{})
// 	fmt.Println("Default Logger Module is registered...")
// }

func (l *DefaultLogger) FastModule() *modules.ModuleInfo {
	return &modules.ModuleInfo{
		ID: "fast.plugin.DefaultLogger",
		New: func() modules.Module {
			return &DefaultLogger{
				logger: log.New(os.Stdout, "", 1),
			}
		},
	}
}

func (l *DefaultLogger) Info(msg string) {
	l.logger.Println("[INFO] ", msg)
}

func (l *DefaultLogger) Error(msg string) {
	l.logger.Println(color.YellowString("[ERROR] ", msg))
}

func (l *DefaultLogger) Debug(msg string) {
	l.logger.Println(color.MagentaString("[DEBUG] ", msg))
}

func (l *DefaultLogger) Warning(msg string) {
	l.logger.Println(color.RedString("[WARNING] ", msg))
}
