package diagnose

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fast-https/infra/logging"
	"fast-https/management/view"
)

const fixtureSecret = "S3cr3t-XYZ"

func TestDiagnoseAllOK(t *testing.T) {
	in := healthyInput(t)
	report := Run(in)
	if report.Overall != StatusOK {
		t.Fatalf("overall = %s\n%s", report.Overall, dump(report))
	}
	if !strings.HasPrefix(report.ReportID, "diag_") {
		t.Fatalf("report_id = %s", report.ReportID)
	}
	if report.ObservedAt == "" {
		t.Fatal("missing observed_at")
	}
	byName := checksByName(report)
	for _, name := range []string{"service", "config", "certs", "ports", "logs"} {
		if byName[name].Status != StatusOK {
			t.Errorf("%s = %+v", name, byName[name])
		}
	}
}

func TestDiagnoseConfigInvalid(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "fast-https.json")
	if err := os.WriteFile(main, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	report := Run(Input{
		ConfigPath: main,
		Inspect:    runningInspect(9),
		Probe:      func(int) bool { return true },
		Timeout:    time.Second,
	})
	if report.Overall != StatusError {
		t.Fatalf("overall = %s", report.Overall)
	}
	got := checksByName(report)["config"]
	if got.Status != StatusError {
		t.Fatalf("config = %+v", got)
	}
	if strings.Contains(dump(report), fixtureSecret) {
		t.Fatal("诊断输出含有夹具口令")
	}
}

func TestDiagnoseUnavailableVsFailed(t *testing.T) {
	report := Run(healthyInput(t))
	if report.Overall != StatusOK {
		t.Fatalf("overall = %s，unavailable 不应拉低", report.Overall)
	}
	byName := checksByName(report)
	for _, name := range []string{"runtime", "monitor", "state"} {
		if byName[name].Status != StatusUnavailable {
			t.Errorf("%s = %+v, want unavailable", name, byName[name])
		}
	}
}

func TestDiagnoseCheckTimeout(t *testing.T) {
	in := healthyInput(t)
	in.Timeout = 40 * time.Millisecond
	in.Extra = []NamedCheck{{
		Name: "stuck",
		Fn: func(ctx context.Context) Check {
			<-ctx.Done()
			return Check{Status: StatusUnavailable, Message: "still blocked"}
		},
	}}
	start := time.Now()
	report := Run(in)
	if time.Since(start) > 2*time.Second {
		t.Fatalf("诊断被卡住的检查拖住了：%s", time.Since(start))
	}
	byName := checksByName(report)
	if byName["config"].Status != StatusOK {
		t.Fatalf("其余检查应仍返回：config=%+v", byName["config"])
	}
	stuck := byName["stuck"]
	if stuck.Status != StatusUnavailable {
		t.Fatalf("stuck = %+v", stuck)
	}
	if stuck.Message != "check timed out" && stuck.Message != "still blocked" {
		t.Fatalf("stuck message = %q", stuck.Message)
	}
	if report.Overall != StatusOK {
		t.Fatalf("超时项是 unavailable，不应拉低 overall：%s", report.Overall)
	}
}

func TestDiagnoseNoSecrets(t *testing.T) {
	in := healthyInput(t)
	report := Run(in)
	raw := dump(report)
	if strings.Contains(raw, fixtureSecret) || strings.Contains(raw, "HMAC-ABC") {
		t.Fatalf("输出含有敏感值：%s", raw)
	}
}

func healthyInput(t *testing.T) Input {
	t.Helper()
	dir := t.TempDir()
	confd := filepath.Join(dir, "conf.d")
	logs := filepath.Join(dir, "logs")
	if err := os.MkdirAll(confd, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(logs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(logs, logging.ErrorLogName), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	site := `{
  "listen": [18080],
  "server_name": "demo.example",
  "location": [{
    "url": "/", "type": "local", "root": ` + jsonString(dir) + `,
    "auth": {"type": "basic", "user": "admin", "pswd": "` + fixtureSecret + `"}
  }]
}`
	if err := os.WriteFile(filepath.Join(confd, "demo.json"), []byte(site), 0o644); err != nil {
		t.Fatal(err)
	}
	main := filepath.Join(dir, "fast-https.json")
	body := `{"log_root":` + jsonString(logs) + `,"http":{"include":[` + jsonString(confd) + `]}}`
	if err := os.WriteFile(main, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	pid := filepath.Join(dir, "fast-https.pid")
	if err := os.WriteFile(pid, []byte("7"), 0o644); err != nil {
		t.Fatal(err)
	}
	return Input{
		ConfigPath: main,
		PidFile:    pid,
		Inspect:    runningInspect(7),
		Probe:      func(int) bool { return true },
		Timeout:    time.Second,
		Now:        func() time.Time { return time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC) },
	}
}

func runningInspect(pid int) func() (view.ProcState, int, error) {
	return func() (view.ProcState, int, error) { return view.ProcRunning, pid, nil }
}

func checksByName(report Report) map[string]Check {
	out := map[string]Check{}
	for _, c := range report.Checks {
		out[c.Name] = c
	}
	return out
}

func dump(report Report) string {
	b, err := json.Marshal(report)
	if err != nil {
		return err.Error()
	}
	return string(b)
}

func jsonString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}
