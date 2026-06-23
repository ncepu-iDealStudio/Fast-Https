package listener_test

import (
	"context"
	"fast-https/config"
	"fast-https/modules/core/listener"
	"testing"
	"time"
)

// setupTestConfig sets up config.GConfig.Servers with the given servers
// and returns a cleanup function to restore the original config.
func setupTestConfig(servers []config.Server) func() {
	origServers := config.GConfig.Servers
	config.GConfig.Servers = servers
	return func() {
		config.GConfig.Servers = origServers
	}
}

// setupTestGLisinfos sets up listener.GLisinfos and returns a cleanup function.
func setupTestGLisinfos(lis []listener.Listener) func() {
	orig := listener.GLisinfos
	listener.GLisinfos = lis
	return func() {
		listener.GLisinfos = orig
	}
}

// assertCtxPreserved verifies that newCtx is the same as oldCtx by checking
// that cancelling oldCancel causes newCtx.Done() to fire. Since context.Context
// is an interface, we can compare with !=, but CancelFunc (func()) cannot be
// compared directly, so we use behavioral verification.
func assertCtxPreserved(t *testing.T, newCtx context.Context, oldCancel context.CancelFunc) {
	t.Helper()
	if newCtx == nil {
		t.Fatal("new Ctx should not be nil")
	}
	// If newCtx == oldCtx, calling oldCancel should cancel newCtx
	oldCancel()
	select {
	case <-newCtx.Done():
		// expected: newCtx was cancelled by oldCancel, so they share the same context
	case <-time.After(100 * time.Millisecond):
		t.Fatal("oldCancel did not cancel newCtx, Ctx was not preserved")
	}
}

// TestReloadCommonPortPreservesCtxAndCancel verifies that after reload,
// a common port (exists in both old and new config with same LisType)
// preserves its Ctx/Cancel from the old listener.
func TestReloadCommonPortPreservesCtxAndCancel(t *testing.T) {
	cleanupCfg := setupTestConfig([]config.Server{
		{
			Listen:     "18080",
			ServerName: "localhost",
			Path: []config.Path{
				{PathName: "/", PathType: config.LOCAL, Root: "."},
			},
		},
	})
	defer cleanupCfg()

	oldCtx, oldCancel := context.WithCancel(context.Background())
	cleanupLis := setupTestGLisinfos([]listener.Listener{
		{
			Port:    "18080",
			LisType: 0,
			Ctx:     oldCtx,
			Cancel:  oldCancel,
			Cfg:     []listener.ListenCfg{{ID: 1, Path: "/"}},
			HostMap: make(map[string][]listener.ListenCfg),
		},
	})
	defer cleanupLis()

	newAll, added, removed := listener.ReloadListenCfg()

	if len(added) != 0 {
		t.Errorf("expected 0 added ports, got %d", len(added))
	}
	if len(removed) != 0 {
		t.Errorf("expected 0 removed ports, got %d", len(removed))
	}
	if len(newAll) != 1 {
		t.Fatalf("expected 1 listener, got %d", len(newAll))
	}

	// Verify Ctx/Cancel are preserved: calling oldCancel should cancel newAll[0].Ctx
	if newAll[0].Ctx != oldCtx {
		t.Error("common port should preserve old Ctx (interface comparison)")
	}
	assertCtxPreserved(t, newAll[0].Ctx, oldCancel)
}

// TestReloadCommonPortUpdatesCfg verifies that after reload, a common
// port's Cfg is updated to reflect the new configuration.
func TestReloadCommonPortUpdatesCfg(t *testing.T) {
	cleanupCfg := setupTestConfig([]config.Server{
		{
			Listen:     "18080",
			ServerName: "localhost",
			Path: []config.Path{
				{PathName: "/", PathType: config.LOCAL, Root: "./old-root"},
			},
		},
	})
	defer cleanupCfg()

	oldCtx, oldCancel := context.WithCancel(context.Background())
	cleanupLis := setupTestGLisinfos([]listener.Listener{
		{
			Port:    "18080",
			LisType: 0,
			Ctx:     oldCtx,
			Cancel:  oldCancel,
			Cfg:     []listener.ListenCfg{{ID: 1, Path: "/", StaticRoot: "./old-root"}},
			HostMap: map[string][]listener.ListenCfg{
				"localhost:18080": {{Path: "/"}},
			},
		},
	})
	defer cleanupLis()

	listener.ReloadListenCfg()

	if len(listener.GLisinfos) != 1 {
		t.Fatalf("expected 1 listener in GLisinfos, got %d", len(listener.GLisinfos))
	}

	updatedLis := listener.GLisinfos[0]
	if len(updatedLis.Cfg) == 0 {
		t.Fatal("Cfg should not be empty after reload")
	}
	if updatedLis.Cfg[0].StaticRoot != "./old-root" {
		t.Errorf("StaticRoot = %q, want %q", updatedLis.Cfg[0].StaticRoot, "./old-root")
	}
}

// TestReloadCommonPortUpdatesCfgAfterConfigChange verifies that changing
// the config (e.g., changing root path) and reloading updates the Cfg
// while preserving Ctx/Cancel.
func TestReloadCommonPortUpdatesCfgAfterConfigChange(t *testing.T) {
	cleanupCfg := setupTestConfig([]config.Server{
		{
			Listen:     "18080",
			ServerName: "localhost",
			Path: []config.Path{
				{PathName: "/", PathType: config.LOCAL, Root: "./old-root"},
			},
		},
	})
	defer cleanupCfg()

	oldCtx, oldCancel := context.WithCancel(context.Background())
	cleanupLis := setupTestGLisinfos([]listener.Listener{
		{
			Port:    "18080",
			LisType: 0,
			Ctx:     oldCtx,
			Cancel:  oldCancel,
			Cfg:     []listener.ListenCfg{{ID: 1, Path: "/", StaticRoot: "./old-root"}},
			HostMap: map[string][]listener.ListenCfg{
				"localhost:18080": {{Path: "/"}},
			},
		},
	})
	defer cleanupLis()

	// Change config: update root path
	config.GConfig.Servers[0].Path[0].Root = "./new-root"

	newAll, _, _ := listener.ReloadListenCfg()

	if len(newAll) != 1 {
		t.Fatalf("expected 1 listener, got %d", len(newAll))
	}

	// Verify Cfg reflects new root
	if newAll[0].Cfg[0].StaticRoot != "./new-root" {
		t.Errorf("StaticRoot = %q, want %q", newAll[0].Cfg[0].StaticRoot, "./new-root")
	}

	// Verify Ctx/Cancel still preserved (calling oldCancel cancels newAll[0].Ctx)
	assertCtxPreserved(t, newAll[0].Ctx, oldCancel)
}

// TestReloadAddedPortGetsNewCtx verifies that newly added ports get
// their own new Ctx/Cancel (not nil).
//
// Note: This test is skipped because added ports trigger listenTcp/listenSsl
// which requires the message system to be initialized. The added-port path
// is covered by the E2E reload test (test/client_test/reload_e2e_test.go).
func TestReloadAddedPortGetsNewCtx(t *testing.T) {
	t.Skip("added ports trigger real listenTcp/listenSsl which requires message system init; covered by E2E test")
}

// TestReloadRemovedPortNotInResult verifies that removed ports don't
// appear in the new listener list.
//
// Note: Skipped for the same reason as TestReloadAddedPortGetsNewCtx -
// removed ports always coexist with added ports in reload scenarios.
func TestReloadRemovedPortNotInResult(t *testing.T) {
	t.Skip("removed ports test requires added ports which trigger real listen; covered by E2E test")
}
