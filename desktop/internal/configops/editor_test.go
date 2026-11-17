package configops

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fast-https-desktop/internal/model"
	"fast-https-desktop/internal/view"
)

// 表单字段的解析/格式化与候选配置树的搭建是本包的核心逻辑。

func TestParseFieldValue(t *testing.T) {
	b := fieldSpec{typ: "bool"}
	if v, err := parseFieldValue(b, "true"); err != nil || v != true {
		t.Fatalf("bool 解析错误：%v %v", v, err)
	}
	if _, err := parseFieldValue(b, "yes"); err == nil {
		t.Fatal("非法布尔值应报错")
	}
	n := fieldSpec{typ: "number"}
	if v, err := parseFieldValue(n, "30"); err != nil {
		t.Fatal(err)
	} else if json.Number("30") != v {
		t.Fatalf("number 应为 json.Number，得到 %#v", v)
	}
	l := fieldSpec{typ: "list"}
	if v, err := parseFieldValue(l, "a\nb, c"); err != nil {
		t.Fatal(err)
	} else if items := v.([]string); len(items) != 3 || items[2] != "c" {
		t.Fatalf("list 解析错误：%#v", v)
	}
	s := fieldSpec{typ: "select", options: []string{"", "DENY"}}
	if _, err := parseFieldValue(s, "X"); err == nil {
		t.Fatal("select 非法值应报错")
	}
}

func TestFormatFieldValueNumbers(t *testing.T) {
	// JSON 默认解码成 float64，大数不能打成科学计数法
	if got := formatFieldValue(float64(1048576), "number"); got != "1048576" {
		t.Fatalf("大数格式化错误：%q", got)
	}
	if got := formatFieldValue(json.Number("30"), "number"); got != "30" {
		t.Fatalf("json.Number 格式化错误：%q", got)
	}
	if got := formatFieldValue(true, "bool"); got != "true" {
		t.Fatalf("bool 格式化错误：%q", got)
	}
	if got := formatFieldValue([]any{"a", "b"}, "list"); got != "a\nb" {
		t.Fatalf("list 格式化错误：%q", got)
	}
}

func TestLookupAndSetPath(t *testing.T) {
	root := map[string]any{"http": map[string]any{"default_type": "text/html"}}
	if v, ok := lookupPath(root, "http.default_type"); !ok || v != "text/html" {
		t.Fatalf("lookupPath 错误：%v %v", v, ok)
	}
	setPath(root, "http.servers_limit.limit", float64(0))
	if v, ok := lookupPath(root, "http.servers_limit.limit"); !ok || v != float64(0) {
		t.Fatalf("setPath 应能建中间层：%v %v", v, ok)
	}
}

// TestConfigSpecsCoverTLSDefaults 桌面设置页必须暴露 tls.default_certificate，
// 否则桌面端与 CLI（config tls / config show --section tls）能力不对等。
func TestConfigSpecsCoverTLSDefaults(t *testing.T) {
	want := map[string]string{
		"tls.default_certificate.certificate": "file",
		"tls.default_certificate.key":         "file",
	}
	got := map[string]string{}
	for _, s := range configSpecs {
		if strings.HasPrefix(s.path, "tls.") {
			got[s.path] = s.browse
		}
	}
	for path, browse := range want {
		if got[path] != browse {
			t.Fatalf("设置页缺少字段 %s（browse=%q），当前 tls 字段：%v", path, browse, got)
		}
	}
	if !configTopLevel["tls"] {
		t.Fatal("tls 必须由表单接管，否则会被当成原样保留的段")
	}
}

// TestPruneEmptyTLS 两项都留空表示关闭兜底证书，应与 CLI 一样删掉该键。
func TestPruneEmptyTLS(t *testing.T) {
	both := func(cert, key string) map[string]any {
		return map[string]any{"tls": map[string]any{
			"default_certificate": map[string]any{"certificate": cert, "key": key},
		}}
	}

	root := both("", "")
	pruneEmptyTLS(root)
	if _, ok := root["tls"]; ok {
		t.Fatalf("两项都空时应删掉 tls 段：%v", root)
	}

	root = both("def.pem", "")
	pruneEmptyTLS(root)
	if _, ok := root["tls"].(map[string]any)["default_certificate"]; !ok {
		t.Fatal("只清空一项时应保留，交给校验报错")
	}

	root = both("def.pem", "def.key")
	pruneEmptyTLS(root)
	if _, ok := root["tls"].(map[string]any)["default_certificate"]; !ok {
		t.Fatal("有值时不应删除")
	}
}

