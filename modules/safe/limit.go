package safe

import (
	"fast-https/modules/core"
	"fast-https/modules/core/response"
	"time"

	"golang.org/x/time/rate"
)

// read global config

var g_limit = rate.Every(300 * time.Millisecond)
var g_limiter = rate.NewLimiter(g_limit, 5)

func Bucket(ev *core.Event) bool {

	// 检查是否允许进行下一个事件
	if g_limiter.Allow() {
		return true
	} else {
		// write <403> and close
		ev.Write_bytes_close(response.Default_too_many())
		return false
	}
}
