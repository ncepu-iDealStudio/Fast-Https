package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCLIAllowlistRejects(t *testing.T) {
	var got []string
	run := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		got = append([]string{name}, args...)
		if strings.Contains(strings.Join(args, " "), "-c") {
			t.Fatal("allow-listed call must not use a shell")
		}
		return []byte(`{"ok":true}`), nil
	}
	if _, err := runAllowed(context.Background(), run, "fast-https", []string{"sh", "-c", "echo"}); err == nil {
		t.Fatal("shell argv was accepted")
	}
	if len(got) != 0 {
		t.Fatal("rejected command was executed")
	}
	body, err := runAllowed(context.Background(), run, "fast-https", []string{"service", "status", "--json"})
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"ok":true}` {
		t.Fatalf("body = %s", body)
	}
	if strings.Join(got, " ") != "fast-https service status --json" {
		t.Fatalf("argv = %q", got)
	}
}

func TestRuntimeSchema(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"schema_version":"2","requests":{"total":1}}`))
	}))
	defer srv.Close()
	if _, err := fetchRuntime(context.Background(), srv.URL); err == nil {
		t.Fatal("schema 2 was accepted")
	}
	doc := buildContext(context.Background(), CollectOptions{
		RuntimeURL: srv.URL,
		Run: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return []byte(`{"ok":true}`), nil
		},
	})
	if string(doc.Runtime) != `"unavailable"` {
		t.Fatalf("runtime = %s", doc.Runtime)
	}
}

func TestCollectRuntime(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s", r.Method)
		}
		_, _ = w.Write([]byte(`{"schema_version":"1","responses_502_total":4}`))
	}))
	defer srv.Close()
	mon := t.TempDir()
	state := t.TempDir()
	line := `{"time":"2026-11-10T01:00:00.000Z","type":"5xx_ratio","severity":"warning","message":"ratio","event_id":"dddddddddddddddddddddddddddddddd"}`
	writeEvents(t, mon, line+"\n")
	now := time.Date(2026, 11, 10, 2, 0, 0, 0, time.UTC)
	if _, err := IngestOnce(mon, state, now); err != nil {
		t.Fatal(err)
	}
	n, err := CollectPending(state, CollectOptions{
		RuntimeURL: srv.URL,
		Now:        now,
		Run: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return []byte(`{"ok":true,"command":"stub"}`), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("collected = %d", n)
	}
	st, err := OpenStore(state)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	raw, err := st.ContextJSON("t_dddddddddddddddddddddddddddddddd")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Runtime struct {
			Responses502 uint64 `json:"responses_502_total"`
		} `json:"runtime"`
	}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Runtime.Responses502 != 4 {
		t.Fatalf("context = %s", raw)
	}
	again, err := CollectPending(state, CollectOptions{RuntimeURL: srv.URL, Now: now, Run: func(ctx context.Context, name string, args ...string) ([]byte, error) {
		t.Fatal("second collect should not call CLI")
		return nil, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if again != 0 {
		t.Fatalf("second collect = %d", again)
	}
}
