package sitecmd

import (
	"bytes"
	"encoding/json"
	"fast-https/config"
	"fast-https/modules/site"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// siteTestEnv 造一个最小主配置：一个 server 段 + 一个临时 include 目录。
func newSiteEnv(t *testing.T) (mainPath, includeDir string) {
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
	mainPath, includeDir := newSiteEnv(t)
	var out bytes.Buffer

	opts := Options{
		ConfigPath:     mainPath,
		Name:           "blog",
		Listens:        []string{"80"},
		ServerName:     "blog.example",
		Root:           "./httpdoc/root",
		LocationType:   "local",
		NonInteractive: true,
		Out:            &out,
	}
	if err := createSiteForTest(t, opts); err != nil {
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
}

// TLS 站点在首次 start 之前没有证书文件，init 仍应成功。
func TestSiteInitTLSAllowsMissingCerts(t *testing.T) {
	mainPath, includeDir := newSiteEnv(t)

	opts := Options{
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
	if err := createSiteForTest(t, opts); err != nil {
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
	mainPath, _ := newSiteEnv(t)
	opts := Options{
		ConfigPath:     mainPath,
		Name:           "api",
		Listens:        []string{"80"},
		ServerName:     "api.example",
		LocationType:   "proxy",
		NonInteractive: true,
		Out:            &bytes.Buffer{},
	}
	if err := createSiteForTest(t, opts); err == nil {
		t.Fatal("proxy site without upstream should fail")
	}
}

func TestSiteListShowCheck(t *testing.T) {
	mainPath, _ := newSiteEnv(t)
	newOpts := func(name string) Options {
		return Options{
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
	if err := createSiteForTest(t, newOpts("blog")); err != nil {
		t.Fatalf("init blog: %v", err)
	}
	if err := createSiteForTest(t, newOpts("shop")); err != nil {
		t.Fatalf("init shop: %v", err)
	}

	var listOut bytes.Buffer
	opts := Options{ConfigPath: mainPath, Out: &listOut}
	if err := RunList(opts); err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(listOut.String(), "blog") || !strings.Contains(listOut.String(), "shop") {
		t.Fatalf("list output:\n%s", listOut.String())
	}

	var showOut bytes.Buffer
	opts = Options{ConfigPath: mainPath, Name: "blog", Out: &showOut}
	if err := RunShow(opts); err != nil {
		t.Fatalf("show: %v", err)
	}
	if !strings.Contains(showOut.String(), "blog.example") || !strings.Contains(showOut.String(), "来源") {
		t.Fatalf("show output:\n%s", showOut.String())
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
		got := resolveListens(Options{TLS: c.tls}, c.given)
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
	mainPath, includeDir := newSiteEnv(t)
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

	opts := Options{
		ConfigPath: mainPath,
		In:         input,
		Out:        &out,
	}
	if err := createSiteForTest(t, opts); err != nil {
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
	mainPath, includeDir := newSiteEnv(t)
	var out bytes.Buffer

	input := newLineReader(
		"static-site", // 站点名
		"",            // 监听
		"",            // server_name
		"1",           // 站点类型：1 = 静态
		"",            // TLS
		"",            // 站点根目录（留空 → ./httpdoc/root）
	)

	opts := Options{
		ConfigPath: mainPath,
		In:         input,
		Out:        &out,
	}
	if err := createSiteForTest(t, opts); err != nil {
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
	mainPath, includeDir := newSiteEnv(t)
	opts := Options{
		ConfigPath:     mainPath,
		Name:           "secure",
		ServerName:     "secure.example",
		LocationType:   "local",
		Root:           "./httpdoc/root",
		TLS:            true,
		NonInteractive: true,
		Out:            &bytes.Buffer{},
	}
	if err := createSiteForTest(t, opts); err != nil {
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

// seedSite 建一个只监听 80 的站点，并手写一个额外 location：
// 同名更新时必须保留它，--force 重建时才允许丢掉。
func seedSite(t *testing.T, mainPath, includeDir string) string {
	t.Helper()

	initOpts := Options{
		ConfigPath:     mainPath,
		Name:           "blog",
		Listens:        []string{"80"},
		ServerName:     "blog.example",
		Root:           "./httpdoc/root",
		LocationType:   "local",
		NonInteractive: true,
		Out:            &bytes.Buffer{},
	}
	if err := createSiteForTest(t, initOpts); err != nil {
		t.Fatalf("init: %v", err)
	}

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
	return path
}

func runSiteSubcmd(t *testing.T, mainPath string, args ...string) (string, error) {
	t.Helper()
	cmd := NewSiteCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(append(args, "--config", mainPath))
	err := cmd.Execute()
	return out.String(), err
}

// createSiteForTest 用与 site add 相同的参数采集与落盘路径造一个站点文件。
// 测试不再依赖已移除的 runSiteInit。
func createSiteForTest(t *testing.T, opts Options) error {
	t.Helper()
	dirs, err := Dirs(opts)
	if err != nil {
		return err
	}
	name, in, err := siteInputFrom(&opts)
	if err != nil {
		return err
	}
	_, err = site.Create(dirs, name, in, opts.Force)
	return err
}
