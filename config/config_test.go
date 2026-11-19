package config_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"fast-https/config"

	"go.yaml.in/yaml/v3"
)

// dataPath 返回 config/testdata 下相对路径的绝对路径，不受测试切换工作目录影响。
func dataPath(name string) string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "testdata", filepath.FromSlash(name))
}

// loadCases 读取 testdata 下的 YAML 用例数据。
func loadCases(t *testing.T, name string, v any) {
	t.Helper()
	raw, err := os.ReadFile(dataPath(name))
	if err != nil {
		t.Fatalf("read test data %s: %v", name, err)
	}
	if err := yaml.Unmarshal(raw, v); err != nil {
		t.Fatalf("parse test data %s: %v", name, err)
	}
}

// repoRoot 把工作目录切到仓库根（LoadFile 依赖相对路径读 mime.json），
// 用例结束后恢复。
func repoRoot(t *testing.T) {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := wd
	for {
		if _, statErr := os.Stat(filepath.Join(root, "go.mod")); statErr == nil {
			break
		}
		next := filepath.Dir(root)
		if next == root {
			t.Fatal("cannot find repository root")
		}
		root = next
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
}

// snapshotConfig 在用例结束后恢复配置与 MIME 快照，避免影响同包其他用例。
func snapshotConfig(t *testing.T) {
	t.Helper()
	previous := *config.Current()
	previousMIME := config.MIME()
	t.Cleanup(func() { config.Store(&previous, previousMIME) })
}

func writeConfigFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// tempConfig 写一份主配置到临时目录，返回路径。
func tempConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fast-https.json")
	writeConfigFile(t, path, body)
	return path
}

// ---- mime 表加载 ----

// mime.json 跟着主配置所在目录走：--config 指到别的目录时，不用把
// mime.json 摆到进程工作目录下也能读到。
func TestMIMEFollowsConfigDir(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "fast-https.json")
	writeConfigFile(t, configPath, `{
  "http": {
    "server": [
      {"listen": 80, "server_name": "local", "location": [{"url": "/", "type": "local", "root": "."}]}
    ]
  }
}`)
	writeConfigFile(t, filepath.Join(dir, "mime.json"), `{"weird": "application/x-weird"}`)
	snapshotConfig(t)

	if err := config.LoadFile(configPath); err != nil {
		t.Fatalf("load config: %v", err)
	}
	if got := config.MIME()["weird"]; got != "application/x-weird" {
		t.Fatalf("mime table = %q for weird, want the one next to the config", got)
	}
}

// 配置目录里没有 mime.json 时回落到安装基准目录下的 config/mime.json。
func TestMIMEFallsBackToBaseDir(t *testing.T) {
	configPath := tempConfig(t, `{
  "http": {
    "server": [
      {"listen": 80, "server_name": "local", "location": [{"url": "/", "type": "local", "root": "."}]}
    ]
  }
}`)
	repoRoot(t)
	snapshotConfig(t)

	if err := config.LoadFile(configPath); err != nil {
		t.Fatalf("load config: %v", err)
	}
	if got := config.MIME()["css"]; got != "text/css" {
		t.Fatalf("fallback mime table = %q for css, want text/css", got)
	}
}

// ---- 加载与相对路径解析 ----

func TestLoadFileResolvesRelativePaths(t *testing.T) {
	repoRoot(t)
	snapshotConfig(t)

	path := tempConfig(t, `{
  "http": {
    "server": [
      {
        "listen": "443 ssl",
        "server_name": "localhost",
        "ssl_certificate": "config/cert/localhost.pem",
        "ssl_certificate_key": "config/cert/localhost-key.pem",
        "location": [
          {"url": "/", "type": "local", "root": "./httpdoc/root", "index": ["index.html"]}
        ]
      }
    ],
    "include": ["./config/conf.d"]
  }
}`)
	if err := config.LoadFile(path); err != nil {
		t.Fatalf("load config: %v", err)
	}
	server := config.Current().Servers[0]
	if !filepath.IsAbs(server.Path[0].Root) {
		t.Fatalf("site root = %q", server.Path[0].Root)
	}
	if !filepath.IsAbs(server.SSLCertificate) || !filepath.IsAbs(server.SSLCertificateKey) {
		t.Fatalf("cert = %q key = %q", server.SSLCertificate, server.SSLCertificateKey)
	}
	if !filepath.IsAbs(config.Current().LogRoot) {
		t.Fatalf("log root = %q", config.Current().LogRoot)
	}
	if len(config.Current().Include) == 0 || !filepath.IsAbs(config.Current().Include[0]) {
		t.Fatalf("include = %#v", config.Current().Include)
	}
}

// ---- reload 语义 ----

func TestReloadFileKeepsPreviousConfigWhenInvalid(t *testing.T) {
	repoRoot(t)
	snapshotConfig(t)

	path := tempConfig(t, `{
  "http": {
    "server": [
      {
        "listen": 8080,
        "server_name": "keep.local",
        "location": [
          {"url": "/", "type": "local", "root": "./httpdoc/root", "index": ["index.html"]}
        ]
      }
    ]
  }
}`)
	if err := config.LoadFile(path); err != nil {
		t.Fatalf("load valid config: %v", err)
	}
	if got := config.Current().Servers[0].ServerName; got != "keep.local" {
		t.Fatalf("server name = %q", got)
	}

	writeConfigFile(t, path, "{")
	if err := config.ReloadFile(path); err == nil {
		t.Fatal("invalid config should be rejected")
	}
	if got := config.Current().Servers[0].ServerName; got != "keep.local" {
		t.Fatalf("invalid reload changed server name to %q", got)
	}

	writeConfigFile(t, path, `{
  "http": {
    "server": [
      {
        "listen": 8081,
        "server_name": "next.local",
        "location": [
          {"url": "/", "type": "local", "root": "./httpdoc/root", "index": ["index.html"]}
        ]
      }
    ]
  }
}`)
	if err := config.ReloadFile(path); err != nil {
		t.Fatalf("valid reload: %v", err)
	}
	if got := config.Current().Servers[0].ServerName; got != "next.local" {
		t.Fatalf("server name = %q", got)
	}
}

