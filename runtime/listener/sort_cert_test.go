package listener

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"testing"
	"time"

	"fast-https/config"
)

func TestSortByPortBuildsListeners(t *testing.T) {
	origServers := config.Current().Servers
	config.Current().Servers = []config.Server{
		{Listen: "19001 ssl h2", ServerName: "a.local"},
		{Listen: "19001", ServerName: "same-port-dedup"},     // 同端口去重
		{Listen: "19002 tcp", ServerName: "tcp.local"},
		{Listen: "19003 ssl", ServerName: "s.local"},
		{Listen: "19004", ServerName: "plain.local"},
	}
	defer func() { config.Current().Servers = origServers }()

	var lis []*Listener
	SortByPort(&lis)

	if len(lis) != 4 { // 19001/19002/19003/19004
		t.Fatalf("listeners = %d, want 4: %+v", len(lis), lis)
	}
	wantType := map[string]uint8{"19001": 10, "19002": 2, "19003": 1, "19004": 0}
	seen := map[string]bool{}
	for _, l := range lis {
		if seen[l.Port] {
			t.Fatalf("port %s appears twice", l.Port)
		}
		seen[l.Port] = true
		if l.LisType != wantType[l.Port] {
			t.Errorf("port %s LisType = %d, want %d", l.Port, l.LisType, wantType[l.Port])
		}
	}
}

func genCert(t *testing.T, dnsNames []string) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(7),
		Subject:      pkix.Name{CommonName: dnsNames[0]},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		DNSNames:     dnsNames,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

func TestCertMatches(t *testing.T) {
	cert := genCert(t, []string{"example.com", "*.wild.example"})

	if !certMatches(cert, "example.com") {
		t.Error("exact DNS name must match")
	}
	if !certMatches(cert, "x.wild.example") {
		t.Error("wildcard must match subdomain")
	}
	if certMatches(cert, "other.com") {
		t.Error("unrelated name must not match")
	}
	if certMatches(tls.Certificate{}, "example.com") {
		t.Error("empty certificate must not match")
	}
	if certMatches(tls.Certificate{Certificate: [][]byte{[]byte("garbage")}}, "example.com") {
		t.Error("unparseable DER must not match")
	}

	// Leaf 已预解析的路径同样生效
	parsed, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	withLeaf := cert
	withLeaf.Leaf = parsed
	if !certMatches(withLeaf, "example.com") {
		t.Error("pre-parsed leaf must still match")
	}
}

var _ = net.ParseIP // 保持 net 引用（后续端口用例扩展）
