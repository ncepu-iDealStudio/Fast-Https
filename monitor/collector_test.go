package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// snapshotBody 造一份最小快照。字段名与 Server 的 /runtime 一致。
func snapshotBody(pid int, startedAt string, total uint64) string {
	return fmt.Sprintf(`{"schema_version":"1","observed_at":"2026-11-08T10:00:00.000+08:00",`+
		`"server":{"state":"running","pid":%d,"version":"1.6.0","started_at":%q},`+
		`"connections":{"active":0,"accepted_total":1,"closed_total":1},`+
		`"requests":{"active":0,"total":%d},"responses_5xx_total":0,`+
		`"request_duration":{"count":0,"buckets":[]}}`, pid, startedAt, total)
}

func TestCollectOK(t *testing.T) {
	srv := snapshotServer(t, func() string { return snapshotBody(1234, "2026-11-08T09:00:00.000+08:00", 42) })
	c := &Collector{URL: srv.URL, Timeout: time.Second}

	obs := c.Collect(context.Background(), time.Now())
	if obs.Kind != CollectOK {
		t.Fatalf("kind = %s, want ok (err=%v)", obs.Kind, obs.Err)
	}
	if got := obs.Snapshot.Server.PID; got != 1234 {
		t.Errorf("pid = %d, want 1234", got)
	}
	if got := obs.Snapshot.Requests.Total; got != 42 {
		t.Errorf("requests total = %d, want 42", got)
	}
	if len(obs.Raw) == 0 {
		t.Fatal("raw snapshot is empty")
	}
	if strings.Contains(string(obs.Raw), "\n") {
		t.Error("raw snapshot must be a single JSONL line")
	}
}

// 四种坏法必须各自归类，且都不 panic。
func TestCollectBadStatusAndJSON(t *testing.T) {
	cases := []struct {
		name    string
		handler http.HandlerFunc
		want    FailKind
	}{
		{
			name:    "http 500",
			handler: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) },
			want:    FailStatus,
		},
		{
			name:    "not json",
			handler: func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "<html>oops</html>") },
			want:    FailJSON,
		},
		{
			name:    "unknown schema",
			handler: func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, `{"schema_version":"2"}`) },
			want:    FailSchema,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()
			c := &Collector{URL: srv.URL, Timeout: time.Second}
			obs := c.Collect(context.Background(), time.Now())
			if obs.Kind != tc.want {
				t.Fatalf("kind = %s, want %s (err=%v)", obs.Kind, tc.want, obs.Err)
			}
			if obs.Err == nil {
				t.Error("failed collection must carry an error")
			}
			if len(obs.Raw) != 0 {
				t.Error("failed collection must not expose a raw snapshot")
			}
		})
	}
}

func TestCollectUnreachableAndTimeout(t *testing.T) {
	t.Run("connection refused", func(t *testing.T) {
		c := &Collector{URL: deadURL(t), Timeout: 500 * time.Millisecond}
		if obs := c.Collect(context.Background(), time.Now()); obs.Kind != FailUnreachable {
			t.Fatalf("kind = %s, want unreachable (err=%v)", obs.Kind, obs.Err)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(300 * time.Millisecond)
			_, _ = io.WriteString(w, snapshotBody(1, "", 0))
		}))
		defer srv.Close()
		c := &Collector{URL: srv.URL, Timeout: 50 * time.Millisecond}
		if obs := c.Collect(context.Background(), time.Now()); obs.Kind != FailUnreachable {
			t.Fatalf("kind = %s, want unreachable", obs.Kind)
		}
	})
}

// 四个失败类别在状态与事件里要能区分开，字符串不能撞。
func TestFailKindStringsAreDistinct(t *testing.T) {
	kinds := []FailKind{FailUnreachable, FailStatus, FailJSON, FailSchema}
	seen := map[string]FailKind{}
	for _, k := range kinds {
		s := k.String()
		if s == "" || s == "ok" || s == "unknown" {
			t.Fatalf("kind %d has no distinct name (%q)", k, s)
		}
		if prev, ok := seen[s]; ok {
			t.Fatalf("kind %d and %d share the name %q", prev, k, s)
		}
		seen[s] = k
	}
}

func snapshotServer(t *testing.T, body func() string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body())
	}))
	t.Cleanup(srv.Close)
	return srv
}

// deadURL 返回一个没人监听的地址：先占端口再关掉，避免占用固定端口。
func deadURL(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	addr := l.Addr().String()
	if err := l.Close(); err != nil {
		t.Fatalf("close reserved port: %v", err)
	}
	return "http://" + addr + "/runtime"
}
