package rtclient

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGetSnapshot(t *testing.T) {
	body := map[string]any{
		"schema_version": "1",
		"observed_at":    "2026-11-08T12:00:00.000+08:00",
		"server": map[string]any{
			"state":             "running",
			"pid":               7,
			"version":           "1.5.0",
			"started_at":        "2026-11-08T11:00:00.000+08:00",
			"config_generation": 3,
		},
		"connections":      map[string]any{"active": 1, "accepted_total": 2},
		"requests":         map[string]any{"active": 0, "total": 9},
		"extra_unknown":    "ignored",
		"request_duration": map[string]any{"unit": "ms", "count": 9, "buckets": []any{}},
		"upstream":         map[string]any{"dial_fail_total": 1, "idle_conns": 2},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/runtime" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(body)
	}))
	defer srv.Close()

	cli := Client{URL: srv.URL + "/runtime", HTTP: srv.Client()}
	snap, err := cli.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snap.Server.State != "running" || snap.Server.PID != 7 || snap.Server.ConfigGeneration != 3 {
		t.Fatalf("server = %+v", snap.Server)
	}
	if snap.Requests.Total != 9 || snap.Upstream == nil || snap.Upstream.DialFailTotal != 1 {
		t.Fatalf("snap = %+v", snap)
	}
	if snap.RequestDuration == nil || snap.RequestDuration.Count != 9 {
		t.Fatalf("duration = %+v", snap.RequestDuration)
	}
}

func TestGetSchemaMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"schema_version": "99"})
	}))
	defer srv.Close()

	_, err := Client{URL: srv.URL, HTTP: srv.Client()}.Get(context.Background())
	if !errors.Is(err, ErrSchema) {
		t.Fatalf("err = %v", err)
	}
}

func TestGetInvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("{"))
	}))
	defer srv.Close()

	_, err := Client{URL: srv.URL, HTTP: srv.Client()}.Get(context.Background())
	if !errors.Is(err, ErrSchema) {
		t.Fatalf("err = %v", err)
	}
}

func TestGetUnreachable(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	url := "http://" + ln.Addr().String() + "/runtime"
	_ = ln.Close()

	_, err = Client{URL: url, HTTP: &http.Client{Timeout: 200 * time.Millisecond}}.Get(context.Background())
	if !errors.Is(err, ErrUnreachable) {
		t.Fatalf("err = %v", err)
	}
}

func TestGetHTTPStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no", http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := Client{URL: srv.URL, HTTP: srv.Client()}.Get(context.Background())
	if !errors.Is(err, ErrUnreachable) {
		t.Fatalf("err = %v", err)
	}
}

func TestDefaultURLEnv(t *testing.T) {
	t.Setenv(EnvURL, "http://127.0.0.1:9/runtime")
	if got := DefaultURL(); got != "http://127.0.0.1:9/runtime" {
		t.Fatalf("DefaultURL = %q", got)
	}
	cli := Default()
	if cli.URL != "http://127.0.0.1:9/runtime" {
		t.Fatalf("Default().URL = %q", cli.URL)
	}
}

func TestUptimeSeconds(t *testing.T) {
	now := time.Date(2026, 11, 8, 12, 0, 10, 0, time.FixedZone("CST", 8*3600))
	s := ServerInfo{StartedAt: "2026-11-08T12:00:00.000+08:00"}
	got := s.UptimeSeconds(now)
	if got == nil || *got != 10 {
		t.Fatalf("uptime = %v", got)
	}
	if (ServerInfo{}).UptimeSeconds(now) != nil {
		t.Fatal("empty started_at should omit uptime")
	}
}
