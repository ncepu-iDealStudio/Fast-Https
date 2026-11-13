package site

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fast-https/config"
)

// writeSiteFile 在临时 include 目录里放一个站点文件，返回目录。
func writeSiteFile(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(Path(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write site file: %v", err)
	}
	return dir
}

func readSiteFile(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(Path(dir, name))
	if err != nil {
		t.Fatalf("read site file: %v", err)
	}
	return string(data)
}

// 站点文件里 site 这一层没有建模的字段，改其它字段后必须原样还在。
// 这是 site init 曾经静默吃掉 auth/proxy_cache/appfirewall 的回归测试。
//
// 注意：auth / proxy_cache / limit / zip 现在都已建模（SiteLocation 有对应指针
// 字段），未点名时靠 nil 保证不参与比对，另有 config/site_advanced_test.go 覆盖。
// 这里留的必须是仍然没有建模的键，否则测不到「未知键透传」这件事。
func TestUpdateKeepsFieldsSiteDoesNotModel(t *testing.T) {
	const content = `{
  // 手写说明：这个站点挂了认证与防火墙
  "listen": 80,
  "server_name": "a.example",
  "location": [
    {
      "url": "/",
      "type": "local",
      "root": "/var/www/old",
      "index": ["index.html"],
      "auth": { "type": "basic", "user": "u", "pswd": "p", "realm": "a" },
      "proxy_cache": { "path": "/tmp/cache", "valid": ["200 1m"] },
      "appfirewall": ["xss"],
      "limit": { "mem": 1024, "rate": 10, "burst": 20, "nodelay": true },
      "allow_methods": ["GET", "POST"],
      "try_files": ["$uri", "/fallback"]
    }
  ]
}`
	dir := writeSiteFile(t, "a", content)

	if _, err := Update([]string{dir}, "a", func(in *config.SiteInput) error {
		in.Locations[0].Root = "/var/www/new"
		return nil
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got := readSiteFile(t, dir, "a")
	for _, want := range []string{
		"手写说明", `"auth"`, `"proxy_cache"`, `"appfirewall"`, `"limit"`,
		`"allow_methods"`, `"try_files"`,
		`"root": "/var/www/new"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("回写后丢了 %q：\n%s", want, got)
		}
	}
	if strings.Contains(got, "/var/www/old") {
		t.Errorf("root 没改掉：\n%s", got)
	}
}

// 切成反代后，local 才用的 root/index 必须删掉，否则两个字段会同时存在。
func TestUpdateSwitchToProxyDropsLocalOnlyFields(t *testing.T) {
	const content = `{
  "listen": 80,
  "server_name": "a.example",
  "location": [
    {
      "url": "/",
      "type": "local",
      "root": "/var/www/old",
      "index": ["index.html"],
      "auth": { "type": "basic", "user": "u", "pswd": "p" }
    }
  ]
}`
	dir := writeSiteFile(t, "a", content)

	if _, err := Update([]string{dir}, "a", func(in *config.SiteInput) error {
		in.Locations[0].Type = "proxy"
		in.Locations[0].ProxyPass = "http://127.0.0.1:8080"
		in.Locations[0].Root = ""
		in.Locations[0].Index = nil
		return nil
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got := readSiteFile(t, dir, "a")
	if strings.Contains(got, `"root"`) || strings.Contains(got, `"index"`) {
		t.Errorf("切到 proxy 后应删掉 root/index：\n%s", got)
	}
	for _, want := range []string{`"proxy_pass": "http://127.0.0.1:8080"`, `"auth"`} {
		if !strings.Contains(got, want) {
			t.Errorf("缺少 %q：\n%s", want, got)
		}
	}
}

// 加监听与证书路径时不能碰到 location 里的任何东西。
func TestUpdateAddsListenAndCerts(t *testing.T) {
	const content = `{
  "listen": 80,
  "server_name": "a.example",
  "location": [
    { "url": "/", "type": "local", "root": "/var/www/a", "appfirewall": ["xss"] }
  ]
}`
	dir := writeSiteFile(t, "a", content)

	if _, err := Update([]string{dir}, "a", func(in *config.SiteInput) error {
		in.Listens = []string{"80", "443 ssl h2"}
		in.SSLCertificate = "/etc/fast-https/cert/a.example.pem"
		in.SSLCertificateKey = "/etc/fast-https/cert/a.example-key.pem"
		return nil
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got := readSiteFile(t, dir, "a")
	for _, want := range []string{
		`"443 ssl h2"`, `"ssl_certificate"`, `"ssl_certificate_key"`,
		`"appfirewall"`, `"root": "/var/www/a"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("缺少 %q：\n%s", want, got)
		}
	}
}

// 文件里没有 location 键时，追加 location 要先把数组建出来。
func TestUpdateCreatesLocationArray(t *testing.T) {
	const content = `{
  "listen": 80,
  "server_name": "a.example"
}`
	dir := writeSiteFile(t, "a", content)

	if _, err := Update([]string{dir}, "a", func(in *config.SiteInput) error {
		in.Locations = append(in.Locations, config.SiteLocation{
			URL:  "/",
			Type: "local",
			Root: "/var/www/a",
		})
		return nil
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got := readSiteFile(t, dir, "a")
	if !strings.Contains(got, `"location"`) || !strings.Contains(got, `"root": "/var/www/a"`) {
		t.Fatalf("没有建出 location 数组：\n%s", got)
	}

	in, err := config.ParseSiteInput([]byte(got))
	if err != nil {
		t.Fatalf("回写结果解析失败：%v\n%s", err, got)
	}
	if len(in.Locations) != 1 || in.Locations[0].Root != "/var/www/a" {
		t.Fatalf("回写结果不对：%+v", in.Locations)
	}
}

// 没有任何变化时文件应当一个字节都不动。
func TestUpdateWithoutChangesKeepsBytes(t *testing.T) {
	const content = `{
  "listen": 80,
  "server_name": "a.example",
  "location": [
    { "url": "/", "type": "local", "root": "/var/www/a" }
  ]
}`
	dir := writeSiteFile(t, "a", content)

	if _, err := Update([]string{dir}, "a", func(in *config.SiteInput) error { return nil }); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got := readSiteFile(t, dir, "a"); got != content {
		t.Fatalf("空操作也应保持原字节：\n%s", got)
	}
}

// 临时文件不能留在目录里。
func TestUpdateLeavesNoTempFile(t *testing.T) {
	dir := writeSiteFile(t, "a", `{"listen":80,"server_name":"a","location":[{"url":"/","type":"local","root":"/x"}]}`)

	if _, err := Update([]string{dir}, "a", func(in *config.SiteInput) error {
		in.Locations[0].Root = "/y"
		return nil
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if _, err := os.Stat(Path(dir, "a") + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("临时文件没清掉：%v", err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || filepath.Base(entries[0].Name()) != "a.json" {
		t.Fatalf("目录里留了多余文件：%v", entries)
	}
}

// zip 里出现不认识的值时，只改别的字段不能把它删掉。
//
// normalizeZip 一度按「只留 gzip/br」过滤，于是手写的 "zip": ["text/html"]
// 规范化成空列表，被当成「用户要求清空」而删除：只改 root 也会丢配置。
// 合法性该由写入前的校验负责，差异比对本就要如实反映文件内容。
func TestUpdateKeepsUnrecognizedZipValue(t *testing.T) {
	const content = `{
  "listen": 80,
  "server_name": "a.example",
  "location": [
    {
      "url": "/",
      "type": "local",
      "root": "/var/www/old",
      "zip": ["text/html"]
    }
  ]
}`
	dir := writeSiteFile(t, "a", content)
	if _, err := Update([]string{dir}, "a", func(in *config.SiteInput) error {
		in.Locations[0].Root = "/var/www/new"
		return nil
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got := readSiteFile(t, dir, "a")
	if !strings.Contains(got, `"zip"`) || !strings.Contains(got, "text/html") {
		t.Fatalf("未知的 zip 值被静默删掉了：\n%s", got)
	}
	if !strings.Contains(got, "/var/www/new") {
		t.Fatalf("root 没改掉：\n%s", got)
	}
}

// 反过来，用户明确要清空 zip 时必须真的删掉这个键。
func TestUpdateZipClearStillWorks(t *testing.T) {
	const content = `{
  "listen": 80,
  "server_name": "a.example",
  "location": [
    {
      "url": "/",
      "type": "local",
      "root": "/var/www",
      "zip": ["gzip", "br"]
    }
  ]
}`
	dir := writeSiteFile(t, "a", content)
	if _, err := Update([]string{dir}, "a", func(in *config.SiteInput) error {
		empty := []string{}
		in.Locations[0].Zip = &empty
		return nil
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got := readSiteFile(t, dir, "a")
	if strings.Contains(got, `"zip"`) {
		t.Fatalf("清空后 zip 键应该被删掉：\n%s", got)
	}
	if !strings.Contains(got, `"type": "local"`) {
		t.Fatalf("基础字段被误删：\n%s", got)
	}
}
