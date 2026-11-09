package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCertExpiringOnlyWarnsNearCert(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)
	soon := filepath.Join(dir, "soon.pem")
	later := filepath.Join(dir, "later.pem")
	writeTestCert(t, soon, now.Add(10*24*time.Hour))
	writeTestCert(t, later, now.Add(90*24*time.Hour))

	body, err := os.ReadFile(soon)
	if err != nil {
		t.Fatal(err)
	}
	c := newCertChecker(Config{
		CertFiles:    []string{soon, later},
		CertWarnDays: 30,
		Cooldown:     Duration(5 * time.Minute),
	})
	events := c.Check(now)
	if len(events) != 1 {
		t.Fatalf("events = %+v, want one", events)
	}
	ev := events[0]
	if ev.Type != EventCertExpiring || ev.Severity != "warning" {
		t.Fatalf("event = %+v", ev)
	}
	if !strings.Contains(ev.Message, "soon.pem") || !strings.Contains(ev.Message, "days_left=10") {
		t.Fatalf("message = %q", ev.Message)
	}
	if strings.Contains(ev.Message, "BEGIN") || strings.Contains(ev.Message, dir) {
		t.Fatalf("message leaked PEM or directory: %q", ev.Message)
	}
	if strings.Contains(ev.Message, string(body[:20])) {
		t.Fatalf("message contains PEM bytes: %q", ev.Message)
	}
	if again := c.Check(now.Add(time.Minute)); len(again) != 0 {
		t.Fatalf("cooldown events = %+v", again)
	}
	if after := c.Check(now.Add(6 * time.Minute)); len(after) != 1 {
		t.Fatalf("after cooldown = %+v, want one", after)
	}
}

func TestCertReadErrorDoesNotPanic(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.pem")
	if err := os.WriteFile(bad, []byte("not a certificate -----BEGIN PRIVATE KEY-----\nsecret"), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)
	c := newCertChecker(Config{
		CertFiles:    []string{bad},
		CertWarnDays: 30,
		Cooldown:     Duration(5 * time.Minute),
	})
	events := c.Check(now)
	if len(events) != 1 {
		t.Fatalf("events = %+v", events)
	}
	ev := events[0]
	if ev.Type != EventCertRead || ev.Severity != "warning" {
		t.Fatalf("event = %+v", ev)
	}
	if ev.Message != "certificate unreadable: bad.pem" {
		t.Fatalf("message = %q", ev.Message)
	}
	if strings.Contains(ev.Message, "PRIVATE KEY") || strings.Contains(ev.Message, "secret") || strings.Contains(ev.Message, dir) {
		t.Fatalf("message leaked file contents or path: %q", ev.Message)
	}
}

func TestConfigCertFiles(t *testing.T) {
	t.Setenv(EnvDir, "")
	dir := t.TempDir()
	path := filepath.Join(dir, "monitor.json")
	body := `{
		"cert_files": [" config/cert/a.pem ", ""],
		"cert_warn_days": 14
	}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.CertFiles) != 1 || cfg.CertFiles[0] != "config/cert/a.pem" {
		t.Fatalf("cert_files = %#v", cfg.CertFiles)
	}
	if cfg.CertWarnDays != 14 {
		t.Fatalf("cert_warn_days = %d", cfg.CertWarnDays)
	}
	absent, err := LoadConfig(filepath.Join(dir, "missing.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(absent.CertFiles) != 0 || absent.CertWarnDays != DefaultCertWarnDays {
		t.Fatalf("defaults = %#v / %d", absent.CertFiles, absent.CertWarnDays)
	}
}

func writeTestCert(t *testing.T, path string, notAfter time.Time) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    notAfter.Add(-time.Hour),
		NotAfter:     notAfter,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	raw := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}
