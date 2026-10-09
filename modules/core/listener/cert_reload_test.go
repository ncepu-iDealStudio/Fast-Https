package listener

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReplaceCertificateWithoutRebind(t *testing.T) {
	dir := t.TempDir()
	aCrt, aKey := writeSelfSigned(t, dir, "a.example")
	bCrt, bKey := writeSelfSigned(t, dir, "b.example")

	store := newCertSet()
	cfgA := []ListenCfg{{
		ServerName: "a.example",
		SSL:        SSLkv{SslKey: aCrt, SslValue: aKey},
	}}
	ln := listenSsl("127.0.0.1:0", store, cfgA, false)
	t.Cleanup(func() { _ = ln.Close() })
	go acceptUntilClose(ln)

	if cn := peerCN(t, ln.Addr().String(), "a.example"); cn != "a.example" {
		t.Fatalf("first certificate = %q", cn)
	}

	if err := store.replace([]ListenCfg{{
		ServerName: "b.example",
		SSL:        SSLkv{SslKey: bCrt, SslValue: bKey},
	}}); err != nil {
		t.Fatal(err)
	}
	if cn := peerCN(t, ln.Addr().String(), "b.example"); cn != "b.example" {
		t.Fatalf("reloaded certificate = %q", cn)
	}

	err := store.replace([]ListenCfg{{
		ServerName: "c.example",
		SSL:        SSLkv{SslKey: filepath.Join(dir, "missing.pem"), SslValue: bKey},
	}})
	if err == nil {
		t.Fatal("missing certificate should be rejected")
	}
	if cn := peerCN(t, ln.Addr().String(), "b.example"); cn != "b.example" {
		t.Fatalf("failed reload changed certificate to %q", cn)
	}
}

func acceptUntilClose(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go func(conn net.Conn) {
			buf := make([]byte, 1)
			_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			_, _ = conn.Read(buf)
			_ = conn.Close()
		}(conn)
	}
}

func peerCN(t *testing.T, addr, serverName string) string {
	t.Helper()
	conn, err := tls.Dial("tcp", addr, &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         serverName,
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

func writeSelfSigned(t *testing.T, dir, name string) (crtPath, keyPath string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: name},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		DNSNames:     []string{name},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	crtPath = filepath.Join(dir, name+".pem")
	keyPath = filepath.Join(dir, name+".key")
	if err := os.WriteFile(crtPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o644); err != nil {
		t.Fatal(err)
	}
	return crtPath, keyPath
}
