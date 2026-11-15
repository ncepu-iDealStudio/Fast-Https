package view

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type certFiles struct{ cert, key string }

func genCert(t *testing.T, dir, name string, dns []string, notBefore, notAfter time.Time) certFiles {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: dns[0]},
		DNSNames:     dns,
		NotBefore:    notBefore,
		NotAfter:     notAfter,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	f := certFiles{cert: filepath.Join(dir, name+".pem"), key: filepath.Join(dir, name+"-key.pem")}
	if err := os.WriteFile(f.cert, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.key, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	return f
}

type certFixture struct {
	main string
	now  time.Time
	keys []string // 私钥文件路径，用来断言它们的内容没有出现在输出里
}

func writeSite(t *testing.T, confd, name, serverName, cert, key string) {
	t.Helper()
	body := fmt.Sprintf(`{"listen": ["443 ssl"], "server_name": %q, "ssl_certificate": %q, "ssl_certificate_key": %q,
  "location": [{"url": "/", "type": "local", "root": "."}]}`, serverName, cert, key)
	if err := os.WriteFile(filepath.Join(confd, name+".json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newCertFixture(t *testing.T) certFixture {
	t.Helper()
	dir := t.TempDir()
	certs := filepath.Join(dir, "certs")
	confd := filepath.Join(dir, "conf.d")
	for _, d := range []string{certs, confd} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	f := certFixture{now: now}

	valid := genCert(t, certs, "valid", []string{"valid.example"}, now.Add(-time.Hour), now.Add(90*24*time.Hour))
	expiring := genCert(t, certs, "expiring", []string{"expiring.example"}, now.Add(-time.Hour), now.Add(10*24*time.Hour+time.Hour))
	expired := genCert(t, certs, "expired", []string{"expired.example"}, now.Add(-90*24*time.Hour), now.Add(-24*time.Hour))
	other := genCert(t, certs, "other", []string{"other.example"}, now.Add(-time.Hour), now.Add(90*24*time.Hour))
	multi := genCert(t, certs, "multi", []string{"m1.example"}, now.Add(-time.Hour), now.Add(90*24*time.Hour))
	f.keys = []string{valid.key, expiring.key, expired.key, other.key, multi.key}

	garbage := filepath.Join(certs, "garbage.pem")
	if err := os.WriteFile(garbage, []byte("not a certificate"), 0o644); err != nil {
		t.Fatal(err)
	}

	writeSite(t, confd, "valid", "valid.example", valid.cert, valid.key)
	writeSite(t, confd, "shared", "shared.example", valid.cert, valid.key) // 与 valid 共用同一张证书
	writeSite(t, confd, "expiring", "expiring.example", expiring.cert, expiring.key)
	writeSite(t, confd, "expired", "expired.example", expired.cert, expired.key)
	writeSite(t, confd, "missing", "missing.example", filepath.Join(certs, "gone.pem"), filepath.Join(certs, "gone-key.pem"))
	writeSite(t, confd, "garbage", "garbage.example", garbage, valid.key)
	writeSite(t, confd, "wronghost", "wrong.example", valid.cert, valid.key) // 证书不覆盖该域名
	writeSite(t, confd, "mismatch", "other.example", other.cert, valid.key)  // 私钥不是这张证书的
	writeSite(t, confd, "nokey", "nokey.example", other.cert, filepath.Join(certs, "nokey-key.pem"))
	writeSite(t, confd, "multi", "m1.example m2.example", multi.cert, multi.key)
	writeSite(t, confd, "acme", "app.example.com", filepath.Join(certs, "app.pem"), filepath.Join(certs, "app-key.pem"))

	plain := `{"listen": [8080], "server_name": "plain.example", "location": [{"url": "/", "type": "local", "root": "."}]}`
	if err := os.WriteFile(filepath.Join(confd, "plain.json"), []byte(plain), 0o644); err != nil {
		t.Fatal(err)
	}

	f.main = filepath.Join(dir, "fast-https.json")
	main := fmt.Sprintf(`{"acme": {"enabled": true, "email": "ops@example.com"}, "http": {"include": [%q]}}`, confd)
	if err := os.WriteFile(f.main, []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}
	return f
}

func entryOf(t *testing.T, data CertListData, domain string) CertEntry {
	t.Helper()
	for _, e := range data.Certificates {
		for _, d := range e.Domains {
			if d == domain {
				return e
			}
		}
	}
	t.Fatalf("没有域名 %s 的条目", domain)
	return CertEntry{}
}

func TestCertListStatuses(t *testing.T) {
	f := newCertFixture(t)
	data, _, err := CertList(f.main, f.now)
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]string{
		"valid.example":    CertValid,
		"expiring.example": CertExpiring,
		"expired.example":  CertExpired,
		"missing.example":  CertMissing,
		"garbage.example":  CertUnreadable,
		"m1.example":       CertValid,
	}
	for domain, status := range want {
		if got := entryOf(t, data, domain).Status; got != status {
			t.Errorf("%s 的状态 = %s, want %s", domain, got, status)
		}
	}
	if data.RenewBeforeDays != 30 {
		t.Errorf("renew_before_days = %d, want 30", data.RenewBeforeDays)
	}

	expiring := entryOf(t, data, "expiring.example")
	if expiring.DaysLeft == nil || *expiring.DaysLeft != 10 {
		t.Errorf("expiring.days_left = %v, want 10", expiring.DaysLeft)
	}
	expired := entryOf(t, data, "expired.example")
	if expired.DaysLeft == nil || *expired.DaysLeft >= 0 {
		t.Errorf("expired.days_left = %v, want negative", expired.DaysLeft)
	}
	if missing := entryOf(t, data, "missing.example"); missing.DaysLeft != nil || missing.Error == "" {
		t.Errorf("缺失证书不应有 days_left，应有 error：%+v", missing)
	}
}

func TestCertListGroupsByFileAndSkipsPlainSites(t *testing.T) {
	f := newCertFixture(t)
	data, _, err := CertList(f.main, f.now)
	if err != nil {
		t.Fatal(err)
	}

	shared := entryOf(t, data, "valid.example")
	if got := strings.Join(shared.Domains, ","); got != "shared.example,valid.example,wrong.example" {
		t.Errorf("同一张证书应归并成一条，域名 = %s", got)
	}
	if !contains(shared.DeployedBy, "valid") || !contains(shared.DeployedBy, "shared") {
		t.Errorf("deployed_by = %v", shared.DeployedBy)
	}
	for _, e := range data.Certificates {
		for _, d := range e.Domains {
			if d == "plain.example" {
				t.Fatal("非 ssl 站点不应出现在证书列表里")
			}
		}
	}
}

// 多域名 server 块：从第二个域名起，证书取运行期的兜底路径 <证书目录>/<域名>.pem。
func TestCertListMultiDomainUsesFallbackPath(t *testing.T) {
	f := newCertFixture(t)
	data, _, _ := CertList(f.main, f.now)

	alias := entryOf(t, data, "m2.example")
	if !strings.HasSuffix(alias.File, filepath.Join("cert", "m2.example.pem")) {
		t.Fatalf("别名域名的证书路径 = %s", alias.File)
	}
	if alias.Status != CertMissing {
		t.Fatalf("别名域名的状态 = %s, want missing", alias.Status)
	}
	if entryOf(t, data, "m1.example").Status != CertValid {
		t.Fatal("主域名应读 ssl_certificate")
	}
}

func TestCertListACMEManaged(t *testing.T) {
	f := newCertFixture(t)
	data, _, _ := CertList(f.main, f.now)

	if !entryOf(t, data, "app.example.com").ACMEManaged {
		t.Error("公网域名在 acme 启用时应由 ACME 管理")
	}
	if !entryOf(t, data, "valid.example").ACMEManaged {
		t.Error("valid.example 是公网形式的域名，应由 ACME 管理")
	}
}

func TestCertCheckHostnameMismatch(t *testing.T) {
	f := newCertFixture(t)
	data, _, err := CertCheck(f.main, "wrong.example", f.now)
	if err != nil {
		t.Fatal(err)
	}
	if data.Checks.Hostname.OK || !data.Checks.Hostname.Checked || data.Passed {
		t.Fatalf("证书不覆盖该域名：%+v", data.Checks.Hostname)
	}
	if !data.Checks.Exists.OK || !data.Checks.Validity.OK {
		t.Fatalf("其它检查应正常：%+v", data.Checks)
	}
}

func TestCertCheckKeyMismatchAndMissingKey(t *testing.T) {
	f := newCertFixture(t)

	data, _, _ := CertCheck(f.main, "other.example", f.now)
	if !data.Checks.KeyPair.Checked || data.Checks.KeyPair.OK || data.Passed {
		t.Fatalf("私钥不配对：%+v", data.Checks.KeyPair)
	}

	data, _, _ = CertCheck(f.main, "nokey.example", f.now)
	if !data.Checks.KeyPair.Checked || data.Checks.KeyPair.OK || data.Checks.KeyPair.Detail != "private key file not found" {
		t.Fatalf("私钥缺失：%+v", data.Checks.KeyPair)
	}
}

func TestCertCheckPasses(t *testing.T) {
	f := newCertFixture(t)
	data, _, err := CertCheck(f.main, "VALID.example.", f.now) // 大小写与结尾的点不影响匹配
	if err != nil {
		t.Fatal(err)
	}
	if !data.Passed || !data.Checks.KeyPair.OK || data.Checks.Chain.Checked || data.Checks.Chain.Certificates != 1 {
		t.Fatalf("data = %+v", data)
	}
}

func TestCertCheckMissingFileSkipsDependentChecks(t *testing.T) {
	f := newCertFixture(t)
	data, _, err := CertCheck(f.main, "missing.example", f.now)
	if err != nil {
		t.Fatal(err)
	}
	if data.Checks.Exists.OK || !data.Checks.Exists.Checked {
		t.Fatalf("exists = %+v", data.Checks.Exists)
	}
	for name, item := range map[string]CheckItem{"validity": data.Checks.Validity, "hostname": data.Checks.Hostname, "key_pair": data.Checks.KeyPair} {
		if item.Checked {
			t.Errorf("%s 不该被执行：%+v", name, item)
		}
	}
	if data.Passed {
		t.Fatal("证书缺失不应通过")
	}
}

func TestCertCheckUnknownDomain(t *testing.T) {
	f := newCertFixture(t)
	_, _, err := CertCheck(f.main, "nobody.example", f.now)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if _, _, err := CertCheck(f.main, "  ", f.now); !errors.Is(err, ErrBadName) {
		t.Fatalf("空域名 err = %v, want ErrBadName", err)
	}
}

// 无论哪个视图，私钥正文与 PEM 头都不能出现在输出里。
func TestNoPrivateKeyInViews(t *testing.T) {
	f := newCertFixture(t)

	list, _, _ := CertList(f.main, f.now)
	out, _ := json.Marshal(list)
	for _, domain := range []string{"valid.example", "other.example", "nokey.example", "missing.example"} {
		check, _, err := CertCheck(f.main, domain, f.now)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(check)
		out = append(out, b...)
	}

	text := string(out)
	if strings.Contains(text, "PRIVATE KEY") {
		t.Fatal("输出里出现了 PEM 私钥头")
	}
	for _, keyFile := range f.keys {
		raw, err := os.ReadFile(keyFile)
		if err != nil {
			t.Fatal(err)
		}
		// 取密钥正文的第一行 base64，足够独特。
		lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
		if body := strings.TrimSpace(lines[1]); len(body) > 20 && strings.Contains(text, body) {
			t.Fatalf("输出里出现了私钥正文 %s", keyFile)
		}
	}
	if strings.Contains(text, "-key.pem") {
		t.Fatal("私钥路径不应出现在输出里")
	}
}

func TestCertListNoSSLSitesWarns(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "fast-https.json")
	if err := os.WriteFile(main, []byte(`{"http": {"server": [{"listen": [80], "server_name": "a.example", "location": [{"url": "/", "root": "."}]}]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	data, warnings, err := CertList(main, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Certificates) != 0 || len(warnings) != 1 {
		t.Fatalf("data = %+v warnings = %v", data, warnings)
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
