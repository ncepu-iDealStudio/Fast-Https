// Package init 在启动阶段串联配置、TLS 证书与各功能模块。
package init

import (
	"fast-https/config"
	"fast-https/infra/logging"
	"fast-https/modules/cache"
	"time"
)

// InitSystem 按固定顺序完成进程初始化：配置、日志、证书、缓存。
func InitSystem() {

	// config initialization
	// 日志尚未就绪时也只能走控制台。
	err := config.Init()
	if err != nil {
		logging.ConsoleLog.Error().Err(err).Msg("config initialization failed")
	}
	logging.ConsoleLog.Info().Msg("config initialization finished")

	// logging initialization
	_ = logging.Open(config.GConfig.LogRoot)
	logging.ConsoleLog.Info().Msg("message initialization finished")

	// cert  initialization
	CertInit()
	logging.ConsoleLog.Info().Msg("certification initialization finished")

	// load cache from desk
	cache.GCacheContainer.LoadCache()
	logging.ConsoleLog.Info().Msg("cache loadcache load disk cache finished")
	CacheManagerInit()
	logging.ConsoleLog.Info().Msg("cache manager initialization finished")
}

// CacheManagerInit 启动缓存过期清理协程。
func CacheManagerInit() {
	go func() {
		for {
			cache.GCacheContainer.ExpireCache()
			time.Sleep(time.Second)
		}
	}()
}
