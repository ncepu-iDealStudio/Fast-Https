package txn

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fast-https/config"
	"fast-https/management/cfgver"
	"fast-https/management/operation"
	"fast-https/management/state"
	"fast-https/management/svcctl"
)

type countReload struct {
	n, fail int
}

func (c *countReload) Reload(context.Context) error {
	c.n++
	if c.fail > 0 && c.n == c.fail {
		return errors.New("reload injected")
	}
	return nil
}

type countHealth struct {
	n, fail int
}

func (c *countHealth) Check(context.Context) error {
	c.n++
	if c.fail > 0 && c.n == c.fail {
		return errors.New("health injected")
	}
	return nil
}

type permFiles struct {
	OSFiles
	deny bool
}

func (p permFiles) Writable(dir string) error {
	if p.deny {
		return os.ErrPermission
	}
	return OSFiles{}.Writable(dir)
}

func writeTree(t *testing.T, root, name, extra string) {
	t.Helper()
	confd := filepath.Join(root, "conf.d")
	if err := os.MkdirAll(confd, 0o755); err != nil {
		t.Fatal(err)
	}
	site := `{
  "listen": [18080],
  "server_name": "` + name + `",
  "location": [{"url": "/", "type": "local", "root": "."}]
}`
	if extra != "" {
		site = extra
	}
	if err := os.WriteFile(filepath.Join(confd, "site.json"), []byte(site), 0o644); err != nil {
		t.Fatal(err)
	}
	main := `{
  "http": {"include": ["conf.d"]}
}`
	if err := os.WriteFile(filepath.Join(root, "fast-https.json"), []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}
}

func setupApply(t *testing.T) (liveMain, cand string, eng *Engine, reload *countReload, health *countHealth) {
	t.Helper()
	work := t.TempDir()
	liveDir := filepath.Join(work, "config")
	candDir := filepath.Join(work, "cand")
	writeTree(t, liveDir, "live.example", "")
	writeTree(t, candDir, "cand.example", "")
	t.Chdir(liveDir)
	reload = &countReload{}
	health = &countHealth{}
	eng = &Engine{
		State:    state.Dir{Root: filepath.Join(work, "state"), Alive: func(int, string) bool { return true }},
		Reloader: reload,
		Health:   health,
		Probe:    func(int) bool { return true },
		Bind:     func(int) error { return nil },
	}
	return filepath.Join(liveDir, "fast-https.json"), candDir, eng, reload, health
}

