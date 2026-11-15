package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSSLTree(t *testing.T, mainSite, includedSite string) string {
	t.Helper()
	dir := t.TempDir()
	confd := filepath.Join(dir, "conf.d")
	if err := os.MkdirAll(confd, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(confd, "site.json"), []byte(includedSite), 0o644); err != nil {
		t.Fatal(err)
	}
	main := fmt.Sprintf(`{"acme": {"enabled": true, "email": "ops@example.com"},
  "http": {"include": [%q], "server": [%s]}}`, confd, mainSite)
	path := filepath.Join(dir, "fast-https.json")
	if err := os.WriteFile(path, []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSSLServersFromFileExpandsDomains(t *testing.T) {
	included := `{"listen": [80, "443 ssl h2"], "server_name": "a.example b.example",
  "ssl_certificate": "./certs/a.pem", "ssl_certificate_key": "./certs/a-key.pem",
  "location": [{"url": "/", "root": "."}]}`
	path := writeSSLTree(t, "", included)

	decls, acme, err := SSLServersFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if acme == nil || !acme.Active() || acme.RenewBeforeDays != DefaultACMERenewBeforeDays {
		t.Fatalf("acme 段应补默认值：%+v", acme)
	}
	if len(decls) != 2 {
		t.Fatalf("decls = %+v", decls)
	}

	first, alias := decls[0], decls[1]
	if first.Domain != "a.example" || first.Site != "site" || len(first.Listens) != 1 || first.Listens[0] != "443 ssl h2" {
		t.Fatalf("first = %+v", first)
	}
	if !strings.HasSuffix(first.Certificate, filepath.Join("certs", "a.pem")) || first.CertificateExists {
		t.Fatalf("主域名应用声明的证书路径（不存在）：%+v", first)
	}
	// 第二个域名起用兜底路径 <证书目录>/<域名>.pem，与 runtime/listener 一致。
	if alias.Domain != "b.example" || !strings.HasSuffix(alias.Certificate, filepath.Join("cert", "b.example.pem")) {
		t.Fatalf("alias = %+v", alias)
	}
	if !filepath.IsAbs(first.Certificate) || !filepath.IsAbs(alias.Certificate) {
		t.Fatal("证书路径应是绝对路径")
	}
}

func TestSSLServersFromFileFindsExistingCert(t *testing.T) {
	dir := t.TempDir()
	cert := filepath.Join(dir, "real.pem")
	key := filepath.Join(dir, "real-key.pem")
	for _, p := range []string{cert, key} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	included := fmt.Sprintf(`{"listen": ["443 ssl"], "server_name": "r.example",
  "ssl_certificate": %q, "ssl_certificate_key": %q, "location": [{"url": "/", "root": "."}]}`, cert, key)
	path := writeSSLTree(t, "", included)

	decls, _, err := SSLServersFromFile(path)
	if err != nil || len(decls) != 1 {
		t.Fatalf("decls = %+v, err = %v", decls, err)
	}
	if !decls[0].CertificateExists || !decls[0].KeyExists || decls[0].Certificate != cert {
		t.Fatalf("decl = %+v", decls[0])
	}
}

func TestSSLServersFromFileIgnoresPlainAndCoversInline(t *testing.T) {
	inline := `{"listen": ["8443 ssl"], "server_name": "inline.example", "ssl_certificate": "i.pem",
    "ssl_certificate_key": "i-key.pem", "location": [{"url": "/", "root": "."}]}`
	plain := `{"listen": [80], "server_name": "plain.example", "location": [{"url": "/", "root": "."}]}`
	path := writeSSLTree(t, inline, plain)

	decls, _, err := SSLServersFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(decls) != 1 || decls[0].Site != "main" || decls[0].Domain != "inline.example" {
		t.Fatalf("decls = %+v", decls)
	}
}

func TestSSLServersFromFileMissing(t *testing.T) {
	if _, _, err := SSLServersFromFile(filepath.Join(t.TempDir(), "nope.json")); err == nil {
		t.Fatal("配置文件不存在应报错")
	}
}
