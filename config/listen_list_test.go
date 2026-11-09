package config

import (
	"os"
	"path/filepath"
	"testing"
)

// 一个站点文件写多个监听：加载后展开成多个运行期 server 块，
// server_name、location、证书共用，端口各自独立。
func TestIncludeServerMultipleListensExpands(t *testing.T) {
	dir := t.TempDir()
	site := `{
  "listen": ["80", "443 ssl h2"],
  "server_name": "multi.local",
  "ssl_certificate": "multi.pem",
  "ssl_certificate_key": "multi-key.pem",
  "location": [{ "url": "/api", "type": "proxy", "proxy_pass": "http://127.0.0.1:9000" }]
}`
	if err := os.WriteFile(filepath.Join(dir, "multi.json"), []byte(site), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &Fast_Https{Include: []string{dir}}
	if err := processIncludeCfg(cfg); err != nil {
		t.Fatalf("processIncludeCfg: %v", err)
	}
	if len(cfg.Servers) != 2 {
		t.Fatalf("servers = %d, want 2", len(cfg.Servers))
	}

	listens := map[string]Server{}
	for _, server := range cfg.Servers {
		listens[server.Listen] = server
		if server.ServerName != "multi.local" {
			t.Fatalf("server_name = %q", server.ServerName)
		}
		if server.SSLCertificate != "multi.pem" || server.SSLCertificateKey != "multi-key.pem" {
			t.Fatalf("cert = %q / %q", server.SSLCertificate, server.SSLCertificateKey)
		}
		if len(server.Path) != 1 || server.Path[0].PathName != "/api" || server.Path[0].ProxyData != "127.0.0.1:9000" {
			t.Fatalf("location = %+v", server.Path)
		}
	}
	if _, ok := listens["80"]; !ok {
		t.Fatal("missing plain 80 listener")
	}
	if _, ok := listens["443 ssl h2"]; !ok {
		t.Fatal("missing tls listener")
	}

	// 展开出来的 server 之间不能共享 Path 底层数组：
	// 改写其中一个的 root 不应影响另一个。
	first, second := cfg.Servers[0], cfg.Servers[1]
	first.Path[0].Root = "changed"
	if second.Path[0].Root == "changed" {
		t.Fatal("expanded servers share the same Path backing array")
	}
}

func TestListenAcceptsScalarNumberStringAndArray(t *testing.T) {
	cases := []struct {
		raw  string
		want []string
	}{
		{raw: `{"listen": 80}`, want: []string{"80"}},
		{raw: `{"listen": "443 ssl h2"}`, want: []string{"443 ssl h2"}},
		{raw: `{"listen": ["80", "443 ssl h2"]}`, want: []string{"80", "443 ssl h2"}},
		{raw: `{"listen": [8080]}`, want: []string{"8080"}},
	}
	for _, c := range cases {
		var server jsonServer
		if err := unmarshalConfig([]byte(c.raw), &server); err != nil {
			t.Fatalf("%s: %v", c.raw, err)
		}
		if len(server.Listen) != len(c.want) {
			t.Fatalf("%s: listens = %v, want %v", c.raw, server.Listen, c.want)
		}
		for i := range c.want {
			if server.Listen[i] != c.want[i] {
				t.Fatalf("%s: listens = %v, want %v", c.raw, server.Listen, c.want)
			}
		}
	}
}

// 多监听里只要有一条带 ssl 就要求证书；端口本身仍逐条校验。
func TestValidateServerMultipleListens(t *testing.T) {
	base := jsonServer{
		Listen:     flexList{"80", "443 ssl h2"},
		ServerName: "multi.local",
		Locations:  []jsonPath{{URL: "/", Type: "local", Root: "./httpdoc/root"}},
	}

	// 两个都不写是合法的：首次 start 的 CertInit 会按站点自动生成证书。
	noCert := base
	if err := validateServer(&noCert, ".", ".", "site", false); err != nil {
		t.Fatalf("证书留空应交给首次 start 生成：%v", err)
	}

	// 但只写一个不行，否则运行期拼不出完整的证书对。
	halfCert := base
	halfCert.SSLCertificate = "multi.pem"
	if err := validateServer(&halfCert, ".", ".", "site", false); err == nil {
		t.Fatal("只写 ssl_certificate 应被拒绝")
	}

	withCert := base
	withCert.SSLCertificate = "multi.pem"
	withCert.SSLCertificateKey = "multi-key.pem"
	if err := validateServer(&withCert, ".", ".", "site", false); err != nil {
		t.Fatalf("multi listen with certs: %v", err)
	}

	badPort := withCert
	badPort.Listen = flexList{"80", "not-a-port"}
	if err := validateServer(&badPort, ".", ".", "site", false); err == nil {
		t.Fatal("invalid port inside a listen list should be rejected")
	}

	empty := withCert
	empty.Listen = nil
	if err := validateServer(&empty, ".", ".", "site", false); err == nil {
		t.Fatal("empty listen should be rejected")
	}
}
