package siteops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fast-https-desktop/internal/model"
)

// 弹窗字段 → CLI 命令 flag 的翻译是本包的核心逻辑，逐项锁住。

// TestPlanAdvancedSymlinkPolicy 锁住符号链接策略的差量：改了才带 flag，
// 从有值改回空串时要显式传 --symlink-policy= 才能清除。
func TestPlanAdvancedSymlinkPolicy(t *testing.T) {
	args, changed := planAdvanced(model.SiteDetail{}, model.SiteInput{SymlinkPolicy: "reject"})
	if !changed || !containsArg(args, "--symlink-policy=reject") {
		t.Fatalf("args = %v, changed = %v", args, changed)
	}

	args, changed = planAdvanced(model.SiteDetail{SymlinkPolicy: "reject"}, model.SiteInput{})
	if !changed || !containsArg(args, "--symlink-policy=") {
		t.Fatalf("清除时 args = %v, changed = %v", args, changed)
	}

	if _, changed = planAdvanced(model.SiteDetail{SymlinkPolicy: "reject"}, model.SiteInput{SymlinkPolicy: "reject"}); changed {
		t.Fatal("取值未变不应生成参数")
	}
	if !hasAdvanced(model.SiteInput{SymlinkPolicy: "allow"}) {
		t.Fatal("填了策略应视为高级配置")
	}
}

