package certmanager

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io/fs"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type testCert struct {
	certPath, keyPath string
}

func pemCert(der []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func pemKey(t *testing.T, key *ecdsa.PrivateKey) []byte {
	t.Helper()
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})
}

// writeSelfSigned 现场生成自签证书与私钥，不依赖仓库里的静态密钥文件。
func writeSelfSigned(t *testing.T, dir, name string, dns []string, notBefore, notAfter time.Time) testCert {
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
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	c := testCert{certPath: filepath.Join(dir, name+".pem"), keyPath: filepath.Join(dir, name+"-key.pem")}
	if err := os.WriteFile(c.certPath, pemCert(der), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(c.keyPath, pemKey(t, key), 0o600); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestInspectSelfSigned(t *testing.T) {
	now := time.Now()
	c := writeSelfSigned(t, t.TempDir(), "a", []string{"a.example", "www.a.example"}, now.Add(-time.Hour), now.Add(60*24*time.Hour+time.Hour))

	info, err := Inspect(c.certPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Subject != "a.example" || info.Issuer != "a.example" || !info.SelfSigned || info.LocalCA {
		t.Fatalf("info = %+v", info)
	}
	if len(info.Names) != 2 || info.Chain != 1 {
		t.Fatalf("names/chain = %v / %d", info.Names, info.Chain)
	}
	if got := info.DaysLeft(now); got != 60 {
		t.Fatalf("DaysLeft = %d, want 60", got)
	}
	if err := info.VerifyHostname("www.a.example"); err != nil {
		t.Fatalf("应覆盖 SAN：%v", err)
	}
	if err := info.VerifyHostname("other.example"); err == nil {
		t.Fatal("不应覆盖未列出的域名")
	}
}

func TestInspectExpiredHasNonPositiveDays(t *testing.T) {
	now := time.Now()
	c := writeSelfSigned(t, t.TempDir(), "old", []string{"old.example"}, now.Add(-90*24*time.Hour), now.Add(-2*24*time.Hour))
	info, err := Inspect(c.certPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.DaysLeft(now); got > -1 {
		t.Fatalf("已过期证书的 DaysLeft = %d", got)
	}
}

// 颁发者组织为 fast-https 的证书被标成本地 CA 签发。
func TestInspectLocalCA(t *testing.T) {
	now := time.Now()
	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	caTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Local Root", Organization: []string{"fast-https"}},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	ca, _ := x509.ParseCertificate(caDER)

	leafKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "localhost"}, DNSNames: []string{"localhost"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(24 * time.Hour),
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, ca, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "leaf.pem")
	if err := os.WriteFile(path, append(pemCert(leafDER), pemCert(caDER)...), 0o644); err != nil {
		t.Fatal(err)
	}

	info, err := Inspect(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.LocalCA || info.SelfSigned || info.Chain != 2 {
		t.Fatalf("info = %+v", info)
	}
}

func TestInspectMissingAndGarbage(t *testing.T) {
	dir := t.TempDir()

	if _, err := Inspect(filepath.Join(dir, "nope.pem")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("缺失文件的错误 = %v, want ErrNotExist", err)
	}

	garbage := filepath.Join(dir, "garbage.pem")
	if err := os.WriteFile(garbage, []byte("this is not a certificate"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Inspect(garbage); !errors.Is(err, ErrNoCertificate) {
		t.Fatalf("乱码文件的错误 = %v, want ErrNoCertificate", err)
	}

	badDER := filepath.Join(dir, "bad.pem")
	if err := os.WriteFile(badDER, pemCert([]byte("not der")), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Inspect(badDER); err == nil {
		t.Fatal("DER 损坏的证书应报错")
	}
}

func TestCheckKeyPair(t *testing.T) {
	now := time.Now()
	dir := t.TempDir()
	a := writeSelfSigned(t, dir, "a", []string{"a.example"}, now.Add(-time.Hour), now.Add(time.Hour))
	b := writeSelfSigned(t, dir, "b", []string{"b.example"}, now.Add(-time.Hour), now.Add(time.Hour))

	cases := []struct {
		name      string
		cert, key string
		want      KeyPairState
	}{
		{"match", a.certPath, a.keyPath, KeyPairOK},
		{"mismatch", a.certPath, b.keyPath, KeyPairInvalid},
		{"key missing", a.certPath, filepath.Join(dir, "none-key.pem"), KeyPairKeyMissing},
		{"cert unreadable", filepath.Join(dir, "none.pem"), a.keyPath, KeyPairCertUnreadable},
	}
	for _, c := range cases {
		if got := CheckKeyPair(c.cert, c.key); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
