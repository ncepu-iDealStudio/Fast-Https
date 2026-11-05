package cmd

import (
	"bytes"
	"encoding/json"
	"fast-https/config"
	"fast-https/modules/certmanager"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// lineReader 每次 Read 只吐一行。huh 的 accessible 模式每个字段都会新建
// 一个 bufio.Scanner，普通 strings.Reader 会被第一个 Scanner 一次性读空，
// 后面的字段只能拿到 EOF。
type lineReader struct {
	lines []string
	next  int
}

func newLineReader(lines ...string) *lineReader {
	readied := make([]string, 0, len(lines))
	for _, line := range lines {
		readied = append(readied, line+"\n")
	}
	return &lineReader{lines: readied}
}

func (r *lineReader) Read(p []byte) (int, error) {
	if r.next >= len(r.lines) {
		return 0, io.EOF
	}
	n := copy(p, r.lines[r.next])
	r.next++
	return n, nil
}

// tempACMEConfig 复制一份随包默认配置到临时目录，测试只改副本。
func tempACMEConfig(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "config", "fast-https.json"))
	if err != nil {
		t.Fatalf("read default config: %v", err)
	}
	path := filepath.Join(t.TempDir(), "fast-https.json")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// readAcmeSection 解析写出的文件。这里不用 config.ValidateConfigFile：
