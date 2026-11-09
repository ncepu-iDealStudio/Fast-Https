package view

import (
	"strings"
	"testing"
)

// CLI data → 界面视图模型的映射是本包的核心逻辑，逐项锁住。

func TestSplitListen(t *testing.T) {
	got := splitListen("80, 443 ssl h2")
	if len(got) != 2 || got[0] != "80" || got[1] != "443 ssl h2" {
		t.Fatalf("splitListen = %#v", got)
	}
	if got := splitListen(""); len(got) != 0 {
		t.Fatalf("空监听应为空，得到 %#v", got)
	}
}

func TestPortOf(t *testing.T) {
	cases := map[string]string{
		"80":          ":80",
		"443 ssl h2":  ":443",
		":8080":       ":8080",
		"unix:/tmp/x": "",
		"":            "",
	}
	for in, want := range cases {
		if got := portOf(in); got != want {
			t.Errorf("portOf(%q) = %q，期望 %q", in, got, want)
		}
	}
}

func TestSiteRowsExpandListens(t *testing.T) {
	data := map[string]any{
		"sites": []any{
			map[string]any{
				"name":        "blog",
				"file":        "/etc/fast-https/conf.d/blog.json",
				"listen":      "80, 443 ssl h2",
				"server_name": "blog.example.com www.example.com",
				"locations":   []any{"/ → local ./httpdoc/root"},
				"enabled":     true,
				"cert_status": "valid",
			},
			map[string]any{
				"name":        "api",
				"file":        "/etc/fast-https/conf.d/api.json.disabled",
				"listen":      "8080",
				"server_name": "api.example.com",
				"enabled":     false,
				"error":       "parse failed",
			},
		},
	}
	rows := siteRows(data)
	if len(rows) != 3 {
		t.Fatalf("应拆出 3 行，得到 %d", len(rows))
	}
	// 多监听拆行、共享 Id、Name 取第一个域名
	if rows[0].Id != "blog" || rows[1].Id != "blog" {
		t.Fatalf("拆行应共享 Id：%#v", rows[:2])
	}
	if rows[0].Name != "blog.example.com" {
		t.Fatalf("Name 应取首个域名，得到 %q", rows[0].Name)
	}
	if rows[0].Port != ":80" || rows[0].TLS {
		t.Fatalf("第一行应是 :80 非 TLS：%#v", rows[0])
	}
	if rows[1].Port != ":443" || !rows[1].TLS {
		t.Fatalf("第二行应是 :443 TLS：%#v", rows[1])
	}
	if rows[0].CertStatus != "valid" {
		t.Fatalf("cert_status 未映射：%#v", rows[0])
	}
	// 停用与错误透出
	if rows[2].Enabled || rows[2].Error != "parse failed" {
		t.Fatalf("停用站点映射错误：%#v", rows[2])
	}
}

func TestCertRowsMapping(t *testing.T) {
	data := map[string]any{
		"certificates": []any{
			map[string]any{
				"domains":      []any{"blog.example.com"},
				"file":         "/etc/fast-https/cert/blog.example.com.pem",
				"status":       "expiring",
				"subject":      "CN=blog.example.com",
				"issuer":       "CN=fast-https local CA",
				"names":        []any{"blog.example.com", "www.example.com"},
				"not_after":    "2026-04-01T00:00:00Z",
				"days_left":    float64(80),
				"self_signed":  false,
				"local_ca":     true,
				"acme_managed": false,
			},
			map[string]any{
				"domains": []any{"gone.example.com"},
				"file":    "/x/gone.pem",
				"status":  "missing",
				"error":   "certificate file not found",
			},
		},
	}
	rows := certRows(data)
	if len(rows) != 2 {
		t.Fatalf("应有 2 张证书，得到 %d", len(rows))
	}
	c := rows[0]
	if c.Domain != "blog.example.com" || c.Status != "expiring" || c.DaysLeft != 80 {
		t.Fatalf("证书映射错误：%#v", c)
	}
	if c.NotAfter != "2026-04-01" {
		t.Fatalf("not_after 应收成日期，得到 %q", c.NotAfter)
	}
	if c.Names != "blog.example.com, www.example.com" {
		t.Fatalf("names 拼接错误：%q", c.Names)
	}
	if !c.Local {
		t.Fatalf("local_ca 应映射为 Local")
	}
	// days_left 缺失不 panic，错误原因进 Note
	if rows[1].DaysLeft != 0 || rows[1].Note != "certificate file not found" {
		t.Fatalf("缺失字段映射错误：%#v", rows[1])
	}
}

