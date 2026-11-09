package siteops

import (
	"strings"
	"testing"

	"fast-https-desktop/internal/model"
)

// 弹窗字段 → CLI 命令 flag 的翻译是本包的核心逻辑，逐项锁住。

func TestPlanAddSingleDomain(t *testing.T) {
	plan, err := planAdd(model.SiteInput{
		Name: "blog.example.com", Listen: "80, 443 ssl h2",
		Type: "local", Root: "./httpdoc/root",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan) != 1 {
		t.Fatalf("单域名应只有一条命令，得到 %d 条", len(plan))
	}
	args := strings.Join(plan[0], " ")
	for _, want := range []string{
		"site add --domain blog.example.com",
		"--listen 80", "--listen 443 ssl h2",
		"--root ./httpdoc/root", "--tls", "--yes",
	} {
		if !strings.Contains(args, want) {
			t.Errorf("参数缺少 %q：%s", want, args)
		}
	}
	if strings.Contains(args, "--proxy-pass") {
		t.Errorf("local 站点不应带 --proxy-pass：%s", args)
	}
}

func TestPlanAddMultiDomain(t *testing.T) {
	plan, err := planAdd(model.SiteInput{
		Name: "blog.example.com www.example.com", Listen: "80",
		Type: "local", Root: "./httpdoc/root",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan) != 2 {
		t.Fatalf("多域名应有 site add + site update 两条命令，得到 %d 条", len(plan))
	}
	upd := strings.Join(plan[1], " ")
	if !strings.Contains(upd, "site update blog.example.com --server-name blog.example.com www.example.com") {
		t.Fatalf("第二条命令应补 server-name：%s", upd)
	}
	// 纯 HTTP 不带 --tls
	if strings.Contains(strings.Join(plan[0], " "), "--tls") {
		t.Fatalf("没有 ssl 监听不应带 --tls")
	}
}

func TestPlanAddProxy(t *testing.T) {
	plan, err := planAdd(model.SiteInput{
		Name: "api.example.com", Listen: "8080",
		Type: "proxy", ProxyPass: "http://127.0.0.1:9000",
	})
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Join(plan[0], " ")
	if !strings.Contains(args, "--proxy-pass http://127.0.0.1:9000") || strings.Contains(args, "--root") {
		t.Fatalf("proxy 站点参数错误：%s", args)
	}
}

func TestParseSiteInputRejectsBadInput(t *testing.T) {
	cases := []model.SiteInput{
		{Name: "", Listen: "80", Type: "local", Root: "./x"},                   // 无域名
		{Name: "a/b.example", Listen: "80", Type: "local", Root: "./x"},        // 非法域名
		{Name: "a.example", Listen: "80", Type: "other", Root: "./x"},          // 非法类型
		{Name: "a.example", Listen: "80", Type: "local"},                       // 缺 root
		{Name: "a.example", Listen: "80", Type: "proxy"},                       // 缺 proxy_pass
		{Name: "a.example", Listen: "80", Type: "proxy", ProxyPass: "ftp://x"}, // 非法 scheme
		{Name: "a.example", Listen: "", Type: "local", Root: "./x"},            // 无监听
		{Name: "a.example", Listen: "99999", Type: "local", Root: "./x"},       // 非法端口
	}
	for i, in := range cases {
		if _, err := planAdd(in); err == nil {
			t.Errorf("case %d 应被拒绝：%#v", i, in)
		}
	}
}

func TestPlanUpdateDiffsAgainstCurrent(t *testing.T) {
	cur := SiteDetail{
		Id: "blog", Name: "blog.example.com", Listen: "80",
		Type: "local", Root: "./httpdoc/root",
	}
	// 无改动 → 拒绝
	if _, err := planUpdate("blog", cur, model.SiteInput{
		Name: "blog.example.com", Listen: "80", Type: "local", Root: "./httpdoc/root",
	}); err == nil {
		t.Fatal("没有改动时应拒绝提交")
	}
	// 改 listen + server_name
	args, err := planUpdate("blog", cur, model.SiteInput{
		Name: "blog.example.com www.example.com", Listen: "80, 443 ssl h2", Type: "local", Root: "./httpdoc/root",
	})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	if !strings.HasPrefix(joined, "site update blog") {
		t.Fatalf("命令头错误：%s", joined)
	}
	for _, want := range []string{
		"--server-name blog.example.com www.example.com",
		"--listen 80", "--listen 443 ssl h2", "--yes",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("参数缺少 %q：%s", want, joined)
		}
	}
	if strings.Contains(joined, "--root") {
		t.Errorf("root 未变不应出现 --root：%s", joined)
	}
	// 类型切换 local → proxy
	args, err = planUpdate("blog", cur, model.SiteInput{
		Name: "blog.example.com", Listen: "80", Type: "proxy", ProxyPass: "http://127.0.0.1:9000",
	})
	if err != nil {
		t.Fatal(err)
	}
	joined = strings.Join(args, " ")
	if !strings.Contains(joined, "--proxy-pass http://127.0.0.1:9000") {
		t.Fatalf("切到 proxy 应带 --proxy-pass：%s", joined)
	}
}

func TestFormatListenValue(t *testing.T) {
	if got := formatListenValue([]any{"80", "443 ssl h2"}); got != "80, 443 ssl h2" {
		t.Fatalf("数组拼接错误：%q", got)
	}
	if got := formatListenValue(80); got != "80" {
		t.Fatalf("数字转字符串错误：%q", got)
	}
	if got := formatListenValue(nil); got != "" {
		t.Fatalf("nil 应为空，得到 %q", got)
	}
}

func TestSetSiteEnabledNormalizesName(t *testing.T) {
	// 只校验名字归一化路径：缺名字报错（不触发 CLI 调用）
	if _, err := SetSiteEnabled(t.TempDir(), "", true); err == nil {
		t.Fatal("缺少站点名应报错")
	}
}
