package certmanager

import (
	"context"
	"fast-https/config"
	"fast-https/core"
	"fast-https/runtime/listener"
	"testing"

	"github.com/mholt/acmez/v3/acme"
)

// acmeTestConfig 造一份启用 ACME 的配置，托管 example.com。
// 存储目录用临时目录，测试之间互不影响。
func acmeTestConfig(t *testing.T, mutate func(*config.AcmeConfig)) *config.Fast_Https {
	t.Helper()
	fallback := true
	cfg := &config.Fast_Https{
		Acme: &config.AcmeConfig{
			Enabled:            true,
			Email:              "ops@example.test",
			Challenge:          []string{config.ACMEChallengeHTTP01},
			RenewBeforeDays:    30,
			FallbackSelfSigned: &fallback,
			Storage:            t.TempDir(),
			CAs:                []config.ACMECA{{Name: "le", Directory: config.ACMELetsEncryptDirectory}},
		},
		Servers: []config.Server{{
			Listen:     "443 ssl",
			ServerName: "example.com",
		}},
	}
	if mutate != nil {
		mutate(cfg.Acme)
	}
	return cfg
}

// useConfig 换上新的配置快照，并在测试结束时停掉管理器、还原快照。
func useConfig(t *testing.T, cfg *config.Fast_Https) {
	t.Helper()
	previous := *config.Current()
	t.Cleanup(func() {
		Stop()
		config.Store(&previous, config.MIME())
	})
	config.Store(cfg, config.MIME())
}

// stubBackground 把后台申请换成计数，测试不出网。
func stubBackground(t *testing.T) *int {
	t.Helper()
	previous := startBackground
	t.Cleanup(func() { startBackground = previous })
	started := 0
	startBackground = func(*Manager) { started++ }
	return &started
}

