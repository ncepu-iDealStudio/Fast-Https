package rewrite

import (
	"fast-https/config"
	"fast-https/modules/core"
	"fast-https/modules/core/listener"
)

func init() {
	core.RRHandlerRegister(config.REWRITE, ReWriteFilter, ReWriteEvent, nil)
}

func rewriteInfo(ev *core.Event, path string) {
	//res := []byte("HTTP/1.1 301 Moved Permanently\r\n" +
	//	"Location: " + path + "\r\n" +
	//	"Connection: close\r\n" +
	//	"\r\n")
	ev.RR.Res.SetFirstLine(301, "Moved Permanently")
	ev.RR.Res.SetHeader("Location", path)
	ev.RR.Res.SetHeader("Connection", "close")
	ev.WriteResponseClose(nil)
}

/*
 *************************************
 ****** Interfaces are as follows ****
 *************************************
 */

func ReWriteFilter(cfg *listener.ListenCfg, ev *core.Event) bool {
	return true
}

func ReWriteEvent(cfg *listener.ListenCfg, ev *core.Event) {
	// path := ev.RR.Req_.Path[ev.RR.PathLocation[0]:ev.RR.PathLocation[1]]
	// fmt.Println(path, cfg.ReWrite)
	rewriteInfo(ev, cfg.ReWrite)
}
