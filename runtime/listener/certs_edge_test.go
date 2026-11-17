package listener

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fast-https/config"
	"fast-https/infra/logging"

	"github.com/rs/zerolog"
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

// TestCertSetUsesDefaultCertificate 配置 tls.default_certificate 后，未知
// SNI（含空 SNI）拿到兜底证书；能匹配的域名仍按原规则选择。
func TestCertSetUsesDefaultCertificate(t *testing.T) {
	def := selfSignedFor(t, "default.example")
	set := &certSet{
		certs: []tls.Certificate{
			selfSignedFor(t, "first.example"),
			selfSignedFor(t, "second.example"),
		},
		defaultCert: &def,
	}

	if got := chosenCN(t, set, "nowhere.test"); got != "default.example" {
		t.Fatalf("未知 SNI 应使用兜底证书，得到 %q", got)
	}
	if got := chosenCN(t, set, ""); got != "default.example" {
		t.Fatalf("空 SNI 应使用兜底证书，得到 %q", got)
	}
	if got := chosenCN(t, set, "second.example"); got != "second.example" {
		t.Fatalf("命中的域名不应被兜底证书覆盖，得到 %q", got)
	}
}

// TestCertSetUnknownSNILogsFallback 未配置兜底证书时回落第一张，并留下
// 可检索的 reject_reason=unknown_sni 日志（R-10 的字段约定）。
func TestCertSetUnknownSNILogsFallback(t *testing.T) {
	var buf bytes.Buffer
	previous := logging.ConsoleLog
	logging.ConsoleLog = zerolog.New(&buf)
	t.Cleanup(func() { logging.ConsoleLog = previous })

	set := &certSet{certs: []tls.Certificate{selfSignedFor(t, "first.example")}}
	if got := chosenCN(t, set, "nowhere.test"); got != "first.example" {
		t.Fatalf("未知 SNI 应回落到第一张，得到 %q", got)
	}

	out := buf.String()
	if !strings.Contains(out, `"reject_reason":"unknown_sni"`) {
		t.Fatalf("日志缺少 reject_reason：%s", out)
	}
	if !strings.Contains(out, `"sni":"nowhere.test"`) {
		t.Fatalf("日志缺少 sni：%s", out)
	}
}

// TestCertSetDefaultCertificateSkipsLog 配置了兜底证书时属于预期行为，
// 不再输出未知 SNI 告警，避免正常流量刷日志。
func TestCertSetDefaultCertificateSkipsLog(t *testing.T) {
	var buf bytes.Buffer
	previous := logging.ConsoleLog
	logging.ConsoleLog = zerolog.New(&buf)
	t.Cleanup(func() { logging.ConsoleLog = previous })

	def := selfSignedFor(t, "default.example")
	set := &certSet{
		certs:       []tls.Certificate{selfSignedFor(t, "first.example")},
		defaultCert: &def,
	}
	chosenCN(t, set, "nowhere.test")
	if buf.Len() != 0 {
		t.Fatalf("配置兜底证书后不应告警：%s", buf.String())
	}
}

// TestLoadDefaultCertificateFromConfig 覆盖配置到运行期的接线：
// config.Current().TLS 指向真实 PEM 时能加载出兜底证书。
func TestLoadDefaultCertificateFromConfig(t *testing.T) {
	cert := selfSignedFor(t, "default.example")
	derKey, err := x509.MarshalECPrivateKey(cert.PrivateKey.(*ecdsa.PrivateKey))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certPath := filepath.Join(dir, "default.pem")
	keyPath := filepath.Join(dir, "default-key.pem")
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: derKey})
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}

	previous := *config.Current()
	config.Store(&config.Fast_Https{TLS: config.TLSConfig{
		DefaultCertificate: config.Certificate{Certificate: certPath, Key: keyPath},
	}}, config.MIME())
	t.Cleanup(func() { config.Store(&previous, config.MIME()) })

	loaded := loadDefaultCertificate()
	if loaded == nil {
		t.Fatal("配置了 tls.default_certificate 时应加载出证书")
	}
	leaf, err := x509.ParseCertificate(loaded.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	if leaf.Subject.CommonName != "default.example" {
		t.Fatalf("加载到错误的证书：%q", leaf.Subject.CommonName)
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

// selfSignedRSAFor 生成一张 RSA 自签证书，用来和 ECDSA 证书组成同名多证书。
func selfSignedRSAFor(t *testing.T, names ...string) tls.Certificate {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
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

// TestCertSetSameNamePicksByClientCapabilities 同名多证书（ECDSA 在前、RSA 在后）
// 时，按客户端 signature_algorithms 选择兼容的那张，而不是永远用第一张。
func TestCertSetSameNamePicksByClientCapabilities(t *testing.T) {
	set := &certSet{certs: []tls.Certificate{
		selfSignedFor(t, "multi.example"),
		selfSignedRSAFor(t, "multi.example"),
	}}

	pick := func(schemes []tls.SignatureScheme) tls.Certificate {
		t.Helper()
		cert, err := set.get(&tls.ClientHelloInfo{
			ServerName:        "multi.example",
			SupportedVersions: []uint16{tls.VersionTLS13},
			SignatureSchemes:  schemes,
		})
		if err != nil {
			t.Fatal(err)
		}
		return *cert
	}
	kind := func(cert tls.Certificate) string {
		switch cert.PrivateKey.(type) {
		case *rsa.PrivateKey:
			return "rsa"
		case *ecdsa.PrivateKey:
			return "ecdsa"
		default:
			return "unknown"
		}
	}

	if got := kind(pick([]tls.SignatureScheme{tls.PSSWithSHA256})); got != "rsa" {
		t.Fatalf("只支持 RSA 的客户端应拿到 RSA 证书，得到 %q", got)
	}
	if got := kind(pick([]tls.SignatureScheme{tls.ECDSAWithP256AndSHA256})); got != "ecdsa" {
		t.Fatalf("只支持 ECDSA 的客户端应拿到 ECDSA 证书，得到 %q", got)
	}
	// 两种都支持：按证书列表顺序，第一张兼容的就是第一张。
	if got := kind(pick([]tls.SignatureScheme{tls.ECDSAWithP256AndSHA256, tls.PSSWithSHA256})); got != "ecdsa" {
		t.Fatalf("两种都支持时应取列表第一张，得到 %q", got)
	}
	// 没有 signature_algorithms 扩展：保持旧行为，用第一张。
	if got := kind(pick(nil)); got != "ecdsa" {
		t.Fatalf("缺少 signature_algorithms 时应退回第一张，得到 %q", got)
	}
}
