package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 造一个最小的主配置工作区：主配置 include conf.d，已有一个监听 80 的站点。
func writeSiteWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	confD := filepath.Join(dir, "config", "conf.d")
	if err := os.MkdirAll(confD, 0755); err != nil {
		t.Fatal(err)
	}
	main := `{"http": {"include": ["./conf.d"]}}`
	if err := os.WriteFile(filepath.Join(dir, "config", "fast-https.json"), []byte(main), 0644); err != nil {
		t.Fatal(err)
	}
	def := `{"listen": 80, "server_name": "localhost", "location": [{"url": "/", "type": "local", "root": "./httpdoc/root"}]}`
	if err := os.WriteFile(filepath.Join(confD, "default.json"), []byte(def), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func readNewSite(t *testing.T, dir, file string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "config", "conf.d", file))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestAddSiteLocalCreatesFile(t *testing.T) {
	dir := writeSiteWorkspace(t)
	_, err := addSiteFile(dir, SiteInput{
		Name: "example.com", Listen: "8080", Type: "local", Root: "./httpdoc/root",
	})
	if err != nil {
		t.Fatal(err)
	}
	m := readNewSite(t, dir, "example.com.json")
	if m["server_name"] != "example.com" {
		t.Fatalf("server_name = %v", m["server_name"])
	}
	listen := m["listen"].([]any)
	if len(listen) != 1 || listen[0] != float64(8080) {
		t.Fatalf("纯端口应写成 JSON 数字：%v", listen)
	}
	loc := m["location"].([]any)[0].(map[string]any)
	if loc["type"] != "local" || loc["root"] != "./httpdoc/root" || loc["url"] != "/" {
		t.Fatalf("location 不对：%v", loc)
	}
	idx := loc["index"].([]any)
	if len(idx) != 2 || idx[0] != "index.html" {
		t.Fatalf("默认 index 应为 index.html/index.htm：%v", idx)
	}
	if _, ok := m["ssl_certificate"]; ok {
		t.Fatal("未填证书时不应写出该字段")
	}
}

func TestAddSiteProxyWithMixedListen(t *testing.T) {
	dir := writeSiteWorkspace(t)
	_, err := addSiteFile(dir, SiteInput{
		Name: "api.local", Listen: "8081, 8443 ssl h2", Type: "proxy",
		ProxyPass: "http://127.0.0.1:9000", Cert: "./cert/a.pem", CertKey: "./cert/a-key.pem",
	})
	if err != nil {
		t.Fatal(err)
	}
	m := readNewSite(t, dir, "api.local.json")
	listen := m["listen"].([]any)
	if len(listen) != 2 || listen[0] != float64(8081) || listen[1] != "8443 ssl h2" {
		t.Fatalf("混合 listen 写法不对：%v", listen)
	}
	loc := m["location"].([]any)[0].(map[string]any)
	if loc["type"] != "proxy" || loc["proxy_pass"] != "http://127.0.0.1:9000" {
		t.Fatalf("反代 location 不对：%v", loc)
	}
	if m["ssl_certificate"] != "./cert/a.pem" || m["ssl_certificate_key"] != "./cert/a-key.pem" {
		t.Fatal("证书字段未写入")
	}
}

func TestAddSiteRejectsInvalidWithoutCreatingFile(t *testing.T) {
	cases := []struct {
		name string
		in   SiteInput
	}{
		{"空 server_name", SiteInput{Listen: "8080", Type: "local", Root: "./x"}},
		{"空 listen", SiteInput{Name: "a.local", Type: "local", Root: "./x"}},
		{"local 缺 root", SiteInput{Name: "a.local", Listen: "8080", Type: "local"}},
		{"proxy 缺 scheme", SiteInput{Name: "a.local", Listen: "8080", Type: "proxy", ProxyPass: "127.0.0.1:9000"}},
		{"proxy 空地址", SiteInput{Name: "a.local", Listen: "8080", Type: "proxy"}},
		{"非法类型", SiteInput{Name: "a.local", Listen: "8080", Type: "magic", Root: "./x"}},
		{"端口越界", SiteInput{Name: "a.local", Listen: "70000", Type: "local", Root: "./x"}},
		{"文件名路径穿越", SiteInput{Name: "a.local", Listen: "8080", Type: "local", Root: "./x", FileName: "../evil"}},
		{"同名同端口", SiteInput{Name: "localhost", Listen: "80", Type: "local", Root: "./x"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := writeSiteWorkspace(t)
			if _, err := addSiteFile(dir, c.in); err == nil {
				t.Fatal("应拒绝")
			}
			entries, err := os.ReadDir(filepath.Join(dir, "config", "conf.d"))
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 1 {
				t.Fatalf("校验失败时不应创建新文件，当前 %d 个", len(entries))
			}
		})
	}
}

