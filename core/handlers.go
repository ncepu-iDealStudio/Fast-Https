package core

import (
	"fast-https/config"
	"fmt"
	"slices"
	"strings"
)

// HandlerReady reports whether a location path type has a request handler.
func HandlerReady(pathType int) bool {
	h, ok := Handler(pathType)
	if !ok {
		return false
	}
	return h.RRHandler != nil
}

// MissingHandlers returns an error when a configured location type has no handler.
// TCP listeners are dispatched in the listen filter and are not checked here.
func MissingHandlers(servers []config.Server) error {
	var missing []string
	for _, srv := range servers {
		for _, path := range srv.Path {
			if path.PathType == config.PROXY_TCP {
				continue
			}
			if HandlerReady(int(path.PathType)) {
				continue
			}
			name := fmt.Sprintf("%s (%d)", PathTypeName(path.PathType), path.PathType)
			if slices.Contains(missing, name) {
				continue
			}
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("location handler not registered: %s", strings.Join(missing, ", "))
}

// PathTypeName returns the config name for a path type code.
func PathTypeName(pathType uint16) string {
	switch pathType {
	case config.LOCAL:
		return "local"
	case config.PROXY_HTTP:
		return "proxy-http"
	case config.PROXY_HTTPS:
		return "proxy-https"
	case config.PROXY_TCP:
		return "proxy-tcp"
	case config.REWRITE:
		return "rewrite"
	case config.DEVMOD:
		return "devmod"
	default:
		return fmt.Sprintf("type-%d", pathType)
	}
}
