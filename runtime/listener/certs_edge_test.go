package listener

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"
)

// selfSignedFor 生成一张只含指定 DNS 名的自签证书，用来驱动 SNI 选择。
func selfSignedFor(t *testing.T, names ...string) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: names[0]},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		DNSNames:     names,
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA:         true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  key,
	}
}

// chosenCN 返回 get 实际选中的证书的 CN，便于区分「命中了某张」和「回落到第一张」。
func chosenCN(t *testing.T, set *certSet, sni string) string {
	t.Helper()
	cert, err := set.get(&tls.ClientHelloInfo{ServerName: sni})
	if err != nil {
		t.Fatalf("sni=%q: %v", sni, err)
	}
	if cert.Leaf == nil {
		leaf, err := x509.ParseCertificate(cert.Certificate[0])
		if err != nil {
			t.Fatal(err)
		}
		cert.Leaf = leaf
	}
	return cert.Leaf.Subject.CommonName
}

// TestCertSetUnknownSNIFallsBack 覆盖「SNI 不在任何证书覆盖范围」：必须回落
// 到第一张证书，而不是返回错误或 panic。这是握手阶段，崩了会带走整个进程。
func TestCertSetUnknownSNIFallsBack(t *testing.T) {
	set := &certSet{certs: []tls.Certificate{
		selfSignedFor(t, "first.example"),
		selfSignedFor(t, "second.example"),
	}}

	if got := chosenCN(t, set, "nowhere.test"); got != "first.example" {
		t.Fatalf("未知 SNI 应回落到第一张证书，得到 %q", got)
	}
	// 空 SNI（IP 直连）走同一条回落路径
	if got := chosenCN(t, set, ""); got != "first.example" {
		t.Fatalf("空 SNI 应回落到第一张证书，得到 %q", got)
	}
}

// TestCertSetEmptyReturnsError 证书列表为空时返回错误，不能 panic。
func TestCertSetEmptyReturnsError(t *testing.T) {
	set := newCertSet()
	if _, err := set.get(&tls.ClientHelloInfo{ServerName: "a.example"}); err == nil {
		t.Fatal("空证书列表应返回错误")
	}
	// nil 接收者同样只报错
	var nilSet *certSet
	if _, err := nilSet.get(&tls.ClientHelloInfo{}); err == nil {
		t.Fatal("nil certSet 应返回错误")
	}
}

// TestCertSetWildcardMatchesOneLabelOnly 覆盖「通配符只跨一层」：
// *.example.com 命中 a.example.com，但**不**命中 a.b.example.com。
//
// 后者会回落到第一张证书，看起来「也有证书」，所以断言必须看 CN 而不是
// 只看有没有返回。
func TestCertSetWildcardMatchesOneLabelOnly(t *testing.T) {
	set := &certSet{certs: []tls.Certificate{
		selfSignedFor(t, "first.example"),
		selfSignedFor(t, "*.example.com"),
	}}

	if got := chosenCN(t, set, "a.example.com"); got != "*.example.com" {
		t.Fatalf("a.example.com 应命中通配符证书，得到 %q", got)
	}
	if got := chosenCN(t, set, "a.b.example.com"); got == "*.example.com" {
		t.Fatal("a.b.example.com 不应命中单层通配符")
	}
	if got := chosenCN(t, set, "a.b.example.com"); got != "first.example" {
		t.Fatalf("跨层匹配应回落到第一张，得到 %q", got)
	}
}

// TestCertSetNormalizesCaseAndTrailingDot 大小写与结尾的点不应影响匹配。
func TestCertSetNormalizesCaseAndTrailingDot(t *testing.T) {
	set := &certSet{certs: []tls.Certificate{
		selfSignedFor(t, "first.example"),
		selfSignedFor(t, "exact.example"),
	}}

	for _, sni := range []string{"exact.example", "EXACT.EXAMPLE", "Exact.Example.", "exact.example."} {
		if got := chosenCN(t, set, sni); got != "exact.example" {
			t.Fatalf("sni=%q 应命中 exact.example，得到 %q", sni, got)
		}
	}
}
