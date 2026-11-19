package loader

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"fast-https/config/internal/model"
	"fast-https/config/internal/schema"
	"fast-https/config/internal/testutil"
)

func TestParsePathType(t *testing.T) {
	byName := map[string]uint16{
		"LOCAL":       model.LOCAL,
		"REWRITE":     model.REWRITE,
		"DEVMOD":      model.DEVMOD,
		"PROXY_HTTP":  model.PROXY_HTTP,
		"PROXY_HTTPS": model.PROXY_HTTPS,
	}

	var cases []struct {
		Name      string `yaml:"name"`
		Type      string `yaml:"type"`
		ProxyPass string `yaml:"proxy_pass"`
		Want      string `yaml:"want"`
	}
	testutil.LoadYAML(t, "loader/path_type.yaml", &cases)
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			want, ok := byName[c.Want]
			if !ok {
				t.Fatalf("测试数据引用了未知 PathType %q", c.Want)
			}
			if got := parsePathType(c.Type, c.ProxyPass); got != want {
				t.Fatalf("parsePathType(%q,%q) = %d, want %d", c.Type, c.ProxyPass, got, want)
			}
		})
	}
}

func TestParseZip(t *testing.T) {
	byName := map[string]uint16{
		"ZIP_NONE":    model.ZIP_NONE,
		"ZIP_BR":      model.ZIP_BR,
		"ZIP_GZIP":    model.ZIP_GZIP,
		"ZIP_GZIP_BR": model.ZIP_GZIP_BR,
	}

	var cases []struct {
		Name string   `yaml:"name"`
		Zip  []string `yaml:"zip"`
		Want string   `yaml:"want"`
	}
	testutil.LoadYAML(t, "loader/zip.yaml", &cases)
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			want, ok := byName[c.Want]
			if !ok {
				t.Fatalf("测试数据引用了未知 ZipType %q", c.Want)
			}
			if got := parseZip(c.Zip); got != want {
				t.Fatalf("parseZip(%v) = %d, want %d", c.Zip, got, want)
			}
		})
	}
}

func TestTrimProxyPass(t *testing.T) {
	var cases []struct {
		In   string `yaml:"in"`
		Want string `yaml:"want"`
	}
	testutil.LoadYAML(t, "loader/trim_proxy_pass.yaml", &cases)
	for _, c := range cases {
		if got := trimProxyPass(c.In); got != c.Want {
			t.Errorf("trimProxyPass(%q) = %q, want %q", c.In, got, c.Want)
		}
	}
}

func TestSplitProxyPass(t *testing.T) {
	cases := []struct {
		in, addr, uri string
	}{
		{"http://127.0.0.1:9000", "127.0.0.1:9000", ""},
		{"http://127.0.0.1:9000/", "127.0.0.1:9000", "/"},
		{"https://backend:8443/v1", "backend:8443", "/v1"},
		{"http://[::1]:9000/api/", "[::1]:9000", "/api/"},
		{"", "", ""},
	}
	for _, c := range cases {
		addr, uri := splitProxyPass(c.in)
		if addr != c.addr || uri != c.uri {
			t.Errorf("splitProxyPass(%q) = (%q,%q), want (%q,%q)", c.in, addr, uri, c.addr, c.uri)
		}
	}
}

func TestNormalizeMethods(t *testing.T) {
	got := normalizeMethods([]string{" get ", "Post", ""})
	if len(got) != 2 || got[0] != "GET" || got[1] != "POST" {
		t.Fatalf("normalizeMethods = %v", got)
	}
}