// R-07：压缩阈值与跳过类型同其他高级字段一样按差量生成参数；
// 清除时用空值传递，不能悄悄留成原值。
func TestPlanAdvancedZipLimits(t *testing.T) {
	args, changed := planAdvanced(model.SiteDetail{}, model.SiteInput{
		ZipMinLength: 1024, ZipSkipTypes: []string{"image/png"},
	})
	if !changed {
		t.Fatal("填了阈值与跳过类型应视为有改动")
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--zip-min-length 1024") ||
		!strings.Contains(joined, "--zip-skip-types image/png") {
		t.Fatalf("args = %v", args)
	}

	args, changed = planAdvanced(
		model.SiteDetail{ZipMinLength: 1024, ZipSkipTypes: []string{"image/png"}},
		model.SiteInput{},
	)
	if !changed {
		t.Fatal("清除阈值与跳过类型应算改动")
	}
	joined = strings.Join(args, " ")
	if !strings.Contains(joined, "--zip-min-length 0") || !strings.Contains(joined, "--zip-skip-types=") {
		t.Fatalf("清除参数 = %v", args)
	}

	if _, changed = planAdvanced(
		model.SiteDetail{ZipMinLength: 1024, ZipSkipTypes: []string{"image/png"}},
		model.SiteInput{ZipMinLength: 1024, ZipSkipTypes: []string{"IMAGE/PNG"}},
	); changed {
		t.Fatal("仅大小写不同不应生成参数")
	}
	if !hasAdvanced(model.SiteInput{ZipMinLength: 1}) || !hasAdvanced(model.SiteInput{ZipSkipTypes: []string{"image/png"}}) {
		t.Fatal("两个新字段都应参与 hasAdvanced")
	}
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

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
		"--zip br", "--zip gzip", "--zip-min-length 1024",
		"--zip-skip-types image/*", "--zip-skip-types application/zip",
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
	// 代理响应同样支持压缩，新增时也带默认值。
	if !strings.Contains(args, "--zip br") || !strings.Contains(args, "--zip-min-length 1024") {
		t.Fatalf("proxy 站点应带压缩默认值：%s", args)
	}
}

// 新建站点时压缩直接随 site add 写入；其余高级配置仍要补一条 site update。
func TestPlanAddWithAdvanced(t *testing.T) {
	plan, err := planAdd(model.SiteInput{
		Name: "blog.example.com", Listen: "80",
		Type: "local", Root: "./httpdoc/root",
		Zip: []string{"gzip", "br"},
		// 新增弹窗会同时提交这三项；只给 Zip 时其余按 0/空处理，属于显式取值。
		ZipMinLength: 1024, ZipSkipTypes: []string{"image/*"},
		LimitRate: 100, LimitBurst: 50, LimitMem: 1024, LimitNodelay: true,
		AuthOn: true, AuthUser: "u", AuthPass: "p", AuthRealm: "realm",
		CacheZone: "edge",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan) != 2 {
		t.Fatalf("应产出 site add + site update 两条命令，得到 %d 条", len(plan))
	}
	add := strings.Join(plan[0], " ")
	for _, want := range []string{"--zip gzip", "--zip br", "--zip-min-length 1024"} {
		if !strings.Contains(add, want) {
			t.Errorf("site add 缺少压缩参数 %q：%s", want, add)
		}
	}
	if strings.Contains(add, "--limit-rate") || strings.Contains(add, "--auth-type") {
		t.Errorf("site add 不应带其他高级配置：%s", add)
	}
	if !strings.Contains(add, "--cache-zone edge") {
		t.Errorf("缓存区应随 site add 写入：%s", add)
	}

	upd := strings.Join(plan[1], " ")
	if !strings.Contains(upd, "site update blog.example.com") {
		t.Fatalf("第二条应是 site update：%s", upd)
	}
	for _, want := range []string{
		"--limit-rate 100", "--limit-burst 50", "--limit-mem 1024", "--limit-nodelay",
		"--auth-type basic", "--auth-user u", "--auth-pass p", "--auth-realm realm",
		"--yes",
	} {
		if !strings.Contains(upd, want) {
			t.Errorf("补全命令缺少 %q：%s", want, upd)
		}
	}
	if strings.Contains(upd, "--zip") || strings.Contains(upd, "--cache-zone") {
		t.Errorf("压缩与缓存区已随 site add 写入，update 不应重复：%s", upd)
	}
}

// TestCacheZoneArgs 锁住缓存区名的传递：留空时只关闭缓存。
func TestCacheZoneArgs(t *testing.T) {
	on := strings.Join(cacheZoneArgs(model.SiteInput{CacheZone: "edge"}), " ")
	if on != "--cache-zone edge" {
		t.Fatalf("cacheZoneArgs = %q", on)
	}
	if off := strings.Join(cacheZoneArgs(model.SiteInput{}), " "); off != "--cache-zone=" {
		t.Fatalf("关闭缓存应只传 --cache-zone=，得到 %s", off)
	}
}

// 没填高级配置时不能多跑一条 update，否则每次建站都多一次配置事务。
func TestPlanAddWithoutAdvancedStaysSingleCommand(t *testing.T) {
	plan, err := planAdd(model.SiteInput{
		Name: "blog.example.com", Listen: "80",
		Type: "local", Root: "./httpdoc/root",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan) != 1 {
		t.Fatalf("没填高级配置应只有一条命令，得到 %d 条", len(plan))
	}
}

// 多域名 + 高级配置要合并成同一条 update，而不是各来一条。
func TestPlanAddMultiDomainWithAdvancedMerges(t *testing.T) {
	plan, err := planAdd(model.SiteInput{
		Name: "a.example.com b.example.com", Listen: "80",
		Type: "local", Root: "./httpdoc/root",
		Zip: []string{"gzip"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan) != 2 {
		t.Fatalf("应合并成两条命令，得到 %d 条", len(plan))
	}
	add := strings.Join(plan[0], " ")
	if !strings.Contains(add, "--zip gzip") {
		t.Errorf("压缩应随 site add 写入：%s", add)
	}
	upd := strings.Join(plan[1], " ")
	if !strings.Contains(upd, "--server-name a.example.com b.example.com") {
		t.Errorf("缺 server-name：%s", upd)
	}
	if strings.Contains(upd, "--zip") {
		t.Errorf("update 不应重复压缩参数：%s", upd)
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

// TLS 但不开 h2：监听串原样传 "443 ssl"，不额外补 h2。
func TestPlanAddTLSWithoutH2(t *testing.T) {
	plan, err := planAdd(model.SiteInput{
		Name: "a.example", Listen: "443 ssl", Type: "local", Root: "./www",
	})
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Join(plan[0], " ")
	if !strings.Contains(args, "--listen 443 ssl") {
		t.Fatalf("缺少监听参数：%s", args)
	}
	if !containsArg(plan[0], "--tls") {
		t.Fatalf("有 ssl 监听应带 --tls：%s", args)
	}
	if strings.Contains(args, "h2") {
		t.Fatalf("未开 h2 不应出现 h2：%s", args)
	}
}

// TestPlanAddWithSharedCert 弹窗里填了证书路径：直接带进 site add，
// 让多个站点共用同一张证书（如通配符证书）。
func TestPlanAddWithSharedCert(t *testing.T) {
	plan, err := planAdd(model.SiteInput{
		Name: "a.example", Listen: "443 ssl h2", Type: "local", Root: "./www",
		Cert: "./cert/wildcard.pem", CertKey: "./cert/wildcard-key.pem",
	})
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Join(plan[0], " ")
	for _, want := range []string{
		"--tls", "--ssl-certificate ./cert/wildcard.pem", "--ssl-certificate-key ./cert/wildcard-key.pem",
	} {
		if !strings.Contains(args, want) {
			t.Errorf("参数缺少 %q：%s", want, args)
		}
	}
}

// TestParseSiteInputCertWithoutTLS 有证书却没有 ssl 监听 → 直接拒绝，
// 不要写出自相矛盾的站点，也别等 CLI 返回费解的错误码。
func TestParseSiteInputCertWithoutTLS(t *testing.T) {
	cases := []model.SiteInput{
		// 只有证书没有私钥
		{Name: "a.example", Listen: "443 ssl h2", Type: "local", Root: "./x", Cert: "./cert/a.pem"},
		// 有证书但没有 ssl 监听
		{Name: "a.example", Listen: "80", Type: "local", Root: "./x",
			Cert: "./cert/a.pem", CertKey: "./cert/a-key.pem"},
	}
	for i, in := range cases {
		if _, err := planAdd(in); err == nil {
			t.Errorf("case %d 应被拒绝：%#v", i, in)
		}
	}
}

// TestPlanUpdateCertChanges 证书路径变化才带参数；清空用 --flag= 形式，
// 让 CLI 把证书交回默认约定。
func TestPlanUpdateCertChanges(t *testing.T) {
	cur := SiteDetail{
		Id: "a", Name: "a.example", Listen: "443 ssl h2", Type: "local", Root: "./www",
		Cert: "./cert/a.example.pem", CertKey: "./cert/a.example-key.pem",
	}
	// 换成共享证书
	args, err := planUpdate("a", cur, model.SiteInput{
		Name: "a.example", Listen: "443 ssl h2", Type: "local", Root: "./www",
		Cert: "./cert/wildcard.pem", CertKey: "./cert/wildcard-key.pem",
	})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--ssl-certificate=./cert/wildcard.pem") ||
		!strings.Contains(joined, "--ssl-certificate-key=./cert/wildcard-key.pem") {
		t.Fatalf("应带共享证书参数：%s", joined)
	}

	// 清空 → 交回默认约定
	args, err = planUpdate("a", cur, model.SiteInput{
		Name: "a.example", Listen: "443 ssl h2", Type: "local", Root: "./www",
	})
	if err != nil {
		t.Fatal(err)
	}
	joined = strings.Join(args, " ")
	if !strings.Contains(joined, "--ssl-certificate= ") && !strings.HasSuffix(joined, "--ssl-certificate=") {
		t.Fatalf("清空证书应传空值：%s", joined)
	}
}

// TestRequireAbsRoot 根目录必须是已存在的绝对路径：相对写法（尤其带 ..）
// 会按谁展开完全说不清，所以在桌面端入口就拦住，而不是算成 E:\front 这种怪路径。
func TestRequireAbsRoot(t *testing.T) {
	dir := t.TempDir()
	if err := requireAbsRoot(dir); err != nil {
		t.Fatalf("存在的绝对目录应通过：%v", err)
	}
	if err := requireAbsRoot(""); err != nil {
		t.Fatalf("空值应放行（proxy 站点无 root）：%v", err)
	}
	for _, rel := range []string{
		filepath.Join("..", "..", "front", "www"),
		"www",
		"./httpdoc/root",
	} {
		if err := requireAbsRoot(rel); err == nil {
			t.Fatalf("相对路径应被拒绝：%s", rel)
		}
	}
	if err := requireAbsRoot(filepath.Join(dir, "not-exist")); err == nil {
		t.Fatal("不存在的目录应被拒绝")
	}
	file := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := requireAbsRoot(file); err == nil {
		t.Fatal("文件（非目录）应被拒绝")
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

// 路径代理（额外 location）的三个纯函数：命令拼装、前置校验、变更判定。
func TestExtraLocationHelpers(t *testing.T) {
	dir := t.TempDir()

	args, err := locationSetArgs("web", []model.SiteLocation{
		{URL: " /api ", Type: "proxy", ProxyPass: "http://127.0.0.1:9000"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"url":"/api","type":"proxy","proxy_pass":"http://127.0.0.1:9000"}`
	if len(args) != 6 || args[0] != "site" || args[1] != "location" || args[2] != "set" ||
		args[3] != "web" || args[4] != "--location" || args[5] != want {
		t.Fatalf("args = %v", args)
	}
	// 空列表也要生成命令，交给 set 清空额外 location。
	if args, _ = locationSetArgs("web", nil); len(args) != 4 {
		t.Fatalf("empty args = %v", args)
	}

	good := []model.SiteLocation{
		{URL: "/api", Type: "proxy", ProxyPass: "https://127.0.0.1:9000"},
		{URL: "/assets", Root: dir},
	}
	if err := validateExtraLocations(good); err != nil {
		t.Fatalf("合法配置被拒绝：%v", err)
	}

	bad := []struct {
		name string
		loc  model.SiteLocation
	}{
		{"空 url", model.SiteLocation{Type: "proxy", ProxyPass: "http://a"}},
		{"主 location", model.SiteLocation{URL: "/", Type: "proxy", ProxyPass: "http://a"}},
		{"无前导斜杠", model.SiteLocation{URL: "api", Type: "proxy", ProxyPass: "http://a"}},
		{"含空格", model.SiteLocation{URL: "/a b", Type: "proxy", ProxyPass: "http://a"}},
		{"上游缺协议", model.SiteLocation{URL: "/x", Type: "proxy", ProxyPass: "127.0.0.1:9000"}},
		{"静态缺目录", model.SiteLocation{URL: "/s", Type: "local"}},
		{"目录不存在", model.SiteLocation{URL: "/s", Root: filepath.Join(dir, "nope")}},
		{"未知类型", model.SiteLocation{URL: "/t", Type: "rewrite"}},
	}
	for _, c := range bad {
		if err := validateExtraLocations([]model.SiteLocation{c.loc}); err == nil {
			t.Fatalf("%s 应被拒绝", c.name)
		}
	}
	if err := validateExtraLocations([]model.SiteLocation{
		{URL: "/api", Type: "proxy", ProxyPass: "http://a"},
		{URL: "/api", Type: "proxy", ProxyPass: "http://b"},
	}); err == nil {
		t.Fatal("重复 url 应被拒绝")
	}

	if !sameLocations(nil, nil) {
		t.Fatal("两个空列表应视为相同")
	}
	if !sameLocations(
		[]model.SiteLocation{{URL: " /api ", Type: " proxy "}},
		[]model.SiteLocation{{URL: "/api", Type: "proxy"}},
	) {
		t.Fatal("忽略首尾空白后应视为相同")
	}
	if sameLocations(
		[]model.SiteLocation{{URL: "/api", Type: "proxy", ProxyPass: "http://a"}},
		[]model.SiteLocation{{URL: "/api", Type: "proxy", ProxyPass: "http://b"}},
	) {
		t.Fatal("上游变化应视为不同")
	}
}
