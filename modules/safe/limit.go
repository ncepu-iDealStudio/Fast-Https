package safe

import (
	"fast-https/config"
	"fast-https/modules/core"
	"fast-https/modules/core/response"
	"fast-https/utils/message"
	"golang.org/x/time/rate"
	"time"
)

// read global config

var g_limit rate.Limit
var g_limiter *rate.Limiter

func limitInit() {
	temp := int((1 / config.GConfig.Servers[0].Limit.Rate) * 1000)
	g_limit = rate.Every(time.Duration(temp) * time.Millisecond)
	g_limiter = rate.NewLimiter(g_limit, config.GConfig.Servers[0].Limit.Burst)
}

func Bucket(ev *core.Event) bool {

	// 检查是否允许进行下一个事件
	if g_limiter.Allow() {
		return true
	} else {
		// write <403> and close
		message.PrintWarn(ev.Conn.RemoteAddr().String(), " INFORMAL Event(Bucket)"+ev.Log, "\"")
		ev.Write_bytes_close(response.Default_too_many())
		return false
	}
}
