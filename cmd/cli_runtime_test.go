package cmd

import (
	"encoding/json"
	"fast-https/cmd/internal/cli"
	"fast-https/cmd/internal/servicecmd"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"fast-https/config"
	"fast-https/infra/logging"
	"fast-https/management/rtclient"
	"fast-https/management/view"
)

func serveRuntimeFixture(t *testing.T, body any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/runtime" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if raw, ok := body.([]byte); ok {
			_, _ = w.Write(raw)
			return
		}
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Setenv(rtclient.EnvURL, srv.URL+"/runtime")
	t.Cleanup(srv.Close)
	return srv
}

func deadRuntimeURL(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	url := "http://" + ln.Addr().String() + "/runtime"
	_ = ln.Close()
	return url
}

func deadRuntime(t *testing.T) {
	t.Helper()
	t.Setenv(rtclient.EnvURL, deadRuntimeURL(t))
}

func sampleRuntime(t *testing.T) rtclient.Snapshot {
	t.Helper()
	started := time.Now().Add(-90 * time.Second).Format(logging.TimeFormat)
	return rtclient.Snapshot{
		SchemaVersion: rtclient.SchemaVersion,
		ObservedAt:    time.Now().Format(logging.TimeFormat),
		Server: rtclient.ServerInfo{
			State:            "running",
			PID:              42,
			Version:          "1.5.0-test",
			StartedAt:        started,
			ConfigGeneration: 4,
		},
		Listeners: []rtclient.ListenerInfo{{Port: "18080"}},
		Connections: rtclient.ConnectionsInfo{
			Active: 2, AcceptedTotal: 10, ClosedTotal: 8, RejectedTotal: 1,
		},
		Requests: rtclient.RequestsInfo{
			Active: 1, Total: 25, ParseErrorsTotal: 0, AbortedTotal: 0,
		},
		Responses2xx: 20, Responses4xx: 4, Responses5xx: 1,
		RequestDuration: &rtclient.DurationHistogram{
			Unit:  "ms",
			Count: 25,
			Buckets: []rtclient.DurationBucket{
				{Le: 1.0, Count: 10},
				{Le: "+Inf", Count: 25},
			},
		},
		Upstream: &rtclient.UpstreamInfo{DialFailTotal: 2, IdleConns: 3},
	}
}

