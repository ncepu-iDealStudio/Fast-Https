package core

import (
	"fast-https/runtime/listener"
	"sync/atomic"
)

// Services holds optional process hooks. The composition layer fills them
// before the server listens. A nil function allows the event or does nothing,
// so tests can run the core without the feature packages.
type Services struct {
	// HandleEarlyRequest handles reserved paths (ACME http-01 challenges, for
	// example) before location matching. The function writes the response
	// itself. Return true when the request must not enter routing.
	HandleEarlyRequest func(*Event) bool
	// RejectConn refuses a new connection, for example by blacklist or rate limit.
	RejectConn func(*Event) bool
	// HandleListen fully handles a connection outside HTTP, such as TCP proxy
	// or WebSocket. Return true when the connection should leave the HTTP loop.
	HandleListen func(*Event) bool
	// RejectLocation refuses a matched location, for example by a request limit.
	// The function writes the refusal response itself.
	RejectLocation func(*listener.ListenCfg, *Event) bool
	// RejectAuth refuses a request that fails authentication and writes the response.
	RejectAuth func(*listener.ListenCfg, *Event) bool
	// RejectWAF refuses a request flagged by the application firewall and
	// writes the refusal response itself. Nil allows the request.
	RejectWAF func(*listener.ListenCfg, *Event) bool
	// BeforeListenReload runs after the config snapshot is replaced and before
	// listeners are reloaded. Hooks that shape listener construction (the ACME
	// acme-tls/1 provider, for example) must land here: listenSsl reads them
	// while building tls.Config, so registering them afterwards is too late.
	BeforeListenReload func()
	// AfterListen runs once listeners exist, including after a reload.
	// Implementations must be idempotent: reload calls it again.
	AfterListen func()
	// BeforeStop runs once when a stop signal arrives, before listeners stop.
	// Use it to stop accepting new work or to flush buffered state.
	BeforeStop func()
	// AfterStop runs once after in-flight handlers drained and before the
	// process exits. Use it to release files, sockets and background workers.
	AfterStop func()
}

// services 用原子指针整体替换：reload 会改 HandleEarlyRequest（ACME 开关），
// 请求路径同时在读，逐字段赋值会产生撕裂读。
var services atomic.Pointer[Services]

// SetServices replaces the process hooks. Call it before ServerInit.
func SetServices(next Services) {
	services.Store(&next)
}

// current 返回当前生效的钩子集合。未设置时返回零值（所有钩子为 nil）。
func current() Services {
	if s := services.Load(); s != nil {
		return *s
	}
	return Services{}
}

// SetEarlyRequestHandler 只替换 early-request 钩子，其余钩子保持不变。
// reload 用它挂上或撤销 ACME 的 http-01 应答：acme 段关闭时必须能置 nil，
// 否则挑战路径会一直留在请求热路径上。
func SetEarlyRequestHandler(fn func(*Event) bool) {
	for {
		old := services.Load()
		next := Services{}
		if old != nil {
			next = *old
		}
		next.HandleEarlyRequest = fn
		if services.CompareAndSwap(old, &next) {
			return
		}
	}
}

// RequestHandled reports whether the composed early-request hook consumed ev.
// It runs before location matching, so reserved paths do not depend on the
// site root or on a location rule that happens to cover them.
func RequestHandled(ev *Event) bool {
	fn := current().HandleEarlyRequest
	if fn == nil {
		return false
	}
	return fn(ev)
}

// ConnRejected reports whether the composed connection hook refuses ev.
func ConnRejected(ev *Event) bool {
	fn := current().RejectConn
	if fn == nil {
		return false
	}
	return fn(ev)
}

// ListenHandled reports whether the composed listen hook consumed ev.
func ListenHandled(ev *Event) bool {
	fn := current().HandleListen
	if fn == nil {
		return false
	}
	return fn(ev)
}

// LocationRejected reports whether the composed location hook refuses ev.
func LocationRejected(cfg *listener.ListenCfg, ev *Event) bool {
	fn := current().RejectLocation
	if fn == nil {
		return false
	}
	return fn(cfg, ev)
}

// AuthRejected reports whether the composed authentication hook refuses ev.
func AuthRejected(cfg *listener.ListenCfg, ev *Event) bool {
	fn := current().RejectAuth
	if fn == nil {
		return false
	}
	return fn(cfg, ev)
}

// WAFRejected reports whether the composed application firewall refuses ev.
// The hook writes the refusal response itself.
func WAFRejected(cfg *listener.ListenCfg, ev *Event) bool {
	fn := current().RejectWAF
	if fn == nil {
		return false
	}
	return fn(cfg, ev)
}

// RunBeforeListenReload calls the pre-listener-reload hook when one is set.
func RunBeforeListenReload() {
	if fn := current().BeforeListenReload; fn != nil {
		fn()
	}
}

// RunAfterListen calls the composed post-listen hook when one is set.
func RunAfterListen() {
	if fn := current().AfterListen; fn != nil {
		fn()
	}
}

// RunBeforeStop calls the composed pre-stop hook when one is set.
func RunBeforeStop() {
	if fn := current().BeforeStop; fn != nil {
		fn()
	}
}

// RunAfterStop calls the composed post-stop hook when one is set.
func RunAfterStop() {
	if fn := current().AfterStop; fn != nil {
		fn()
	}
}
