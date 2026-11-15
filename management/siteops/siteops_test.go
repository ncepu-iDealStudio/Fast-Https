package siteops

import (
	"context"
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
			_, err := Delete(dirs, live, "gone.example")
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