func TestRuntimeStatusMatchesSnapshot(t *testing.T) {
	snap := sampleRuntime(t)
	serveRuntimeFixture(t, snap)

	out, _, code := runCLI(t, "runtime", "status", "--json")
	if code != 0 {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	doc := decodeOneDocument(t, out)
	if doc["command"] != "runtime.status" {
		t.Fatalf("command = %v", doc["command"])
	}
	data := doc["data"].(map[string]any)
	if data["observed_at"] == "" || data["source"] != "runtime" {
		t.Fatalf("data = %v", data)
	}
	if data["state"] != "running" {
		t.Fatalf("state = %v", data["state"])
	}
	if int(data["pid"].(float64)) != 42 {
		t.Fatalf("pid = %v", data["pid"])
	}
	req := data["requests"].(map[string]any)
	if req["total"].(float64) != 25 || req["active"].(float64) != 1 {
		t.Fatalf("requests = %v", req)
	}
	conn := data["connections"].(map[string]any)
	if conn["active"].(float64) != 2 {
		t.Fatalf("connections = %v", conn)
	}
}

func TestRuntimeStatusUnreachable(t *testing.T) {
	deadRuntime(t)
	out, _, code := runCLI(t, "runtime", "status", "--json")
	if code != 4 {
		t.Fatalf("exit = %d, want 4\n%s", code, out)
	}
	doc := decodeOneDocument(t, out)
	body := doc["error"].(map[string]any)
	if body["code"] != cli.CodeServiceDown {
		t.Fatalf("error = %v", body)
	}
}

func TestRuntimeSchemaMismatch(t *testing.T) {
	serveRuntimeFixture(t, map[string]any{"schema_version": "99", "server": map[string]any{"state": "running"}})
	out, _, code := runCLI(t, "runtime", "status", "--json")
	if code != 11 {
		t.Fatalf("exit = %d, want 11\n%s", code, out)
	}
	body := decodeOneDocument(t, out)["error"].(map[string]any)
	if body["code"] != cli.CodeUnmetDependency {
		t.Fatalf("error = %v", body)
	}
}

func TestRuntimeOmitsUnavailable(t *testing.T) {
	snap := sampleRuntime(t)
	serveRuntimeFixture(t, snap)
	out, _, code := runCLI(t, "runtime", "status", "--json")
	if code != 0 {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	doc := decodeOneDocument(t, out)
	data := doc["data"].(map[string]any)
	conn := data["connections"].(map[string]any)
	for _, absent := range []string{"reading", "writing", "waiting"} {
		if _, has := conn[absent]; has {
			t.Errorf("connections.%s 不应出现", absent)
		}
	}
	if _, has := data["resources"]; has {
		t.Fatal("resources 不应出现")
	}
	if _, has := data["cpu_percent"]; has {
		t.Fatal("cpu_percent 不应出现")
	}
	warns, _ := doc["warnings"].([]any)
	if len(warns) == 0 {
		t.Fatal("应有 warnings 说明省略字段")
	}
}

func TestRuntimeStatsUnsupportedFlags(t *testing.T) {
	deadRuntime(t)
	out, _, code := runCLI(t, "runtime", "stats", "--since", "5m", "--json")
	if code != 11 {
		t.Fatalf("--since exit = %d, want 11\n%s", code, out)
	}
	if decodeOneDocument(t, out)["error"].(map[string]any)["code"] != cli.CodeNotSupported {
		t.Fatalf("error = %v", decodeOneDocument(t, out)["error"])
	}
	out, _, code = runCLI(t, "runtime", "stats", "--domain", "demo.example", "--json")
	if code != 11 {
		t.Fatalf("--domain exit = %d, want 11\n%s", code, out)
	}
}

func TestRuntimeStatsWindowAndDuration(t *testing.T) {
	snap := sampleRuntime(t)
	serveRuntimeFixture(t, snap)
	out, _, code := runCLI(t, "runtime", "stats", "--json")
	if code != 0 {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	data := decodeOneDocument(t, out)["data"].(map[string]any)
	if data["window"] != "since_process_start" {
		t.Fatalf("window = %v", data["window"])
	}
	if _, has := data["request_duration"]; !has {
		t.Fatal("应包含 request_duration")
	}
	if _, has := data["upstream"]; !has {
		t.Fatal("快照有 upstream 时应输出")
	}
	resp := data["responses"].(map[string]any)
	if resp["4xx_total"].(float64) != 4 || resp["5xx_total"].(float64) != 1 {
		t.Fatalf("responses = %v", resp)
	}
}

func TestServiceStatusFull(t *testing.T) {
	t.Run("with snapshot", func(t *testing.T) {
		pid := 42
		st := view.ServiceStatus{Status: "running", PID: &pid, Source: "pid_file+port_probe"}
		snap := sampleRuntime(t)
		got, warnings := servicecmd.EnrichServiceStatus(st, nil, snap, nil, "v003")
		if got.Version != "1.5.0-test" || got.ConfigVersion != "v003" || got.StartedAt == "" {
			t.Fatalf("got = %+v", got)
		}
		if got.UptimeSeconds == nil || *got.UptimeSeconds < 80 {
			t.Fatalf("uptime = %v", got.UptimeSeconds)
		}
		if got.Source != "pid_file+port_probe+runtime" {
			t.Fatalf("source = %s", got.Source)
		}
		if len(warnings) != 0 {
			t.Fatalf("warnings = %v", warnings)
		}
	})
	t.Run("without runtime", func(t *testing.T) {
		skipIfPidFilePresent(t)
		deadRuntime(t)
		f := writeFixture(t, defaultAcmeJSON)
		out, _, code := runCLI(t, "service", "status", "--config", f.main, "--json")
		if code != 0 {
			t.Fatalf("exit = %d\n%s", code, out)
		}
		data := decodeOneDocument(t, out)["data"].(map[string]any)
		for _, absent := range []string{"uptime_seconds", "started_at", "version"} {
			if _, has := data[absent]; has {
				t.Errorf("无 /runtime 时 %s 不应出现：%v", absent, data)
			}
		}
	})
}

func TestServiceVersionFromRuntime(t *testing.T) {
	snap := sampleRuntime(t)
	serveRuntimeFixture(t, snap)
	out, _, code := runCLI(t, "service", "version", "--json")
	if code != 0 {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	data := decodeOneDocument(t, out)["data"].(map[string]any)
	if data["cli_version"] != config.CURRENT_VERSION {
		t.Fatalf("cli_version = %v", data["cli_version"])
	}
	if data["server_version"] != "1.5.0-test" {
		t.Fatalf("server_version = %v", data["server_version"])
	}
}
