package certmanager

import (
	"fast-https/config"
	"os"
	"path/filepath"
	"testing"
)

func TestACMEEnabledServersOnly(t *testing.T) {
	cfg := &config.Fast_Https{
		Servers: []config.Server{
			{Listen: "80", ServerName: "plain.example"},
			{Listen: "443 ssl", ServerName: "Secure.Example."},
			{Listen: "443 ssl", ServerName: "secure.example"},
			{Listen: "443 ssl", ServerName: "10.0.0.1"},
			{Listen: "443 ssl", ServerName: "localhost"},
			{Listen: "443 ssl", ServerName: "*.wild.example"},
			{Listen: "443 ssl", ServerName: "intranet"},
		},
	}

	got := managedDomains(cfg)
	if len(got) != 1 || got[0] != "secure.example" {
		t.Fatalf("managed domains = %v, want [secure.example]", got)
	}
}

func TestACMEEligibleReasons(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"example.com", true},
		{"sub.example.com", true},
		{"", false},
		{"localhost", false},
		{"intranet", false},
		{"127.0.0.1", false},
		{"::1", false},
		{"*.example.com", false},
	}
	for _, tc := range cases {
		ok, reason := acmeEligible(tc.name)
		if ok != tc.want {
			t.Fatalf("acmeEligible(%q) = %v (%s)", tc.name, ok, reason)
		}
		if !ok && reason == "" {
			t.Fatalf("acmeEligible(%q) rejected without a reason", tc.name)
		}
	}
}

func TestNormalizeName(t *testing.T) {
	cases := map[string]string{
		"Example.COM":   "example.com",
		"example.com.":  "example.com",
		" example.com ": "example.com",
	}
	for in, want := range cases {
		if got := normalizeName(in); got != want {
			t.Fatalf("normalizeName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRenewalWindowRatio(t *testing.T) {
	cases := map[int]float64{
		0:   0,
		30:  1.0 / 3.0,
		45:  0.5,
		200: 0.9,
	}
	for days, want := range cases {
		got := renewalWindowRatio(days)
		if diff := got - want; diff > 1e-9 || diff < -1e-9 {
			t.Fatalf("renewalWindowRatio(%d) = %v, want %v", days, got, want)
		}
	}
}

func TestCertPathsFollowConfiguredServer(t *testing.T) {
	previous := *config.Current()
	t.Cleanup(func() { config.Store(&previous, config.MIME()) })

	cert := filepath.Join(t.TempDir(), "site.pem")
	key := filepath.Join(t.TempDir(), "site-key.pem")
	config.Store(&config.Fast_Https{
		Servers: []config.Server{{
			Listen:            "443 ssl",
			ServerName:        "Site.Example",
			SSLCertificate:    cert,
			SSLCertificateKey: key,
		}},
	}, nil)

	gotCert, gotKey := certPaths("site.example")
	if gotCert != cert || gotKey != key {
		t.Fatalf("certPaths = %q, %q, want configured paths", gotCert, gotKey)
	}

	// 没有匹配的 server 时退回 CertInit 的命名约定。
	fallbackCert, fallbackKey := certPaths("other.example")
	if filepath.Base(fallbackCert) != "other.example.pem" || filepath.Base(fallbackKey) != "other.example-key.pem" {
		t.Fatalf("fallback = %q, %q", fallbackCert, fallbackKey)
	}
	if filepath.Dir(fallbackCert) != filepath.Clean(config.CERT_DIR) {
		t.Fatalf("fallback cert = %q, want under %q", fallbackCert, config.CERT_DIR)
	}
}

func TestNewManagerBuildsSourcesWithoutNetwork(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.AcmeConfig{
		Enabled:   true,
		Email:     "ops@example.test",
		Challenge: []string{config.ACMEChallengeAuto},
		Storage:   dir,
		CAs:       []config.ACMECA{{Name: "le", Directory: config.ACMELetsEncryptDirectory}},
	}

	m, err := newManager(cfg, []string{"example.com"})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	t.Cleanup(m.stop)

	if m.http == nil || m.alpn == nil {
		t.Fatalf("solvers: http=%v alpn=%v, want both", m.http != nil, m.alpn != nil)
	}
	if m.magic == nil || len(m.magic.Issuers) != 1 {
		t.Fatalf("issuers = %v, want 1", m.magic)
	}
	// 存储目录在构造期就建好，签发前不会因为目录缺失失败。
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("storage dir: %v", err)
	}
}

func TestNewManagerSkipsCAWithoutEAB(t *testing.T) {
	cfg := &config.AcmeConfig{
		Enabled:   true,
		Email:     "ops@example.test",
		Challenge: []string{config.ACMEChallengeHTTP01},
		Storage:   t.TempDir(),
		CAs:       []config.ACMECA{{Name: "zerossl", Directory: config.ACMEZeroSSLDirectory}},
	}

	if _, err := newManager(cfg, []string{"example.com"}); err == nil {
		t.Fatal("a CA requiring EAB without credentials must be skipped")
	}
}

func TestNewManagerRejectsEmptyChallengeSet(t *testing.T) {
	cfg := &config.AcmeConfig{
		Enabled:   true,
		Email:     "ops@example.test",
		Challenge: []string{"dns-01"},
		Storage:   t.TempDir(),
		CAs:       []config.ACMECA{{Name: "le", Directory: config.ACMELetsEncryptDirectory}},
	}

	if _, err := newManager(cfg, []string{"example.com"}); err == nil {
		t.Fatal("unsupported challenge types must be rejected")
	}
}

func TestRequiresEAB(t *testing.T) {
	if !requiresEAB(config.ACMECA{Name: "zerossl", Directory: config.ACMEZeroSSLDirectory}) {
		t.Fatal("zerossl should require external account binding")
	}
	if requiresEAB(config.ACMECA{Name: "le", Directory: config.ACMELetsEncryptDirectory}) {
		t.Fatal("letsencrypt should not require external account binding")
	}
}
