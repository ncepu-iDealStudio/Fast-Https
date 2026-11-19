package cacheconf

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"fast-https/config/internal/model"
)

func writeMain(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fast-https.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestWriteThenReadRoundTrip(t *testing.T) {
	path := writeMain(t, `{
  // 手写注释要保留
  "http": {
    "server": [{"listen": 80, "server_name": "a.local", "location": [{"url": "/", "type": "local", "root": "."}]}]
  }
}`)
	zones := []model.CacheZone{
		{Name: "edge", Path: "./cache/edge", Valid: []string{"200:10m", "any:30s"}, MaxSize: 1 << 20},
		{Name: "spare", Path: "./cache/spare"},
	}
	if err := WriteCacheZones(path, zones); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "手写注释") {
		t.Fatalf("注释被删：\n%s", raw)
	}
	got, err := ReadCacheZones(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "edge" || got[1].Name != "spare" {
		t.Fatalf("zones = %+v", got)
	}
	if !reflect.DeepEqual(got[0].Valid, zones[0].Valid) || got[0].MaxSize != zones[0].MaxSize {
		t.Fatalf("edge = %+v", got[0])
	}
}

func TestWriteEmptyZonesRemovesSection(t *testing.T) {
	path := writeMain(t, `{
  "cache": [{"name": "edge", "path": "./cache", "valid": [], "max_size": 0}],
  "http": {"server": [{"listen": 80, "server_name": "a.local", "location": [{"url": "/", "type": "local", "root": "."}]}]}
}`)
	if err := WriteCacheZones(path, nil); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), `"cache"`) {
		t.Fatalf("空列表应删除 cache 段：\n%s", raw)
	}
	got, err := ReadCacheZones(path)
	if err != nil || len(got) != 0 {
		t.Fatalf("zones = %+v, err = %v", got, err)
	}
}

// CacheZoneRefs 同时扫描主配置 http.server[] 与 include 目录（含停用文件）。
func TestCacheZoneRefs(t *testing.T) {
	dir := t.TempDir()
	sites := filepath.Join(dir, "conf.d")
	if err := os.MkdirAll(sites, 0o755); err != nil {
		t.Fatal(err)
	}
	main := filepath.Join(dir, "fast-https.json")
	body := fmt.Sprintf(`{
  "cache": [{"name": "edge", "path": "./cache", "valid": [], "max_size": 0}],
  "http": {
    "include": [%q],
    "server": [{"listen": 80, "server_name": "inline.local",
      "location": [{"url": "/", "type": "proxy", "proxy_pass": "http://127.0.0.1:8080", "proxy_cache": "edge"}]}]
  }
}`, sites)
	if err := os.WriteFile(main, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	site := `{"listen": 81, "server_name": "site.local",
  "location": [{"url": "/", "type": "proxy", "proxy_pass": "http://127.0.0.1:8081", "proxy_cache": "edge"}]}`
	if err := os.WriteFile(filepath.Join(sites, "site.json"), []byte(site), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sites, "off.json.disabled"), []byte(site), 0o644); err != nil {
		t.Fatal(err)
	}

	refs, err := CacheZoneRefs(main, "edge")
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 3 {
		t.Fatalf("refs = %v", refs)
	}
	for _, want := range []string{"http.server[0]", "site.json", "off.json.disabled"} {
		found := false
		for _, ref := range refs {
			if strings.Contains(ref, want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("refs 缺少 %s：%v", want, refs)
		}
	}

	other, err := CacheZoneRefs(main, "unused")
	if err != nil || len(other) != 0 {
		t.Fatalf("unused refs = %v, err = %v", other, err)
	}
}