func TestAddSiteRejectsExistingFile(t *testing.T) {
	dir := writeSiteWorkspace(t)
	in := SiteInput{Name: "example.com", Listen: "8080", Type: "local", Root: "./x"}
	if _, err := addSiteFile(dir, in); err != nil {
		t.Fatal(err)
	}
	if _, err := addSiteFile(dir, in); err == nil || !strings.Contains(err.Error(), "已存在") {
		t.Fatal("同名文件应拒绝覆盖")
	}
}

func TestAddSiteAllowsSamePortDifferentName(t *testing.T) {
	dir := writeSiteWorkspace(t)
	// default.json 已用 localhost:80，换一个域名共用 80 是合法的虚拟主机写法
	if _, err := addSiteFile(dir, SiteInput{Name: "other.local", Listen: "80", Type: "local", Root: "./x"}); err != nil {
		t.Fatal("不同域名共用同一端口应允许：", err)
	}
}

func TestRelativeToEtc(t *testing.T) {
	etc := t.TempDir()
	inside := filepath.Join(etc, "cert", "a.pem")
	if got := relativeToEtc(etc, inside); got != "./cert/a.pem" {
		t.Fatalf("config 内应转成 ./cert/a.pem，实际 %q", got)
	}
	outside := filepath.Join(t.TempDir(), "b.pem")
	if got := relativeToEtc(etc, outside); !filepath.IsAbs(filepath.FromSlash(got)) {
		t.Fatalf("config 外应保留绝对路径，实际 %q", got)
	}
}

func TestAddSiteSanitizesNonASCIIName(t *testing.T) {
	dir := writeSiteWorkspace(t)
	if _, err := addSiteFile(dir, SiteInput{Name: "示例 站点", Listen: "8080", Type: "local", Root: "./x"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "config", "conf.d", "site.json")); err != nil {
		t.Fatal("非 ASCII 名称应回落到安全文件名 site.json：", err)
	}
}

func TestReadSiteDetail(t *testing.T) {
	dir := writeSiteWorkspace(t)
	d, err := readSiteDetail(dir, "conf.d/default.json")
	if err != nil {
		t.Fatal(err)
	}
	if !d.Editable || d.Source != "conf.d/default.json" {
		t.Fatalf("可编辑标记不对：%+v", d)
	}
	if d.Name != "localhost" || d.Type != "local" || d.Root != "./httpdoc/root" {
		t.Fatalf("字段读取不对：%+v", d)
	}
	if d.Listen != "80" {
		t.Fatalf("listen = %q", d.Listen)
	}
	if d.Raw == "" {
		t.Fatal("应返回原始 JSON 文本")
	}
}

func TestReadSiteDetailRejectsTraversal(t *testing.T) {
	dir := writeSiteWorkspace(t)
	for _, src := range []string{"../../etc/passwd", "conf.d/../../../../x.json"} {
		if _, err := readSiteDetail(dir, src); err == nil {
			t.Fatalf("%s 应被拒绝", src)
		}
	}
}

func TestReadSiteDetailInlineMainConfig(t *testing.T) {
	dir := writeSiteWorkspace(t)
	d, err := readSiteDetail(dir, "fast-https.json")
	if err != nil {
		t.Fatal(err)
	}
	if d.Editable || d.Note == "" {
		t.Fatal("主配置内联站点应标记不可编辑并给出提示")
	}
}

func TestSaveSiteDetailKeepsOtherFields(t *testing.T) {
	dir := writeSiteWorkspace(t)
	path := filepath.Join(dir, "config", "conf.d", "default.json")
	extra := `{
  "listen": 80,
  "server_name": "localhost",
  "custom_key": 1,
  "location": [
    {"url": "/", "type": "local", "root": "./httpdoc/root", "rewrite": "/x", "index": ["index.html"]},
    {"url": "/api", "type": "proxy", "proxy_pass": "http://127.0.0.1:9000"}
  ]
}`
	if err := os.WriteFile(path, []byte(extra), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := saveSiteDetail(dir, SiteInput{
		Source: "conf.d/default.json", Name: "changed.local", Listen: "8080", Type: "local", Root: "./new-root",
	}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m["server_name"] != "changed.local" {
		t.Fatalf("server_name 未更新：%v", m["server_name"])
	}
	if listen := m["listen"].([]any); len(listen) != 1 || listen[0] != float64(8080) {
		t.Fatalf("listen 未更新：%v", m["listen"])
	}
	locs := m["location"].([]any)
	if len(locs) != 2 {
		t.Fatal("其余 location 被丢弃")
	}
	first := locs[0].(map[string]any)
	if first["root"] != "./new-root" {
		t.Fatalf("root 未更新：%v", first["root"])
	}
	if first["rewrite"] != "/x" {
		t.Fatal("location 里未涉及的 rewrite 被丢弃")
	}
	if _, ok := m["custom_key"]; !ok {
		t.Fatal("未知顶层字段被丢弃")
	}
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Fatal("应生成 .bak 备份：", err)
	}
}
