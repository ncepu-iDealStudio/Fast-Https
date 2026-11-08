package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeCandidateTree(t *testing.T, siteBody string) string {
	t.Helper()
	dir := t.TempDir()
	confd := filepath.Join(dir, "conf.d")
	if err := os.MkdirAll(confd, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(confd, "site.json"), []byte(siteBody), 0o644); err != nil {
		t.Fatal(err)
	}
	main := `{"http":{"include":["conf.d"]}}`
	if err := os.WriteFile(filepath.Join(dir, "fast-https.json"), []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

const goodCandidateSite = `{"listen":[8080],"server_name":"example.com","location":[{"url":"/","type":"local","root":"."}]}`

func TestTreeValidateDoesNotReadLiveConfd(t *testing.T) {
	cand := writeCandidateTree(t, goodCandidateSite)

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	live := t.TempDir()
	if err := os.Chdir(live); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	if err := os.MkdirAll(filepath.Join("config", "conf.d"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("config", "conf.d", "evil.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("config", "fast-https.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := ValidateConfigTree(cand); err != nil {
		t.Fatalf("候选树应在线上 conf.d 不可用/有毒时仍通过：%v", err)
	}
}

func TestTreeValidateReportsCandidateErrors(t *testing.T) {
	cand := writeCandidateTree(t, `{`)
	_, err := ValidateConfigTree(cand)
	if err == nil {
		t.Fatal("坏的候选站点文件应失败")
	}
	bad := filepath.Join(cand, "conf.d", "site.json")
	if !strings.Contains(err.Error(), bad) {
		t.Fatalf("错误应指向候选文件 %s，实际：%v", bad, err)
	}
}

func TestTreeMissingCert(t *testing.T) {
	site := `{
  "listen": ["8443 ssl"],
  "server_name": "ssl.example",
  "ssl_certificate": "no-such-s7-cert.pem",
  "ssl_certificate_key": "no-such-s7-key.pem",
  "location": [{"url": "/", "type": "local", "root": "."}]
}`
	cand := writeCandidateTree(t, site)
	_, err := ValidateConfigTree(cand)
	if err == nil {
		t.Fatal("缺证书应 CONFIG_INVALID")
	}
	if !strings.Contains(err.Error(), "ssl_certificate not found") {
		t.Fatalf("err = %v", err)
	}
}

func TestValidateConfigFileUnchanged(t *testing.T) {
	path := writeTree(t, `{"enabled": false}`, `[8080]`)
	warnings, err := ValidateConfigFileWithWarnings(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("无 http-01 时不应有告警：%v", warnings)
	}

	path = writeTree(t, http01Acme, `[8080]`)
	warnings, err = ValidateConfigFileWithWarnings(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "no http.server listens on port 80") {
		t.Fatalf("http-01 告警快照变了：%v", warnings)
	}

	dir := t.TempDir()
	main := filepath.Join(dir, "fast-https.json")
	if err := os.WriteFile(main, []byte(`{
  "http": {
    "server": [{"listen": "bad-port", "server_name": "x", "location": [{"url": "/", "root": "."}]}]
  }
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = ValidateConfigFileWithWarnings(main)
	if err == nil || !strings.Contains(err.Error(), "invalid listen port: bad-port") {
		t.Fatalf("无效 listen 快照变了：%v", err)
	}
}

func TestCandidatePathRejectsDotDot(t *testing.T) {
	_, err := ResolveCandidateRoot("../x")
	if !errors.Is(err, ErrCandidatePath) {
		t.Fatalf("err = %v, want ErrCandidatePath", err)
	}
}

func TestCandidateSymlinkEscape(t *testing.T) {
	cand := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(cand, "fast-https.json"), []byte(`{"http":{"include":["conf.d"]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(cand, "conf.d")
	if err := os.Symlink(outside, target); err != nil {
		t.Skipf("无法创建符号链接：%v", err)
	}
	if err := os.WriteFile(filepath.Join(outside, "site.json"), []byte(goodCandidateSite), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ValidateConfigTree(cand)
	if !errors.Is(err, ErrCandidatePath) {
		t.Fatalf("符号链接逃逸 err = %v, want ErrCandidatePath", err)
	}
}

func TestTreeValidateAcceptsRuntimeCert(t *testing.T) {
	certDir := t.TempDir()
	cert := filepath.Join(certDir, "s7-runtime.pem")
	key := filepath.Join(certDir, "s7-runtime-key.pem")
	if err := os.WriteFile(cert, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(key, []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	site := fmt.Sprintf(`{
  "listen": ["8443 ssl"],
  "server_name": "ssl.example",
  "ssl_certificate": %q,
  "ssl_certificate_key": %q,
  "location": [{"url": "/", "type": "local", "root": "."}]
}`, cert, key)
	cand := writeCandidateTree(t, site)
	if _, err := ValidateConfigTree(cand); err != nil {
		t.Fatalf("绝对路径证书应通过：%v", err)
	}
}
