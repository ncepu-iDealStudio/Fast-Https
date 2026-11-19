package sitecmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fast-https/config"
)

// location 子命令：新增 / 修改 / 删除额外路径，主 location / 不允许删除。
func TestLocationCLI(t *testing.T) {
	mainPath, includeDir := newSiteEnv(t)
	opts := Options{
		ConfigPath:     mainPath,
		Name:           "web",
		Listens:        []string{"18081"},
		ServerName:     "web.local",
		Root:           t.TempDir(),
		LocationType:   "local",
		NonInteractive: true,
		Out:            &bytes.Buffer{},
	}
	if err := createSiteForTest(t, opts); err != nil {
		t.Fatalf("create site: %v", err)
	}
	path := filepath.Join(includeDir, "web.json")

	if _, err := runSiteSubcmd(t, mainPath, "location", "add", "web",
		"--url", "/api", "--type", "proxy", "--proxy-pass", "http://127.0.0.1:9000"); err != nil {
		t.Fatalf("location add: %v", err)
	}
	locs := readLocations(t, path)
	if len(locs) != 2 || locs[1].URL != "/api" || locs[1].ProxyPass != "http://127.0.0.1:9000" {
		t.Fatalf("locations after add = %+v", locs)
	}

	if _, err := runSiteSubcmd(t, mainPath, "location", "add", "web",
		"--url", "/api", "--type", "proxy", "--proxy-pass", "http://127.0.0.1:9001"); err == nil {
		t.Fatal("重复 url 应拒绝")
	}

	if _, err := runSiteSubcmd(t, mainPath, "location", "update", "web",
		"--url", "/api", "--proxy-pass", "http://127.0.0.1:9100"); err != nil {
		t.Fatalf("location update: %v", err)
	}
	locs = readLocations(t, path)
	if locs[1].ProxyPass != "http://127.0.0.1:9100" {
		t.Fatalf("proxy_pass not updated: %+v", locs[1])
	}

	if _, err := runSiteSubcmd(t, mainPath, "location", "delete", "web", "--url", "/api"); err == nil {
		t.Fatal("删除应要求 --yes")
	}
	if _, err := runSiteSubcmd(t, mainPath, "location", "delete", "web", "--url", "/api", "--yes"); err != nil {
		t.Fatalf("location delete: %v", err)
	}
	if locs = readLocations(t, path); len(locs) != 1 || locs[0].URL != "/" {
		t.Fatalf("locations after delete = %+v", locs)
	}

	if _, err := runSiteSubcmd(t, mainPath, "location", "delete", "web", "--url", "/", "--yes"); err == nil {
		t.Fatal("主 location / 不允许删除")
	}

	// list：文本表格要同时列出主 location 与额外路径。
	if _, err := runSiteSubcmd(t, mainPath, "location", "add", "web",
		"--url", "/assets", "--type", "local", "--root", t.TempDir()); err != nil {
		t.Fatalf("location add local: %v", err)
	}
	out, err := runSiteSubcmd(t, mainPath, "location", "list", "web")
	if err != nil {
		t.Fatalf("location list: %v", err)
	}
	if !strings.Contains(out, "/") || !strings.Contains(out, "/assets") || !strings.Contains(out, "local") {
		t.Fatalf("list output missing rows:\n%s", out)
	}

	// set：整体替换额外 location，主 / 保持不动。
	if _, err := runSiteSubcmd(t, mainPath, "location", "set", "web",
		"--location", `{"url":"/api","type":"proxy","proxy_pass":"http://127.0.0.1:9200"}`); err != nil {
		t.Fatalf("location set: %v", err)
	}
	locs = readLocations(t, path)
	if len(locs) != 2 || locs[0].URL != "/" || locs[1].URL != "/api" ||
		locs[1].ProxyPass != "http://127.0.0.1:9200" {
		t.Fatalf("locations after set = %+v", locs)
	}

	// set 不传 --location：清空额外 location。
	if _, err := runSiteSubcmd(t, mainPath, "location", "set", "web"); err != nil {
		t.Fatalf("location set empty: %v", err)
	}
	if locs = readLocations(t, path); len(locs) != 1 || locs[0].URL != "/" {
		t.Fatalf("locations after empty set = %+v", locs)
	}

	// 非法 JSON 在进入事务前拒绝。
	if _, err := runSiteSubcmd(t, mainPath, "location", "set", "web", "--location", "{"); err == nil {
		t.Fatal("非法 --location JSON 应拒绝")
	}
}

func readLocations(t *testing.T, path string) []config.SiteLocation {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	in, err := config.ParseSiteInput(raw)
	if err != nil {
		t.Fatal(err)
	}
	return in.Locations
}
