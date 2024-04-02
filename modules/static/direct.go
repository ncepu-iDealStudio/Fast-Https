package static

import (
	"fast-https/modules/core"
)

// for static requests which not end with "/"
// attention: if backends use API interface, they
// must end with "/"
func event301(ev *core.Event, path string) {
	res := []byte("HTTP/1.1 301 Moved Permanently\r\n" +
		"Location: " + path + "\r\n" +
		"Connection: close\r\n" +
		"\r\n")
	ev.WriteDataClose(res)
}