// 它会连 include 目录与 ssl 证书文件一起校验，而测试用的是临时目录副本，
// 那些路径本来就指不到。acme 段自身的合法性由 config 包的写回测试覆盖。
func readAcmeSection(t *testing.T, path string) *config.AcmeConfig {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var probe struct {
		Acme *config.AcmeConfig `json:"acme"`
		HTTP json.RawMessage    `json:"http"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		t.Fatalf("written config does not parse: %v\n%s", err, raw)
	}
	if len(probe.HTTP) == 0 {
		t.Fatalf("http section disappeared:\n%s", raw)
	}
	if probe.Acme == nil {
		t.Fatalf("acme section missing:\n%s", raw)
	}
	return probe.Acme
}

func defaultAcmeAnswers() acmeAnswers {
	return acmeAnswers{
		Enabled:         true,
		Email:           "ops@example.com",
		CA:              acmeCALetsEncrypt,
		Challenge:       config.ACMEChallengeAuto,
		RenewBeforeDays: config.DefaultACMERenewBeforeDays,
		Storage:         config.ACMECertStorageDir,
		Fallback:        true,
	}
}

func TestAcmeBuildConfig(t *testing.T) {
	t.Run("letsencrypt", func(t *testing.T) {
		cfg, err := defaultAcmeAnswers().buildConfig()
		if err != nil {
			t.Fatalf("build: %v", err)
		}
		if len(cfg.CAs) != 1 || cfg.CAs[0].Directory != config.ACMELetsEncryptDirectory {
			t.Fatalf("cas = %+v", cfg.CAs)
		}
		if cfg.Staging {
			t.Fatal("staging should be off")
		}
		if !cfg.FallbackSelfSignedEnabled() {
			t.Fatal("fallback should be on")
		}
	})

	t.Run("staging", func(t *testing.T) {
		answers := defaultAcmeAnswers()
		answers.CA = acmeCAStaging
		cfg, err := answers.buildConfig()
		if err != nil {
			t.Fatalf("build: %v", err)
		}
		if !cfg.Staging {
			t.Fatal("staging flag should be set")
		}
		// 目录保持生产值，由 applyDefaults 在 staging=true 时替换，
		// 这样配置文件读起来仍是一份标准 CA 列表。
		if cfg.CAs[0].Directory != config.ACMELetsEncryptDirectory {
			t.Fatalf("directory = %q", cfg.CAs[0].Directory)
		}
	})

	t.Run("zerossl requires eab", func(t *testing.T) {
		answers := defaultAcmeAnswers()
		answers.CA = acmeCAZeroSSL
		if _, err := answers.buildConfig(); err == nil {
			t.Fatal("zerossl without eab should be rejected")
		}
		answers.EABKid = "kid"
		answers.EABHmacKey = "hmac"
		cfg, err := answers.buildConfig()
		if err != nil {
			t.Fatalf("build: %v", err)
		}
		if cfg.CAs[0].EABKid != "kid" || cfg.CAs[0].Directory != config.ACMEZeroSSLDirectory {
			t.Fatalf("cas = %+v", cfg.CAs)
		}
	})

	t.Run("custom directory must be https", func(t *testing.T) {
		answers := defaultAcmeAnswers()
		answers.CA = acmeCACustom
		answers.Directory = "http://acme.example.com/directory"
		if _, err := answers.buildConfig(); err == nil {
			t.Fatal("insecure directory should be rejected")
		}
		answers.Directory = "https://acme.example.com/directory"
		cfg, err := answers.buildConfig()
		if err != nil {
			t.Fatalf("build: %v", err)
		}
		if cfg.CAs[0].Directory != answers.Directory {
			t.Fatalf("directory = %q", cfg.CAs[0].Directory)
		}
	})

	t.Run("enabled requires an email", func(t *testing.T) {
		answers := defaultAcmeAnswers()
		answers.Email = "  "
		if _, err := answers.buildConfig(); err == nil {
			t.Fatal("empty email should be rejected when enabled")
		}
		answers.Enabled = false
		if _, err := answers.buildConfig(); err != nil {
			t.Fatalf("disabled without email should be accepted: %v", err)
		}
	})

	t.Run("unknown challenge", func(t *testing.T) {
		answers := defaultAcmeAnswers()
		answers.Challenge = "dns-01"
		if _, err := answers.buildConfig(); err == nil {
			t.Fatal("unsupported challenge should be rejected")
		}
	})
}

func TestAcmeSetupNonInteractiveWrites(t *testing.T) {
	path := tempACMEConfig(t)
	var out bytes.Buffer

	if err := setupACME(defaultAcmeAnswers(), acmeOptions{Path: path, NonInteractive: true, Out: &out}); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if !strings.Contains(out.String(), "acme 配置已写入") {
		t.Fatalf("output = %q", out.String())
	}
	if !strings.Contains(out.String(), "reload 不会重建 ACME 管理器") {
		t.Fatal("next steps should warn that reload is not enough")
	}

	cfg := readAcmeSection(t, path)
	if !cfg.Enabled || cfg.Email != "ops@example.com" {
		t.Fatalf("acme = %+v", cfg)
	}
	if len(cfg.CAs) != 1 || cfg.CAs[0].Name != "letsencrypt" {
		t.Fatalf("cas = %+v", cfg.CAs)
	}
}

func TestAcmeSetupNonInteractiveRequiresEmail(t *testing.T) {
	path := tempACMEConfig(t)
	answers := defaultAcmeAnswers()
	answers.Email = ""

	err := setupACME(answers, acmeOptions{Path: path, NonInteractive: true, Out: &bytes.Buffer{}})
	if err == nil {
		t.Fatal("non-interactive setup without --email should fail")
	}
	if !strings.Contains(err.Error(), "--email") {
		t.Fatalf("error should mention --email: %v", err)
	}
}

func TestAcmeSetupDryRunKeepsFile(t *testing.T) {
	path := tempACMEConfig(t)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	answers := defaultAcmeAnswers()
	answers.RenewBeforeDays = 15
	var out bytes.Buffer
	if err := setupACME(answers, acmeOptions{Path: path, NonInteractive: true, DryRun: true, Out: &out}); err != nil {
		t.Fatalf("dry run: %v", err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("dry run must not touch the config file")
	}
	if !strings.Contains(out.String(), `"renew_before_days": 15`) {
		t.Fatalf("dry run output missing acme section:\n%s", out.String())
	}
	if !strings.Contains(out.String(), `"http"`) {
		t.Fatal("dry run should print the whole file")
	}
}

// 交互向导走 huh 的 accessible 模式：逐行回答，最后确认回车即写入。
func TestAcmeWizardAccessible(t *testing.T) {
	path := tempACMEConfig(t)
	input := newLineReader(
		"",                 // 启用 ACME：默认是
		"ops@example.com",  // 邮箱
		"1",                // CA：Let's Encrypt
		"",                 // 挑战：auto
		"",                 // 续期提前：30 天
		"config/cert/acme", // 缓存目录
		"",                 // 失败回落自签：默认是
		"",                 // 确认写入：默认是
	)
	var out bytes.Buffer

	answers := defaultAcmeAnswers()
	answers.Email = ""
	err := setupACME(answers, acmeOptions{Path: path, Accessible: true, In: input, Out: &out})
	if err != nil {
		t.Fatalf("wizard: %v\noutput:\n%s", err, out.String())
	}

	cfg := readAcmeSection(t, path)
	if !cfg.Enabled || cfg.Email != "ops@example.com" {
		t.Fatalf("acme = %+v\noutput:\n%s", cfg, out.String())
	}
	if len(cfg.CAs) != 1 || cfg.CAs[0].Name != "letsencrypt" {
		t.Fatalf("cas = %+v", cfg.CAs)
	}
}

func TestAcmeWizardCancelLeavesFileUntouched(t *testing.T) {
	path := tempACMEConfig(t)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	input := newLineReader(
		"",
		"ops@example.com",
		"1",
		"",
		"",
		"config/cert/acme",
		"",
		"n", // 确认写入：否
	)
	var out bytes.Buffer

	answers := defaultAcmeAnswers()
	answers.Email = ""
	if err := setupACME(answers, acmeOptions{Path: path, Accessible: true, In: input, Out: &out}); err == nil {
		t.Fatal("cancelling the confirmation should return an error")
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("cancelled wizard must not write the config")
	}
}

func TestAcmeShowMasksEABAndListsDomains(t *testing.T) {
	cfg := &config.AcmeConfig{
		Enabled:            true,
		Email:              "ops@example.com",
		Challenge:          []string{config.ACMEChallengeAuto},
		RenewBeforeDays:    30,
		FallbackSelfSigned: boolPtr(true),
		Storage:            "/var/lib/fast-https/acme",
		CAs: []config.ACMECA{{
			Name:       "zerossl",
			Directory:  config.ACMEZeroSSLDirectory,
			EABKid:     "kid-1",
			EABHmacKey: "super-secret-hmac",
		}},
	}
	plan := certmanager.DomainPlan{
		Managed: []string{"site.example"},
		Skipped: []certmanager.DomainSkip{{Name: "10.0.0.1", Reason: "IP addresses cannot be validated"}},
	}

	var out bytes.Buffer
	writeACMEShow(&out, cfg, plan)
	got := out.String()

	if strings.Contains(got, "super-secret-hmac") {
		t.Fatalf("EAB hmac key leaked:\n%s", got)
	}
	if !strings.Contains(got, "kid-1") {
		t.Fatalf("EAB kid should stay visible:\n%s", got)
	}
	if !strings.Contains(got, "site.example") || !strings.Contains(got, "10.0.0.1") {
		t.Fatalf("domain plan missing:\n%s", got)
	}

	out.Reset()
	writeACMEShow(&out, nil, certmanager.DomainPlan{})
	if !strings.Contains(out.String(), "未配置 acme 段") {
		t.Fatalf("missing hint for absent section:\n%s", out.String())
	}
}

func TestAcmeCmdTreeIsWired(t *testing.T) {
	root := RootCmd()
	found := root.Commands()
	names := make([]string, 0, len(found))
	hasACME := false
	for _, c := range found {
		names = append(names, c.Name())
		if c.Name() == "acme" {
			hasACME = true
			subs := []string{}
			for _, sub := range c.Commands() {
				subs = append(subs, sub.Name())
			}
			if len(subs) != 2 || !contains(subs, "setup") || !contains(subs, "show") {
				t.Fatalf("acme subcommands = %v, want setup and show", subs)
			}
		}
	}
	if !hasACME {
		t.Fatalf("acme command is not registered: %v", names)
	}
}

// TestAcmeSetupWarnsWhenNoSSLServer 守住这条提示：只有 listen 带 ssl 的站点
// 才会被 ACME 申请证书。一个这样的站点都没有时，重启也不会有任何证书请求，
// 提示必须直说并给出改法——否则用户只看到"配置已写入"，以为已经配好了。
func TestAcmeSetupWarnsWhenNoSSLServer(t *testing.T) {
	path := tempACMEConfig(t)
	defer withServers(t, []config.Server{{Listen: "80", ServerName: "example.com"}})()

	var out bytes.Buffer
	if err := setupACME(defaultAcmeAnswers(), acmeOptions{Path: path, NonInteractive: true, Out: &out}); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if !strings.Contains(out.String(), "不会发出任何证书请求") {
		t.Fatalf("output 应说明没有可申请的域名：%q", out.String())
	}
	if !strings.Contains(out.String(), "443 ssl h2") {
		t.Fatalf("output 应给出加 443 的具体写法：%q", out.String())
	}
}

func TestAcmeSetupQuietWhenSSLServerExists(t *testing.T) {
	path := tempACMEConfig(t)
	defer withServers(t, []config.Server{{Listen: "443 ssl h2", ServerName: "example.com"}})()

	var out bytes.Buffer
	if err := setupACME(defaultAcmeAnswers(), acmeOptions{Path: path, NonInteractive: true, Out: &out}); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if strings.Contains(out.String(), "不会发出任何证书请求") {
		t.Fatalf("有 ssl 站点时不应提示无域名：%q", out.String())
	}
}

// withServers 临时替换配置快照，返回恢复函数。
func withServers(t *testing.T, servers []config.Server) func() {
	t.Helper()
	old := config.Current()
	config.Store(&config.Fast_Https{Servers: servers}, nil)
	return func() { config.Store(old, nil) }
}

func boolPtr(v bool) *bool { return &v }

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
