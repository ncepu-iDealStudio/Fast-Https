package cmd_test

import (
	"fast-https/cmd"
	"fast-https/config"
	"fast-https/modules/core"
	"strings"
	"testing"
)

func TestRegisterHandlersCoversConfiguredTypes(t *testing.T) {
	types := []struct {
		code uint16
		name string
	}{
		{config.LOCAL, "local"},
		{config.PROXY_HTTP, "proxy-http"},
		{config.PROXY_HTTPS, "proxy-https"},
		{config.REWRITE, "rewrite"},
		{config.DEVMOD, "devmod"},
	}
	for _, item := range types {
		core.RRHandlerRegister(int(item.code), nil, nil, nil)
	}
	t.Cleanup(cmd.RegisterHandlers)

	paths := make([]config.Path, len(types))
	for i, item := range types {
		paths[i].PathType = item.code
	}
	servers := []config.Server{{Path: paths}}

	err := core.MissingHandlers(servers)
	if err == nil {
		t.Fatal("unregistered location types should be reported before listening")
	}
	for _, item := range types {
		if !strings.Contains(err.Error(), item.name) {
			t.Errorf("error %q should name %s", err, item.name)
		}
	}

	cmd.RegisterHandlers()
	if err := core.MissingHandlers(servers); err != nil {
		t.Fatal(err)
	}
	for _, item := range types {
		if !core.HandlerReady(int(item.code)) {
			t.Errorf("%s handler is not ready after RegisterHandlers", item.name)
		}
	}
}
