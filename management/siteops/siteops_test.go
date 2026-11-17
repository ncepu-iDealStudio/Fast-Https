package siteops

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fast-https/management/cfgver"
	"fast-https/management/operation"
	"fast-https/management/state"
	"fast-https/management/txn"
	"fast-https/modules/site"
)

type countReload struct{ n, fail int }

func (c *countReload) Reload(context.Context) error {
	c.n++
	if c.fail > 0 && c.n == c.fail {
		return errors.New("reload injected")
	}
	return nil
}

func writeLive(t *testing.T, name string) (liveMain, docroot string, eng *txn.Engine, reload *countReload) {
	t.Helper()
	work := t.TempDir()
	liveDir := filepath.Join(work, "config")
	docroot = filepath.Join(work, "www")
	if err := os.MkdirAll(filepath.Join(liveDir, "conf.d"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(docroot, 0o755); err != nil {
		t.Fatal(err)
	}
	site := `{
  "listen": [18080],
  "server_name": "` + name + `",
  "location": [{"url": "/", "type": "local", "root": "."}]
}`
	if err := os.WriteFile(filepath.Join(liveDir, "conf.d", "live.json"), []byte(site), 0o644); err != nil {
		t.Fatal(err)
	}
	main := `{"http":{"include":["conf.d"]}}`
	liveMain = filepath.Join(liveDir, "fast-https.json")
	if err := os.WriteFile(liveMain, []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(liveDir)
	reload = &countReload{}
	eng = &txn.Engine{
		State:    state.Dir{Root: filepath.Join(work, "state"), Alive: func(int, string) bool { return true }},
		Reloader: reload,
		Probe:    func(int) bool { return true },
		Bind:     func(int) error { return nil },
	}
	return liveMain, docroot, eng, reload
}

func runAdd(t *testing.T, eng *txn.Engine, live, domain, root, req string, dry bool) (txn.Result, error) {
	t.Helper()
	return Run(context.Background(), eng, RunInput{
		LiveMain: live, RequestID: req, Command: "site.add", Summary: "add " + domain, DryRun: dry,
		Args: map[string]string{"domain": domain},
		Mutate: func(_ string, dirs []string) error {
			_, err := Add(dirs, AddInput{Domain: domain, Root: root, Listens: []string{"18081"}})
			return err
		},
	})
}

func TestSiteAddDryRun(t *testing.T) {
	live, doc, eng, reload := writeLive(t, "live.example")
	before, _ := os.ReadDir(filepath.Join(filepath.Dir(live), "conf.d"))
	res, err := runAdd(t, eng, live, "demo.example", doc, "d1", true)
	if err != nil {
		t.Fatal(err)
	}
	if !res.DryRun || reload.n != 0 {
		t.Fatalf("dry-run 不应 reload：%+v n=%d", res, reload.n)
	}
	after, _ := os.ReadDir(filepath.Join(filepath.Dir(live), "conf.d"))
	if len(after) != len(before) {
		t.Fatal("dry-run 不应改线上文件")
	}
	if res.Diff.Identical {
		t.Fatal("应有差异摘要")
	}
}

func TestSiteAddCreatesAndReloads(t *testing.T) {
	live, doc, eng, reload := writeLive(t, "live.example")
	res, err := runAdd(t, eng, live, "demo.example", doc, "a1", false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Operation.Status != operation.StatusSucceeded {
		t.Fatalf("status = %s", res.Operation.Status)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(live), "conf.d", "demo.example.json")); err != nil {
		t.Fatal("站点文件应出现")
	}
	if reload.n != 1 {
		t.Fatalf("reload n=%d", reload.n)
	}
	list, err := cfgver.List(eng.State, live)
	if err != nil || len(list.Versions) < 1 {
		t.Fatalf("versions = %+v err=%v", list, err)
	}
}

// 新建 local 站点默认带压缩：br+gzip、1 KB 阈值、整类跳过媒体与压缩包。
// 显式给空压缩列表时一个 zip 键都不写。
func TestSiteAddCompressionDefaults(t *testing.T) {
	live, doc, _, _ := writeLive(t, "live.example")
	dirs := []string{filepath.Join(filepath.Dir(live), "conf.d")}

	if _, err := Add(dirs, AddInput{Domain: "demo.example", Root: doc, Listens: []string{"18081"}}); err != nil {
		t.Fatal(err)
	}
	loc := readSiteLocation(t, filepath.Join(dirs[0], "demo.example.json"))
	if strings.Join(loc.Zip, ",") != "br,gzip" {
		t.Fatalf("zip = %v", loc.Zip)
	}
	if loc.ZipMinLength != DefaultZipMinLength {
		t.Fatalf("zip_min_length = %d", loc.ZipMinLength)
	}
	if strings.Join(loc.ZipSkipTypes, ",") != strings.Join(DefaultZipSkipTypes, ",") {
		t.Fatalf("zip_skip_types = %v", loc.ZipSkipTypes)
	}

	off := []string{}
	if _, err := Add(dirs, AddInput{
		Domain: "plain.example", Root: doc, Listens: []string{"18082"}, Zip: &off,
	}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dirs[0], "plain.example.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"zip`) {
		t.Fatalf("显式关闭压缩后不应写 zip 键：\n%s", raw)
	}
}

type siteLocationProbe struct {
	Zip          []string `json:"zip"`
	ZipMinLength int      `json:"zip_min_length"`
	ZipSkipTypes []string `json:"zip_skip_types"`
}

func readSiteLocation(t *testing.T, path string) siteLocationProbe {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Location []siteLocationProbe `json:"location"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Location) == 0 {
		t.Fatalf("站点缺少 location：\n%s", raw)
	}
	return got.Location[0]
}

func TestSiteAddDuplicate(t *testing.T) {
	live, doc, eng, _ := writeLive(t, "live.example")
	if _, err := runAdd(t, eng, live, "demo.example", doc, "a1", false); err != nil {
		t.Fatal(err)
	}
	before := mustHash(t, live)
	_, err := runAdd(t, eng, live, "demo.example", doc, "a2", false)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("err = %v", err)
	}
	if mustHash(t, live) != before {
		t.Fatal("冲突时线上应不变")
	}
}

// TestSiteAddServerNameConflictByPort 冲突判据是「域名 + 端口」：
// 同一 server_name 挂在不同端口上是两个独立 server 块（HostMap 键是
// server_name:port），nginx 语义里也合法；只有同端口重复才拒绝。
func TestSiteAddServerNameConflictByPort(t *testing.T) {
	live, doc, _, _ := writeLive(t, "live.example")
	dirs := []string{filepath.Join(filepath.Dir(live), "conf.d")}

	// 已有 live.json：live.example @ 18080，这里换 18081 再加一个
	if _, err := Add(dirs, AddInput{Domain: "live.example", Root: doc, Listens: []string{"18081"}}); err != nil {
		t.Fatalf("同域名不同端口应允许：%v", err)
	}

	// 另一个站点文件声明了同一域名并占了 18081
	other := `{"listen": [18081], "server_name": "taken.example",` +
		`"location": [{"url": "/", "type": "local", "root": "."}]}`
	if err := os.WriteFile(filepath.Join(dirs[0], "other.json"), []byte(other), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Add(dirs, AddInput{Domain: "taken.example", Root: doc, Listens: []string{"18081"}})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("同域名同端口应拒绝，得到 %v", err)
	}
	if !strings.Contains(err.Error(), "on port 18081") {
		t.Fatalf("报错应指出冲突端口：%v", err)
	}

	// 同域名换成没被占用的端口 → 允许
	if _, err := Add(dirs, AddInput{Domain: "taken.example", Root: doc, Listens: []string{"18083"}}); err != nil {
		t.Fatalf("同域名换端口应允许：%v", err)
	}
}

// TestSiteAddTLSWithoutCert 新增 TLS 站点时证书文件还没生成（要等首次
// start 的 CertInit），校验不能因此拒绝，否则新装的机器永远加不了 HTTPS 站点。
func TestSiteAddTLSWithoutCert(t *testing.T) {
	live, doc, _, _ := writeLive(t, "live.example")
	dirs := []string{filepath.Join(filepath.Dir(live), "conf.d")}
	if _, err := Add(dirs, AddInput{
		Domain: "tls.example", Root: doc, Listens: []string{"18443 ssl h2"}, TLS: true,
	}); err != nil {
		t.Fatalf("新增 TLS 站点不应因证书未生成而失败：%v", err)
	}
	if _, err := os.Stat(filepath.Join(dirs[0], "tls.example.json")); err != nil {
		t.Fatalf("站点文件应已写入：%v", err)
	}
}

// TestSiteAddOnFreshInstall 复刻装机现场：安装包自带 conf.d/default.json
// （443 ssl，指向 ./cert/localhost.pem），但 config/cert 是空的、服务还没
// start 过。此时新增站点不能被「默认站点证书还没生成」拦死。
func TestSiteAddOnFreshInstall(t *testing.T) {
	work := t.TempDir()
	cfg := filepath.Join(work, "config")
	confd := filepath.Join(cfg, "conf.d")
	doc := filepath.Join(work, "www")
	for _, dir := range []string{confd, filepath.Join(cfg, "cert"), doc} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	shipped := `{
  "listen": [80, "443 ssl h2"],
  "server_name": "localhost",
  "ssl_certificate": "./cert/localhost.pem",
  "ssl_certificate_key": "./cert/localhost-key.pem",
  "location": [{"url": "/", "type": "local", "root": "."}]
}`
	if err := os.WriteFile(filepath.Join(confd, "default.json"), []byte(shipped), 0o644); err != nil {
		t.Fatal(err)
	}
	live := filepath.Join(cfg, "fast-https.json")
	if err := os.WriteFile(live, []byte(`{"http":{"include":["conf.d"]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(cfg)
	eng := &txn.Engine{
		State:    state.Dir{Root: filepath.Join(work, "state"), Alive: func(int, string) bool { return true }},
		Reloader: &countReload{},
		Probe:    func(int) bool { return true },
		Bind:     func(int) error { return nil },
	}
	res, err := Run(context.Background(), eng, RunInput{
		LiveMain: live, RequestID: "fresh1", Command: "site.add", Summary: "add site.example",
		Args: map[string]string{"domain": "site.example"},
		Mutate: func(_ string, dirs []string) error {
			_, err := Add(dirs, AddInput{Domain: "site.example", Root: doc, Listens: []string{"8080"}})
			return err
		},
	})
	if err != nil {
		t.Fatalf("新装环境下新增站点应成功：%v", err)
	}
	if _, err := os.Stat(filepath.Join(confd, "site.example.json")); err != nil {
		t.Fatalf("站点文件应已落盘：%v", err)
	}
	if len(res.Warnings) == 0 {
		t.Fatal("应给出「证书待首次 start 生成」的告警")
	}
}

// TestSiteAddWithSharedCertificate 显式指定证书路径：多站点共用同一张证书
// （如通配符证书）时，--ssl-certificate 的路径要原样写进站点文件。
func TestSiteAddWithSharedCertificate(t *testing.T) {
	live, doc, _, _ := writeLive(t, "live.example")
	dirs := []string{filepath.Join(filepath.Dir(live), "conf.d")}
	certDir := filepath.Join(filepath.Dir(live), "cert")
	if err := os.MkdirAll(certDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cert := filepath.Join(certDir, "wildcard.pem")
	key := filepath.Join(certDir, "wildcard-key.pem")
	if err := os.WriteFile(cert, []byte("cert"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(key, []byte("key"), 0o600); err != nil {
		t.Fatal(err)
	}

	path, err := Add(dirs, AddInput{
		Domain: "a.example", Root: doc, Listens: []string{"18443 ssl h2"}, TLS: true,
		SSLCertificate: cert, SSLCertificateKey: key,
	})
	if err != nil {
		t.Fatalf("显式证书路径应允许：%v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "wildcard.pem") || !strings.Contains(string(raw), "wildcard-key.pem") {
		t.Fatalf("站点文件应写入给定证书路径：%s", raw)
	}
}

// TestSiteUpdateServerNameFollowsCert 改 server_name 时，约定证书路径
// （文件名就是主域名）要跟着改名，否则新域名握手会拿到一张 SAN 不含它的证书。
func TestSiteUpdateServerNameFollowsCert(t *testing.T) {
	live, _, _, _ := writeLive(t, "live.example")
	dirs := []string{filepath.Join(filepath.Dir(live), "conf.d")}
	site := `{
  "listen": ["18443 ssl h2"],
  "server_name": "old.example",
  "ssl_certificate": "./cert/old.example.pem",
  "ssl_certificate_key": "./cert/old.example-key.pem",
  "location": [{"url": "/", "type": "local", "root": "."}]
}`
	path := filepath.Join(dirs[0], "old.example.json")
	if err := os.WriteFile(path, []byte(site), 0o644); err != nil {
		t.Fatal(err)
	}

	sn := "new.example"
	if _, err := Update(dirs, live, UpdateInput{Name: "old.example", ServerName: &sn}); err != nil {
		t.Fatalf("改名应成功：%v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	if !strings.Contains(got, "new.example.pem") || !strings.Contains(got, "new.example-key.pem") {
		t.Fatalf("约定证书路径应跟随改名：%s", got)
	}
	if strings.Contains(got, "old.example.pem") {
		t.Fatalf("不应残留旧域名证书路径：%s", got)
	}
}

// TestSiteUpdateServerNameKeepsCustomCert 自定义证书路径（通配符 / 多站点共享）
// 在改名时不能被改写，否则会把别人的证书指错地方。
func TestSiteUpdateServerNameKeepsCustomCert(t *testing.T) {
	live, _, _, _ := writeLive(t, "live.example")
	dirs := []string{filepath.Join(filepath.Dir(live), "conf.d")}
	site := `{
  "listen": ["18443 ssl h2"],
  "server_name": "old.example",
  "ssl_certificate": "./certs/wildcard.pem",
  "ssl_certificate_key": "./certs/wildcard-key.pem",
  "location": [{"url": "/", "type": "local", "root": "."}]
}`
	path := filepath.Join(dirs[0], "old.example.json")
	if err := os.WriteFile(path, []byte(site), 0o644); err != nil {
		t.Fatal(err)
	}

	sn := "new.example"
	if _, err := Update(dirs, live, UpdateInput{Name: "old.example", ServerName: &sn}); err != nil {
		t.Fatalf("改名应成功：%v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	if !strings.Contains(got, "wildcard.pem") || !strings.Contains(got, "wildcard-key.pem") {
		t.Fatalf("自定义证书路径不应被改写：%s", got)
	}
}

// TestSiteDeleteKeepsCerts 删除站点只删 json，证书文件要保留并能报出来，
// 这样重建同名站点可以直接复用，不必重新签发。
func TestSiteDeleteKeepsCerts(t *testing.T) {
	live, _, _, _ := writeLive(t, "live.example")
	dirs := []string{filepath.Join(filepath.Dir(live), "conf.d")}
	certDir := filepath.Join(filepath.Dir(live), "cert")
	if err := os.MkdirAll(certDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cert := filepath.Join(certDir, "keep.example.pem")
	key := filepath.Join(certDir, "keep.example-key.pem")
	for _, p := range []string{cert, key} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	site := `{
  "listen": ["18443 ssl h2"],
  "server_name": "keep.example",
  "ssl_certificate": "./cert/keep.example.pem",
  "ssl_certificate_key": "./cert/keep.example-key.pem",
  "location": [{"url": "/", "type": "local", "root": "."}]
}`
	if err := os.WriteFile(filepath.Join(dirs[0], "keep.example.json"), []byte(site), 0o644); err != nil {
		t.Fatal(err)
	}

	_, kept, err := Delete(dirs, live, "keep.example")
	if err != nil {
		t.Fatalf("删除应成功：%v", err)
	}
	if len(kept) != 2 {
		t.Fatalf("应报出两张保留的证书文件，得到 %v", kept)
	}
	for _, p := range []string{cert, key} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("证书文件不应被删除：%s", p)
		}
	}
	if _, err := os.Stat(filepath.Join(dirs[0], "keep.example.json")); err == nil {
		t.Fatal("站点 json 应已删除")
	}
}

// TestSiteAddCertWithoutTLS 给了证书但监听里没有 ssl：参数互相矛盾，
// 按用法错误拒绝，别写出一个带证书却不走 TLS 的怪站点。
func TestSiteAddCertWithoutTLS(t *testing.T) {
	live, doc, _, _ := writeLive(t, "live.example")
	dirs := []string{filepath.Join(filepath.Dir(live), "conf.d")}
	_, err := Add(dirs, AddInput{
		Domain: "plain.example", Root: doc, Listens: []string{"8080"},
		SSLCertificate: "./cert/plain.example.pem", SSLCertificateKey: "./cert/plain.example-key.pem",
	})
	if !errors.Is(err, ErrUsage) {
		t.Fatalf("没有 TLS 监听却指定证书应报 Usage 错误，得到 %v", err)
	}
}

// TestSiteAddSameNameDifferentPortOnShippedConfig 复刻装机现场：
// 安装包自带 default.json（server_name localhost，监听 80），
// 再建一个 localhost:8080 不应被判成冲突 —— 端口不同就是两个 server 块。
func TestSiteAddSameNameDifferentPortOnShippedConfig(t *testing.T) {
	work := t.TempDir()
	cfg := filepath.Join(work, "config")
	confd := filepath.Join(cfg, "conf.d")
	doc := filepath.Join(work, "www")
	for _, dir := range []string{confd, filepath.Join(cfg, "cert"), doc} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// 只留 80：这条用例只验「端口维度」，不引入「证书还没生成」的干扰。
	shipped := `{
  "listen": [80],
  "server_name": "localhost",
  "location": [{"url": "/", "type": "local", "root": "."}]
}`
	if err := os.WriteFile(filepath.Join(confd, "default.json"), []byte(shipped), 0o644); err != nil {
		t.Fatal(err)
	}
	live := filepath.Join(cfg, "fast-https.json")
	if err := os.WriteFile(live, []byte(`{"http":{"include":["conf.d"]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(cfg)
	eng := &txn.Engine{
		State:    state.Dir{Root: filepath.Join(work, "state"), Alive: func(int, string) bool { return true }},
		Reloader: &countReload{},
		Probe:    func(int) bool { return true },
		Bind:     func(int) error { return nil },
	}
	_, err := Run(context.Background(), eng, RunInput{
		LiveMain: live, RequestID: "sameport", Command: "site.add", Summary: "add localhost:8080",
		Args: map[string]string{"domain": "localhost", "listen": "8080"},
		Mutate: func(_ string, dirs []string) error {
			_, err := Add(dirs, AddInput{Domain: "localhost", Root: doc, Listens: []string{"8080"}})
			return err
		},
	})
	if err != nil {
		t.Fatalf("localhost:8080 与 localhost:80/443 端口不同，不应冲突：%v", err)
	}
	if _, err := os.Stat(filepath.Join(confd, "localhost.json")); err != nil {
		t.Fatalf("站点文件应已写入：%v", err)
	}
}

func TestSiteAddInvalidDomainOrListen(t *testing.T) {
	live, doc, eng, _ := writeLive(t, "live.example")
	_, err := runAdd(t, eng, live, "../evil", doc, "bad", false)
	if !errors.Is(err, ErrUsage) {
		t.Fatalf("非法域名 err=%v", err)
	}
	_, err = Run(context.Background(), eng, RunInput{
		LiveMain: live, Command: "site.add",
		Mutate: func(_ string, dirs []string) error {
			_, err := Add(dirs, AddInput{Domain: "ok.example", Root: doc, Listens: []string{"bad-port"}})
			return err
		},
	})
	if err == nil || errors.Is(err, ErrUsage) {
		t.Fatalf("非法 listen 应为配置校验错误，got %v", err)
	}
}

func TestSiteRootPathTraversal(t *testing.T) {
	live, _, _, _ := writeLive(t, "live.example")
	cfgDir := filepath.Dir(live)
	_, err := Add([]string{filepath.Join(cfgDir, "conf.d")}, AddInput{
		Domain: "x.example", Root: filepath.Join("..", "..", "etc"), Listens: []string{"80"},
		Banned: []string{cfgDir},
	})
	if err == nil {
		t.Fatal("应拒绝 ..")
	}
	_, err = Add([]string{filepath.Join(cfgDir, "conf.d")}, AddInput{
		Domain: "x.example", Root: cfgDir, Listens: []string{"80"},
		Banned: []string{cfgDir},
	})
	if err == nil {
		t.Fatal("应拒绝配置目录")
	}
	stateDir := filepath.Join(filepath.Dir(cfgDir), "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err = Add([]string{filepath.Join(cfgDir, "conf.d")}, AddInput{
		Domain: "x.example", Root: stateDir, Listens: []string{"80"},
		Banned: []string{cfgDir, stateDir},
	})
	if err == nil {
		t.Fatal("应拒绝状态目录")
	}
}

func TestSiteUpdateWhitelistAndComments(t *testing.T) {
	live, doc, eng, _ := writeLive(t, "live.example")
	if _, err := runAdd(t, eng, live, "blog.example", doc, "u0", false); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(filepath.Dir(live), "conf.d", "blog.example.json")
	raw := []byte(`{
  // keep me
  "listen": [18081],
  "server_name": "blog.example",
  "location": [{"url": "/", "type": "local", "root": "."}]
}`)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	name := "blog.example"
	sn := "blog2.example"
	_, err := Run(context.Background(), eng, RunInput{
		LiveMain: live, Command: "site.update", RequestID: "u1",
		Mutate: func(_ string, dirs []string) error {
			_, err := Update(dirs, live, UpdateInput{Name: name, ServerName: &sn})
			return err
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if !strings.Contains(string(got), "keep me") {
		t.Fatalf("应保留注释：%s", got)
	}
	if !strings.Contains(string(got), "blog2.example") {
		t.Fatalf("应改 server_name：%s", got)
	}
}

func TestSiteUpdateInlineNotSupported(t *testing.T) {
	work := t.TempDir()
	liveDir := filepath.Join(work, "config")
	if err := os.MkdirAll(filepath.Join(liveDir, "conf.d"), 0o755); err != nil {
		t.Fatal(err)
	}
	main := `{
  "http": {
    "include": ["conf.d"],
    "server": [{"listen":[80],"server_name":"inline.example","location":[{"url":"/","type":"local","root":"."}]}]
  }
}`
	live := filepath.Join(liveDir, "fast-https.json")
	if err := os.WriteFile(live, []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(liveDir)
	sn := "other.example"
	_, err := Update([]string{filepath.Join(liveDir, "conf.d")}, live, UpdateInput{Name: "inline.example", ServerName: &sn})
	if !errors.Is(err, ErrNotSupported) {
		t.Fatalf("err = %v", err)
	}
}

func TestSiteDeleteKeepsContent(t *testing.T) {
	live, doc, eng, _ := writeLive(t, "live.example")
	sent := filepath.Join(doc, "index.html")
	cert := filepath.Join(filepath.Dir(live), "cert.pem")
	if err := os.WriteFile(sent, []byte("KEEP"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cert, []byte("CERT"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runAdd(t, eng, live, "gone.example", doc, "del1", false); err != nil {
		t.Fatal(err)
	}
	_, err := Run(context.Background(), eng, RunInput{
		LiveMain: live, Command: "site.delete", RequestID: "del2",
		Mutate: func(_ string, dirs []string) error {
			_, _, err := Delete(dirs, live, "gone.example")
			return err
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(live), "conf.d", "gone.example.json")); !os.IsNotExist(err) {
		t.Fatal("配置文件应删掉")
	}
	if b, _ := os.ReadFile(sent); string(b) != "KEEP" {
		t.Fatal("内容目录哨兵应还在")
	}
	if b, _ := os.ReadFile(cert); string(b) != "CERT" {
		t.Fatal("证书应还在")
	}
}

func TestSiteDisableEnableRoundTrip(t *testing.T) {
	live, doc, eng, _ := writeLive(t, "live.example")
	if _, err := runAdd(t, eng, live, "flip.example", doc, "f1", false); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(filepath.Dir(live), "conf.d", "flip.example.json")
	orig, _ := os.ReadFile(path)
	_, err := Run(context.Background(), eng, RunInput{
		LiveMain: live, Command: "site.disable", RequestID: "f2",
		Mutate: func(_ string, dirs []string) error {
			_, err := SetEnabled(dirs, live, "flip.example", false)
			return err
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("停用后不应再加载 .json")
	}
	dis := path + ".disabled"
	if _, err := os.Stat(dis); err != nil {
		t.Fatal("应存在 .json.disabled")
	}
	entries, err := site.ListAll([]string{filepath.Join(filepath.Dir(live), "conf.d")})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range entries {
		if e.Name == "flip.example" {
			found = true
			if e.Enabled {
				t.Fatal("停用后 ListAll 应为 enabled:false")
			}
		}
	}
	if !found {
		t.Fatal("停用站点应出现在 ListAll")
	}
	_, err = Run(context.Background(), eng, RunInput{
		LiveMain: live, Command: "site.enable", RequestID: "f3",
		Mutate: func(_ string, dirs []string) error {
			_, err := SetEnabled(dirs, live, "flip.example", true)
			return err
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != string(orig) {
		t.Fatalf("启用后字节应一致\n%s\n%s", orig, got)
	}
}

func TestSiteWriteIdempotent(t *testing.T) {
	live, doc, eng, reload := writeLive(t, "live.example")
	a, err := runAdd(t, eng, live, "idem.example", doc, "same", false)
	if err != nil {
		t.Fatal(err)
	}
	n := reload.n
	vers, _ := cfgver.List(eng.State, live)
	b, err := runAdd(t, eng, live, "idem.example", doc, "same", false)
	if err != nil {
		t.Fatal(err)
	}
	if a.Operation.ID != b.Operation.ID || !b.Idempotent {
		t.Fatalf("应返回同一 operation")
	}
	if reload.n != n {
		t.Fatal("不应再 reload")
	}
	vers2, _ := cfgver.List(eng.State, live)
	if len(vers2.Versions) != len(vers.Versions) {
		t.Fatal("版本数不应增加")
	}
}

func TestSiteWriteRollback(t *testing.T) {
	live, doc, eng, reload := writeLive(t, "live.example")
	reload.fail = 1
	_, err := runAdd(t, eng, live, "rb.example", doc, "rb1", false)
	if err == nil {
		t.Fatal("注入 reload 失败应报错")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(live), "conf.d", "rb.example.json")); !os.IsNotExist(err) {
		t.Fatal("失败回滚后站点文件应恢复原状（不存在）")
	}
}

func mustHash(t *testing.T, live string) string {
	t.Helper()
	h, err := cfgver.TreeHash(live)
	if err != nil {
		t.Fatal(err)
	}
	return h
}