func TestAcmeDiffReportsRuntimeChanges(t *testing.T) {
	base := acmeTestConfig(t, nil).Acme

	cases := []struct {
		name   string
		mutate func(*config.AcmeConfig)
		want   string
	}{
		{"same", func(*config.AcmeConfig) {}, ""},
		{"email", func(c *config.AcmeConfig) { c.Email = "other@example.test" }, "email"},
		{"staging", func(c *config.AcmeConfig) { c.Staging = true }, "staging"},
		{"renew", func(c *config.AcmeConfig) { c.RenewBeforeDays = 45 }, "renew_before_days"},
		{"storage", func(c *config.AcmeConfig) { c.Storage = "/tmp/other-acme" }, "storage"},
		{"challenge", func(c *config.AcmeConfig) {
			c.Challenge = []string{config.ACMEChallengeTLSALPN01}
		}, "challenge"},
		{"cas", func(c *config.AcmeConfig) { c.CAs = config.DefaultACMECAs() }, "cas"},
		// 回落自签只在 CLI 摘要与校验里用，运行期不读，不该触发重启。
		{"fallback is not runtime", func(c *config.AcmeConfig) {
			v := false
			c.FallbackSelfSigned = &v
		}, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			next := *base
			next.Challenge = append([]string(nil), base.Challenge...)
			next.CAs = append([]config.ACMECA(nil), base.CAs...)
			tc.mutate(&next)
			if got := acmeDiff(base, &next); got != tc.want {
				t.Fatalf("acmeDiff = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestSameChallengesExpandsAuto 确认 auto 与显式开启两种挑战等价：
// 只是换了写法，不该为此重建管理器。
func TestSameChallengesExpandsAuto(t *testing.T) {
	// 三份配置共用同一个 base：storage 每次 t.TempDir() 都不同，
	// 直接各造一份会让 acmeDiff 报 storage 而不是 challenge。
	base := acmeTestConfig(t, func(c *config.AcmeConfig) {
		c.Challenge = []string{config.ACMEChallengeAuto}
	}).Acme

	both := *base
	both.Challenge = []string{config.ACMEChallengeHTTP01, config.ACMEChallengeTLSALPN01}
	reversed := *base
	reversed.Challenge = []string{config.ACMEChallengeTLSALPN01, config.ACMEChallengeHTTP01}
	auto := base

	if diff := acmeDiff(auto, &both); diff != "" {
		t.Fatalf("auto vs explicit both = %q, want no diff", diff)
	}
	if diff := acmeDiff(&both, &reversed); diff != "" {
		t.Fatalf("challenge order = %q, want no diff", diff)
	}
}

func TestReconcileKeepsManagerWhenNothingChanged(t *testing.T) {
	useConfig(t, acmeTestConfig(t, nil))
	started := stubBackground(t)

	Reconcile()
	first := activeManager.Load()
	if first == nil {
		t.Fatal("manager should start for an enabled acme section")
	}
	if *started != 1 {
		t.Fatalf("background starts = %d, want 1", *started)
	}

	// 配置没变：不重建、不打断续期调度。
	Reconcile()
	if activeManager.Load() != first {
		t.Fatal("unchanged config must keep the same manager instance")
	}
	if *started != 1 {
		t.Fatalf("background starts = %d, want 1 (no restart)", *started)
	}
	if first.stopped.Load() {
		t.Fatal("unchanged config must not stop the manager")
	}
}

func TestReconcileRestartsOnRuntimeChange(t *testing.T) {
	cfg := acmeTestConfig(t, nil)
	useConfig(t, cfg)
	started := stubBackground(t)

	Reconcile()
	first := activeManager.Load()
	if first == nil {
		t.Fatal("manager should start")
	}

	next := *cfg
	acmeCfg := *cfg.Acme
	acmeCfg.RenewBeforeDays = 45
	next.Acme = &acmeCfg
	config.Store(&next, config.MIME())

	Reconcile()
	second := activeManager.Load()
	if second == nil || second == first {
		t.Fatal("changed renew_before_days must rebuild the manager")
	}
	if !first.stopped.Load() {
		t.Fatal("old manager should be stopped after a rebuild")
	}
	if *started != 2 {
		t.Fatalf("background starts = %d, want 2", *started)
	}
}

func TestReconcileRestartsOnlyWhenDomainSetChanges(t *testing.T) {
	cfg := acmeTestConfig(t, nil)
	useConfig(t, cfg)
	stubBackground(t)

	Reconcile()
	first := activeManager.Load()

	// 只是 server 块的排列顺序变了：域名集合不变，不该重建。
	reordered := *cfg
	reordered.Servers = []config.Server{
		{Listen: "80", ServerName: "plain.example"},
		{Listen: "443 ssl", ServerName: "example.com"},
	}
	config.Store(&reordered, config.MIME())
	Reconcile()
	if activeManager.Load() != first {
		t.Fatal("reordering server blocks must not rebuild the manager")
	}

	// 真的多了一个域名：必须重建，否则新域名永远不申请。
	grown := *cfg
	grown.Servers = []config.Server{
		{Listen: "443 ssl", ServerName: "example.com"},
		{Listen: "443 ssl", ServerName: "www.example.com"},
	}
	config.Store(&grown, config.MIME())
	Reconcile()
	second := activeManager.Load()
	if second == nil || second == first {
		t.Fatal("adding a domain must rebuild the manager")
	}
	if !first.stopped.Load() {
		t.Fatal("old manager should be stopped")
	}
	if len(second.names) != 2 {
		t.Fatalf("names = %v, want both domains", second.names)
	}
}

func TestReconcileStopsManagerWhenAcmeDisabled(t *testing.T) {
	cfg := acmeTestConfig(t, nil)
	useConfig(t, cfg)
	stubBackground(t)

	Reconcile()
	first := activeManager.Load()
	if first == nil {
		t.Fatal("manager should start")
	}

	next := *cfg
	acmeCfg := *cfg.Acme
	acmeCfg.Enabled = false
	next.Acme = &acmeCfg
	config.Store(&next, config.MIME())

	Reconcile()
	if activeManager.Load() != nil {
		t.Fatal("disabling acme must leave no active manager")
	}
	if !first.stopped.Load() {
		t.Fatal("disabled acme should stop the running manager")
	}
}

// TestReconcileKeepsPreviousManagerWhenNewConfigUnusable 守住「配置写坏别动旧的」：
// 新配置构造不出管理器时，原来的续期链路必须继续跑。
func TestReconcileKeepsPreviousManagerWhenNewConfigUnusable(t *testing.T) {
	cfg := acmeTestConfig(t, nil)
	useConfig(t, cfg)
	stubBackground(t)

	Reconcile()
	first := activeManager.Load()
	if first == nil {
		t.Fatal("manager should start")
	}

	next := *cfg
	acmeCfg := *cfg.Acme
	acmeCfg.Challenge = []string{"bogus"}
	next.Acme = &acmeCfg
	config.Store(&next, config.MIME())

	Reconcile()
	if activeManager.Load() != first {
		t.Fatal("unusable config must keep the running manager")
	}
	if first.stopped.Load() {
		t.Fatal("running manager must not be stopped when the new config is unusable")
	}
}

// TestReconcileHooksFollowConfig 覆盖 reload 挂/撤钩子：
// 关闭 ACME 后挑战路径必须回到路由（否则请求热路径永远被截），
// 打开 tls-alpn-01 后 provider 必须就位（否则端口不会通告 acme-tls/1）。
func TestReconcileHooksFollowConfig(t *testing.T) {
	previous := *config.Current()
	t.Cleanup(func() {
		Stop()
		core.SetEarlyRequestHandler(nil)
		listener.RegisterTLSALPNProvider(nil)
		config.Store(&previous, config.MIME())
	})
	stubBackground(t)

	// 1. 启用 + http-01：early-request 钩子挂上，挑战能被应答。
	on := acmeTestConfig(t, nil)
	config.Store(on, config.MIME())
	ReconcileHooks()
	Reconcile()

	manager := activeManager.Load()
	if manager == nil || manager.http == nil {
		t.Fatal("http-01 solver should exist")
	}
	if err := manager.http.Present(context.Background(), acme.Challenge{
		Token:            "tok",
		KeyAuthorization: "tok.keyauth",
	}); err != nil {
		t.Fatalf("present: %v", err)
	}
	if !core.RequestHandled(challengeEvent("/.well-known/acme-challenge/tok")) {
		t.Fatal("http-01 challenge should be answered while acme is enabled")
	}
	if listener.TLSALPNEnabled() {
		t.Fatal("http-01 only must not advertise acme-tls/1")
	}

	// 2. 关闭 ACME：钩子必须撤掉，即使旧管理器还在内存里。
	off := acmeTestConfig(t, func(c *config.AcmeConfig) { c.Enabled = false })
	config.Store(off, config.MIME())
	ReconcileHooks()
	if core.RequestHandled(challengeEvent("/.well-known/acme-challenge/tok")) {
		t.Fatal("challenge path must fall back to routing once acme is disabled")
	}
	if listener.TLSALPNEnabled() {
		t.Fatal("disabled acme must not advertise acme-tls/1")
	}

	// 3. 改成 tls-alpn-01：provider 就位，监听器重建时才会通告 acme-tls/1。
	alpnCfg := acmeTestConfig(t, func(c *config.AcmeConfig) {
		c.Challenge = []string{config.ACMEChallengeTLSALPN01}
	})
	config.Store(alpnCfg, config.MIME())
	ReconcileHooks()
	if !listener.TLSALPNEnabled() {
		t.Fatal("tls-alpn-01 should register the challenge certificate provider")
	}
}
