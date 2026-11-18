package cachecmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fast-https/config"
)

// writeEnv 写一个带 include 目录的最小主配置（一个内联站点，无缓存区）。
func writeEnv(t *testing.T) (mainPath, sites string) {
	t.Helper()
	dir := t.TempDir()
	sites = filepath.Join(dir, "conf.d")
	if err := os.MkdirAll(sites, 0o755); err != nil {
		t.Fatal(err)
	}
	mainPath = filepath.Join(dir, "fast-https.json")
	body := fmt.Sprintf(`{
  "http": {
    "include": [%q],
    "server": [{"listen": 80, "server_name": "a.local",
      "location": [{"url": "/", "type": "local", "root": "."}]}]
  }
}`, sites)
	if err := os.WriteFile(mainPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return mainPath, sites
}

func runCache(t *testing.T, mainPath string, args ...string) (string, error) {
	t.Helper()
	cmd := NewCacheCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(append(args, "--config", mainPath))
	err := cmd.Execute()
	return out.String(), err
}

func TestCacheAddListDelete(t *testing.T) {
	mainPath, _ := writeEnv(t)

	if _, err := runCache(t, mainPath, "add", "--name", "edge", "--path", "./cache/edge",
		"--valid", "200:10m", "--valid", "any:30s", "--max-size", "1024"); err != nil {
		t.Fatalf("add: %v", err)
	}
	zones, err := config.ReadCacheZones(mainPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(zones) != 1 || zones[0].Name != "edge" || zones[0].MaxSize != 1024 {
		t.Fatalf("zones = %+v", zones)
	}

	// 重名按 CONFLICT 拒绝。
	if _, err := runCache(t, mainPath, "add", "--name", "edge", "--path", "./other"); err == nil {
		t.Fatal("重名应报错")
	}

	if _, err := runCache(t, mainPath, "update", "edge", "--max-size", "2048"); err != nil {
		t.Fatalf("update: %v", err)
	}
	zones, _ = config.ReadCacheZones(mainPath)
	if zones[0].MaxSize != 2048 {
		t.Fatalf("max_size = %d", zones[0].MaxSize)
	}

	if _, err := runCache(t, mainPath, "delete", "edge"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	zones, _ = config.ReadCacheZones(mainPath)
	if len(zones) != 0 {
		t.Fatalf("zones = %+v", zones)
	}
}

// 仍被站点引用的缓存区不能删：CLI 要把引用位置说出来。
func TestCacheDeleteRefusesReferencedZone(t *testing.T) {
	mainPath, sites := writeEnv(t)
	if _, err := runCache(t, mainPath, "add", "--name", "edge", "--path", "./cache", "--valid", "any:10s"); err != nil {
		t.Fatal(err)
	}
	site := `{"listen": 81, "server_name": "b.local",
  "location": [{"url": "/", "type": "proxy", "proxy_pass": "http://127.0.0.1:8081", "proxy_cache": "edge"}]}`
	if err := os.WriteFile(filepath.Join(sites, "b.json"), []byte(site), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := runCache(t, mainPath, "delete", "edge")
	if err == nil {
		t.Fatal("被引用的缓存区应拒绝删除")
	}
	if !strings.Contains(err.Error(), "still referenced") || !strings.Contains(err.Error(), "b.json") {
		t.Fatalf("err = %v", err)
	}
}

// 非法 valid 必须在写盘前被完整校验拦下。
func TestCacheAddRejectsInvalidValid(t *testing.T) {
	mainPath, _ := writeEnv(t)
	if _, err := runCache(t, mainPath, "add", "--name", "edge", "--path", "./cache", "--valid", "10s"); err == nil {
		t.Fatal("非法 valid 应报错")
	}
	zones, _ := config.ReadCacheZones(mainPath)
	if len(zones) != 0 {
		t.Fatalf("校验失败不应写入：%+v", zones)
	}
}
