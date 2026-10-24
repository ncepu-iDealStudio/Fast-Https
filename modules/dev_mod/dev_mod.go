// Package devmod 提供 dev 命令使用的开发模式处理器。
package devmod

import (
	"fast-https/config"
	"fast-https/core"
	"fast-https/runtime/listener"
)

// Register installs the development handler for location type devmod.
func Register() {
	core.RegisterHandler(config.DEVMOD, DevFilter, DevEvent, nil)
}

func rewriteInfo(ev *core.Event, path string) {
	ev.RR.Res.SetFirstLine(200, "OK")
	ev.RR.Res.SetHeader("Connection", "close")
	ev.RR.Res.Body = []byte("this is dev mod")
	ev.WriteResponseClose(nil)
}

/*
 *************************************
 ****** Interfaces are as follows ****
 *************************************
 */

func DevFilter(cfg *listener.ListenCfg, ev *core.Event) bool {
	return true
}

func DevEvent(cfg *listener.ListenCfg, ev *core.Event) {
	// path := ev.RR.Req_.Path[ev.RR.PathLocation[0]:ev.RR.PathLocation[1]]
	// fmt.Println(path, cfg.ReWrite)
	rewriteInfo(ev, cfg.ReWrite)
}