// include 目录遍历分支：合法 server 块、非 json 跳过、空目录、坏 json 报错
func TestProcessIncludeCfg(t *testing.T) {
	dir := t.TempDir()

	write := func(rel, content string) {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, rel)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	const serverA = `{"listen":"18081","server_name":"a.local","location":[{"url":"/","type":"local","root":"./a"}]}`
	const serverB = `{"listen":"18082","server_name":"b.local","location":[{"url":"/x","type":"rewrite","rewrite":"/y"}]}`
	write("a.json", serverA)
	write("nested/b.json", serverB)    // 嵌套目录也要展开
	write("notes.txt", "not a config") // 非 json 跳过
	if err := os.MkdirAll(filepath.Join(dir, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := &model.Fast_Https{Include: []string{dir}}
	if err := processIncludeCfg(cfg); err != nil {
		t.Fatalf("processIncludeCfg: %v", err)
	}
	if len(cfg.Servers) != 2 {
		t.Fatalf("servers = %d, want 2 (json files only)", len(cfg.Servers))
	}
	// 按文件名遍历顺序不可假设，按键找
	byName := map[string]model.Server{}
	for _, s := range cfg.Servers {
		byName[s.ServerName] = s
	}
	if s := byName["a.local"]; s.Listen != "18081" || len(s.Path) != 1 || s.Path[0].PathType != model.LOCAL {
		t.Fatalf("server a parsed wrong: %+v", s)
	}
	if s := byName["b.local"]; s.Path[0].Rewrite != "/y" {
		t.Fatalf("server b parsed wrong: %+v", s)
	}

	// 坏 json：报错而不是 panic
	write("broken.json", "{not json")
	if err := processIncludeCfg(&model.Fast_Https{Include: []string{dir}}); err == nil {
		t.Fatal("broken json must fail the include walk")
	}

	// 不存在的 include 路径：Walk 直接报错
	if err := processIncludeCfg(&model.Fast_Https{Include: []string{filepath.Join(dir, "ghost")}}); err == nil {
		t.Fatal("missing include dir must fail")
	}

	// 无 include：no-op
	if err := processIncludeCfg(&model.Fast_Https{}); err != nil {
		t.Fatalf("empty include must be a no-op: %v", err)
	}
}

// 一个站点文件写多个监听：加载后展开成多个运行期 server 块，
// server_name、location、证书共用，端口各自独立。
func TestIncludeServerMultipleListensExpands(t *testing.T) {
	dir := t.TempDir()
	site := `{
  "listen": ["80", "443 ssl h2"],
  "server_name": "multi.local",
  "ssl_certificate": "multi.pem",
  "ssl_certificate_key": "multi-key.pem",
  "location": [{ "url": "/api", "type": "proxy", "proxy_pass": "http://127.0.0.1:9000" }]
}`
	if err := os.WriteFile(filepath.Join(dir, "multi.json"), []byte(site), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &model.Fast_Https{Include: []string{dir}}
	if err := processIncludeCfg(cfg); err != nil {
		t.Fatalf("processIncludeCfg: %v", err)
	}
	if len(cfg.Servers) != 2 {
		t.Fatalf("servers = %d, want 2", len(cfg.Servers))
	}

	listens := map[string]model.Server{}
	for _, server := range cfg.Servers {
		listens[server.Listen] = server
		if server.ServerName != "multi.local" {
			t.Fatalf("server_name = %q", server.ServerName)
		}
		if server.SSLCertificate != "multi.pem" || server.SSLCertificateKey != "multi-key.pem" {
			t.Fatalf("cert = %q / %q", server.SSLCertificate, server.SSLCertificateKey)
		}
		if len(server.Path) != 1 || server.Path[0].PathName != "/api" || server.Path[0].ProxyData != "127.0.0.1:9000" {
			t.Fatalf("location = %+v", server.Path)
		}
	}
	if _, ok := listens["80"]; !ok {
		t.Fatal("missing plain 80 listener")
	}
	if _, ok := listens["443 ssl h2"]; !ok {
		t.Fatal("missing tls listener")
	}

	// 展开出来的 server 之间不能共享 Path 底层数组：
	// 改写其中一个的 root 不应影响另一个。
	first, second := cfg.Servers[0], cfg.Servers[1]
	first.Path[0].Root = "changed"
	if second.Path[0].Root == "changed" {
		t.Fatal("expanded servers share the same Path backing array")
	}
}

func TestConvertPathReadsErrorPage(t *testing.T) {
	var jp schema.Path
	err := schema.Unmarshal([]byte(`{
		"url": "/",
		"type": "local",
		"error_page": {
			"not_found": "404.html",
			"proxy": "proxy.html"
		}
	}`), &jp)
	if err != nil {
		t.Fatal(err)
	}
	path := convertPath(jp)
	if path.ErrorNotFound != "404.html" || path.ErrorProxy != "proxy.html" {
		t.Fatalf("not_found=%q proxy=%q", path.ErrorNotFound, path.ErrorProxy)
	}
}

// 十个并发 reload 必须串行发布：最终快照是一份完整配置，而不是
// 两次加载拼出来的「新站点 + 旧日志目录」。-race 下同时验证没有数据竞争。
func TestConcurrentReloadSerialized(t *testing.T) {
	before := model.Current()
	beforeMIME := model.MIME()
	t.Cleanup(func() { model.Store(before, beforeMIME) })

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "mime.json"), []byte(`{"html": "text/html"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	const n = 10
	paths := make([]string, n)
	for i := 0; i < n; i++ {
		body := fmt.Sprintf(`{
  "log_root": "reload-%d",
  "http": {"server": [{"listen": %d, "server_name": "site-%d.local",
    "location": [{"url": "/", "type": "local", "root": "."}]}]}
}`, i, 18000+i, i)
		path := filepath.Join(dir, fmt.Sprintf("site-%d.json", i))
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		paths[i] = path
	}

	var wg sync.WaitGroup
	errs := make(chan error, n)
	for _, path := range paths {
		wg.Add(1)
		go func(path string) {
			defer wg.Done()
			errs <- ReloadFile(path)
		}(path)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent reload: %v", err)
		}
	}

	cur := model.Current()
	if len(cur.Servers) != 1 {
		t.Fatalf("servers = %d, want 1 (snapshot must not merge reloads)", len(cur.Servers))
	}
	name := cur.Servers[0].ServerName
	idx := strings.TrimSuffix(strings.TrimPrefix(name, "site-"), ".local")
	if idx == name || idx == "" {
		t.Fatalf("unexpected server name %q", name)
	}
	if want := "reload-" + idx; filepath.Base(cur.LogRoot) != want {
		t.Fatalf("log_root = %q, want %q from the same load as %q", cur.LogRoot, want, name)
	}
}

// Reload 指向不存在的文件：报错且当前快照保持不动
func TestReloadMissingFileKeepsSnapshot(t *testing.T) {
	before := model.Current()
	if err := ReloadFile(filepath.Join(t.TempDir(), "absent.json")); err == nil {
		t.Fatal("missing file must fail reload")
	}
	if model.Current() != before {
		t.Fatal("failed reload must leave the snapshot untouched")
	}
}

// 默认 check-at-load 在加载层生效：不经过 Reload/validate 直接调用 LoadFile
// （InitSystem、acme/管理命令）也要拒绝带符号链接的静态根。
func TestLoadFileRejectsSymlinkRootByDefault(t *testing.T) {
	before := model.Current()
	beforeMIME := model.MIME()
	t.Cleanup(func() { model.Store(before, beforeMIME) })

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skipf("创建符号链接失败：%v", err)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "mime.json"), []byte(`{"html": "text/html"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	writeConfig := func(name, policy string) string {
		extra := ""
		if policy != "" {
			extra = fmt.Sprintf(`, "symlink_policy": %q`, policy)
		}
		body := fmt.Sprintf(`{"http": {"server": [{"listen": 8080, "server_name": "link.local",
  "location": [{"url": "/", "type": "local", "root": %q%s}]}]}}`, root, extra)
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}

	err := LoadFile(writeConfig("default.json", ""))
	if err == nil || !strings.Contains(err.Error(), "symlink found under root") {
		t.Fatalf("默认策略应拒绝带符号链接的 root，err = %v", err)
	}

	if err := LoadFile(writeConfig("allow.json", model.SYMLINK_ALLOW)); err != nil {
		t.Fatalf("allow 应正常加载：%v", err)
	}
	servers := model.Current().Servers
	if len(servers) != 1 || servers[0].Path[0].SymlinkPolicy != model.SYMLINK_ALLOW {
		t.Fatalf("快照未按 allow 更新：%+v", servers)
	}
}
