package config

import (
	"os"
	"path/filepath"
	"testing"
)

// mime.json 跟着主配置所在目录走：--config 指到别的目录时，不用把
// mime.json 摆到进程工作目录下也能读到。
func TestMIMEFollowsConfigDir(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "fast-https.json")
	writeTestFile(t, configPath, `{
  "http": {
    "server": [
      {"listen": 80, "server_name": "local", "location": [{"url": "/", "type": "local", "root": "."}]}
    ]
  }
}`)
	writeTestFile(t, filepath.Join(dir, "mime.json"), `{"weird": "application/x-weird"}`)

	previous := Current()
	previousMIME := MIME()
	t.Cleanup(func() { Store(previous, previousMIME) })

	if err := LoadFile(configPath); err != nil {
		t.Fatalf("load config: %v", err)
	}
	if got := MIME()["weird"]; got != "application/x-weird" {
		t.Fatalf("mime table = %q for weird, want the one next to the config", got)
	}
}

// 配置目录里没有 mime.json 时回落到安装基准目录下的 config/mime.json。
func TestMIMEFallsBackToBaseDir(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "fast-https.json")
	writeTestFile(t, configPath, `{
  "http": {
    "server": [
      {"listen": 80, "server_name": "local", "location": [{"url": "/", "type": "local", "root": "."}]}
    ]
  }
}`)

	chdirToRepoRoot(t)

	previous := Current()
	previousMIME := MIME()
	t.Cleanup(func() { Store(previous, previousMIME) })

	if err := LoadFile(configPath); err != nil {
		t.Fatalf("load config: %v", err)
	}
	if got := MIME()["css"]; got != "text/css" {
		t.Fatalf("fallback mime table = %q for css, want text/css", got)
	}
}

// chdirToRepoRoot 把工作目录切到仓库根，让安装基准目录 ./ 指向真实文件。
func chdirToRepoRoot(t *testing.T) {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := wd
	if filepath.Base(root) == "config" {
		root = filepath.Dir(root)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
