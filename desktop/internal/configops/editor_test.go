package configops

import (
	"encoding/json"
	"fast-https-desktop/internal/view"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 带 JSONC 注释、尾逗号、未知顶层字段、复杂数组字段（server / cas）的样例。
const sampleFormConfig = `{
  // 主配置注释
  "pid": "",
  "custom_keep": {"x": 1},
  "security": {
    "host_check": "off",
    "headers": {"nosniff": true, "hsts": ""}
  },
  "waf": {"mode": "disabled"},
  "engine": {"is_master": false, "id": 1, "register_port": 9099},
  "acme": {"enabled": false, "email": "<EMAIL>", "cas": [{"name":"letsencrypt"}]},
  "http": {
    "include": ["./conf.d"],
    "default_type": "application/octet-stream",
    "servers_limit": {"max_header_size": 4096, "max_body_size": 524288, "limit": 0, "burst": 0},
    "blaklist": [],
    "log_format": ["ip_port", "time"],
    "server": [{"listen": 8080, "server_name": "example.com"}],
  },
}
`

func writeFormConfig(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "fast-https.json")
	if err := os.WriteFile(p, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

func fieldByPath(form ConfigForm, path string) (ConfigField, bool) {
	for _, f := range form.Fields {
		if f.Path == path {
			return f, true
		}
	}
	return ConfigField{}, false
}

func TestReadConfigFormValues(t *testing.T) {
	p := writeFormConfig(t, sampleFormConfig)
	form, err := ReadConfigForm(p)
	if err != nil {
		t.Fatal(err)
	}
	if !form.Writable || form.Size == 0 || form.Modified == "" {
		t.Fatalf("元数据不全：%+v", form)
	}
	cases := map[string]string{
		"security.host_check":                "off",
		"security.headers.nosniff":           "true",
		"waf.mode":                           "disabled",
		"acme.enabled":                       "false",
		"acme.email":                         "<EMAIL>",
		"http.include":                       "./conf.d",
		"http.servers_limit.max_header_size": "4096",
		"http.log_format":                    "ip_port\ntime",
	}
	for path, want := range cases {
		f, ok := fieldByPath(form, path)
		if !ok {
			t.Fatalf("缺少字段 %s", path)
		}
		if f.Value != want {
			t.Fatalf("%s 期望 %q，实际 %q", path, want, f.Value)
		}
		if f.Missing {
			t.Fatalf("%s 不应标记为缺失", path)
		}
	}
	if _, ok := fieldByPath(form, "engine.id"); ok {
		t.Fatal("engine 集群占位不应出现在表单里")
	}
	// 固定候选的字段用多选，前端按选项渲染勾选组
	if f, ok := fieldByPath(form, "http.log_format"); !ok || f.Type != "multiselect" {
		t.Fatalf("log_format 应为 multiselect：%+v", f)
	}
	if f, ok := fieldByPath(form, "security.headers.referrer_policy"); !ok || f.Type != "select" {
		t.Fatalf("referrer_policy 应为 select：%+v", f)
	}
	if f, ok := fieldByPath(form, "security.headers.content_security_policy"); !ok || f.Type != "textarea" {
		t.Fatalf("CSP 应为 textarea：%+v", f)
	}
	if f, ok := fieldByPath(form, "acme.renew_before_days"); !ok || !f.Missing || f.Value != "0" {
		t.Fatal("未配置的字段应标记 missing 并给默认值")
	}
	if len(form.Preserved) != 2 || form.Preserved[0] != "custom_keep" || form.Preserved[1] != "engine" {
		t.Fatalf("未知或未纳入表单的顶层字段应列出：%v", form.Preserved)
	}
}

func readFormJSON(t *testing.T, p string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestSaveConfigFormKeepsUnknownFields(t *testing.T) {
	p := writeFormConfig(t, sampleFormConfig)
	_, err := SaveConfigForm(p, map[string]string{
		"waf.mode":                         "monitor",
		"http.servers_limit.max_body_size": "1024",
		"acme.enabled":                     "true",
		"http.log_format":                  "ip_port, time, status",
	})
	if err != nil {
		t.Fatal(err)
	}
	m := readFormJSON(t, p)
	if got := m["waf"].(map[string]any)["mode"]; got != "monitor" {
		t.Fatalf("waf.mode = %v", got)
	}
	limit := m["http"].(map[string]any)["servers_limit"].(map[string]any)
	if got := limit["max_body_size"]; got != float64(1024) {
		t.Fatalf("max_body_size 应为数字 1024，实际 %#v", got)
	}
	if got := m["acme"].(map[string]any)["enabled"]; got != true {
		t.Fatalf("acme.enabled = %v", got)
	}
	logs := m["http"].(map[string]any)["log_format"].([]any)
	if len(logs) != 3 || logs[2] != "status" {
		t.Fatalf("log_format 未按列表写回：%v", logs)
	}
	// 未纳入表单的复杂字段与未改动字段必须原样保留
	servers := m["http"].(map[string]any)["server"].([]any)
	if len(servers) != 1 {
		t.Fatal("http.server[] 被丢弃")
	}
	if _, ok := m["acme"].(map[string]any)["cas"]; !ok {
		t.Fatal("acme.cas 被丢弃")
	}
	if _, ok := m["custom_keep"]; !ok {
		t.Fatal("未知顶层字段被丢弃")
	}
	if got := m["engine"].(map[string]any)["register_port"]; got != float64(9099) {
		t.Fatalf("存量 engine 段应原样保留，实际 %v", m["engine"])
	}
	if got := m["security"].(map[string]any)["headers"].(map[string]any)["nosniff"]; got != true {
		t.Fatal("未改动字段被意外改写")
	}
	bak, err := os.ReadFile(p + ".bak")
	if err != nil || !strings.Contains(string(bak), "// 主配置注释") {
		t.Fatal("备份应保留带注释的原文")
	}
}

func TestSaveConfigFormRejectsBadValues(t *testing.T) {
	p := writeFormConfig(t, sampleFormConfig)
	original, _ := os.ReadFile(p)
	bad := []map[string]string{
		{"http.servers_limit.limit": "abc"},
		{"waf.mode": "bogus"},
		{"acme.enabled": "yes"},
		{"security.host_check": "strict"},
	}
	for _, values := range bad {
		if _, err := SaveConfigForm(p, values); err == nil {
			t.Fatalf("应拒绝 %v", values)
		}
		got, _ := os.ReadFile(p)
		if string(got) != string(original) {
			t.Fatal("校验失败时原文件不能变")
		}
	}
	if _, err := SaveConfigForm(p, map[string]string{}); err == nil {
		t.Fatal("空改动应拒绝")
	}
}

func TestResolvePathsFallsBackToRepoLayoutInDev(t *testing.T) {
	repo := t.TempDir()
	bin := filepath.Join(repo, "desktop", "bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatal(err)
	}
	cfgDir := filepath.Join(repo, "config")
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(cfgDir, "fast-https.json")
	if err := os.WriteFile(want, []byte(`{"http":{}}`), 0644); err != nil {
		t.Fatal(err)
	}
	if got := view.ResolvePaths(bin).Config; got != want {
		t.Fatalf("开发布局应回退到 %s，实际 %s", want, got)
	}
}
