package init

import (
	"fast-https/config"
	"fast-https/modules/cache"
	"fast-https/modules/safe"
	"fast-https/utils"
	"fast-https/utils/logger"
	"fast-https/utils/message"
	"fmt"
	"os"
	"sync"
	"time"
)

// Init setup necessary modules of the whole system
func Init() *sync.WaitGroup {

	// config initialization
	// can't use message
	err := config.Init()
	if err != nil {
		logger.Error("%s", err)
	}
	fmt.Fprintln(os.Stdout, time.Now().Format(config.SERVER_TIME_FORMAT), " [SYSTEM INFO]config initialization finished")

	// message initialization
	waitGroup := MessageInit(config.GConfig.LogRoot)
	fmt.Fprintln(os.Stdout, time.Now().Format(config.SERVER_TIME_FORMAT), " [SYSTEM INFO]message initialization finished")

	// cert  initialization
	CertInit()
	fmt.Fprintln(os.Stdout, time.Now().Format(config.SERVER_TIME_FORMAT), " [SYSTEM INFO]certification initialization finished")

	// load cache from desk
	cache.GCacheContainer.LoadCache()
	fmt.Fprintln(os.Stdout, time.Now().Format(config.SERVER_TIME_FORMAT), " [SYSTEM INFO]cache loadcache load disk cache finished")
	CacheManagerInit()
	fmt.Fprintln(os.Stdout, time.Now().Format(config.SERVER_TIME_FORMAT), " [SYSTEM INFO]cache manager initialization finished")

	safe.Init()
	fmt.Fprintln(os.Stdout, time.Now().Format(config.SERVER_TIME_FORMAT), " [SYSTEM INFO]safe moudle initialization finished")

	return waitGroup
}

func CacheManagerInit() {
	go func() {
		for {
			cache.GCacheContainer.ExpireCache()
			time.Sleep(time.Second)
		}
	}()
}

func MessageInit(logRootPath string) *sync.WaitGroup {
	waitGroup := utils.GetWaitGroup()
	waitGroup.Add(1)
	go message.InitMsg(logRootPath)
	return waitGroup
}
