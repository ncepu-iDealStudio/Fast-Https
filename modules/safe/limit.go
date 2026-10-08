package safe

import (
	"fast-https/config"
	"fast-https/modules/core"
	"fast-https/modules/core/dynlog"
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

// allow reports whether the limiter accepts one more event.
// A nil limiter accepts the event.
func allow(lim *rate.Limiter) bool {
	if lim == nil {
		return true
	}
	return lim.Allow()
}

func Bucket(ev *core.Event) bool {

	// 检查是否允许进行下一个事件
	if allow(g_limiter) {
		return true
	} else {
		// write <403> and close
		log := dynlog.DynLogger{}
		dynlog.Log(&log, ev, "")
		//message.PrintSafe(ev.Conn.RemoteAddr().String(), " INFORMAL Event(Bucket)"+ev.Log, "\"")

		buffer := make([]byte, 1024)
		ev.Conn.Read(buffer)
		ev.RR.Res = response.DefaultTooMany()
		ev.WriteResponseClose(nil)
		return false
	}
}
