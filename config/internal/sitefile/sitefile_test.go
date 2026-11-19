package sitefile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fast-https/config/internal/model"
	"fast-https/config/internal/testutil"
)

func TestSiteInputRoundTrip(t *testing.T) {
	in := SiteInput{
		Listens:           []string{"80", "443 ssl h2"},
		ServerName:        "blog.example",
		SSLCertificate:    "./config/cert/blog.example.pem",
		SSLCertificateKey: "./config/cert/blog.example-key.pem",
		Locations: []SiteLocation{
			{URL: "/", Type: "local", Root: "./httpdoc/root", Index: []string{"index.html"}},
			{URL: "/api", Type: "proxy", ProxyPass: "http://127.0.0.1:8080"},
		},
	}

	raw, err := in.MarshalJSONFile()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	parsed, err := ParseSiteInput(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(parsed.Listens) != 2 || parsed.Listens[0] != "80" || parsed.Listens[1] != "443 ssl h2" {
		t.Fatalf("listens = %v", parsed.Listens)
	}
	if parsed.ServerName != in.ServerName {
		t.Fatalf("parsed = %+v", parsed)
	}
	if parsed.SSLCertificate != in.SSLCertificate || parsed.SSLCertificateKey != in.SSLCertificateKey {
		t.Fatalf("cert paths = %q / %q", parsed.SSLCertificate, parsed.SSLCertificateKey)
	}
	if len(parsed.Locations) != 2 || parsed.Locations[1].ProxyPass != "http://127.0.0.1:8080" {
		t.Fatalf("locations = %+v", parsed.Locations)
	}
	if !strings.HasSuffix(string(raw), "\n") {
		t.Fatal("file should end with a newline")
	}
}

func TestSiteListenNumberVsStringVsArray(t *testing.T) {
	plain, err := SiteInput{Listens: []string{"80"}, ServerName: "a.example",
		Locations: []SiteLocation{{URL: "/", Type: "local", Root: "./httpdoc/root"}}}.MarshalJSONFile()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(plain), `"listen": 80,`) {
		t.Fatalf("plain port should be a number:\n%s", plain)
	}

	tls, err := SiteInput{Listens: []string{"443 ssl h2"}, ServerName: "a.example",
		Locations: []SiteLocation{{URL: "/", Type: "local", Root: "./httpdoc/root"}}}.MarshalJSONFile()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(tls), `"listen": "443 ssl h2"`) {
		t.Fatalf("listen with flags should be a string:\n%s", tls)
	}

	multi, err := SiteInput{Listens: []string{"80", "443 ssl h2"}, ServerName: "a.example",
		Locations: []SiteLocation{{URL: "/", Type: "local", Root: "./httpdoc/root"}}}.MarshalJSONFile()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(multi), `"listen": [`) {
		t.Fatalf("multiple listens should be an array:\n%s", multi)
	}
	parsed, err := ParseSiteInput(multi)
	if err != nil {
		t.Fatalf("parse multi: %v", err)
	}
	if len(parsed.Listens) != 2 {
		t.Fatalf("listens = %v", parsed.Listens)
	}
}