func TestReloadFileRejectsMissingCertificate(t *testing.T) {
	repoRoot(t)
	snapshotConfig(t)

	path := tempConfig(t, `{
  "http": {
    "server": [
      {
        "listen": 8080,
        "server_name": "keep.local",
        "location": [
          {"url": "/", "type": "local", "root": "./httpdoc/root", "index": ["index.html"]}
        ]
      }
    ]
  }
}`)
	if err := config.LoadFile(path); err != nil {
		t.Fatal(err)
	}

	writeConfigFile(t, path, `{
  "http": {
    "server": [
      {
        "listen": "8443 ssl",
        "server_name": "broken.local",
        "ssl_certificate": "missing.pem",
        "ssl_certificate_key": "missing-key.pem",
        "location": [
          {"url": "/", "type": "local", "root": "./httpdoc/root", "index": ["index.html"]}
        ]
      }
    ]
  }
}`)
	if err := config.ReloadFile(path); err == nil {
		t.Fatal("missing certificate should be rejected")
	}
	if got := config.Current().Servers[0].ServerName; got != "keep.local" {
		t.Fatalf("server name = %q", got)
	}
}

// TestReloadInvalidKeepsPreviousConfig 覆盖「reload 失败不能把服务带进半死
// 状态」：新配置非法时，运行时快照必须保持 reload 之前的样子。
//
// ReloadFile 是「先校验、再解析、最后 Store」的顺序，任何一步失败都不该
// 触碰 Current()。这条用例把那个顺序钉死——一旦有人改成先清空再加载，
// 正在服务的实例会拿着空配置继续跑。
func TestReloadInvalidKeepsPreviousConfig(t *testing.T) {
	prev := config.Current()
	t.Cleanup(func() { config.Store(prev, map[string]string{}) })

	const marker = "before-reload"
	config.Store(&config.Fast_Https{LogRoot: marker}, map[string]string{})

	path := dataPath("invalid-listen.json")
	if err := config.ReloadFile(path); err == nil {
		t.Fatal("listen 非法的配置应当校验失败")
	}

	got := config.Current()
	if got == nil {
		t.Fatal("reload 失败后 Current() 变成 nil")
	}
	if got.LogRoot != marker {
		t.Fatalf("reload 失败后旧配置被改动：LogRoot=%q，期望 %q", got.LogRoot, marker)
	}
}

// TestReloadMissingFileKeepsPreviousConfig 覆盖「配置文件不存在」这条路径：
// 同样不能把快照清空。
func TestReloadMissingFileKeepsPreviousConfig(t *testing.T) {
	prev := config.Current()
	t.Cleanup(func() { config.Store(prev, map[string]string{}) })

	const marker = "before-missing"
	config.Store(&config.Fast_Https{LogRoot: marker}, map[string]string{})

	if err := config.ReloadFile(dataPath("does-not-exist.json")); err == nil {
		t.Fatal("不存在的配置应当报错")
	}
	if got := config.Current(); got == nil || got.LogRoot != marker {
		t.Fatalf("reload 失败后旧配置被改动：%+v", got)
	}
}

// 配置已改为 atomic 快照（架构收敛计划阶段 4b），本测试在 -race 下运行，
// 验证 reload 期间读取端永远看到完整的新配置或完整的旧配置。
func TestReloadNeverExposesClearedConfig(t *testing.T) {
	repoRoot(t)
	snapshotConfig(t)

	path := tempConfig(t, `{
  "http": {
    "server": [
      {
        "listen": 8080,
        "server_name": "window.local",
        "location": [
          {"url": "/", "type": "local", "root": "./httpdoc/root", "index": ["index.html"]}
        ]
      }
    ]
  }
}`)
	if err := config.LoadFile(path); err != nil {
		t.Fatal(err)
	}

	var bad atomic.Int64
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
			}
			if config.Current().Limit.MaxBodySize == 0 || len(config.Current().Servers) == 0 {
				bad.Add(1)
			}
			if config.MIME()["css"] == "" {
				bad.Add(1)
			}
		}
	}()

	for i := 0; i < 50; i++ {
		if err := config.ReloadFile(path); err != nil {
			close(stop)
			<-done
			t.Fatalf("reload %d: %v", i, err)
		}
	}
	close(stop)
	<-done

	if n := bad.Load(); n != 0 {
		t.Fatalf("reader saw cleared config or MIME table %d times", n)
	}
}

// ---- acme 段 ----

// acmeTestEnv 准备一个可加载的最小配置环境：工作目录切到仓库根，
// 并在用例结束后恢复配置快照。
func acmeTestEnv(t *testing.T) {
	t.Helper()
	repoRoot(t)
	snapshotConfig(t)
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
	writeConfigFile(t, path, body)
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

	var cases []struct {
		Name     string `yaml:"name"`
		AcmeJSON string `yaml:"acme_json"`
		Want     string `yaml:"want"`
	}
	loadCases(t, "acme/invalid.yaml", &cases)
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			block := ", \"acme\": " + tc.AcmeJSON
			path := writeACMETestConfig(t, acmeTestConfig(block))
			err := config.ValidateConfigFile(path)
			if err == nil {
				t.Fatalf("want error containing %q, got nil", tc.Want)
			}
			if !strings.Contains(err.Error(), tc.Want) {
				t.Fatalf("error = %v, want containing %q", err, tc.Want)
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
