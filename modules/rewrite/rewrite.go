package rewrite

import (
	"fast-https/config"
	"fast-https/modules/core"
	"fast-https/modules/core/listener"
)

func init() {
	core.RRHandlerRegister(config.REWRITE, ReWriteFliter, ReWriteEvent)
}

// for static requests which not end with "/"
// attention: if backends use API interface, they
// must end with "/"
func rewriteInfo(ev *core.Event, path string) {
	res := []byte("HTTP/1.1 301 Moved Permanently\r\n" +
		"Location: " + path + "\r\n" +
		"Connection: close\r\n" +
		"\r\n")
	ev.WriteDataClose(res)
}

func ReWriteFliter(cfg listener.ListenCfg, ev *core.Event) bool {
	return true
}

// handle static events
// if requests want to keep-alive, we use write bytes,
// if Content-Type is close, we write bytes and close this connection
// Recursion "Handle_event" isn't a problem, because it
// will pause when TCP buffer is None.
func ReWriteEvent(cfg listener.ListenCfg, ev *core.Event) {
	// path := ev.RR.Req_.Path[ev.RR.PathLocation[0]:ev.RR.PathLocation[1]]
	// fmt.Println(path, cfg.ReWrite)
	rewriteInfo(ev, cfg.ReWrite)
}
