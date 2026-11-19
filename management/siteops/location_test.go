package siteops

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fast-https/config"
	"fast-https/modules/site"
)

// newLocationEnv 建一个只有主 location 的站点，返回 conf.d、主配置与站点根。
func newLocationEnv(t *testing.T) (dirs []string, mainPath, root string) {
	t.Helper()
	base := t.TempDir()
	dirs = []string{filepath.Join(base, "conf.d")}
	if err := os.MkdirAll(dirs[0], 0o755); err != nil {
		t.Fatal(err)
	}
	mainPath = filepath.Join(base, "fast-https.json")
	if err := os.WriteFile(mainPath, []byte(`{"http":{"include":["./conf.d"],"server":[]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	root = t.TempDir()
	in := config.SiteInput{
		Listens: []string{"18081"}, ServerName: "web.local",
		Locations: []config.SiteLocation{{URL: "/", Type: "local", Root: root}},
	}
	if _, err := site.Create(dirs, "web", in, false); err != nil {
		t.Fatalf("create site: %v", err)
	}
	return dirs, mainPath, root
}

func readLocations(t *testing.T, dirs []string) []config.SiteLocation {
	t.Helper()
	locs, err := ListLocations(dirs, "web")
	if err != nil {
		t.Fatalf("list locations: %v", err)
	}
	return locs
}

// 额外 location 的增改删与整体替换；主 location / 始终保持不变。
func TestLocationCRUD(t *testing.T) {
	dirs, mainPath, root := newLocationEnv(t)

	if _, err := AddLocation(dirs, mainPath, LocationInput{
		Site: "web", URL: "/api", Type: "proxy", ProxyPass: "http://127.0.0.1:9000",
	}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := AddLocation(dirs, mainPath, LocationInput{
		Site: "web", URL: "/static", Type: "local", Root: root,
	}); err != nil {
		t.Fatalf("add local: %v", err)
	}
	locs := readLocations(t, dirs)
	if len(locs) != 3 || locs[1].URL != "/api" || locs[2].Type != "local" || locs[2].Root == "" {
		t.Fatalf("locations = %+v", locs)
	}

	// 重复 url、主 location、非法类型都必须拒绝。
	if _, err := AddLocation(dirs, mainPath, LocationInput{Site: "web", URL: "/api", Type: "proxy", ProxyPass: "http://x"}); err == nil {
		t.Fatal("重复 /api 应拒绝")
	}
	if _, err := AddLocation(dirs, mainPath, LocationInput{Site: "web", URL: "/", Type: "proxy", ProxyPass: "http://x"}); err == nil {
		t.Fatal("主 location 应拒绝")
	}
	if _, err := AddLocation(dirs, mainPath, LocationInput{Site: "web", URL: "/bad", Type: "nope"}); err == nil {
		t.Fatal("非法类型应拒绝")
	}
	// url 在运行期编译成正则，非法模式会让 reload panic。
	if _, err := AddLocation(dirs, mainPath, LocationInput{
		Site: "web", URL: "/api(", Type: "proxy", ProxyPass: "http://127.0.0.1:9000",
	}); err == nil {
		t.Fatal("非法正则 URL 应拒绝")
	}

	pass := "http://127.0.0.1:9100"
	if _, err := UpdateLocation(dirs, mainPath, LocationUpdateInput{Site: "web", URL: "/api", ProxyPass: &pass}); err != nil {
		t.Fatalf("update: %v", err)
	}
	if got := readLocations(t, dirs)[1].ProxyPass; got != pass {
		t.Fatalf("proxy_pass = %q", got)
	}

	// 整体替换：只保留一个额外 location，主 / 不动。
	if _, err := SetExtraLocations(dirs, mainPath, "web", []config.SiteLocation{
		{URL: "/v2", Type: "proxy", ProxyPass: "http://127.0.0.1:9200"},
	}, nil); err != nil {
		t.Fatalf("set: %v", err)
	}
	locs = readLocations(t, dirs)
	if len(locs) != 2 || locs[0].URL != "/" || locs[1].URL != "/v2" {
		t.Fatalf("after set = %+v", locs)
	}

	// 清空额外 location。
	if _, err := SetExtraLocations(dirs, mainPath, "web", nil, nil); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if locs = readLocations(t, dirs); len(locs) != 1 || locs[0].URL != "/" {
		t.Fatalf("after clear = %+v", locs)
	}

	if _, err := AddLocation(dirs, mainPath, LocationInput{
		Site: "web", URL: "/api", Type: "proxy", ProxyPass: "http://127.0.0.1:9000",
	}); err != nil {
		t.Fatalf("re-add: %v", err)
	}
	if _, err := DeleteLocation(dirs, mainPath, "web", "/api"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if locs = readLocations(t, dirs); len(locs) != 1 {
		t.Fatalf("after delete = %+v", locs)
	}
	if _, err := DeleteLocation(dirs, mainPath, "web", "/"); err == nil {
		t.Fatal("主 location / 不允许删除")
	}
}

// 桌面端整体保存只带基础字段：同一 URL 上已有的高级配置必须保留。
func TestSetExtraLocationsKeepsAdvancedFields(t *testing.T) {
	dirs, mainPath, root := newLocationEnv(t)
	if _, err := AddLocation(dirs, mainPath, LocationInput{
		Site: "web", URL: "/static", Type: "local", Root: root,
	}); err != nil {
		t.Fatalf("add static: %v", err)
	}

	path, ok := site.Find(dirs, "web")
	if !ok {
		t.Fatal("site file not found")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	for _, item := range doc["location"].([]any) {
		m := item.(map[string]any)
		if m["url"] == "/static" {
			m["zip"] = []any{"gzip"}
			m["index"] = []any{"home.html"}
			m["allow_methods"] = []any{"GET", "HEAD"}
		}
	}
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatal(err)
	}

	// 与桌面端保存一致：只回传基础字段。
	if _, err := SetExtraLocations(dirs, mainPath, "web", []config.SiteLocation{
		{URL: "/static", Type: "local", Root: root},
	}, nil); err != nil {
		t.Fatalf("set: %v", err)
	}
	for _, loc := range readLocations(t, dirs) {
		if loc.URL != "/static" {
			continue
		}
		if loc.Zip == nil || len(*loc.Zip) != 1 || (*loc.Zip)[0] != "gzip" ||
			len(loc.Index) != 1 || loc.Index[0] != "home.html" {
			t.Fatalf("高级配置被丢弃：%+v", loc)
		}
		// allow_methods 不在 SiteLocation 模型里，必须靠原地补丁保留。
		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(after), `"allow_methods"`) {
			t.Fatalf("未建模字段被丢弃：\n%s", after)
		}
		return
	}
	t.Fatal("location /static not found")
}

// 改基础字段走原地补丁：类型切换时清掉另一类型的专属键。
func TestSetExtraLocationsInPlaceEdits(t *testing.T) {
	dirs, mainPath, root := newLocationEnv(t)
	if _, err := AddLocation(dirs, mainPath, LocationInput{
		Site: "web", URL: "/static", Type: "local", Root: root,
	}); err != nil {
		t.Fatalf("add static: %v", err)
	}

	// local → proxy：写 proxy_pass，清掉 root/index。
	if _, err := SetExtraLocations(dirs, mainPath, "web", []config.SiteLocation{
		{URL: "/static", Type: "proxy", ProxyPass: "http://127.0.0.1:9300"},
	}, nil); err != nil {
		t.Fatalf("to proxy: %v", err)
	}
	for _, loc := range readLocations(t, dirs) {
		if loc.URL != "/static" {
			continue
		}
		if loc.Type != "proxy" || loc.ProxyPass != "http://127.0.0.1:9300" ||
			loc.Root != "" || len(loc.Index) != 0 {
			t.Fatalf("to proxy result = %+v", loc)
		}
	}

	// proxy → local：写 root，清掉 proxy_pass。
	if _, err := SetExtraLocations(dirs, mainPath, "web", []config.SiteLocation{
		{URL: "/static", Type: "local", Root: root},
	}, nil); err != nil {
		t.Fatalf("to local: %v", err)
	}
	for _, loc := range readLocations(t, dirs) {
		if loc.URL != "/static" {
			continue
		}
		if loc.Type != "local" || loc.Root == "" || loc.ProxyPass != "" || len(loc.Index) == 0 {
			t.Fatalf("to local result = %+v", loc)
		}
		return
	}
	t.Fatal("location /static not found")
}
