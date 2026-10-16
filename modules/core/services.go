package core

import "fast-https/modules/core/listener"

// Services holds optional process hooks. The composition layer fills them
// before the server listens. A nil function allows the event or does nothing,
// so tests can run the core without the feature packages.
type Services struct {
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
	// AfterListen runs once listeners exist, including after a reload.
	AfterListen func()
}

var services Services

// SetServices replaces the process hooks. Call it before ServerInit.
func SetServices(next Services) {
	services = next
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

// RunAfterListen calls the composed post-listen hook when one is set.
func RunAfterListen() {
	if services.AfterListen != nil {
		services.AfterListen()
	}
}
