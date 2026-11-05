package cmd

import (
	"bytes"
	"encoding/json"
	"fast-https/config"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// siteTestEnv 造一个最小主配置：一个 server 段 + 一个临时 include 目录。
func siteTestEnv(t *testing.T) (mainPath, includeDir string) {
	t.Helper()
	base := t.TempDir()
	includeDir = filepath.Join(base, "conf.d")
	if err := os.MkdirAll(includeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	mainPath = filepath.Join(base, "fast-https.json")
	payload := map[string]any{
		"http": map[string]any{
			"server": []any{
				map[string]any{
					"listen":      80,
					"server_name": "main.example",
					"location": []any{
						map[string]any{"url": "/", "type": "local", "root": "./httpdoc/root"},
					},
				},
			},
			"include": []string{includeDir},
		},
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mainPath, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	return mainPath, includeDir
}

func TestSiteInitNonInteractiveCreatesFile(t *testing.T) {
	mainPath, includeDir := siteTestEnv(t)
	var out bytes.Buffer

	opts := siteOptions{
		ConfigPath:     mainPath,
		Name:           "blog",
		Listens:        []string{"80"},
		ServerName:     "blog.example",
		Root:           "./httpdoc/root",
		LocationType:   "local",
		NonInteractive: true,
		Out:            &out,
	}
	if err := runSiteInit(opts); err != nil {
		t.Fatalf("init: %v\n%s", err, out.String())
	}

	path := filepath.Join(includeDir, "blog.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read generated site: %v", err)
	}
	if !strings.Contains(string(raw), `"server_name": "blog.example"`) {
		t.Fatalf("generated file:\n%s", raw)
	}
	if !strings.Contains(string(raw), `"listen": 80`) {
		t.Fatalf("generated file:\n%s", raw)
	}
	if !strings.Contains(out.String(), "fast-https reload") {
		t.Fatalf("output should point at reload: %q", out.String())
	}
}

// TLS 站点在首次 start 之前没有证书文件，init 仍应成功。
func TestSiteInitTLSAllowsMissingCerts(t *testing.T) {
	mainPath, includeDir := siteTestEnv(t)

	opts := siteOptions{
		ConfigPath:     mainPath,
		Name:           "secure",
		Listens:        []string{"443"},
		ServerName:     "secure.example",
		LocationType:   "local",
		Root:           "./httpdoc/root",
		TLS:            true,
		NonInteractive: true,
		Out:            &bytes.Buffer{},
	}
	if err := runSiteInit(opts); err != nil {
		t.Fatalf("tls init: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(includeDir, "secure.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"listen": "443 ssl h2"`) {
		t.Fatalf("listen should gain ssl h2:\n%s", raw)
	}
	if !strings.Contains(string(raw), "secure.example.pem") {
		t.Fatalf("cert path missing:\n%s", raw)
	}
}

func TestSiteInitProxyRequiresUpstream(t *testing.T) {
	mainPath, _ := siteTestEnv(t)
	opts := siteOptions{
		ConfigPath:     mainPath,
		Name:           "api",
		Listens:        []string{"80"},
		ServerName:     "api.example",
		LocationType:   "proxy",
		NonInteractive: true,
		Out:            &bytes.Buffer{},
	}
	if err := runSiteInit(opts); err == nil {
		t.Fatal("proxy site without upstream should fail")
	}
}

func TestSiteListShowCheckRemove(t *testing.T) {
	mainPath, includeDir := siteTestEnv(t)
	newOpts := func(name string) siteOptions {
		return siteOptions{
			ConfigPath:     mainPath,
			Name:           name,
			Listens:        []string{"80"},
			ServerName:     name + ".example",
			LocationType:   "local",
			Root:           "./httpdoc/root",
			NonInteractive: true,
			Out:            &bytes.Buffer{},
		}
	}
	if err := runSiteInit(newOpts("blog")); err != nil {
		t.Fatalf("init blog: %v", err)
	}
	if err := runSiteInit(newOpts("shop")); err != nil {
		t.Fatalf("init shop: %v", err)
	}

	var listOut bytes.Buffer
	opts := siteOptions{ConfigPath: mainPath, Out: &listOut}
	if err := runSiteList(opts); err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(listOut.String(), "blog") || !strings.Contains(listOut.String(), "shop") {
		t.Fatalf("list output:\n%s", listOut.String())
	}

	var showOut bytes.Buffer
	opts = siteOptions{ConfigPath: mainPath, Name: "blog", Out: &showOut}
	if err := runSiteShow(opts); err != nil {
		t.Fatalf("show: %v", err)
	}
	if !strings.Contains(showOut.String(), "blog.example") || !strings.Contains(showOut.String(), "来源") {
		t.Fatalf("show output:\n%s", showOut.String())
	}

	var checkOut bytes.Buffer
	opts = siteOptions{ConfigPath: mainPath, Name: "blog", Out: &checkOut}
	if err := runSiteCheck(opts); err != nil {
		t.Fatalf("check: %v", err)
	}
	if !strings.Contains(checkOut.String(), "ok") {
		t.Fatalf("check output:\n%s", checkOut.String())
	}

	if err := runSiteRemove(siteOptions{ConfigPath: mainPath, Name: "blog", Out: &bytes.Buffer{}}); err == nil {
		t.Fatal("remove without --yes should fail")
	}
	if err := runSiteRemove(siteOptions{ConfigPath: mainPath, Name: "blog", Yes: true, Out: &bytes.Buffer{}}); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Stat(filepath.Join(includeDir, "blog.json")); !os.IsNotExist(err) {
		t.Fatal("site file should be gone")
	}
}

// TLS 不再往 80 上贴 ssl：默认给出 80 与 443 两个监听。
func TestResolveListens(t *testing.T) {
	cases := []struct {
		name  string
		tls   bool
		given []string
		want  []string
	}{
		{"plain default", false, nil, []string{"80"}},
		{"tls default is http plus https", true, nil, []string{"80", "443 ssl h2"}},
		{"tls keeps 80 and adds 443", true, []string{"80"}, []string{"80", "443 ssl h2"}},
		{"tls upgrades explicit 443", true, []string{"443"}, []string{"443 ssl h2"}},
		{"tls upgrades the only custom port", true, []string{"8443"}, []string{"8443 ssl h2"}},
		{"tls leaves plain 80 and upgrades the rest", true, []string{"80", "8443"}, []string{"80", "8443 ssl h2"}},
		{"tls keeps explicit ssl listen", true, []string{"18443 ssl h2"}, []string{"18443 ssl h2"}},
		{"non tls keeps ssl listen as authored", false, []string{"8443 ssl h2"}, []string{"8443 ssl h2"}},
		{"multiple listens kept in order", false, []string{"80", "8080"}, []string{"80", "8080"}},
		{"blank entries dropped", false, []string{"80", "  "}, []string{"80"}},
	}
	for _, c := range cases {
		got := resolveListens(siteOptions{TLS: c.tls}, c.given)
		if len(got) != len(c.want) {
			t.Fatalf("%s: listens = %v, want %v", c.name, got, c.want)
		}
		for i := range c.want {
			if got[i] != c.want[i] {
				t.Fatalf("%s: listens = %v, want %v", c.name, got, c.want)
			}
		}
	}
}

// 向导里把类型改成反向代理后，第二屏必须问上游地址，而不是继续问站点根目录。
func TestSiteInitWizardAsksUpstreamForProxy(t *testing.T) {
	mainPath, includeDir := siteTestEnv(t)
	var out bytes.Buffer

	// accessible 模式逐行读取：站点名、监听、server_name、类型、TLS、上游地址。
	// 用 lineReader 而不是 strings.Reader：huh 每屏都新建 bufio.Scanner，
	// 一次 Read 会把后面的行一起吞掉。
	input := newLineReader(
		"proxy-site",            // 站点名
		"",                      // 监听（留空 → 80）
		"",                      // server_name（留空 → 站点名）
		"2",                     // 站点类型：2 = 反向代理
		"",                      // 启用 TLS？默认否
		"http://127.0.0.1:9000", // 上游地址
	)

	opts := siteOptions{
		ConfigPath: mainPath,
		In:         input,
		Out:        &out,
	}
	if err := runSiteInit(opts); err != nil {
		t.Fatalf("wizard init: %v\n%s", err, out.String())
	}

	raw, err := os.ReadFile(filepath.Join(includeDir, "proxy-site.json"))
	if err != nil {
		t.Fatalf("expected proxy-site.json: %v\n%s", err, out.String())
	}
	if !strings.Contains(string(raw), `"proxy_pass": "http://127.0.0.1:9000"`) {
		t.Fatalf("proxy_pass missing:\n%s\n--- wizard output ---\n%s", raw, out.String())
	}
	if strings.Contains(string(raw), `"root"`) {
		t.Fatalf("proxy site must not carry a static root:\n%s", raw)
	}
	if !strings.Contains(out.String(), "上游地址") {
		t.Fatalf("wizard should ask for the upstream, got:\n%s", out.String())
	}
	if strings.Contains(out.String(), "站点根目录") {
		t.Fatalf("wizard must not ask for a root after choosing proxy:\n%s", out.String())
	}
}

func TestSiteInitWizardLocalAsksRoot(t *testing.T) {
	mainPath, includeDir := siteTestEnv(t)
	var out bytes.Buffer

	input := newLineReader(
		"static-site", // 站点名
		"",            // 监听
		"",            // server_name
		"1",           // 站点类型：1 = 静态
		"",            // TLS
		"",            // 站点根目录（留空 → ./httpdoc/root）
	)

	opts := siteOptions{
		ConfigPath: mainPath,
		In:         input,
		Out:        &out,
	}
	if err := runSiteInit(opts); err != nil {
		t.Fatalf("wizard init: %v\n%s", err, out.String())
	}

	raw, err := os.ReadFile(filepath.Join(includeDir, "static-site.json"))
	if err != nil {
		t.Fatalf("expected static-site.json: %v\n%s", err, out.String())
	}
	if !strings.Contains(string(raw), `"type": "local"`) || !strings.Contains(string(raw), `"root": "./httpdoc/root"`) {
		t.Fatalf("static site body:\n%s", raw)
	}
	if !strings.Contains(out.String(), "站点根目录") {
		t.Fatalf("wizard should ask for the root, got:\n%s", out.String())
	}
}

func TestSiteInitTLSDefaultsToHTTPAndHTTPS(t *testing.T) {
	mainPath, includeDir := siteTestEnv(t)
	opts := siteOptions{
		ConfigPath:     mainPath,
		Name:           "secure",
		ServerName:     "secure.example",
		LocationType:   "local",
		Root:           "./httpdoc/root",
		TLS:            true,
		NonInteractive: true,
		Out:            &bytes.Buffer{},
	}
	if err := runSiteInit(opts); err != nil {
		t.Fatalf("tls init: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(includeDir, "secure.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"listen": [`, "80,", `"443 ssl h2"`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("generated file should contain %s:\n%s", want, raw)
		}
	}
	if strings.Contains(string(raw), `"80 ssl h2"`) {
		t.Fatalf("TLS must not be put on port 80:\n%s", raw)
	}
	// 证书要等首次 start 才生成，这里用允许证书缺失的校验。
	if err := config.ValidateNewSiteFile(filepath.Join(includeDir, "secure.json")); err != nil {
		t.Fatalf("generated tls site should validate: %v", err)
	}
}

// 站点名带 .json 时不能再拼一次扩展名。
func TestSiteNameWithJSONSuffixIsNotDoubled(t *testing.T) {
	mainPath, includeDir := siteTestEnv(t)
	out := &bytes.Buffer{}

	cmd := newSiteCmd()
	cmd.SetArgs([]string{
		"init", "blog.json",
		"--config", mainPath,
		"--listen", "80",
		"--server-name", "blog.example",
		"--non-interactive",
	})
	cmd.SetOut(out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init: %v\n%s", err, out.String())
	}

	if _, err := os.Stat(filepath.Join(includeDir, "blog.json")); err != nil {
		t.Fatalf("expected blog.json: %v", err)
	}
	if _, err := os.Stat(filepath.Join(includeDir, "blog.json.json")); !os.IsNotExist(err) {
		t.Fatal("must not create blog.json.json")
	}
}

func TestSiteCheckWholeConfig(t *testing.T) {
	mainPath, _ := siteTestEnv(t)
	var out bytes.Buffer
	if err := runSiteCheck(siteOptions{ConfigPath: mainPath, Out: &out}); err != nil {
		t.Fatalf("check main config: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "ok") {
		t.Fatalf("output = %q", out.String())
	}
}

// siteUpdateHelper 造一个站点文件，然后以子命令方式跑一次 site update，返回输出。
func siteUpdateHelper(t *testing.T, mainPath, includeDir string, args ...string) string {
	t.Helper()

	initOpts := siteOptions{
		ConfigPath:     mainPath,
		Name:           "blog",
		Listens:        []string{"80"},
		ServerName:     "blog.example",
		Root:           "./httpdoc/root",
		LocationType:   "local",
		NonInteractive: true,
		Out:            &bytes.Buffer{},
	}
	if err := runSiteInit(initOpts); err != nil {
		t.Fatalf("init: %v", err)
	}
	// 手写一个额外 location，update 必须原样保留它。
	path := filepath.Join(includeDir, "blog.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read site: %v", err)
	}
	in, err := config.ParseSiteInput(raw)
	if err != nil {
		t.Fatalf("parse site: %v", err)
	}
	in.Locations = append(in.Locations, config.SiteLocation{
		URL: "/api", Type: "proxy", ProxyPass: "http://127.0.0.1:9000",
	})
	data, err := in.MarshalJSONFile()
	if err != nil {
		t.Fatalf("marshal site: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write site: %v", err)
	}

	cmd := newSiteCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(append(args, "--config", mainPath))
	if err := cmd.Execute(); err != nil {
		t.Fatalf("site %v: %v\n%s", args, err, out.String())
	}
	return out.String()
}

func TestSiteUpdateAddsTLSAndKeepsOtherFields(t *testing.T) {
	mainPath, includeDir := siteTestEnv(t)
	out := siteUpdateHelper(t, mainPath, includeDir, "update", "blog", "--tls")

	if !strings.Contains(out, "站点已更新") {
		t.Fatalf("output = %q", out)
	}

	raw, err := os.ReadFile(filepath.Join(includeDir, "blog.json"))
	if err != nil {
		t.Fatalf("read updated site: %v", err)
	}
	in, err := config.ParseSiteInput(raw)
	if err != nil {
		t.Fatalf("parse updated site: %v", err)
	}

	if len(in.Listens) != 2 || !strings.Contains(in.ListenSummary(), "ssl") {
		t.Fatalf("listen 应升级为 80 + 443 ssl h2，实际 %v", in.Listens)
	}
	if in.ServerName != "blog.example" {
		t.Fatalf("未传 --server-name 时不应改 server_name，实际 %q", in.ServerName)
	}
	if in.SSLCertificate == "" || in.SSLCertificateKey == "" {
		t.Fatalf("加了 ssl 就该补默认证书路径，实际 %q / %q", in.SSLCertificate, in.SSLCertificateKey)
	}
	if len(in.Locations) != 2 {
		t.Fatalf("手写的 /api location 被删了：%+v", in.Locations)
	}
	if in.Locations[1].ProxyPass != "http://127.0.0.1:9000" {
		t.Fatalf("手写 location 被改动：%+v", in.Locations[1])
	}
}

func TestSiteUpdateServerNameOnly(t *testing.T) {
	mainPath, includeDir := siteTestEnv(t)
	siteUpdateHelper(t, mainPath, includeDir, "update", "blog", "--server-name", "new.example")

	raw, err := os.ReadFile(filepath.Join(includeDir, "blog.json"))
	if err != nil {
		t.Fatalf("read updated site: %v", err)
	}
	in, err := config.ParseSiteInput(raw)
	if err != nil {
		t.Fatalf("parse updated site: %v", err)
	}
	if in.ServerName != "new.example" {
		t.Fatalf("server_name = %q", in.ServerName)
	}
	if len(in.Listens) != 1 || in.Listens[0] != "80" {
		t.Fatalf("没传 --listen/--tls 时监听不应变化，实际 %v", in.Listens)
	}
}

func TestSiteUpdateUnknownSiteFails(t *testing.T) {
	mainPath, _ := siteTestEnv(t)
	var out bytes.Buffer
	cmd := newSiteCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"update", "nosuch", "--tls", "--config", mainPath})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("update 不存在的站点应该失败：%s", out.String())
	}
	if !strings.Contains(out.String(), "site init") {
		t.Fatalf("错误信息应指向 init：%q", out.String())
	}
}
