package siteops

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fast-https/config"
)

// updateAdvanced 在临时站点上跑一次 site.update，返回写入后的文件内容。
func updateAdvanced(t *testing.T, in UpdateInput) string {
	t.Helper()
	live, doc, eng, _ := writeLive(t, "live.example")
	if _, err := runAdd(t, eng, live, "adv.example", doc, "adv0", false); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(filepath.Dir(live), "conf.d", "adv.example.json")
	in.Name = "adv.example"
	if _, err := Run(context.Background(), eng, RunInput{
		LiveMain: live, Command: "site.update", RequestID: "adv1",
		Mutate: func(_ string, dirs []string) error {
			_, err := Update(dirs, live, in)
			return err
		},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(got)
}

func TestUpdateZip(t *testing.T) {
	zip := []string{"gzip", "br"}
	out := updateAdvanced(t, UpdateInput{Zip: &zip})
	if !strings.Contains(out, `"zip": ["gzip","br"]`) {
		t.Fatalf("zip 没写进去:\n%s", out)
	}
}

func TestUpdateLimit(t *testing.T) {
	limit := config.PathLimit{Rate: 50, Burst: 100}
	out := updateAdvanced(t, UpdateInput{Limit: &limit})
	if !strings.Contains(out, `"limit"`) {
		t.Fatalf("limit 没写进去:\n%s", out)
	}
	if !strings.Contains(out, `"rate":50`) {
		t.Fatalf("limit.rate 不对:\n%s", out)
	}
}

func TestUpdateAuth(t *testing.T) {
	auth := config.PathAuth{AuthType: "basic", User: "admin", Pswd: "s3cret"}
	out := updateAdvanced(t, UpdateInput{Auth: &auth})
	if !strings.Contains(out, `"user":"admin"`) {
		t.Fatalf("auth 没写进去:\n%s", out)
	}
	if !strings.Contains(out, `"pswd":"s3cret"`) {
		t.Fatalf("口令没写进去:\n%s", out)
	}
}

func TestUpdateCache(t *testing.T) {
	cache := config.Cache{Path: "./cache", Valid: []string{"10s"}}
	out := updateAdvanced(t, UpdateInput{Cache: &cache})
	if !strings.Contains(out, `"proxy_cache"`) {
		t.Fatalf("proxy_cache 没写进去:\n%s", out)
	}
	if !strings.Contains(out, `"./cache"`) {
		t.Fatalf("缓存目录不对:\n%s", out)
	}
}

// 密码传 *** 或空串：必须沿用文件里的原口令，绝不能把星号写进去。
func TestUpdateAuthKeepsPassword(t *testing.T) {
	for _, pass := range []string{"***", ""} {
		auth := config.PathAuth{AuthType: "basic", User: "admin", Pswd: "s3cret"}
		out := updateAdvanced(t, UpdateInput{Auth: &auth})
		if !strings.Contains(out, `"s3cret"`) {
			t.Fatalf("前置条件失败，原始口令没写进去:\n%s", out)
		}

		// 第二次：只改 realm，密码传脱敏值
		auth2 := config.PathAuth{AuthType: "basic", User: "admin", Pswd: pass, Realm: "R2"}
		out2 := updateAdvanced2(t, out, UpdateInput{Auth: &auth2})
		if strings.Contains(out2, `"***"`) {
			t.Fatalf("脱敏值被写进配置了:\n%s", out2)
		}
		if !strings.Contains(out2, `"s3cret"`) {
			t.Fatalf("原口令没被保留（pass=%q）:\n%s", pass, out2)
		}
		if !strings.Contains(out2, `"R2"`) {
			t.Fatalf("realm 没改:\n%s", out2)
		}
	}
}

// updateAdvanced2 在已有站点文件内容的基础上再跑一次 update。
func updateAdvanced2(t *testing.T, existing string, in UpdateInput) string {
	t.Helper()
	live, _, eng, _ := writeLive(t, "live2.example")
	confDir := filepath.Join(filepath.Dir(live), "conf.d")
	if err := os.MkdirAll(confDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(confDir, "adv2.example.json")
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	in.Name = "adv2.example"
	if _, err := Run(context.Background(), eng, RunInput{
		LiveMain: live, Command: "site.update", RequestID: "adv2",
		Mutate: func(_ string, dirs []string) error {
			_, err := Update(dirs, live, in)
			return err
		},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(got)
}

// 指向零值 = 关闭该配置，键应被删掉。
func TestUpdateAdvancedClears(t *testing.T) {
	zip := []string{"gzip"}
	limit := config.PathLimit{Rate: 10}
	auth := config.PathAuth{AuthType: "basic", User: "admin", Pswd: "pw"}
	cache := config.Cache{Path: "./c"}
	out := updateAdvanced(t, UpdateInput{Zip: &zip, Limit: &limit, Auth: &auth, Cache: &cache})
	for _, key := range []string{`"zip"`, `"limit"`, `"auth"`, `"proxy_cache"`} {
		if !strings.Contains(out, key) {
			t.Fatalf("前置条件失败，%s 没写进去:\n%s", key, out)
		}
	}

	// 一次性清掉四个
	ez, el, ea, ec := []string{}, config.PathLimit{}, config.PathAuth{}, config.Cache{}
	out2 := updateAdvanced2(t, out, UpdateInput{Zip: &ez, Limit: &el, Auth: &ea, Cache: &ec})
	for _, key := range []string{`"zip"`, `"limit"`, `"auth"`, `"proxy_cache"`} {
		if strings.Contains(out2, key) {
			t.Fatalf("%s 应被删掉:\n%s", key, out2)
		}
	}
}

// 非法输入要被拦下。校验发生在落盘之前，所以直接打 applyAdvanced。
func TestUpdateAdvancedValidation(t *testing.T) {
	cases := []struct {
		name string
		in   UpdateInput
	}{
		{"不支持的编码", UpdateInput{Zip: &[]string{"deflate"}}},
		{"负数限流", UpdateInput{Limit: &config.PathLimit{Rate: -1}}},
		{"认证缺用户名", UpdateInput{Auth: &config.PathAuth{AuthType: "basic", Pswd: "pw"}}},
		{"首次开户缺口令", UpdateInput{Auth: &config.PathAuth{AuthType: "basic", User: "admin"}}},
		{"缓存缺目录", UpdateInput{Cache: &config.Cache{Valid: []string{"10s"}}}},
	}
	for _, c := range cases {
		cur := &config.SiteInput{Locations: []config.SiteLocation{{URL: "/", Type: "local", Root: "."}}}
		if err := applyAdvanced(cur, c.in); err == nil {
			t.Errorf("%s 应该报错", c.name)
		}
	}
}