func TestSiteDirsFromFileAndSnapshot(t *testing.T) {
	base := t.TempDir()
	includeDir := filepath.Join(base, "conf.d")
	if err := os.MkdirAll(includeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	mainPath := filepath.Join(base, "fast-https.json")
	mainConfig := `{
  "http": {
    "server": [
      { "listen": 80, "server_name": "main.example",
        "location": [{ "url": "/", "type": "local", "root": "./httpdoc/root" }] }
    ],
    "include": [` + quote(includeDir) + `]
  }
}`
	if err := os.WriteFile(mainPath, []byte(mainConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	dirs, err := SiteDirsFromFile(mainPath)
	if err != nil {
		t.Fatalf("site dirs: %v", err)
	}
	if len(dirs) != 1 || dirs[0] != includeDir {
		t.Fatalf("dirs = %v, want [%s]", dirs, includeDir)
	}

	sitePath := filepath.Join(includeDir, "blog.json")
	siteFile := `{
  "listen": "443 ssl h2",
  "server_name": "blog.example",
  "location": [{ "url": "/", "type": "local", "root": "./httpdoc/root" }]
}`
	if err := os.WriteFile(sitePath, []byte(siteFile), 0o644); err != nil {
		t.Fatal(err)
	}

	data, source, err := SnapshotServer(mainPath, "blog.example")
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if source != sitePath {
		t.Fatalf("source = %q, want %q", source, sitePath)
	}
	if !strings.Contains(string(data), `"server_name": "blog.example"`) {
		t.Fatalf("snapshot body:\n%s", data)
	}

	data, source, err = SnapshotServer(mainPath, "main.example")
	if err != nil {
		t.Fatalf("snapshot main: %v", err)
	}
	if source != mainPath {
		t.Fatalf("main source = %q", source)
	}

	if _, _, err := SnapshotServer(mainPath, "missing.example"); err == nil {
		t.Fatal("unknown server_name should be reported")
	}
}

func quote(s string) string {
	return `"` + strings.ReplaceAll(s, `\`, `\\`) + `"`
}

// patchAdvanced 跑一次 PatchSiteFile，只改主 location 的高级配置。
func patchAdvanced(t *testing.T, raw string, want SiteInput) string {
	t.Helper()
	existing, err := ParseSiteInput([]byte(raw))
	if err != nil {
		t.Fatalf("ParseSiteInput: %v", err)
	}
	out, err := PatchSiteFile([]byte(raw), existing, want)
	if err != nil {
		t.Fatalf("PatchSiteFile: %v", err)
	}
	return string(out)
}

// advancedBase 返回 testdata 里的基础站点文件，高级配置用例在它上面打补丁。
func advancedBase(t *testing.T) string {
	t.Helper()
	return readFixture(t, "sitefile/advanced_base.json")
}

func readFixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(testutil.DataDir(), filepath.FromSlash(name)))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(raw)
}

// 权重：patch 之后文件仍然能解析回同样的值。
func reparse(t *testing.T, raw string) SiteInput {
	t.Helper()
	in, err := ParseSiteInput([]byte(raw))
	if err != nil {
		t.Fatalf("回读失败: %v\n%s", err, raw)
	}
	return in
}

func TestPatchZip(t *testing.T) {
	zip := []string{"gzip", "br"}
	want := SiteInput{
		Listens:    []string{"8080"},
		ServerName: "a.local",
		Locations: []SiteLocation{{
			URL: "/", Type: "local", Root: "./httpdoc/root",
			Index: []string{"index.html"}, Zip: &zip,
		}},
	}
	out := patchAdvanced(t, advancedBase(t), want)
	got := reparse(t, out)
	if got.Locations[0].Zip == nil || strings.Join(*got.Locations[0].Zip, ",") != "gzip,br" {
		t.Fatalf("zip 没写进去:\n%s", out)
	}
	// 没动过的键必须原样保留
	if !strings.Contains(out, `"index": ["index.html"]`) {
		t.Fatalf("既有 index 被改动了:\n%s", out)
	}
}

// R-07：阈值与跳过类型都要能写入、能清除，且互不影响 zip 本身。
func TestPatchZipLimits(t *testing.T) {
	minLen := 1024
	skip := []string{"image/png", "font/woff2"}
	want := SiteInput{
		Listens:    []string{"8080"},
		ServerName: "a.local",
		Locations: []SiteLocation{{
			URL: "/", Type: "local", Root: "./httpdoc/root",
			Index: []string{"index.html"}, ZipMinLength: &minLen, ZipSkipTypes: &skip,
		}},
	}
	out := patchAdvanced(t, advancedBase(t), want)
	got := reparse(t, out)
	loc := got.Locations[0]
	if loc.ZipMinLength == nil || *loc.ZipMinLength != 1024 {
		t.Fatalf("zip_min_length 没写进去:\n%s", out)
	}
	if loc.ZipSkipTypes == nil || strings.Join(*loc.ZipSkipTypes, ",") != "image/png,font/woff2" {
		t.Fatalf("zip_skip_types 没写进去:\n%s", out)
	}

	// 再传零值/空列表：两个键都要被删掉，回到默认行为。
	zero, empty := 0, []string{}
	clear := SiteInput{
		Listens:    []string{"8080"},
		ServerName: "a.local",
		Locations: []SiteLocation{{
			URL: "/", Type: "local", Root: "./httpdoc/root",
			Index: []string{"index.html"}, ZipMinLength: &zero, ZipSkipTypes: &empty,
		}},
	}
	out = patchAdvanced(t, out, clear)
	if strings.Contains(out, "zip_min_length") || strings.Contains(out, "zip_skip_types") {
		t.Fatalf("清除后不应残留键:\n%s", out)
	}
}

func TestPatchLimit(t *testing.T) {
	limit := model.PathLimit{Rate: 50, Burst: 100}
	want := SiteInput{
		Listens:    []string{"8080"},
		ServerName: "a.local",
		Locations:  []SiteLocation{{URL: "/", Type: "local", Root: "./httpdoc/root", Limit: &limit}},
	}
	out := patchAdvanced(t, advancedBase(t), want)
	got := reparse(t, out)
	if got.Locations[0].Limit == nil {
		t.Fatalf("limit 没写进去:\n%s", out)
	}
	if l := got.Locations[0].Limit; l.Rate != 50 || l.Burst != 100 {
		t.Fatalf("limit 值不对: %+v", l)
	}
}

func TestPatchAuth(t *testing.T) {
	auth := model.PathAuth{AuthType: "basic", User: "admin", Pswd: "s3cret"}
	want := SiteInput{
		Listens:    []string{"8080"},
		ServerName: "a.local",
		Locations:  []SiteLocation{{URL: "/", Type: "local", Root: "./httpdoc/root", Auth: &auth}},
	}
	out := patchAdvanced(t, advancedBase(t), want)
	got := reparse(t, out)
	if got.Locations[0].Auth == nil || got.Locations[0].Auth.User != "admin" {
		t.Fatalf("auth 没写进去:\n%s", out)
	}
	if got.Locations[0].Auth.Pswd != "s3cret" {
		t.Fatalf("auth 口令不对: %+v", got.Locations[0].Auth)
	}
}

func TestPatchCache(t *testing.T) {
	want := SiteInput{
		Listens:    []string{"8080"},
		ServerName: "a.local",
		Locations:  []SiteLocation{{URL: "/", Type: "local", Root: "./httpdoc/root", ProxyCache: "edge"}},
	}
	out := patchAdvanced(t, advancedBase(t), want)
	got := reparse(t, out)
	if got.Locations[0].ProxyCache != "edge" {
		t.Fatalf("proxy_cache 值不对: %+v\n%s", got.Locations[0], out)
	}
}

// 指向零值 = 清除，补丁应把键删掉而不是写成 {}。
func TestPatchAdvancedClears(t *testing.T) {
	withAll := readFixture(t, "sitefile/with_all_advanced.json")
	emptyZip := []string{}
	emptyLimit := model.PathLimit{}
	emptyAuth := model.PathAuth{}
	want := SiteInput{
		Listens:    []string{"8080"},
		ServerName: "a.local",
		Locations: []SiteLocation{{
			URL: "/", Type: "local", Root: "./httpdoc/root",
			Zip: &emptyZip, Limit: &emptyLimit, Auth: &emptyAuth,
		}},
	}
	out := patchAdvanced(t, withAll, want)
	for _, key := range []string{`"zip"`, `"limit"`, `"auth"`, `"proxy_cache"`} {
		if strings.Contains(out, key) {
			t.Fatalf("%s 应该被删掉:\n%s", key, out)
		}
	}
	// root 等基础字段不能被连带清掉
	if !strings.Contains(out, `"root": "./httpdoc/root"`) {
		t.Fatalf("root 被误删:\n%s", out)
	}
}

// 指针为 nil = 这个字段不参与比对，文件里的原值必须原样保留。
func TestPatchAdvancedNilKeepsExisting(t *testing.T) {
	withAll := `{
  "listen": [8080],
  "server_name": "a.local",
  "location": [
    {
      "url": "/",
      "type": "local",
      "root": "./httpdoc/root",
      "zip": ["gzip"],
      "limit": {"mem":0,"rate":50,"burst":100,"nodelay":false}
    }
  ]
}
`
	// 只改 root，不动 zip / limit
	want := SiteInput{
		Listens:    []string{"8080"},
		ServerName: "a.local",
		Locations:  []SiteLocation{{URL: "/", Type: "local", Root: "./newroot"}},
	}
	out := patchAdvanced(t, withAll, want)
	if !strings.Contains(out, `"zip": ["gzip"]`) {
		t.Fatalf("zip 被误删:\n%s", out)
	}
	if !strings.Contains(out, `"limit"`) {
		t.Fatalf("limit 被误删:\n%s", out)
	}
	if !strings.Contains(out, `"./newroot"`) {
		t.Fatalf("root 没改:\n%s", out)
	}
}

// 未知键与注释必须在多次 patch 后依然存在。
func TestPatchAdvancedKeepsUnknownAndComments(t *testing.T) {
	raw := `{
  "listen": [8080],
  "server_name": "a.local",
  // 手写的未知键，patch 不能碰
  "try": [{"uri": "/x", "next": "/y"}],
  "location": [
    {
      "url": "/",
      "type": "local",
      "root": "./httpdoc/root",
      "appfirewall": ["sql"],
      "rewrite": "/login"
    }
  ]
}
`
	zip := []string{"br"}
	want := SiteInput{
		Listens:    []string{"8080"},
		ServerName: "a.local",
		Locations:  []SiteLocation{{URL: "/", Type: "local", Root: "./httpdoc/root", Zip: &zip}},
	}
	out := patchAdvanced(t, raw, want)
	if !strings.Contains(out, `"appfirewall": ["sql"]`) {
		t.Fatalf("appfirewall 被删:\n%s", out)
	}
	if !strings.Contains(out, `"rewrite": "/login"`) {
		t.Fatalf("rewrite 被删:\n%s", out)
	}
	if !strings.Contains(out, `"try"`) {
		t.Fatalf("try 被删:\n%s", out)
	}
	if !strings.Contains(out, "手写的未知键") {
		t.Fatalf("注释被删:\n%s", out)
	}
	if !strings.Contains(out, `"zip": ["br"]`) {
		t.Fatalf("zip 没写进去:\n%s", out)
	}
}

// 连续插入多个新键不应该越积越多空行。
func TestPatchAdvancedNoBlankLineGrowth(t *testing.T) {
	zip := []string{"gzip"}
	limit := model.PathLimit{Rate: 10}
	want := SiteInput{
		Listens:    []string{"8080"},
		ServerName: "a.local",
		Locations:  []SiteLocation{{URL: "/", Type: "local", Root: "./httpdoc/root", Zip: &zip, Limit: &limit}},
	}
	out := patchAdvanced(t, advancedBase(t), want)
	if strings.Contains(out, "\n\n\n") {
		t.Fatalf("出现连续空行:\n%s", out)
	}
	// location 对象内部不应有空行
	loc := out[strings.Index(out, `"location"`):]
	if strings.Contains(loc, "\n\n") {
		t.Fatalf("location 里出现空行:\n%s", out)
	}
}

// 只写一个高级字段时，另外三个没配置过的不该被凭空造出来。
func TestPatchAdvancedDoesNotCreateUnrequested(t *testing.T) {
	zip := []string{"gzip"}
	want := SiteInput{
		Listens:    []string{"8080"},
		ServerName: "a.local",
		Locations:  []SiteLocation{{URL: "/", Type: "local", Root: "./httpdoc/root", Zip: &zip}},
	}
	out := patchAdvanced(t, advancedBase(t), want)
	for _, key := range []string{`"limit"`, `"auth"`, `"proxy_cache"`} {
		if strings.Contains(out, key) {
			t.Fatalf("%s 不该出现:\n%s", key, out)
		}
	}
}
