package config

import (
	"bytes"
	"errors"
	"fast-https/infra/logging"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

// writeTree 写一个主配置加一个 include 站点，返回主配置路径。
func writeTree(t *testing.T, acme, siteListen string) string {
	t.Helper()
	dir := t.TempDir()
	sites := filepath.Join(dir, "conf.d")
	if err := os.MkdirAll(sites, 0o755); err != nil {
		t.Fatal(err)
	}
	site := fmt.Sprintf(`{"listen": %s, "server_name": "example.com",
  "location": [{"url": "/", "type": "local", "root": %q}]}`, siteListen, dir)
	if err := os.WriteFile(filepath.Join(sites, "site.json"), []byte(site), 0o644); err != nil {
		t.Fatal(err)
	}
	main := fmt.Sprintf(`{"acme": %s, "http": {"include": [%q]}}`, acme, sites)
	path := filepath.Join(dir, "fast-https.json")
	if err := os.WriteFile(path, []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

const http01Acme = `{"enabled": true, "email": "ops@example.com", "challenge": ["http-01"]}`

func TestValidateWithWarningsReturnsAndDoesNotLog(t *testing.T) {
	path := writeTree(t, http01Acme, `[8080]`)

	var buf bytes.Buffer
	old := logging.ConsoleLog
	logging.ConsoleLog = zerolog.New(&buf)
	t.Cleanup(func() { logging.ConsoleLog = old })

	warnings, err := ValidateConfigFileWithWarnings(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "no http.server listens on port 80") {
		t.Fatalf("warnings = %v", warnings)
	}
	if buf.Len() != 0 {
		t.Fatalf("带告警的入口不应写日志：%s", buf.String())
	}
}

func TestValidateWithWarningsQuietWhenPort80Listens(t *testing.T) {
	path := writeTree(t, http01Acme, `[80]`)
	warnings, err := ValidateConfigFileWithWarnings(path)
	if err != nil || len(warnings) != 0 {
		t.Fatalf("warnings = %v, err = %v", warnings, err)
	}
}

func TestValidateWithWarningsFailureHasNoWarnings(t *testing.T) {
	path := writeTree(t, http01Acme, `[]`)
	warnings, err := ValidateConfigFileWithWarnings(path)
	if err == nil {
		t.Fatal("没有 listen 的站点应校验失败")
	}
	if warnings != nil {
		t.Fatalf("失败时不返回告警：%v", warnings)
	}
}

// 旧入口的行为不变：仍然写同一条日志，返回值仍是 error。
func TestValidateConfigFileStillLogsWarning(t *testing.T) {
	path := writeTree(t, http01Acme, `[8080]`)

	var buf bytes.Buffer
	old := logging.ConsoleLog
	logging.ConsoleLog = zerolog.New(&buf)
	t.Cleanup(func() { logging.ConsoleLog = old })

	if err := ValidateConfigFile(path); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "no http.server listens on port 80") {
		t.Fatalf("旧入口应继续写告警日志：%s", buf.String())
	}
}

func TestListenPortsFromFile(t *testing.T) {
	path := writeTree(t, `{"enabled": false}`, `[8080, "8443 ssl h2", 8080]`)

	ports, err := ListenPortsFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := []int{8080, 8443}; !reflect.DeepEqual(ports, want) {
		t.Fatalf("ports = %v, want %v", ports, want)
	}
}

func TestListenPortsFromFileMissing(t *testing.T) {
	if _, err := ListenPortsFromFile(filepath.Join(t.TempDir(), "nope.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err = %v, want ErrNotExist", err)
	}
}

func TestSnapshotServerNotFoundIsSentinel(t *testing.T) {
	path := writeTree(t, `{"enabled": false}`, `[8080]`)

	_, _, err := SnapshotServer(path, "missing.example")
	if !errors.Is(err, ErrNoServerBlock) {
		t.Fatalf("err = %v, want ErrNoServerBlock", err)
	}
	// 文案保持原样：旧命令直接把它打印给用户。
	if want := `no server block with server_name "missing.example"`; err.Error() != want {
		t.Fatalf("err = %q, want %q", err.Error(), want)
	}
}
