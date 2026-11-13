package config

import (
	"strings"
	"testing"
)

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

const advancedBase = `{
  "listen": [8080],
  "server_name": "a.local",
  "location": [
    {
      "url": "/",
      "type": "local",
      "root": "./httpdoc/root",
      "index": ["index.html"]
    }
  ]
}
`

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
	out := patchAdvanced(t, advancedBase, want)
	got := reparse(t, out)
	if got.Locations[0].Zip == nil || strings.Join(*got.Locations[0].Zip, ",") != "gzip,br" {
		t.Fatalf("zip 没写进去:\n%s", out)
	}
	// 没动过的键必须原样保留
	if !strings.Contains(out, `"index": ["index.html"]`) {
		t.Fatalf("既有 index 被改动了:\n%s", out)
	}
}

func TestPatchLimit(t *testing.T) {
	limit := PathLimit{Rate: 50, Burst: 100}
	want := SiteInput{
		Listens:    []string{"8080"},
		ServerName: "a.local",
		Locations:  []SiteLocation{{URL: "/", Type: "local", Root: "./httpdoc/root", Limit: &limit}},
	}
	out := patchAdvanced(t, advancedBase, want)
	got := reparse(t, out)
	if got.Locations[0].Limit == nil {
		t.Fatalf("limit 没写进去:\n%s", out)
	}
	if l := got.Locations[0].Limit; l.Rate != 50 || l.Burst != 100 {
		t.Fatalf("limit 值不对: %+v", l)
	}
}

func TestPatchAuth(t *testing.T) {
	auth := PathAuth{AuthType: "basic", User: "admin", Pswd: "s3cret"}
	want := SiteInput{
		Listens:    []string{"8080"},
		ServerName: "a.local",
		Locations:  []SiteLocation{{URL: "/", Type: "local", Root: "./httpdoc/root", Auth: &auth}},
	}
	out := patchAdvanced(t, advancedBase, want)
	got := reparse(t, out)
	if got.Locations[0].Auth == nil || got.Locations[0].Auth.User != "admin" {
		t.Fatalf("auth 没写进去:\n%s", out)
	}
	if got.Locations[0].Auth.Pswd != "s3cret" {
		t.Fatalf("auth 口令不对: %+v", got.Locations[0].Auth)
	}
}

func TestPatchCache(t *testing.T) {
	cache := Cache{Path: "./cache", Valid: []string{"10s", "1m"}}
	want := SiteInput{
		Listens:    []string{"8080"},
		ServerName: "a.local",
		Locations:  []SiteLocation{{URL: "/", Type: "local", Root: "./httpdoc/root", Cache: &cache}},
	}
	out := patchAdvanced(t, advancedBase, want)
	got := reparse(t, out)
	if got.Locations[0].Cache == nil {
		t.Fatalf("proxy_cache 没写进去:\n%s", out)
	}
	c := got.Locations[0].Cache
	if c.Path != "./cache" || strings.Join(c.Valid, ",") != "10s,1m" {
		t.Fatalf("proxy_cache 值不对: %+v", c)
	}
}

// 指向零值 = 清除，补丁应把键删掉而不是写成 {}。
func TestPatchAdvancedClears(t *testing.T) {
	withAll := `{
  "listen": [8080],
  "server_name": "a.local",
  "location": [
    {
      "url": "/",
      "type": "local",
      "root": "./httpdoc/root",
      "zip": ["gzip"],
      "limit": {"mem":0,"rate":50,"burst":100,"nodelay":false},
      "auth": {"type":"basic","user":"admin","pswd":"s3cret","realm":""},
      "proxy_cache": {"path":"./cache","valid":["10s"],"key":"","methods":null,"max_size":0}
    }
  ]
}
`
	emptyZip := []string{}
	emptyLimit := PathLimit{}
	emptyAuth := PathAuth{}
	emptyCache := Cache{}
	want := SiteInput{
		Listens:    []string{"8080"},
		ServerName: "a.local",
		Locations: []SiteLocation{{
			URL: "/", Type: "local", Root: "./httpdoc/root",
			Zip: &emptyZip, Limit: &emptyLimit, Auth: &emptyAuth, Cache: &emptyCache,
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
	limit := PathLimit{Rate: 10}
	want := SiteInput{
		Listens:    []string{"8080"},
		ServerName: "a.local",
		Locations:  []SiteLocation{{URL: "/", Type: "local", Root: "./httpdoc/root", Zip: &zip, Limit: &limit}},
	}
	out := patchAdvanced(t, advancedBase, want)
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
	out := patchAdvanced(t, advancedBase, want)
	for _, key := range []string{`"limit"`, `"auth"`, `"proxy_cache"`} {
		if strings.Contains(out, key) {
			t.Fatalf("%s 不该出现:\n%s", key, out)
		}
	}
}
