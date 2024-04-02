package static

import (
	"fast-https/modules/core"
)

// for static requests which not end with "/"
// attention: if backends use API interface, they
// must end with "/"
func event301(ev *core.Event, path string) {

	ev.RR.Res_.SetFirstLine(301, "Moved Permanently")
	ev.RR.Res_.SetHeader("Location", path)
	ev.RR.Res_.SetHeader("Connection", "close")

	ev.WriteResponseClose(nil)
}