func applyOK(t *testing.T, eng *Engine, live, cand, req string) Result {
	t.Helper()
	res, err := eng.Apply(context.Background(), Input{
		Candidate: cand, LiveMain: live, RequestID: req,
		Command: "config.apply", Summary: "test apply",
	})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func fileOf(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestApplySuccess(t *testing.T) {
	live, cand, eng, _, _ := setupApply(t)
	res := applyOK(t, eng, live, cand, "req-ok")
	if res.Operation.Status != operation.StatusSucceeded {
		t.Fatalf("status = %s", res.Operation.Status)
	}
	if res.RestorePoint == "" || res.Version == "" {
		t.Fatalf("restore=%s version=%s", res.RestorePoint, res.Version)
	}
	list, err := cfgver.List(eng.State, live)
	if err != nil {
		t.Fatal(err)
	}
	if list.Drift {
		t.Fatal("drift after success")
	}
	var active string
	for _, v := range list.Versions {
		if v.Active {
			active = v.Version
		}
	}
	if active != res.Version {
		t.Fatalf("active = %s want %s", active, res.Version)
	}
	if !strings.Contains(fileOf(t, filepath.Join(filepath.Dir(live), "conf.d", "site.json")), "cand.example") {
		t.Fatal("线上站点应等于候选")
	}
	if _, err := os.Stat(filepath.Join(eng.State.VersionsDir(), res.RestorePoint)); err != nil {
		t.Fatal("恢复点应存在")
	}
}

func TestApplyDryRunWritesNothing(t *testing.T) {
	live, cand, eng, reload, _ := setupApply(t)
	before, err := cfgver.TreeHash(live)
	if err != nil {
		t.Fatal(err)
	}
	res, err := eng.Apply(context.Background(), Input{Candidate: cand, LiveMain: live, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if !res.DryRun || reload.n != 0 {
		t.Fatalf("dry-run 不应 reload：%+v n=%d", res, reload.n)
	}
	after, err := cfgver.TreeHash(live)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("dry-run 改变了线上哈希")
	}
	list, _ := cfgver.List(eng.State, live)
	if len(list.Versions) != 0 {
		t.Fatalf("dry-run 不应建版本：%v", list.Versions)
	}
}

func TestApplyValidationFails(t *testing.T) {
	live, cand, eng, _, _ := setupApply(t)
	bad := `{
  "listen": [18080],
  "server_name": "",
  "location": [{"url": "/", "type": "local", "root": "."}]
}`
	if err := os.WriteFile(filepath.Join(cand, "conf.d", "site.json"), []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	before := fileOf(t, filepath.Join(filepath.Dir(live), "conf.d", "site.json"))
	_, err := eng.Apply(context.Background(), Input{Candidate: cand, LiveMain: live})
	var te *Error
	if !errors.As(err, &te) || te.Code != CodeConfigInvalid {
		t.Fatalf("err = %v", err)
	}
	if fileOf(t, filepath.Join(filepath.Dir(live), "conf.d", "site.json")) != before {
		t.Fatal("校验失败不应改线上")
	}
	list, _ := cfgver.List(eng.State, live)
	if len(list.Versions) != 0 {
		t.Fatal("校验失败不应建恢复点")
	}
}

func TestApplyReloadFailsRollsBack(t *testing.T) {
	live, cand, eng, reload, _ := setupApply(t)
	reload.fail = 1
	before := fileOf(t, filepath.Join(filepath.Dir(live), "conf.d", "site.json"))
	_, err := eng.Apply(context.Background(), Input{Candidate: cand, LiveMain: live, RequestID: "r1"})
	var te *Error
	if !errors.As(err, &te) || te.Code != CodeFailed || te.Operation == nil {
		t.Fatalf("err = %v", err)
	}
	if te.Operation.Status != operation.StatusFailed || te.Operation.Result["rolled_back"] != true {
		t.Fatalf("op = %+v", te.Operation)
	}
	if fileOf(t, filepath.Join(filepath.Dir(live), "conf.d", "site.json")) != before {
		t.Fatal("线上应还原")
	}
	if reload.n != 2 {
		t.Fatalf("补偿应再 reload，n=%d", reload.n)
	}
}

func TestApplyHealthFailsRollsBack(t *testing.T) {
	live, cand, eng, reload, health := setupApply(t)
	health.fail = 1
	before := fileOf(t, filepath.Join(filepath.Dir(live), "conf.d", "site.json"))
	_, err := eng.Apply(context.Background(), Input{Candidate: cand, LiveMain: live})
	var te *Error
	if !errors.As(err, &te) || te.Code != CodeFailed {
		t.Fatalf("err = %v", err)
	}
	if te.Operation.Result["rolled_back"] != true {
		t.Fatalf("op = %+v", te.Operation)
	}
	if fileOf(t, filepath.Join(filepath.Dir(live), "conf.d", "site.json")) != before {
		t.Fatal("线上应还原")
	}
	if reload.n != 2 || health.n != 2 {
		t.Fatalf("reload=%d health=%d", reload.n, health.n)
	}
}

func TestApplyRollbackFailsRecoveryRequired(t *testing.T) {
	live, cand, eng, reload, _ := setupApply(t)
	reload.fail = 1
	eng.RestoreErr = errors.New("restore injected")
	_, err := eng.Apply(context.Background(), Input{Candidate: cand, LiveMain: live})
	var te *Error
	if !errors.As(err, &te) || te.Code != CodeRecovery {
		t.Fatalf("err = %v", err)
	}
	if te.Operation.Status != operation.StatusRecoveryRequired {
		t.Fatalf("status = %s", te.Operation.Status)
	}
	if te.Operation.Result["restore_point"] == nil || te.Operation.Result["restore_point"] == "" {
		t.Fatal("应写出恢复点路径")
	}
}

func TestApplyCrashBetweenPhases(t *testing.T) {
	phases := []string{PhaseValidated, PhaseRestorePointSaved, PhaseFilesSwitched, PhaseReloaded, PhaseVerified}
	for _, ph := range phases {
		t.Run(ph, func(t *testing.T) {
			live, cand, eng, _, _ := setupApply(t)
			eng.State.PID = 42
			eng.State.Alive = func(pid int, _ string) bool { return pid == 42 }
			eng.StopAfter = ph
			_, err := eng.Apply(context.Background(), Input{Candidate: cand, LiveMain: live, RequestID: "crash-" + ph})
			if !errors.Is(err, errStopped) {
				t.Fatalf("err = %v", err)
			}
			eng2 := &Engine{State: state.Dir{Root: eng.State.Root, Alive: func(int, string) bool { return false }}}
			if err := eng2.Recover(); err != nil {
				t.Fatal(err)
			}
			store := operation.Store{Dir: eng2.State}
			list, err := store.List(operation.ListFilter{Limit: 10})
			if err != nil || len(list) != 1 {
				t.Fatalf("list = %v err=%v", list, err)
			}
			got, err := store.Get(list[0].ID)
			if err != nil {
				t.Fatal(err)
			}
			switch got.Status {
			case operation.StatusUnknown, operation.StatusRecoveryRequired:
			default:
				t.Fatalf("status = %s", got.Status)
			}
			if ph == PhaseFilesSwitched && got.Status != operation.StatusRecoveryRequired {
				t.Fatalf("切换后崩溃应为 recovery_required，得到 %s", got.Status)
			}
		})
	}
}

func TestApplyIdempotent(t *testing.T) {
	live, cand, eng, reload, _ := setupApply(t)
	a := applyOK(t, eng, live, cand, "same")
	nVer := mustCountVersions(t, eng, live)
	nReload := reload.n
	b, err := eng.Apply(context.Background(), Input{Candidate: cand, LiveMain: live, RequestID: "same", Command: "config.apply"})
	if err != nil {
		t.Fatal(err)
	}
	if a.Operation.ID != b.Operation.ID || !b.Idempotent {
		t.Fatalf("应返回同一 operation：%s vs %s idem=%v", a.Operation.ID, b.Operation.ID, b.Idempotent)
	}
	if reload.n != nReload {
		t.Fatalf("不应再 reload：%d vs %d", reload.n, nReload)
	}
	if mustCountVersions(t, eng, live) != nVer {
		t.Fatal("版本数不应增加")
	}
}

func TestApplyRequestIDConflict(t *testing.T) {
	live, cand, eng, _, _ := setupApply(t)
	applyOK(t, eng, live, cand, "x")
	other := filepath.Join(t.TempDir(), "other")
	writeTree(t, other, "other.example", "")
	_, err := eng.Apply(context.Background(), Input{Candidate: other, LiveMain: live, RequestID: "x", Command: "config.apply"})
	var te *Error
	if !errors.As(err, &te) || te.Code != CodeRequestClash {
		t.Fatalf("err = %v", err)
	}
}

func TestApplyPortInUse(t *testing.T) {
	live, cand, eng, reload, _ := setupApply(t)
	before := fileOf(t, filepath.Join(filepath.Dir(live), "conf.d", "site.json"))
	eng.Probe = func(int) bool { return false }
	eng.Bind = func(port int) error { return &svcctl.PortError{Port: port} }
	_, err := eng.Apply(context.Background(), Input{Candidate: cand, LiveMain: live})
	var te *Error
	if !errors.As(err, &te) || te.Code != CodeConflict {
		t.Fatalf("err = %v", err)
	}
	if reload.n != 0 {
		t.Fatal("不应 reload")
	}
	if fileOf(t, filepath.Join(filepath.Dir(live), "conf.d", "site.json")) != before {
		t.Fatal("线上应不变")
	}
}

func TestApplyNoPermission(t *testing.T) {
	live, cand, eng, reload, _ := setupApply(t)
	eng.Files = permFiles{deny: true}
	before := fileOf(t, filepath.Join(filepath.Dir(live), "conf.d", "site.json"))
	_, err := eng.Apply(context.Background(), Input{Candidate: cand, LiveMain: live})
	var te *Error
	if !errors.As(err, &te) || te.Code != CodePermission {
		t.Fatalf("err = %v", err)
	}
	if reload.n != 0 || fileOf(t, filepath.Join(filepath.Dir(live), "conf.d", "site.json")) != before {
		t.Fatal("无权限时线上应不变")
	}
}

func TestApplyDoesNotTouchCertsOrDocroot(t *testing.T) {
	live, cand, eng, _, _ := setupApply(t)
	cert := filepath.Join(filepath.Dir(live), "cert.pem")
	doc := filepath.Join(filepath.Dir(live), "www", "index.html")
	if err := os.MkdirAll(filepath.Dir(doc), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cert, []byte("CERT"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(doc, []byte("DOC"), 0o644); err != nil {
		t.Fatal(err)
	}
	cinfo, _ := os.Stat(cert)
	dinfo, _ := os.Stat(doc)
	time.Sleep(10 * time.Millisecond)
	applyOK(t, eng, live, cand, "touch")
	cinfo2, _ := os.Stat(cert)
	dinfo2, _ := os.Stat(doc)
	if fileOf(t, cert) != "CERT" || fileOf(t, doc) != "DOC" {
		t.Fatal("证书或站点内容被改写")
	}
	if !cinfo.ModTime().Equal(cinfo2.ModTime()) || !dinfo.ModTime().Equal(dinfo2.ModTime()) {
		t.Fatal("哨兵文件修改时间变了")
	}
}

func TestRollbackToVersion(t *testing.T) {
	live, cand, eng, _, _ := setupApply(t)
	first := applyOK(t, eng, live, cand, "a1")
	n := mustCountVersions(t, eng, live)
	v1 := filepath.Join(eng.State.VersionsDir(), first.RestorePoint)
	res, err := eng.Apply(context.Background(), Input{
		Candidate: v1, LiveMain: live, RequestID: "rb1",
		Command: "config.rollback", Summary: "rollback",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Operation.Command != "config.rollback" {
		t.Fatalf("command = %s", res.Operation.Command)
	}
	if mustCountVersions(t, eng, live) < n {
		t.Fatal("历史版本数不应减少")
	}
	if !strings.Contains(fileOf(t, filepath.Join(filepath.Dir(live), "conf.d", "site.json")), "live.example") {
		t.Fatal("回滚后应等于恢复点内容")
	}
}

func TestRollbackMissingCert(t *testing.T) {
	live, _, eng, _, _ := setupApply(t)
	ssl := `{
  "listen": ["18443 ssl"],
  "server_name": "ssl.example",
  "ssl_certificate": "no-such-s8-cert.pem",
  "ssl_certificate_key": "no-such-s8-key.pem",
  "location": [{"url": "/", "type": "local", "root": "."}]
}`
	cand := t.TempDir()
	writeTree(t, cand, "ssl.example", ssl)
	before := fileOf(t, filepath.Join(filepath.Dir(live), "conf.d", "site.json"))
	_, err := eng.Apply(context.Background(), Input{Candidate: cand, LiveMain: live, Command: "config.rollback"})
	var te *Error
	if !errors.As(err, &te) || te.Code != CodeConfigInvalid {
		t.Fatalf("err = %v", err)
	}
	if fileOf(t, filepath.Join(filepath.Dir(live), "conf.d", "site.json")) != before {
		t.Fatal("证书缺失时线上应不变")
	}
}

func TestNoSecretsInOperationRecord(t *testing.T) {
	live, cand, eng, _, _ := setupApply(t)
	secret := "S3cr3t-S8-XYZ"
	site := `{
  "listen": [18080],
  "server_name": "sec.example",
  "location": [{"url": "/", "type": "local", "root": ".", "auth": {"type": "basic", "user": "a", "pswd": "` + secret + `"}}]
}`
	if err := os.WriteFile(filepath.Join(cand, "conf.d", "site.json"), []byte(site), 0o644); err != nil {
		t.Fatal(err)
	}
	res := applyOK(t, eng, live, cand, "sec")
	raw, err := os.ReadFile(filepath.Join(eng.State.OperationsDir(), res.Operation.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), secret) {
		t.Fatal("operation 记录含有口令")
	}
}

func mustCountVersions(t *testing.T, eng *Engine, live string) int {
	t.Helper()
	list, err := cfgver.List(eng.State, live)
	if err != nil {
		t.Fatal(err)
	}
	return len(list.Versions)
}

func TestApplyUsesCandidatePathGuard(t *testing.T) {
	_, _, eng, _, _ := setupApply(t)
	_, err := eng.Apply(context.Background(), Input{Candidate: filepath.Join("..", "x")})
	if !errors.Is(err, config.ErrCandidatePath) {
		var te *Error
		if !errors.As(err, &te) || te.Code != CodeUsage {
			t.Fatalf("err = %v", err)
		}
	}
}
