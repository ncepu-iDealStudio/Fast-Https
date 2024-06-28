package proxy

import (
	"fast-https/modules/cache"
	"fast-https/modules/core"
	"fast-https/utils"
	"strings"
)

type ProxyCache struct {
	ProxyCachePath  string
	ProxyCacheKey   string
	ProxyCacheValid []string
}

func NewProxyCache(key string, valid []string, path string) *ProxyCache {
	return &ProxyCache{
		ProxyCacheKey:   key,
		ProxyCacheValid: valid,
		ProxyCachePath:  path,
	}
}

// to do: improve this function
func (pc *ProxyCache) ProcessCacheConfig(ev *core.Event,
	resCode string) (md5 string, expire int) {

	cacheKeyRule := pc.ProxyCacheKey

	ruleString := ev.GetCommandParsedStr(cacheKeyRule)

	// fmt.Println("-------------------", ev.RR.Req_.Path)
	// fmt.Println("generate cache key value=", ruleString)
	md5 = cache.GetMd5(ruleString)

	// convert ["200:1h", "304:1h", "any:30m"]
	valid := pc.ProxyCacheValid
	for _, c := range valid {
		split := strings.Split(c, ":")
		if split[0] != resCode || split[0] == "any" {
			expire = utils.ParseTime(split[1])
			// fmt.Println("generate cache expire time=", expire)
			return
		}
	}
	return
}

func (pc *ProxyCache) CacheData(ev *core.Event, resCode string,
	data []byte, size int) {

	// according to usr's config, create a key
	uriStringMd5, expireTime := pc.ProcessCacheConfig(ev, resCode)
	cache.GCacheContainer.WriteCache(uriStringMd5, expireTime,
		pc.ProxyCachePath, data, size)
	// fmt.Println(cfg.ProxyCache.Key, cfg.ProxyCache.Path,
	// cfg.ProxyCache.MaxSize, cfg.ProxyCache.Valid)
}
