package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type testCA struct {
	cert *x509.Certificate
	key  *rsa.PrivateKey
}

func newTestCA(t *testing.T, cn, org string) testCA {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: cn, Organization: []string{org}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(5, 0, 0),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return testCA{cert: cert, key: key}
}

// issueTestCert 生成一张证书并写成 PEM。parent 为零值时生成自签证书。
func issueTestCert(t *testing.T, parent testCA, cn, org string, dns []string, ips []net.IP) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: cn, Organization: []string{org}},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(1, 0, 0),
		DNSNames:     dns,
		IPAddresses:  ips,
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	signerCert, signerKey := parent.cert, parent.key
	if signerCert == nil {
		signerCert, signerKey = tmpl, key
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, signerCert, &key.PublicKey, signerKey)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "leaf.pem")
	if err := os.WriteFile(path, pem.EncodeToMemory(
		&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// 自动生成的证书由本地 CA 签发（Issuer != Subject），旧的 selfSigned
// 判定会把它误标成外部已签发；Local 必须识别出来。
func TestInspectPEMMarksLocalCAIssuedCert(t *testing.T) {
	ca := newTestCA(t, "fast-https Local CA", "fast-https")
	path := issueTestCert(t, ca, "localhost", "fast-https",
		[]string{"localhost", "example.com"}, []net.IP{net.ParseIP("192.168.1.10")})

	c := inspectPEM("localhost", path)
	if !c.Local {
		t.Fatal("本地 CA 签发的证书应标记为本地生成")
	}
	if c.SelfSigned {
		t.Fatal("由 CA 签发，不是自签")
	}
	if c.Subject != "localhost" || c.Issuer != "fast-https Local CA" {
		t.Fatalf("Subject=%q Issuer=%q", c.Subject, c.Issuer)
	}
	if !strings.Contains(c.Names, "example.com") || !strings.Contains(c.Names, "192.168.1.10") {
		t.Fatalf("SAN 未完整展示：%q", c.Names)
	}
}

func TestInspectPEMExternalCAIsNotLocal(t *testing.T) {
	ca := newTestCA(t, "Example Root CA", "Example Org")
	path := issueTestCert(t, ca, "example.com", "Example Org", []string{"example.com"}, nil)

	c := inspectPEM("example.com", path)
	if c.Local || c.SelfSigned {
		t.Fatalf("外部 CA 签发不应标为本地/自签：%+v", c)
	}
}

func TestInspectPEMSelfSignedIsLocal(t *testing.T) {
	path := issueTestCert(t, testCA{}, "localhost", "Me", []string{"localhost"}, nil)
	c := inspectPEM("localhost", path)
	if !c.Local || !c.SelfSigned {
		t.Fatalf("自签证书应同时是本地与自签：%+v", c)
	}
}

// 旧版本生成的叶子证书没有 CN，只有 SAN；展示时回落到第一个 DNSName。
func TestInspectPEMSubjectFallsBackToSAN(t *testing.T) {
	ca := newTestCA(t, "fast-https fast-https@ncepu.edu.cn", "fast-https")
	path := issueTestCert(t, ca, "", "fast-https", []string{"localhost"}, nil)
	c := inspectPEM("localhost", path)
	if c.Subject != "localhost" {
		t.Fatalf("没有 CN 时应回落到 SAN，实际 %q", c.Subject)
	}
	if !c.Local {
		t.Fatal("旧版本本地 CA（O=fast-https）也应识别为本地生成")
	}
}
