package init

import (
	"fast-https/config"
	"fast-https/utils"
	"fast-https/utils/loggers"
	"fast-https/utils/message"
	"fmt"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
)

// Init setup necessary modules of the whole system
func Init() *sync.WaitGroup {
	// message initialization
	waitGroup := MessageInit()

	// config initialization
	config.Init()

	//logger object initialization
	loggers.InitLogger(config.G_config.LogRoot, "system.log")

	// cert  initialization
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
