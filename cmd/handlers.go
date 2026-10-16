package cmd

import (
	"fast-https/modules/dev_mod"
	"fast-https/modules/proxy"
	"fast-https/modules/rewrite"
	"fast-https/modules/static"
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