func TestNormalizeBound(t *testing.T) {
	if got := normalizeBound("1h", false); got != "1h" {
		t.Fatalf("时长应原样透传，得到 %q", got)
	}
	since := normalizeBound("2026-11-07", false)
	if !strings.HasPrefix(since, "2026-11-07T00:00:00") {
		t.Fatalf("起始日应补 00:00，得到 %q", since)
	}
	until := normalizeBound("2026-11-08", true)
	if !strings.HasPrefix(until, "2026-11-08T23:59:59") {
		t.Fatalf("截止日应补 23:59:59，得到 %q", until)
	}
	if got := normalizeBound("", false); got != "" {
		t.Fatalf("空值应保持空，得到 %q", got)
	}
}

func TestLogsQueryArgs(t *testing.T) {
	paths := RuntimePaths{Config: "/etc/fast-https/fast-https.json"}
	args := strings.Join(logsQueryArgs(LogQuery{
		Kind: "error", Limit: 30, Level: "warn", Status: "4xx",
		Host: "blog.example.com", Method: "GET", PathPrefix: "/api", Reason: "blacklist",
		Since: "1h", Until: "2026-11-08",
	}, paths), " ")
	for _, want := range []string{
		"logs query --kind error --limit 30",
		"--level warn", "--status 4xx", "--domain blog.example.com",
		"--method GET", "--path-prefix /api", "--reason blacklist",
		"--since 1h", "--config /etc/fast-https/fast-https.json",
	} {
		if !strings.Contains(args, want) {
			t.Errorf("参数缺少 %q：%s", want, args)
		}
	}
	// 分页游标收窄 until
	withCursor := strings.Join(logsQueryArgs(LogQuery{Until: "2026-11-08", Cursor: "2026-01-02T03:04:05"}, paths), " ")
	if !strings.Contains(withCursor, "--until 2026-01-02T03:04:05") {
		t.Errorf("游标应覆盖筛选 until：%s", withCursor)
	}
	if strings.Contains(withCursor, "2026-11-08") {
		t.Errorf("游标模式不应再带筛选 until：%s", withCursor)
	}
	// 缺省 kind 与 limit
	def := strings.Join(logsQueryArgs(LogQuery{}, paths), " ")
	if !strings.Contains(def, "--kind access --limit 30") {
		t.Errorf("缺省参数错误：%s", def)
	}
}

func TestLogEntryMapping(t *testing.T) {
	rec := map[string]any{
		"time":   "2026-01-02T03:04:05.123",
		"kind":   "access",
		"level":  "info",
		"method": "GET",
		"path":   "/index.html",
		"status": float64(200),
		"size":   float64(1234),
		"host":   "blog.example.com",
	}
	e := logEntry(rec)
	if e.Time != "2026-01-02T03:04:05.123" || e.Method != "GET" || e.Host != "blog.example.com" {
		t.Fatalf("字段映射错误：%#v", e)
	}
	if e.Status != "200" || e.Size != "1234" {
		t.Fatalf("数字字段应收成字符串：status=%q size=%q", e.Status, e.Size)
	}
	if !strings.Contains(e.Raw, `"status":200`) {
		t.Fatalf("Raw 应是记录的紧凑 JSON：%q", e.Raw)
	}
}

func TestShortDate(t *testing.T) {
	if got := shortDate("2026-04-01T00:00:00Z"); got != "2026-04-01" {
		t.Fatalf("shortDate = %q", got)
	}
	if got := shortDate("not-a-date"); got != "not-a-date" {
		t.Fatalf("解析不了应原样返回，得到 %q", got)
	}
}

func TestTailLogRejectsUnknownKind(t *testing.T) {
	if _, err := TailLog(t.TempDir(), "bogus", 5); err == nil {
		t.Fatal("不支持的日志类型应报错")
	}
}
