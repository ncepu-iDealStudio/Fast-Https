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

	ev.WriteResponseClose(nil)
}
