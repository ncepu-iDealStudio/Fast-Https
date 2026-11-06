package listener

import (
	"context"
	"crypto/tls"
	"fast-https/config"
	"os"
	"testing"
)

// TestALPNOnlyAdvertisedForH2Listener 确认只有 listen 含 h2 的端口才通告 ALPN "h2"。
// 否则纯 ssl 端口会被浏览器协商成 h2，却由 HTTP/1.1 解析器处理。
func TestALPNOnlyAdvertisedForH2Listener(t *testing.T) {
	dir := t.TempDir()
	crt, key := writeSelfSigned(t, dir, "a.example")
	cfg := []ListenCfg{{
		ServerName: "a.example",
		SSL:        SSLkv{SslKey: crt, SslValue: key},
	}}

	cases := []struct {
		name string
		h2   bool
		want string
	}{
		{"listen 443 ssl", false, ""},
		{"listen 443 ssl h2", true, "h2"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ln := listenSsl("127.0.0.1:0", newCertSet(), cfg, tc.h2, false)
			t.Cleanup(func() { _ = ln.Close() })
			go acceptUntilClose(ln)

			conn, err := tls.Dial("tcp", ln.Addr().String(), &tls.Config{
				InsecureSkipVerify: true, // 自签名测试证书
				ServerName:         "a.example",
				NextProtos:         []string{"h2", "http/1.1"},
			})
			if err != nil {
				t.Fatalf("tls dial: %v", err)
			}
			defer conn.Close()

			if got := conn.ConnectionState().NegotiatedProtocol; got != tc.want {
				t.Fatalf("negotiated protocol = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestTLSALPNChallengeCertServedForACMEProto 覆盖 TLS-ALPN-01 的握手分支：
// 客户端点名 acme-tls/1 且 SNI 命中时返回挑战证书，否则返回站点证书。
func TestTLSALPNChallengeCertServedForACMEProto(t *testing.T) {
	dir := t.TempDir()
	crt, key := writeSelfSigned(t, dir, "site.example")
	challengeCrt, challengeKey := writeSelfSigned(t, dir, "challenge.example")
	challenge, err := tls.LoadX509KeyPair(challengeCrt, challengeKey)
	if err != nil {
		t.Fatal(err)
	}

	RegisterTLSALPNProvider(func(hello *tls.ClientHelloInfo) (*tls.Certificate, bool) {
		if hello.ServerName != "challenge.example" {
			return nil, false
		}
		return &challenge, true
	})
	t.Cleanup(func() {
		RegisterTLSALPNProvider(nil)
		if TLSALPNEnabled() {
			t.Fatal("provider should be unregistered")
		}
	})

	cfg := []ListenCfg{{ServerName: "site.example", SSL: SSLkv{SslKey: crt, SslValue: key}}}
	ln := listenSsl("127.0.0.1:0", newCertSet(), cfg, false, false)
	t.Cleanup(func() { _ = ln.Close() })
	go acceptUntilClose(ln)

	// 挑战握手：必须拿到临时证书，而不是站点证书。
	if got := peerCNWithProtos(t, ln.Addr().String(), "challenge.example", []string{ACMEALPNProto}); got != "challenge.example" {
		t.Fatalf("challenge certificate = %q, want challenge.example", got)
	}

	// 同一端口的普通握手不受影响。
	if got := peerCN(t, ln.Addr().String(), "site.example"); got != "site.example" {
		t.Fatalf("normal certificate = %q, want site.example", got)
	}

	// 客户端没带 acme-tls/1 时，即使 SNI 命中挑战域名也回落站点证书。
	if got := peerCN(t, ln.Addr().String(), "challenge.example"); got != "site.example" {
		t.Fatalf("fallback certificate = %q, want site.example", got)
	}

	// SNI 未命中挑战表时回落站点证书。
	if got := peerCNWithProtos(t, ln.Addr().String(), "other.example", []string{ACMEALPNProto}); got != "site.example" {
		t.Fatalf("unknown SNI certificate = %q, want site.example", got)
	}
}

// TestACMEProtoAdvertisedOnlyWhenProviderRegistered 确认 acme-tls/1 只在
// 注册了挑战证书来源之后才出现在 ALPN 通告里。
func TestACMEProtoAdvertisedOnlyWhenProviderRegistered(t *testing.T) {
	dir := t.TempDir()
	crt, key := writeSelfSigned(t, dir, "site.example")
	cfg := []ListenCfg{{ServerName: "site.example", SSL: SSLkv{SslKey: crt, SslValue: key}}}

	ln := listenSsl("127.0.0.1:0", newCertSet(), cfg, false, false)
	t.Cleanup(func() { _ = ln.Close() })
	go acceptUntilClose(ln)

	if got := negotiateProto(t, ln.Addr().String(), []string{ACMEALPNProto}); got != "" {
		t.Fatalf("acme-tls/1 advertised without provider: %q", got)
	}

	cert, err := tls.LoadX509KeyPair(crt, key)
	if err != nil {
		t.Fatal(err)
	}
	RegisterTLSALPNProvider(func(*tls.ClientHelloInfo) (*tls.Certificate, bool) { return &cert, true })
	t.Cleanup(func() { RegisterTLSALPNProvider(nil) })

	ln2 := listenSsl("127.0.0.1:0", newCertSet(), cfg, false, false)
	t.Cleanup(func() { _ = ln2.Close() })
	go acceptUntilClose(ln2)

	if got := negotiateProto(t, ln2.Addr().String(), []string{ACMEALPNProto}); got != ACMEALPNProto {
		t.Fatalf("negotiated protocol = %q, want %q", got, ACMEALPNProto)
	}
}

// TestNeedsRebuildWhenALPNAdvertisementFlips 覆盖 reload 的重建判定。
// tls.Config 的 NextProtos 建好后改不了，所以 acme-tls/1 的通告位一变，
// 这个端口就必须换监听器，而不是只热更新配置。
func TestNeedsRebuildWhenALPNAdvertisementFlips(t *testing.T) {
	t.Cleanup(func() { RegisterTLSALPNProvider(nil) })
	RegisterTLSALPNProvider(nil)

	before := &Listener{LisType: 1, Port: "18443", alpnACMESnapshot: TLSALPNEnabled()}
	if before.needsRebuild(&Listener{LisType: 1, Port: "18443"}) {
		t.Fatal("same listen type and same ALPN advertisement should reuse the listener")
	}

	// ACME 打开 tls-alpn-01：provider 就位，通告位从此不同。
	RegisterTLSALPNProvider(func(*tls.ClientHelloInfo) (*tls.Certificate, bool) { return nil, false })
	if !before.needsRebuild(&Listener{LisType: 1, Port: "18443"}) {
		t.Fatal("flipped acme-tls/1 advertisement must rebuild the TLS listener")
	}

	// 纯 HTTP 端口没有 NextProtos 可换：ACME 打开 tls-alpn-01 不该牵连 80。
	if (&Listener{LisType: 0}).needsRebuild(&Listener{LisType: 0}) {
		t.Fatal("plain HTTP listener must not rebuild when acme-tls/1 is enabled")
	}

	// 监听类型变化一直都要重建。
	if !(&Listener{LisType: 0}).needsRebuild(&Listener{LisType: 1}) {
		t.Fatal("listen type change must rebuild the listener")
	}
}

// TestReloadKeepsListenerWhenALPNUnchanged 确认通告位没变时沿用旧 *Listener：
// reload 不该顺手重建 TLS 监听器（重建会让端口短暂不可用）。
func TestReloadKeepsListenerWhenALPNUnchanged(t *testing.T) {
	origServers := config.Current().Servers
	t.Cleanup(func() { config.Current().Servers = origServers })
	config.Current().Servers = []config.Server{{
		Listen:     "18443 ssl",
		ServerName: "a.example",
		Path:       []config.Path{{PathName: "/", PathType: config.LOCAL, Root: "."}},
	}}

	origListeners := GLisinfos
	t.Cleanup(func() { setListeners(origListeners) })

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	old := &Listener{
		Port:             "18443",
		LisType:          1,
		Ctx:              ctx,
		Cancel:           cancel,
		alpnACMESnapshot: TLSALPNEnabled(),
	}
	setListeners([]*Listener{old})

	all, added, removed := ReloadListenCfg()
	if len(added) != 0 || len(removed) != 0 {
		t.Fatalf("added=%d removed=%d, want 0/0", len(added), len(removed))
	}
	if len(all) != 1 || all[0] != old {
		t.Fatal("unchanged ALPN advertisement should reuse the existing listener")
	}
}

// TestReloadCertsPicksUpOverwrittenPEM 模拟控制面覆盖 PEM 后热替换：
// 同一个监听地址必须换成新证书，且失败时保留旧证书。
func TestReloadCertsPicksUpOverwrittenPEM(t *testing.T) {
	dir := t.TempDir()
	crt, key := writeSelfSigned(t, dir, "a.example")
	cfg := []ListenCfg{{ServerName: "a.example", SSL: SSLkv{SslKey: crt, SslValue: key}}}

	lis := &Listener{Port: "0", LisType: 1, Cfg: cfg}
	lis.certs = newCertSet()
	lis.publish()
	ln := listenSsl("127.0.0.1:0", lis.certs, cfg, false, false)
	t.Cleanup(func() { _ = ln.Close() })
	go acceptUntilClose(ln)

	previous := GLisinfos
	GLisinfos = []*Listener{lis}
	t.Cleanup(func() { GLisinfos = previous })

	if got := peerCN(t, ln.Addr().String(), "a.example"); got != "a.example" {
		t.Fatalf("initial certificate = %q", got)
	}

	// 覆盖同一路径的 PEM（真实签发路径就是这样落盘），再触发热替换。
	replacementCrt, replacementKey := writeSelfSigned(t, dir, "b.example")
	copyFile(t, replacementCrt, crt)
	copyFile(t, replacementKey, key)

	if err := ReloadCerts(); err != nil {
		t.Fatalf("reload certs: %v", err)
	}
	if got := peerCN(t, ln.Addr().String(), "b.example"); got != "b.example" {
		t.Fatalf("certificate after reload = %q", got)
	}

	// 文件被删掉后热替换失败，监听器继续用旧证书。
	if err := os.Remove(crt); err != nil {
		t.Fatal(err)
	}
	if err := ReloadCerts(); err == nil {
		t.Fatal("missing PEM should be reported")
	}
	if got := peerCN(t, ln.Addr().String(), "b.example"); got != "b.example" {
		t.Fatalf("failed reload changed certificate to %q", got)
	}
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func peerCNWithProtos(t *testing.T, addr, serverName string, protos []string) string {
	t.Helper()
	conn, err := tls.Dial("tcp", addr, &tls.Config{
		InsecureSkipVerify: true, // 自签测试证书
		ServerName:         serverName,
		NextProtos:         protos,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	state := conn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		t.Fatal("no peer certificate")
	}
	return state.PeerCertificates[0].Subject.CommonName
}

func negotiateProto(t *testing.T, addr string, protos []string) string {
	t.Helper()
	conn, err := tls.Dial("tcp", addr, &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         "site.example",
		NextProtos:         protos,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	return conn.ConnectionState().NegotiatedProtocol
}
