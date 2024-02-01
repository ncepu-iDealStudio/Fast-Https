package init

import (
	"fast-https/config"
	"fast-https/modules/cache"
	"fast-https/modules/safe"
	"fast-https/utils"
	"fast-https/utils/loggers"
	"fast-https/utils/message"
	"fmt"
	"log"
	"os"
	"sync"
	"time"
)

// Init setup necessary modules of the whole system
func Init() *sync.WaitGroup {
	// message initialization
	waitGroup := MessageInit()
	fmt.Fprintln(os.Stdout, time.Now().Format("2006-01-02 15:04:05"), " [SYSTEM INFO]message initialization finished")

	// config initialization
	err := config.Init()
	if err != nil {
		log.Fatalln(err)
	}
	fmt.Fprintln(os.Stdout, time.Now().Format("2006-01-02 15:04:05"), " [SYSTEM INFO]config initialization finished")

	//logger object initialization
	loggers.InitLogger(config.GConfig.LogRoot)
	fmt.Fprintln(os.Stdout, time.Now().Format("2006-01-02 15:04:05"), " [SYSTEM INFO]log initialization finished")

	// cert  initialization
	CertInit()
	fmt.Fprintln(os.Stdout, time.Now().Format("2006-01-02 15:04:05"), " [SYSTEM INFO]certification initialization finished")

	// load cache from desk
	cache.GCacheContainer.LoadCache()
	fmt.Fprintln(os.Stdout, time.Now().Format("2006-01-02 15:04:05"), " [SYSTEM INFO]cache loadcache load disk cache finished")
	CacheManagerInit()
	fmt.Fprintln(os.Stdout, time.Now().Format("2006-01-02 15:04:05"), " [SYSTEM INFO]cache manager initialization finished")

	safe.Init()
	fmt.Fprintln(os.Stdout, time.Now().Format("2006-01-02 15:04:05"), " [SYSTEM INFO]safe moudle initialization finished")

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

func MessageInit() *sync.WaitGroup {
	waitGroup := utils.GetWaitGroup()
	waitGroup.Add(1)
	go message.InitMsg()
	return waitGroup
}
