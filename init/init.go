package init

import (
	"fast-https/utils"
	"fast-https/utils/loggers"
	"fast-https/utils/message"
	"fmt"
	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
	"sync"
)

func Init() *sync.WaitGroup {
	// 消息初始化
	waitGroup := MessageInit()

	// 配置读取初始化
	//err := ViperInit()
	//if err != nil {
	//	message.PrintErr(err)
	//	message.Exit()
	//}

	// logger object initial
	//LoggerInit(viper.GetString("log.type"))

	// sysLog  initial
	SysLogInit()
	Cert_init()

	return waitGroup
}

func MessageInit() *sync.WaitGroup {
	waitGroup := utils.GetWaitGroup()
	waitGroup.Add(1)
	go message.InitMsg()
	return waitGroup
}

// ViperInit viper object init
func ViperInit() (err error) {
	viper.SetConfigName("config")
	viper.AddConfigPath("./configs") // 添加搜索路径
	viper.SetConfigType("yaml")

	err = viper.ReadInConfig()
	if err != nil {
		fmt.Println("Fatal error config file: ", err)
		return
	}
	viper.WatchConfig()

	viper.OnConfigChange(func(e fsnotify.Event) {
		fmt.Println("Config file:", e.Name, "Op: ", e.Op)
	})

	return
}

// LoggerInit Log object init
func LoggerInit(logType string) {
	loggers.InitLogger(logType)
}
