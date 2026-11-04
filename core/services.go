package core

import "fast-https/runtime/listener"

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

var services Services

// SetServices replaces the process hooks. Call it before ServerInit.
func SetServices(next Services) {
	services = next
}

// RequestHandled reports whether the composed early-request hook consumed ev.
// It runs before location matching, so reserved paths do not depend on the
// site root or on a location rule that happens to cover them.
func RequestHandled(ev *Event) bool {
	if services.HandleEarlyRequest == nil {
		return false
	}
	return services.HandleEarlyRequest(ev)
}

// ConnRejected reports whether the composed connection hook refuses ev.
func ConnRejected(ev *Event) bool {
	if services.RejectConn == nil {
		return false
	}
	return services.RejectConn(ev)
}

// ListenHandled reports whether the composed listen hook consumed ev.
func ListenHandled(ev *Event) bool {
	if services.HandleListen == nil {
		return false
	}
	return services.HandleListen(ev)
}

// LocationRejected reports whether the composed location hook refuses ev.
func LocationRejected(cfg *listener.ListenCfg, ev *Event) bool {
	if services.RejectLocation == nil {
		return false
	}
	return services.RejectLocation(cfg, ev)
}

// AuthRejected reports whether the composed authentication hook refuses ev.
func AuthRejected(cfg *listener.ListenCfg, ev *Event) bool {
	if services.RejectAuth == nil {
		return false
	}
	return services.RejectAuth(cfg, ev)
}

// WAFRejected reports whether the composed application firewall refuses ev.
// The hook writes the refusal response itself.
func WAFRejected(cfg *listener.ListenCfg, ev *Event) bool {
	if services.RejectWAF == nil {
		return false
	}
	return services.RejectWAF(cfg, ev)
}

// RunAfterListen calls the composed post-listen hook when one is set.
func RunAfterListen() {
	if services.AfterListen != nil {
		services.AfterListen()
	}
}

// RunBeforeStop calls the composed pre-stop hook when one is set.
func RunBeforeStop() {
	if services.BeforeStop != nil {
		services.BeforeStop()
	}
}

// RunAfterStop calls the composed post-stop hook when one is set.
func RunAfterStop() {
	if services.AfterStop != nil {
		services.AfterStop()
	}
}
