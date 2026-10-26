package static

import (
	"fast-https/core"
)

// for static requests which not end with "/"
// attention: if backends use API interface, they
// must end with "/"
func event301(ev *core.Event, path string) {

	ev.RR.Res.SetFirstLine(301, "Moved Permanently")
	ev.RR.Res.SetHeader("Location", path)
	ev.RR.Res.SetHeader("Connection", "close")

	// h2 下响应必须经协议层按流写出，直接写底层连接会串到别的流上
	if ev.RR.Req != nil && ev.RR.Req.H2 {
		ev.RR.Res.SetHeader("Content-Length", "0")
		_ = ev.WriteResponse(nil)
		return
	}

	ev.WriteResponseClose(nil)
}
