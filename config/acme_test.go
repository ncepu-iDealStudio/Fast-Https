package config_test

import (
	"fast-https/config"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// acmeTestEnv 把工作目录切到仓库根目录（LoadFile 依赖相对路径读 mime.json），
// 并在用例结束后恢复配置快照，避免影响同包其他用例。
func acmeTestEnv(t *testing.T) {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := wd
	if _, statErr := os.Stat(filepath.Join(wd, "go.mod")); statErr != nil {
		root = filepath.Dir(wd)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	previousConfig := *config.Current()
	previousMIME := config.MIME()
	t.Cleanup(func() {
		_ = os.Chdir(wd)
		config.Store(&previousConfig, previousMIME)
	})
}

// acmeTestConfig 拼一份最小可加载配置，acme 段按需替换。
func acmeTestConfig(acmeBlock string) string {
	return `{
  "http": {
    "server": [
      {
        "listen": 8080,
        "server_name": "acme.local",
        "location": [
          {"url": "/", "type": "local", "root": "./httpdoc/root", "index": ["index.html"]}
        ]
      }
    ]
  }` + acmeBlock + `
}`
}

func writeACMETestConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fast-https.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestACMEMissingKeepsOldBehavior(t *testing.T) {
	acmeTestEnv(t)

	path := writeACMETestConfig(t, acmeTestConfig(""))
	if err := config.LoadFile(path); err != nil {
		t.Fatalf("load config without acme: %v", err)
	}
	if cfg := config.Current(); cfg.Acme != nil {
		t.Fatalf("acme = %+v, want nil", cfg.Acme)
	}
}

func TestACMEDefaults(t *testing.T) {
	acmeTestEnv(t)

	path := writeACMETestConfig(t, acmeTestConfig(`, "acme": {"enabled": false}`))
	if err := config.LoadFile(path); err != nil {
		t.Fatalf("load config: %v", err)
	}
	acme := config.Current().Acme
	if acme == nil {
		t.Fatal("acme section was dropped")
	}
	if len(acme.Challenge) != 1 || acme.Challenge[0] != config.ACMEChallengeAuto {
		t.Fatalf("challenge = %v, want [auto]", acme.Challenge)
	}
	if acme.RenewBeforeDays != config.DefaultACMERenewBeforeDays {
		t.Fatalf("renew_before_days = %d", acme.RenewBeforeDays)
	}
	if !acme.FallbackSelfSignedEnabled() {
		t.Fatal("fallback_self_signed should default to true")
	}
	if !filepath.IsAbs(acme.Storage) {
		t.Fatalf("storage = %q, want absolute path", acme.Storage)
	}
	if filepath.Base(acme.Storage) != "acme" || filepath.Base(filepath.Dir(acme.Storage)) != "cert" {
		t.Fatalf("storage = %q, want <base>/config/cert/acme", acme.Storage)
	}
	if len(acme.CAs) != 2 || acme.CAs[0].Directory != config.ACMELetsEncryptDirectory {
		t.Fatalf("cas = %+v", acme.CAs)
	}
}

func TestACMEStagingOnlyRewritesLetsEncrypt(t *testing.T) {
	acmeTestEnv(t)

	body := acmeTestConfig(`, "acme": {
    "enabled": false,
    "staging": true,
    "cas": [
      {"name": "le", "directory": "https://acme-v02.api.letsencrypt.org/directory"},
      {"name": "custom", "directory": "https://ca.example.test/directory"}
    ]
  }`)
	path := writeACMETestConfig(t, body)
	if err := config.LoadFile(path); err != nil {
		t.Fatalf("load config: %v", err)
	}
	cas := config.Current().Acme.CAs
	if cas[0].Directory != config.ACMELetsEncryptStagingDirectory {
		t.Fatalf("letsencrypt directory = %q", cas[0].Directory)
	}
	if cas[1].Directory != "https://ca.example.test/directory" {
		t.Fatalf("custom directory was rewritten: %q", cas[1].Directory)
	}
}

func TestACMEStorageRelativeToBaseDir(t *testing.T) {
	acmeTestEnv(t)

	body := acmeTestConfig(`, "acme": {"enabled": false, "storage": "var/acme-store"}`)
	path := writeACMETestConfig(t, body)
	if err := config.LoadFile(path); err != nil {
		t.Fatalf("load config: %v", err)
	}
	storage := config.Current().Acme.Storage
	if !filepath.IsAbs(storage) || !strings.HasSuffix(filepath.ToSlash(storage), "var/acme-store") {
		t.Fatalf("storage = %q", storage)
	}
}

func TestACMEExplicitFallbackFalse(t *testing.T) {
	acmeTestEnv(t)

	body := acmeTestConfig(`, "acme": {"enabled": false, "fallback_self_signed": false}`)
	path := writeACMETestConfig(t, body)
	if err := config.LoadFile(path); err != nil {
		t.Fatalf("load config: %v", err)
	}
	if config.Current().Acme.FallbackSelfSignedEnabled() {
		t.Fatal("explicit false should disable self-signed fallback")
	}
}

func TestACMEValidateRejectsInvalidConfig(t *testing.T) {
	acmeTestEnv(t)

	cases := []struct {
		name  string
		block string
		want  string
	}{
		{
			name:  "invalid challenge",
			block: `, "acme": {"enabled": false, "challenge": ["dns-01"]}`,
			want:  "challenge[0] is invalid",
		},
		{
			name:  "auto mixed with explicit",
			block: `, "acme": {"enabled": false, "challenge": ["auto", "http-01"]}`,
			want:  "auto cannot be combined",
		},
		{
			name:  "enabled without email",
			block: `, "acme": {"enabled": true}`,
			want:  "acme.email is required",
		},
		{
			name:  "negative renew days",
			block: `, "acme": {"enabled": false, "renew_before_days": -1}`,
			want:  "renew_before_days",
		},
		{
			name:  "duplicate ca name",
			block: `, "acme": {"enabled": false, "cas": [{"name": "a", "directory": "https://a.test/d"}, {"name": "a", "directory": "https://b.test/d"}]}`,
			want:  "duplicated",
		},
		{
			name:  "ca without directory",
			block: `, "acme": {"enabled": false, "cas": [{"name": "a"}]}`,
			want:  "directory is required",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeACMETestConfig(t, acmeTestConfig(tc.block))
			err := config.ValidateConfigFile(path)
			if err == nil {
				t.Fatalf("want error containing %q, got nil", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestACMEEnabledWithEmailPassesValidation(t *testing.T) {
	acmeTestEnv(t)

	body := acmeTestConfig(`, "acme": {"enabled": true, "email": "ops@example.test"}`)
	path := writeACMETestConfig(t, body)
	if err := config.ValidateConfigFile(path); err != nil {
		t.Fatalf("valid acme config rejected: %v", err)
	}
	if err := config.LoadFile(path); err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !config.Current().Acme.Active() {
		t.Fatal("acme should be active")
	}
}

func TestACMEUsesChallenge(t *testing.T) {
	auto := &config.AcmeConfig{Challenge: []string{config.ACMEChallengeAuto}}
	if !auto.UsesChallenge(config.ACMEChallengeHTTP01) || !auto.UsesChallenge(config.ACMEChallengeTLSALPN01) {
		t.Fatal("auto should accept both challenges")
	}

	alpnOnly := &config.AcmeConfig{Challenge: []string{config.ACMEChallengeTLSALPN01}}
	if alpnOnly.UsesChallenge(config.ACMEChallengeHTTP01) {
		t.Fatal("tls-alpn-01 only config should not accept http-01")
	}

	var nilCfg *config.AcmeConfig
	if nilCfg.Active() || nilCfg.UsesChallenge(config.ACMEChallengeHTTP01) {
		t.Fatal("nil acme config should be inert")
	}
}
