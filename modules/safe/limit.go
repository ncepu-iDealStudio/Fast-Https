package safe

import (
	"fast-https/config"
	"fast-https/modules/core"
	"fast-https/modules/core/response"
	"time"

	"golang.org/x/time/rate"
)

// read global config

var g_limit rate.Limit
var g_limiter *rate.Limiter

func limitInit() {
	temp := float64(1.00 / float64(config.GConfig.Limit.Rate) * 1000)
	g_limit = rate.Every(time.Duration(int(temp)) * time.Millisecond)
	g_limiter = rate.NewLimiter(g_limit, config.GConfig.Limit.Burst)

}

func Bucket(ev *core.Event) bool {

	// 检查是否允许进行下一个事件
	if g_limiter.Allow() {
		return true
	} else {
		// write <403> and close
		core.Log(&ev.Log, ev, "")
		//message.PrintSafe(ev.Conn.RemoteAddr().String(), " INFORMAL Event(Bucket)"+ev.Log, "\"")

		buffer := make([]byte, 1024)
		ev.Conn.Read(buffer)
		ev.RR.Res = response.DefaultTooMany()
		ev.WriteResponseClose(nil)
		return false
	}
}
