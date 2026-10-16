package cmd

import (
	"fast-https/config"
	"fast-https/modules/auth"
	"fast-https/modules/core"
	"fast-https/modules/core/listener"
	"fast-https/modules/dev_mod"
	"fast-https/modules/proxy"
	"fast-https/modules/proxy_tcp"
	"fast-https/modules/rewrite"
	"fast-https/modules/safe"
	"fast-https/modules/static"
	"fast-https/modules/websocket"
	"strings"
)

// RegisterHandlers installs the production location handlers.
// start and dev call it before ServerInit. Importing a handler package
// no longer registers it.
func RegisterHandlers() {
	static.Register()
	proxy.Register()
	rewrite.Register()
	devmod.Register()
}

// RegisterServices installs connection filters, location limits, authentication,
// TCP proxy, WebSocket, and the post-listen security initialization.
// A nil hook inside core allows the event, so this must run before ServerInit.
func RegisterServices() {
	core.SetServices(core.Services{
		RejectConn:     rejectConn,
		HandleListen:   handleListen,
		RejectLocation: rejectLocation,
		RejectAuth:     rejectAuth,
		AfterListen:    safe.Init,
	})
}

func rejectConn(ev *core.Event) bool {
	if !safe.Bucket(ev) {
		return true
	}
	return safe.IsInBlacklist(ev)
}

func handleListen(ev *core.Event) bool {
	// Kept from the previous listen filter: it compares the listener type with
	// config.PROXY_TCP (3). Listener creation stores TCP as LisType 2, so this
	// branch does not run for a "tcp" listen. ProxyEventTcp itself is covered
	// by modules/proxy_tcp/event_proxy_tcp_test.go.
	if ev.LisInfo.LisType == config.PROXY_TCP {
		proxy_tcp.ProxyEventTcp(ev, ev.LisInfo.Cfg[0].ProxyAddr)
		return true
	}
	if ev.Upgrade == "websocket" {
		websocket.WebSocketHandler(ev)
		return true
	}
	return false
}

func rejectLocation(cfg *listener.ListenCfg, ev *core.Event) bool {
	cl := safe.CountLimitFor(cfg.ID)
	ip := ""
	index := strings.LastIndex(ev.Conn.RemoteAddr().String(), ":")
	if index != -1 {
		ip = ev.Conn.RemoteAddr().String()[:index]
	}
	if cl != nil && !cl.Insert(ip) {
		safe.CountHandler(ev.RR)
		return true
	}
	return false
}

func rejectAuth(cfg *listener.ListenCfg, ev *core.Event) bool {
	return !auth.AuthHandler(cfg, ev)
}
