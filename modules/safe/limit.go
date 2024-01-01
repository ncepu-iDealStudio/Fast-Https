package safe

import (
	"fast-https/config"
	"fast-https/modules/core"
	"fast-https/modules/core/response"
	"fast-https/utils/message"
	"time"

	"golang.org/x/time/rate"
)

// read global config

var g_limit rate.Limit
var g_limiter *rate.Limiter

func limit_init() {
	// fmt.Println(config.GConfig.Servers[0].Limit.Rate, config.GConfig.Servers[0].Limit.Burst)
	temp := float64(1.00 / float64(config.GConfig.Limit.Rate) * 1000)
	g_limit = rate.Every(time.Duration(int(temp)) * time.Millisecond)
	g_limiter = rate.NewLimiter(g_limit, config.GConfig.Limit.Burst)

	// g_limit = rate.Every(1 * time.Millisecond)
	// g_limiter = rate.NewLimiter(g_limit, 50)
}

func Bucket(ev *core.Event) bool {

	// 检查是否允许进行下一个事件
	if g_limiter.Allow() {
		return true
	} else {
		// write <403> and close
		message.PrintWarn(ev.Conn.RemoteAddr().String(), " INFORMAL Event(Bucket)"+ev.Log, "\"")
		buffer := make([]byte, 1024)
		ev.Conn.Read(buffer)
		ev.Write_bytes_close(response.Default_too_many())
		return false
	}
}
