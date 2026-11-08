package cmd

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// 下面这些已知口令写进夹具，所有只读命令的任何输出里都不能出现。
var fixtureSecrets = []string{"S3cr3t-XYZ", "HMAC-ABC", "KID-1", "Bearer tok-ABC"}

type cfgFixture struct {
	dir   string
	main  string
	confd string
	site  string
	port  int
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

const defaultAcmeJSON = `{"enabled": false, "cas": [{"name": "zerossl", "directory": "https://acme.zerossl.com/v2/DV90", "eab_kid": "KID-1", "eab_hmac_key": "HMAC-ABC"}]}`

func writeFixture(t *testing.T, acmeJSON string) cfgFixture {
	t.Helper()
	dir := t.TempDir()
	f := cfgFixture{
		dir:   dir,
		main:  filepath.Join(dir, "fast-https.json"),
		confd: filepath.Join(dir, "conf.d"),
		port:  freePort(t),
	}
	f.site = filepath.Join(f.confd, "demo.json")
	if err := os.MkdirAll(f.confd, 0o755); err != nil {
		t.Fatal(err)
	}

	site := fmt.Sprintf(`{
  "listen": [%d],
  "server_name": "demo.example",
  "location": [{
    "url": "/", "type": "local", "root": %q,
    "auth": {"type": "basic", "user": "admin", "pswd": "S3cr3t-XYZ"},
    "proxy_set_header": [{"HeaderKey": "Authorization", "HeaderValue": "Bearer tok-ABC"}]
  }]
}`, f.port, dir)
	if err := os.WriteFile(f.site, []byte(site), 0o644); err != nil {
		t.Fatal(err)
	}

	main := fmt.Sprintf(`{
  // main config
  "acme": %s,
  "http": {"include": [%q]}
}`, acmeJSON, f.confd)
	if err := os.WriteFile(f.main, []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}
	return f
}

func hashTree(t *testing.T, root string) map[string]string {
	t.Helper()
	sums := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sums[path] = fmt.Sprintf("%x", sha256.Sum256(raw))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return sums
}

func TestServiceStatusStopped(t *testing.T) {
	skipIfPidFilePresent(t)
	f := writeFixture(t, defaultAcmeJSON)

	out, _, code := runCLI(t, "service", "status", "--config", f.main, "--json")
	if code != 0 {
		t.Fatalf("exit = %d, want 0（服务没在跑也是一次成功的查询）", code)
	}
	data := decodeOneDocument(t, out)["data"].(map[string]any)
	if data["status"] != "stopped" {
		t.Fatalf("status = %v, want stopped", data["status"])
	}
	for _, absent := range []string{"pid", "stale_pid", "uptime_seconds", "started_at", "config_version"} {
		if _, has := data[absent]; has {
			t.Errorf("拿不到的字段 %q 不应出现：%v", absent, data)
		}
	}
	if data["source"] == "" || data["observed_at"] == "" {
		t.Fatalf("缺少 source 或 observed_at：%v", data)
	}
	ports := data["ports"].([]any)
	if len(ports) != 1 || ports[0].(map[string]any)["reachable"] != false {
		t.Fatalf("ports = %v", ports)
	}
}

func TestServiceStatusText(t *testing.T) {
	skipIfPidFilePresent(t)
	f := writeFixture(t, defaultAcmeJSON)

	out, _, code := runCLI(t, "service", "status", "--config", f.main)
	if code != 0 || !strings.HasPrefix(out, "fast-https: stopped") {
		t.Fatalf("exit=%d out=%q", code, out)
	}
}

func TestConfigShowAndSiteViewsRedact(t *testing.T) {
	f := writeFixture(t, defaultAcmeJSON)

	invocations := [][]string{
		{"config", "show", "--config", f.main, "--json"},
		{"config", "show", "--config", f.main},
		{"config", "show", "--config", f.main, "--section", "acme", "--json"},
		{"config", "show", "--config", f.main, "--section", "sites", "--json"},
		{"site", "list", "--config", f.main, "--json"},
		{"site", "show", "demo", "--config", f.main, "--json"},
		{"site", "show", "demo", "--config", f.main, "--merged", "--json"},
	}
	for _, args := range invocations {
		out, errOut, code := runCLI(t, args...)
		if code != 0 {
			t.Fatalf("%v exit = %d\n%s\n%s", args, code, out, errOut)
		}
		for _, secret := range fixtureSecrets {
			if strings.Contains(out, secret) || strings.Contains(errOut, secret) {
				t.Errorf("%v 的输出泄露了 %q", args, secret)
			}
		}
	}

	// 脱敏不能把整段内容抹掉：非敏感字段与遮蔽标记都要在。
	out, _, _ := runCLI(t, "site", "show", "demo", "--config", f.main, "--json")
	if !strings.Contains(out, "demo.example") || !strings.Contains(out, "***") || !strings.Contains(out, "admin") {
		t.Fatalf("脱敏后的内容不对：%s", out)
	}
}

func TestConfigShowSection(t *testing.T) {
	f := writeFixture(t, defaultAcmeJSON)

	out, _, code := runCLI(t, "config", "show", "--config", f.main, "--section", "acme", "--json")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	data := decodeOneDocument(t, out)["data"].(map[string]any)
	cfg := data["config"].(map[string]any)
	if _, ok := cfg["acme"]; !ok || len(cfg) != 1 {
		t.Fatalf("--section acme 应只含 acme：%v", cfg)
	}
	if _, has := data["sites"]; has {
		t.Fatal("非 sites 的 section 不应带站点")
	}

	if _, _, code := runCLI(t, "config", "show", "--config", f.main, "--section", "bogus", "--json"); code != 2 {
		t.Fatalf("非法 section exit = %d, want 2", code)
	}
}

func TestConfigShowWholeHasSourceAndFiles(t *testing.T) {
	f := writeFixture(t, defaultAcmeJSON)

	out, _, _ := runCLI(t, "config", "show", "--config", f.main, "--json")
	data := decodeOneDocument(t, out)["data"].(map[string]any)
	if data["source"] != "disk" {
		t.Fatalf("source = %v", data["source"])
	}
	if files := data["files"].([]any); len(files) != 2 {
		t.Fatalf("files = %v", files)
	}
	if sites := data["sites"].([]any); len(sites) != 1 {
		t.Fatalf("sites = %v", sites)
	}
}

func TestConfigShowMissingFile(t *testing.T) {
	_, _, code := runCLI(t, "config", "show", "--config", filepath.Join(t.TempDir(), "nope.json"), "--json")
	if code != 9 {
		t.Fatalf("exit = %d, want 9", code)
	}
}

func TestConfigValidateOK(t *testing.T) {
	f := writeFixture(t, defaultAcmeJSON)

	out, _, code := runCLI(t, "config", "validate", "--config", f.main, "--json")
	if code != 0 {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	data := decodeOneDocument(t, out)["data"].(map[string]any)
	if data["valid"] != true || data["candidate"] != false {
		t.Fatalf("data = %v", data)
	}
}

func TestConfigValidateInvalid(t *testing.T) {
	f := writeFixture(t, defaultAcmeJSON)
	bad := strings.Replace(mustRead(t, f.site), `"server_name": "demo.example"`, `"server_name": ""`, 1)
	if err := os.WriteFile(f.site, []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}

	out, _, code := runCLI(t, "config", "validate", "--config", f.main, "--json")
	if code != 5 {
		t.Fatalf("exit = %d, want 5", code)
	}
	doc := decodeOneDocument(t, out)
	body := doc["error"].(map[string]any)
	if doc["ok"] != false || body["code"] != CodeConfigInvalid {
		t.Fatalf("错误封套不对：%v", doc)
	}
	if details := body["details"].([]any); len(details) == 0 {
		t.Fatal("details 不应为空")
	}
	if !strings.Contains(body["message"].(string), "server_name") {
		t.Fatalf("message 应指出问题：%v", body["message"])
	}
}

func TestConfigValidateCandidate(t *testing.T) {
	f := writeFixture(t, defaultAcmeJSON)

	out, _, code := runCLI(t, "config", "validate", "--candidate", f.main, "--json")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	doc := decodeOneDocument(t, out)
	if doc["data"].(map[string]any)["candidate"] != true {
		t.Fatalf("data = %v", doc["data"])
	}
	found := false
	for _, w := range doc["warnings"].([]any) {
		found = found || strings.Contains(w.(string), "candidate scope is main file only")
	}
	if !found {
		t.Fatalf("候选校验应提示范围限制：%v", doc["warnings"])
	}

	if _, _, code := runCLI(t, "config", "validate", "--candidate", filepath.Join(t.TempDir(), "x.json"), "--json"); code != 9 {
		t.Fatalf("候选文件不存在 exit = %d, want 9", code)
	}
}

func TestConfigValidateWarnings(t *testing.T) {
	// http-01 启用而没有任何站点监听 80：校验通过，但要有告警。
	f := writeFixture(t, `{"enabled": true, "email": "ops@example.com", "challenge": ["http-01"]}`)

	out, _, code := runCLI(t, "config", "validate", "--config", f.main, "--json")
	if code != 0 {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	doc := decodeOneDocument(t, out)
	found := false
	for _, w := range doc["warnings"].([]any) {
		found = found || strings.Contains(w.(string), "no http.server listens on port 80")
	}
	if !found {
		t.Fatalf("warnings 里应有 http-01 告警：%v", doc["warnings"])
	}
}

func TestSiteListJSON(t *testing.T) {
	f := writeFixture(t, defaultAcmeJSON)

	out, _, code := runCLI(t, "site", "list", "--config", f.main, "--json")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	data := decodeOneDocument(t, out)["data"].(map[string]any)
	sites := data["sites"].([]any)
	if len(sites) != 1 {
		t.Fatalf("sites = %v", sites)
	}
	row := sites[0].(map[string]any)
	if row["name"] != "demo" || row["server_name"] != "demo.example" || row["listen"] != fmt.Sprint(f.port) {
		t.Fatalf("row = %v", row)
	}
	if _, has := row["error"]; has {
		t.Fatalf("合法站点不应带 error：%v", row)
	}
}

func TestSiteShowNotFoundAndBadName(t *testing.T) {
	f := writeFixture(t, defaultAcmeJSON)

	if _, _, code := runCLI(t, "site", "show", "nope", "--config", f.main, "--json"); code != 9 {
		t.Fatalf("不存在的站点 exit = %d, want 9", code)
	}
	if _, _, code := runCLI(t, "site", "show", "../evil", "--config", f.main, "--json"); code != 2 {
		t.Fatalf("带路径的站点名 exit = %d, want 2", code)
	}
}

// 文本形态是旧契约：没加 --json 时 site list 的表头与行格式不能变。
func TestLegacySiteListTextUnchanged(t *testing.T) {
	f := writeFixture(t, defaultAcmeJSON)

	out, _, code := runCLI(t, "site", "list", "--config", f.main)
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	wantHeader := fmt.Sprintf("%-16s %-14s %-28s %s", "NAME", "LISTEN", "SERVER_NAME", "LOCATION")
	if len(lines) != 2 || lines[0] != wantHeader {
		t.Fatalf("文本输出变了：\n%s", out)
	}
	if !strings.HasPrefix(lines[1], fmt.Sprintf("%-16s %-14s %-28s ", "demo", fmt.Sprint(f.port), "demo.example")) {
		t.Fatalf("行格式变了：%q", lines[1])
	}
}

// 所有 S2 命令都只读：跑完之后配置目录一个字节都没变，也没有新文件。
func TestReadOnlyCommandsDoNotWrite(t *testing.T) {
	f := writeFixture(t, defaultAcmeJSON)
	before := hashTree(t, f.dir)

	for _, args := range [][]string{
		{"service", "status", "--config", f.main, "--json"},
		{"config", "show", "--config", f.main, "--json"},
		{"config", "validate", "--config", f.main, "--json"},
		{"site", "list", "--config", f.main, "--json"},
		{"site", "show", "demo", "--config", f.main, "--json"},
		{"site", "show", "demo", "--config", f.main, "--merged", "--json"},
	} {
		if _, _, code := runCLI(t, args...); code != 0 {
			t.Fatalf("%v exit = %d", args, code)
		}
	}

	after := hashTree(t, f.dir)
	if len(before) != len(after) {
		t.Fatalf("文件数变了：%d -> %d", len(before), len(after))
	}
	var changed []string
	for path, sum := range before {
		if after[path] != sum {
			changed = append(changed, path)
		}
	}
	sort.Strings(changed)
	if len(changed) > 0 {
		t.Fatalf("只读命令改了文件：%v", changed)
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