// TestConfigSpecsCoverProxy 桌面设置页必须暴露 R-05 的三个字段，
// 否则桌面端与 CLI（config proxy）能力不对等。
func TestConfigSpecsCoverProxy(t *testing.T) {
	want := map[string]string{
		"proxy.trusted_proxies":        "list",
		"proxy.forwarded_for.max_hops": "number",
		"proxy.preserve_host":          "bool",
	}
	got := map[string]string{}
	for _, s := range configSpecs {
		if strings.HasPrefix(s.path, "proxy.") {
			got[s.path] = s.typ
		}
	}
	for path, typ := range want {
		if got[path] != typ {
			t.Fatalf("设置页缺少字段 %s（typ=%q），当前 proxy 字段：%v", path, typ, got)
		}
	}
	if !configTopLevel["proxy"] {
		t.Fatal("proxy 必须由表单接管，否则会被当成原样保留的段")
	}
}

// TestPruneEmptyProxy 三项都是默认值时删掉整段，否则保留。
func TestPruneEmptyProxy(t *testing.T) {
	section := func(trusted any, maxHops any, preserve bool) map[string]any {
		ff := map[string]any{}
		if maxHops != nil {
			ff["max_hops"] = maxHops
		}
		return map[string]any{"proxy": map[string]any{
			"trusted_proxies": trusted,
			"forwarded_for":   ff,
			"preserve_host":   preserve,
		}}
	}

	root := section([]string{}, json.Number("0"), true)
	pruneEmptyProxy(root)
	if _, ok := root["proxy"]; ok {
		t.Fatalf("全默认值时应删掉 proxy 段：%v", root)
	}

	for name, root := range map[string]map[string]any{
		"有可信代理":    section([]string{"10.0.0.0/8"}, json.Number("0"), true),
		"限制链长":     section([]string{}, json.Number("5"), true),
		"不透传 Host": section([]any{}, nil, false),
	} {
		pruneEmptyProxy(root)
		if _, ok := root["proxy"]; !ok {
			t.Fatalf("%s 时不应删除 proxy 段", name)
		}
	}
}

// writeTreeWorkspace 建一棵最小的线上配置树：主配置 + conf.d 站点 + 停用文件。
func writeTreeWorkspace(t *testing.T) (string, model.RuntimePaths) {
	t.Helper()
	dir := t.TempDir()
	etc := filepath.Join(dir, "config")
	confd := filepath.Join(etc, "conf.d")
	if err := os.MkdirAll(confd, 0o755); err != nil {
		t.Fatal(err)
	}
	main := filepath.Join(etc, "fast-https.json")
	body := `{
  // JSONC 注释也要能解析
  "http": {
    "include": ["./conf.d"],
    "server": []
  },
  "engine": {"is_master": true}
}`
	if err := os.WriteFile(main, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(confd, "blog.json"), []byte(`{"server_name":"blog.example.com"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(confd, "off.json.disabled"), []byte(`{"server_name":"off.example.com"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// 非配置文件不应被收进候选树
	if err := os.WriteFile(filepath.Join(confd, "readme.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, model.RuntimePaths{Config: main}
}

func TestBuildCandidateClonesTree(t *testing.T) {
	_, paths := writeTreeWorkspace(t)
	raw, err := os.ReadFile(paths.Config)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	dec := json.NewDecoder(strings.NewReader(string(view.StripJSONC(raw))))
	dec.UseNumber()
	if err := dec.Decode(&root); err != nil {
		t.Fatal(err)
	}
	setPath(root, "log_root", "logs")
	cand, err := buildCandidate(paths, root)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(cand)

	// 候选主配置：字段已改、JSONC 注释已丢、未知顶层键保留
	b, err := os.ReadFile(filepath.Join(cand, "fast-https.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got["log_root"] != "logs" {
		t.Fatalf("改动字段未写进候选：%#v", got)
	}
	if _, ok := got["engine"]; !ok {
		t.Fatalf("未知顶层键应保留：%#v", got)
	}
	// 站点文件（含停用）都进 conf.d，非配置文件不进
	for _, name := range []string{"blog.json", "off.json.disabled"} {
		if _, err := os.Stat(filepath.Join(cand, "conf.d", name)); err != nil {
			t.Errorf("候选缺少 %s：%v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(cand, "conf.d", "readme.txt")); err == nil {
		t.Error("非配置文件不应进候选树")
	}
}

func TestBuildCandidateRewritesAbsoluteInclude(t *testing.T) {
	_, paths := writeTreeWorkspace(t)
	root := map[string]any{
		"http": map[string]any{"include": []any{paths.Config[:len(paths.Config)-len("fast-https.json")] + "conf.d"}},
	}
	cand, err := buildCandidate(paths, root)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(cand)
	b, err := os.ReadFile(filepath.Join(cand, "fast-https.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		HTTP struct {
			Include []string `json:"include"`
		} `json:"http"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.HTTP.Include) != 1 || got.HTTP.Include[0] != "conf.d" {
		t.Fatalf("绝对 include 应改写成 conf.d：%#v", got.HTTP.Include)
	}
}
