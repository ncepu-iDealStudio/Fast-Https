package validate

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"fast-https/config/internal/model"
	"fast-https/config/internal/paths"
	"fast-https/config/internal/scan"
	"fast-https/config/internal/schema"
	"fast-https/config/internal/sitefile"
	"fast-https/config/internal/testutil"
	"fast-https/infra/logging"

	"github.com/rs/zerolog"
)

// writeTree 写一个主配置加一个 include 站点，返回主配置路径。
func writeTree(t *testing.T, acme, siteListen string) string {
	t.Helper()
	dir := t.TempDir()
	sites := filepath.Join(dir, "conf.d")
	if err := os.MkdirAll(sites, 0o755); err != nil {
		t.Fatal(err)
	}
	site := fmt.Sprintf(`{"listen": %s, "server_name": "example.com",
  "location": [{"url": "/", "type": "local", "root": %q}]}`, siteListen, dir)
	if err := os.WriteFile(filepath.Join(sites, "site.json"), []byte(site), 0o644); err != nil {
		t.Fatal(err)
	}
	main := fmt.Sprintf(`{"acme": %s, "http": {"include": [%q]}}`, acme, sites)
	path := filepath.Join(dir, "fast-https.json")
	if err := os.WriteFile(path, []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

const http01Acme = `{"enabled": true, "email": "ops@example.com", "challenge": ["http-01"]}`

func TestValidateWithWarningsReturnsAndDoesNotLog(t *testing.T) {
	path := writeTree(t, http01Acme, `[8080]`)

	var buf bytes.Buffer
	old := logging.ConsoleLog
	logging.ConsoleLog = zerolog.New(&buf)
	t.Cleanup(func() { logging.ConsoleLog = old })

	warnings, err := ValidateConfigFileWithWarnings(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "no http.server listens on port 80") {
		t.Fatalf("warnings = %v", warnings)
	}
	if buf.Len() != 0 {
		t.Fatalf("带告警的入口不应写日志：%s", buf.String())
	}
}

func TestValidateWithWarningsQuietWhenPort80Listens(t *testing.T) {
	path := writeTree(t, http01Acme, `[80]`)
	warnings, err := ValidateConfigFileWithWarnings(path)
	if err != nil || len(warnings) != 0 {
		t.Fatalf("warnings = %v, err = %v", warnings, err)
	}
}

func TestValidateWithWarningsFailureHasNoWarnings(t *testing.T) {
	path := writeTree(t, http01Acme, `[]`)
	warnings, err := ValidateConfigFileWithWarnings(path)
	if err == nil {
		t.Fatal("没有 listen 的站点应校验失败")
	}
	if warnings != nil {
		t.Fatalf("失败时不返回告警：%v", warnings)
	}
}

// 旧入口的行为不变：仍然写同一条日志，返回值仍是 error。
func TestValidateConfigFileStillLogsWarning(t *testing.T) {
	path := writeTree(t, http01Acme, `[8080]`)

	var buf bytes.Buffer
	old := logging.ConsoleLog
	logging.ConsoleLog = zerolog.New(&buf)
	t.Cleanup(func() { logging.ConsoleLog = old })

	if err := ValidateConfigFile(path); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "no http.server listens on port 80") {
		t.Fatalf("旧入口应继续写告警日志：%s", buf.String())
	}
}

func TestValidateConfigFileUnchanged(t *testing.T) {
	path := writeTree(t, `{"enabled": false}`, `[8080]`)
	warnings, err := ValidateConfigFileWithWarnings(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("无 http-01 时不应有告警：%v", warnings)
	}

	path = writeTree(t, http01Acme, `[8080]`)
	warnings, err = ValidateConfigFileWithWarnings(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "no http.server listens on port 80") {
		t.Fatalf("http-01 告警快照变了：%v", warnings)
	}

	dir := t.TempDir()
	main := filepath.Join(dir, "fast-https.json")
	if err := os.WriteFile(main, []byte(`{
  "http": {
    "server": [{"listen": "bad-port", "server_name": "x", "location": [{"url": "/", "root": "."}]}]
  }
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = ValidateConfigFileWithWarnings(main)
	if err == nil || !strings.Contains(err.Error(), "invalid listen port: bad-port") {
		t.Fatalf("无效 listen 快照变了：%v", err)
	}
}

func TestListenPortsFromFile(t *testing.T) {
	path := writeTree(t, `{"enabled": false}`, `[8080, "8443 ssl h2", 8080]`)

	ports, err := scan.ListenPortsFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := []int{8080, 8443}; !reflect.DeepEqual(ports, want) {
		t.Fatalf("ports = %v, want %v", ports, want)
	}
}

func TestListenPortsFromFileMissing(t *testing.T) {
	if _, err := scan.ListenPortsFromFile(filepath.Join(t.TempDir(), "nope.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err = %v, want ErrNotExist", err)
	}
}

func TestSnapshotServerNotFoundIsSentinel(t *testing.T) {
	path := writeTree(t, `{"enabled": false}`, `[8080]`)

	_, _, err := sitefile.SnapshotServer(path, "missing.example")
	if !errors.Is(err, sitefile.ErrNoServerBlock) {
		t.Fatalf("err = %v, want ErrNoServerBlock", err)
	}
	// 文案保持原样：旧命令直接把它打印给用户。
	if want := `no server block with server_name "missing.example"`; err.Error() != want {
		t.Fatalf("err = %q, want %q", err.Error(), want)
	}
}

// TestWarnHTTP01ConsidersIncludedSites 守一个很容易误报的判断：
// 站点通常只写在 conf.d 里（由 http.include 展开），主配置的 http.server 是空的。
// 只看主配置就会对"80 明明在听"的站点配置告警，
// 把真正的配置问题（没有 443、没有公网域名）盖在一条假警告下面。
func TestWarnHTTP01ConsidersIncludedSites(t *testing.T) {
	cases := []struct {
		name     string
		listen   string
		wantWarn bool
	}{
		{"include 里的站点监听 80 时不告警", `[80, "443 ssl h2"]`, false},
		{"include 里的站点没有 80 时告警", `["443 ssl h2"]`, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			sites := filepath.Join(dir, "conf.d")
			if err := os.MkdirAll(sites, 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			cert := filepath.Join(dir, "site.pem")
			key := filepath.Join(dir, "site-key.pem")
			for _, path := range []string{cert, key} {
				if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
					t.Fatalf("write %s: %v", path, err)
				}
			}

			site := fmt.Sprintf(`{
  "listen": %s,
  "server_name": "example.com",
  "ssl_certificate": %q,
  "ssl_certificate_key": %q,
  "location": [{"url": "/", "type": "local", "root": %q}]
}`, c.listen, cert, key, dir)
			if err := os.WriteFile(filepath.Join(sites, "site.json"), []byte(site), 0o644); err != nil {
				t.Fatalf("write site: %v", err)
			}

			main := fmt.Sprintf(`{
  "acme": {"enabled": true, "email": "ops@example.com", "challenge": ["http-01"]},
  "http": {"include": [%q]}
}`, sites)
			mainPath := filepath.Join(dir, "fast-https.json")
			if err := os.WriteFile(mainPath, []byte(main), 0o644); err != nil {
				t.Fatalf("write main: %v", err)
			}

			var buf bytes.Buffer
			old := logging.ConsoleLog
			logging.ConsoleLog = zerolog.New(&buf)
			t.Cleanup(func() { logging.ConsoleLog = old })

			if err := ValidateConfigFile(mainPath); err != nil {
				t.Fatalf("validate: %v", err)
			}

			warned := strings.Contains(buf.String(), "no http.server listens on port 80")
			if warned != c.wantWarn {
				t.Fatalf("warned = %v, want %v；输出：%s", warned, c.wantWarn, buf.String())
			}
		})
	}
}

// auth 段：只支持 basic，且 user/pswd 必须写全，realm 不能破坏响应头。
func TestValidateLocationAuth(t *testing.T) {
	var cases []struct {
		Name      string `yaml:"name"`
		Type      string `yaml:"type"`
		User      string `yaml:"user"`
		Pswd      string `yaml:"pswd"`
		Realm     string `yaml:"realm"`
		WantError string `yaml:"want_error"`
	}
	testutil.LoadYAML(t, "validate/location_auth.yaml", &cases)

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			auth := model.PathAuth{AuthType: c.Type, User: c.User, Pswd: c.Pswd, Realm: c.Realm}
			err := validateLocationAuth(&auth, "http.server[0]", 0)
			if c.WantError == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.WantError) {
				t.Fatalf("error = %v, want it to contain %q", err, c.WantError)
			}
		})
	}
}

// validateServer 必须真正调用 auth 校验，否则只写 type 的站点仍可保存。
func TestValidateServerRejectsIncompleteAuth(t *testing.T) {
	server := schema.Server{
		Listen:     schema.FlexList{"80"},
		ServerName: "local",
		Locations: []schema.Path{{
			URL:  "/admin",
			Type: "local",
			Root: ".",
			Auth: model.PathAuth{AuthType: "basic", User: "alice"},
		}},
	}

	err := validateServer(&server, ".", ".", "site", false)
	if err == nil || !strings.Contains(err.Error(), "auth.pswd is required") {
		t.Fatalf("error = %v, want incomplete auth rejected", err)
	}
}

// 多监听里只要有一条带 ssl 就要求证书；端口本身仍逐条校验。
func TestValidateServerMultipleListens(t *testing.T) {
	var cases []struct {
		Name      string   `yaml:"name"`
		Listen    []string `yaml:"listen"`
		Cert      string   `yaml:"ssl_certificate"`
		Key       string   `yaml:"ssl_certificate_key"`
		WantError string   `yaml:"want_error"`
	}
	testutil.LoadYAML(t, "validate/server_listens.yaml", &cases)

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			server := schema.Server{
				Listen:            schema.FlexList(c.Listen),
				ServerName:        "multi.local",
				SSLCertificate:    c.Cert,
				SSLCertificateKey: c.Key,
				Locations:         []schema.Path{{URL: "/", Type: "local", Root: "./httpdoc/root"}},
			}
			err := validateServer(&server, ".", ".", "site", false)
			if c.WantError == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.WantError) {
				t.Fatalf("error = %v, want it to contain %q", err, c.WantError)
			}
		})
	}
}

func TestAllowMethodsValidated(t *testing.T) {
	var cases []struct {
		Name      string   `yaml:"name"`
		Methods   []string `yaml:"methods"`
		WantError string   `yaml:"want_error"`
	}
	testutil.LoadYAML(t, "validate/allow_methods.yaml", &cases)

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			server := schema.Server{
				Listen:     schema.FlexList{"80"},
				ServerName: "a.example",
				Locations: []schema.Path{{
					URL: "/", Type: "local", Root: "./httpdoc/root", AllowMethods: c.Methods,
				}},
			}
			err := validateServer(&server, ".", ".", "site", false)
			if err == nil || !strings.Contains(err.Error(), c.WantError) {
				t.Fatalf("error = %v, want it to contain %q", err, c.WantError)
			}
		})
	}
}

func TestValidateConfigFileAcceptsComments(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "fast-https.json")
	body := `{
  // 主配置
  "http": {
    "server": [
      {
        "listen": 80,
        "server_name": "localhost",
        "location": [
          {"url": "/", "type": "local", "root": "./httpdoc/root"}, // 静态根
        ],
      },
    ],
    "include": []
  }
}`
	if err := os.WriteFile(main, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ValidateConfigFile(main); err != nil {
		t.Fatalf("commented config should validate: %v", err)
	}
}

func TestValidateIncludeAcceptsComments(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "fast-https.json")
	include := filepath.Join(dir, "conf.d", "site.json")

	if err := os.MkdirAll(filepath.Dir(include), 0o755); err != nil {
		t.Fatal(err)
	}
	// include 用绝对路径：相对路径的解析基准是安装目录而不是配置文件目录，
	// 那是既有行为，不属于本用例要覆盖的范围。
	if err := os.WriteFile(main, []byte(`{
  "http": {
    "server": [{"listen": 80, "server_name": "a.local", "location": [{"url": "/", "type": "local", "root": "./httpdoc/root"}]}],
    "include": ["`+filepath.ToSlash(filepath.Dir(include))+`"]
  }
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(include, []byte(`{
  // 拆分出来的站点
  "listen": "8081",
  "server_name": "api.local",
  "location": [{"url": "/api", "type": "proxy", "proxy_pass": "http://127.0.0.1:9000"}],
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ValidateConfigFile(main); err != nil {
		t.Fatalf("commented include should validate: %v", err)
	}
}

func TestValidateSiteFileReportsFieldPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	raw := `{
  "listen": 80,
  "server_name": "bad.example",
  "location": [{ "url": "/api", "type": "proxy" }]
}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	err := ValidateSiteFile(path)
	if err == nil {
		t.Fatal("proxy location without proxy_pass should be rejected")
	}
	if !strings.Contains(err.Error(), "proxy_pass") {
		t.Fatalf("error should name the field: %v", err)
	}
	if !strings.Contains(err.Error(), path) {
		t.Fatalf("error should name the file: %v", err)
	}
}

func symlinkPolicyConfig(root, policy string) string {
	policyLine := ""
	if policy != "" {
		policyLine = `, "symlink_policy": "` + policy + `"`
	}
	return `{
  "http": {
    "server": [{
      "listen": 18081,
      "server_name": "localhost",
      "location": [{
        "url": "/", "type": "local",
        "root": "` + filepath.ToSlash(root) + `"` + policyLine + `,
        "index": ["index.html"]
      }]
    }]
  }
}`
}

func writeSymlinkPolicyConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fast-https.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// rootWithSymlink 返回一个内部含符号链接的站点目录。
func rootWithSymlink(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skipf("创建符号链接失败：%v", err)
	}
	return root
}

// TestCheckAtLoadRejectsSymlink 默认策略在加载配置时拒绝含符号链接的 root。
func TestCheckAtLoadRejectsSymlink(t *testing.T) {
	root := rootWithSymlink(t)
	err := ValidateConfigFile(writeSymlinkPolicyConfig(t, symlinkPolicyConfig(root, "")))
	if err == nil {
		t.Fatal("默认 check-at-load 应拒绝含符号链接的 root")
	}
	if !strings.Contains(err.Error(), "symlink found under root") {
		t.Fatalf("错误信息应说明符号链接，得到：%v", err)
	}
}

// TestSymlinkPolicyAllowSkipsLoadCheck allow 策略保持旧行为。
func TestSymlinkPolicyAllowSkipsLoadCheck(t *testing.T) {
	root := rootWithSymlink(t)
	if err := ValidateConfigFile(writeSymlinkPolicyConfig(t, symlinkPolicyConfig(root, model.SYMLINK_ALLOW))); err != nil {
		t.Fatalf("allow 不应因符号链接拒绝加载：%v", err)
	}
	if err := ValidateConfigFile(writeSymlinkPolicyConfig(t, symlinkPolicyConfig(root, model.SYMLINK_REJECT))); err != nil {
		t.Fatalf("reject 交给运行期处理，加载不应失败：%v", err)
	}
}

// TestSymlinkPolicyInvalid 未知取值直接报错，避免拼错后静默退回默认。
func TestSymlinkPolicyInvalid(t *testing.T) {
	root := t.TempDir()
	err := ValidateConfigFile(writeSymlinkPolicyConfig(t, symlinkPolicyConfig(root, "deny")))
	if err == nil {
		t.Fatal("非法 symlink_policy 应被拒绝")
	}
	if !strings.Contains(err.Error(), "symlink_policy is invalid") {
		t.Fatalf("错误信息应指出策略非法，得到：%v", err)
	}
}

// TestSymlinkPolicyDefaultNormalized 空值按 check-at-load 转换。
func TestSymlinkPolicyDefaultNormalized(t *testing.T) {
	if got := model.NormalizeSymlinkPolicy(" "); got != model.SYMLINK_CHECK_AT_LOAD {
		t.Fatalf("空值应归一化为 %s，得到 %s", model.SYMLINK_CHECK_AT_LOAD, got)
	}
	if got := model.NormalizeSymlinkPolicy(" ALLOW "); got != model.SYMLINK_ALLOW {
		t.Fatalf("大小写与空白应被归一化，得到 %s", got)
	}
}

func writeCandidateTree(t *testing.T, siteBody string) string {
	t.Helper()
	dir := t.TempDir()
	confd := filepath.Join(dir, "conf.d")
	if err := os.MkdirAll(confd, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(confd, "site.json"), []byte(siteBody), 0o644); err != nil {
		t.Fatal(err)
	}
	main := `{"http":{"include":["conf.d"]}}`
	if err := os.WriteFile(filepath.Join(dir, "fast-https.json"), []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

const goodCandidateSite = `{"listen":[8080],"server_name":"example.com","location":[{"url":"/","type":"local","root":"."}]}`

func TestTreeValidateDoesNotReadLiveConfd(t *testing.T) {
	cand := writeCandidateTree(t, goodCandidateSite)

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	live := t.TempDir()
	if err := os.Chdir(live); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	if err := os.MkdirAll(filepath.Join("config", "conf.d"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("config", "conf.d", "evil.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("config", "fast-https.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := ValidateConfigTree(cand); err != nil {
		t.Fatalf("候选树应在线上 conf.d 不可用/有毒时仍通过：%v", err)
	}
}

func TestTreeValidateReportsCandidateErrors(t *testing.T) {
	cand := writeCandidateTree(t, `{`)
	_, err := ValidateConfigTree(cand)
	if err == nil {
		t.Fatal("坏的候选站点文件应失败")
	}
	bad := filepath.Join(cand, "conf.d", "site.json")
	if !strings.Contains(err.Error(), bad) {
		t.Fatalf("错误应指向候选文件 %s，实际：%v", bad, err)
	}
}

func TestTreeWithOnlyDisabledSitesIsValid(t *testing.T) {
	// 把唯一站点停用时，候选树里只剩 *.json.disabled。事务的
	// ValidateConfigTree 不应报 "no server configured" 而让 site disable 失败。
	dir := t.TempDir()
	confd := filepath.Join(dir, "conf.d")
	if err := os.MkdirAll(confd, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(confd, "site.json.disabled"), []byte(goodCandidateSite), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "fast-https.json"), []byte(`{"http":{"include":["conf.d"]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateConfigTree(dir); err != nil {
		t.Fatalf("只有停用站点的候选树应通过（证明配置过站点）：%v", err)
	}

	// 但「一个站点文件都没有」仍然要报错——别把语义放宽成永远通过。
	empty := t.TempDir()
	if err := os.MkdirAll(filepath.Join(empty, "conf.d"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(empty, "fast-https.json"), []byte(`{"http":{"include":["conf.d"]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ValidateConfigTree(empty)
	if err == nil || !strings.Contains(err.Error(), "no server configured") {
		t.Fatalf("空 conf.d 仍应报 no server configured，实际：%v", err)
	}
}

// TestTreeAutoCertMissingIsWarning 约定证书（<主域名>.pem）在候选树里缺失
// 只告警：候选树按设计不带证书文件，证书要等首次 start 由本地 CA 生成。
// 装完包还没启动过、或新增 TLS 站点，都不该被这条拦住。
func TestTreeAutoCertMissingIsWarning(t *testing.T) {
	site := `{
  "listen": ["8443 ssl"],
  "server_name": "localhost",
  "ssl_certificate": "./cert/localhost.pem",
  "ssl_certificate_key": "./cert/localhost-key.pem",
  "location": [{"url": "/", "type": "local", "root": "."}]
}`
	cand := writeCandidateTree(t, site)
	warnings, err := ValidateConfigTree(cand)
	if err != nil {
		t.Fatalf("约定证书缺失应降级为告警：%v", err)
	}
	if len(warnings) == 0 || !strings.Contains(strings.Join(warnings, "\n"), "ssl_certificate not found") {
		t.Fatalf("应给出证书缺失告警，实际：%v", warnings)
	}
}

// TestTreeCertResolvedFromLiveDir 相对证书路径要能按「线上配置目录」解析，
// 不能依赖进程工作目录：桌面端从快捷方式启动时 CWD 未必是安装目录。
func TestTreeCertResolvedFromLiveDir(t *testing.T) {
	site := `{
  "listen": ["8443 ssl"],
  "server_name": "dep.example",
  "ssl_certificate": "./cert/dep.example.pem",
  "ssl_certificate_key": "./cert/dep.example-key.pem",
  "location": [{"url": "/", "type": "local", "root": "."}]
}`
	cand := writeCandidateTree(t, site)

	live := t.TempDir()
	if err := os.MkdirAll(filepath.Join(live, "cert"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"dep.example.pem", "dep.example-key.pem"} {
		if err := os.WriteFile(filepath.Join(live, "cert", name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	warnings, err := ValidateConfigTree(cand, live)
	if err != nil {
		t.Fatalf("证书在线上配置目录里，应校验通过：%v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("证书找得到就不应告警：%v", warnings)
	}
}

func TestTreeMissingCert(t *testing.T) {
	site := `{
  "listen": ["8443 ssl"],
  "server_name": "ssl.example",
  "ssl_certificate": "no-such-s7-cert.pem",
  "ssl_certificate_key": "no-such-s7-key.pem",
  "location": [{"url": "/", "type": "local", "root": "."}]
}`
	cand := writeCandidateTree(t, site)
	_, err := ValidateConfigTree(cand)
	if err == nil {
		t.Fatal("缺证书应 CONFIG_INVALID")
	}
	if !strings.Contains(err.Error(), "ssl_certificate not found") {
		t.Fatalf("err = %v", err)
	}
}

func TestCandidatePathRejectsDotDot(t *testing.T) {
	_, err := paths.ResolveCandidateRoot("../x")
	if !errors.Is(err, paths.ErrCandidatePath) {
		t.Fatalf("err = %v, want ErrCandidatePath", err)
	}
}

func TestCandidateSymlinkEscape(t *testing.T) {
	cand := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(cand, "fast-https.json"), []byte(`{"http":{"include":["conf.d"]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(cand, "conf.d")
	if err := os.Symlink(outside, target); err != nil {
		t.Skipf("无法创建符号链接：%v", err)
	}
	if err := os.WriteFile(filepath.Join(outside, "site.json"), []byte(goodCandidateSite), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ValidateConfigTree(cand)
	if !errors.Is(err, paths.ErrCandidatePath) {
		t.Fatalf("符号链接逃逸 err = %v, want ErrCandidatePath", err)
	}
}

func TestTreeValidateAcceptsRuntimeCert(t *testing.T) {
	certDir := t.TempDir()
	cert := filepath.Join(certDir, "s7-runtime.pem")
	key := filepath.Join(certDir, "s7-runtime-key.pem")
	if err := os.WriteFile(cert, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(key, []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	site := fmt.Sprintf(`{
  "listen": ["8443 ssl"],
  "server_name": "ssl.example",
  "ssl_certificate": %q,
  "ssl_certificate_key": %q,
  "location": [{"url": "/", "type": "local", "root": "."}]
}`, cert, key)
	cand := writeCandidateTree(t, site)
	if _, err := ValidateConfigTree(cand); err != nil {
		t.Fatalf("绝对路径证书应通过：%v", err)
	}
}

// TestValidateProxy 校验 proxy 段：trusted_proxies 每项是 IP 或 CIDR，
// max_hops 不能为负。错误信息要能定位到具体字段。
func TestValidateProxy(t *testing.T) {
	dir := t.TempDir()
	writeConfig := func(t *testing.T, proxyBody string) string {
		t.Helper()
		body := `{"proxy": ` + proxyBody + `,
  "http": {"server": [{"listen": 8080, "server_name": "proxy.local",
    "location": [{"url": "/", "type": "local", "root": "."}]}]}}`
		path := filepath.Join(dir, "fast-https.json")
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}

	cases := []struct {
		name  string
		proxy string
		want  string
	}{
		{name: "未配置", proxy: `{}`},
		{name: "CIDR 与单 IP", proxy: `{"trusted_proxies": ["10.0.0.0/8", "192.168.1.10", "2001:db8::/32"]}`},
		{name: "链长不限制", proxy: `{"forwarded_for": {"max_hops": 0}}`},
		{name: "非法 CIDR", proxy: `{"trusted_proxies": ["10.0.0.0/33"]}`, want: "proxy.trusted_proxies[0]"},
		{name: "空项", proxy: `{"trusted_proxies": ["10.0.0.1", "  "]}`, want: "proxy.trusted_proxies[1]"},
		{name: "负数链长", proxy: `{"forwarded_for": {"max_hops": -1}}`, want: "proxy.forwarded_for.max_hops"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateConfigFile(writeConfig(t, c.proxy))
			if c.want == "" {
				if err != nil {
					t.Fatalf("应通过校验：%v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want containing %q", err, c.want)
			}
		})
	}
}

// TestValidateDefaultCertificate 校验 tls.default_certificate：要么不配，
// 要么证书/私钥成对且文件存在。
func TestValidateDefaultCertificate(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"default.pem", "default-key.pem"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeConfig := func(t *testing.T, tlsBody string) string {
		t.Helper()
		body := `{"tls": ` + tlsBody + `,
  "http": {"server": [{"listen": 8080, "server_name": "tls.local",
    "location": [{"url": "/", "type": "local", "root": "."}]}]}}`
		path := filepath.Join(dir, "fast-https.json")
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}

	cases := []struct {
		name string
		tls  string
		want string
	}{
		{name: "未配置", tls: `{}`},
		{name: "成对存在", tls: `{"default_certificate": {"certificate": "default.pem", "key": "default-key.pem"}}`},
		{name: "只写证书", tls: `{"default_certificate": {"certificate": "default.pem"}}`, want: "set together"},
		{name: "只写私钥", tls: `{"default_certificate": {"key": "default-key.pem"}}`, want: "set together"},
		{name: "证书不存在", tls: `{"default_certificate": {"certificate": "missing.pem", "key": "default-key.pem"}}`, want: "certificate not found"},
		{name: "私钥不存在", tls: `{"default_certificate": {"certificate": "default.pem", "key": "missing-key.pem"}}`, want: "key not found"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateConfigFile(writeConfig(t, c.tls))
			if c.want == "" {
				if err != nil {
					t.Fatalf("应通过校验：%v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want containing %q", err, c.want)
			}
		})
	}
}
