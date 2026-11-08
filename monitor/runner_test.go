package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type harness struct {
	t      *testing.T
	dir    string
	clock  *FakeClock
	runner *Runner
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.Dir = dir
	cfg.PidFile = filepath.Join(dir, "fast-https.pid")
	cfg.Timeout = Duration(500 * time.Millisecond)
	clk := NewFakeClock(time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC))
	return &harness{
		t:      t,
		dir:    dir,
		clock:  clk,
		runner: NewRunner(cfg, clk, log.New(io.Discard, "", 0)),
	}
}

// pointAt 换掉采集地址：单元测试不碰本机真实的 9088。
func (h *harness) pointAt(url string) {
	h.runner.col = &Collector{URL: url, Timeout: h.runner.cfg.Timeout.D()}
}

func (h *harness) observe() (Observation, []Event) {
	h.t.Helper()
	return h.runner.RunOnce(context.Background())
}

func (h *harness) readEvents() []Event {
	h.t.Helper()
	raw, err := os.ReadFile(filepath.Join(h.dir, EventsFile))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		h.t.Fatalf("read events: %v", err)
	}
	var out []Event
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var ev Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			h.t.Fatalf("event line is not JSON: %q (%v)", line, err)
		}
		out = append(out, ev)
	}
	return out
}

func (h *harness) readSnapshots() []string {
	h.t.Helper()
	raw, err := os.ReadFile(filepath.Join(h.dir, SnapshotsFile))
	if err != nil {
		h.t.Fatalf("read snapshots: %v", err)
	}
	return strings.Split(strings.TrimSpace(string(raw)), "\n")
}

func (h *harness) readStatus() MonitorStatus {
	h.t.Helper()
	raw, err := os.ReadFile(filepath.Join(h.dir, StatusFile))
	if err != nil {
		h.t.Fatalf("read status: %v", err)
	}
	var st MonitorStatus
	if err := json.Unmarshal(raw, &st); err != nil {
		h.t.Fatalf("status is not JSON: %v", err)
	}
	return st
}

func countEvents(events []Event, kind string) int {
	n := 0
	for _, ev := range events {
		if ev.Type == kind {
			n++
		}
	}
	return n
}

func TestCollectWritesSnapshotAndStatus(t *testing.T) {
	h := newHarness(t)
	h.pointAt(snapshotServer(t, func() string {
		return snapshotBody(4242, "2026-11-08T09:00:00.000+08:00", 7)
	}).URL)

	obs, events := h.observe()
	if obs.Kind != CollectOK {
		t.Fatalf("kind = %s (err=%v)", obs.Kind, obs.Err)
	}
	if len(events) != 0 {
		t.Fatalf("first collection must not emit events, got %v", events)
	}
	lines := h.readSnapshots()
	if len(lines) != 1 {
		t.Fatalf("snapshots = %d lines, want 1", len(lines))
	}
	var snap Snapshot
	if err := json.Unmarshal([]byte(lines[0]), &snap); err != nil {
		t.Fatalf("snapshot line is not JSON: %v", err)
	}
	if snap.Server.PID != 4242 || snap.SchemaVersion != SchemaVersion {
		t.Fatalf("snapshot content = %+v", snap.Server)
	}

	st := h.readStatus()
	if st.Status != StatusRunning || !st.Healthy {
		t.Fatalf("status = %+v, want running/healthy", st)
	}
	if st.LastCollectAt != "2026-11-08T10:00:00.000Z" {
		t.Errorf("last_collect_at = %q", st.LastCollectAt)
	}
	if st.SchemaVersion != StatusSchemaVersion {
		t.Errorf("schema_version = %q, want %q", st.SchemaVersion, StatusSchemaVersion)
	}
}

func TestUnreachableAfterThreeFailures(t *testing.T) {
	h := newHarness(t)
	h.pointAt(deadURL(t))

	for i := 0; i < failThreshold; i++ {
		if obs, _ := h.observe(); obs.Kind != FailUnreachable {
			t.Fatalf("run %d: kind = %s, want unreachable", i+1, obs.Kind)
		}
	}
	events := h.readEvents()
	if n := countEvents(events, EventUnreachable); n != 1 {
		t.Fatalf("unreachable events = %d, want 1 (events=%v)", n, events)
	}
	// 继续失败不重复写
	h.observe()
	if n := countEvents(h.readEvents(), EventUnreachable); n != 1 {
		t.Fatalf("unreachable events after further failures = %d, want 1", n)
	}

	st := h.readStatus()
	if st.Status != StatusUnhealthy || st.Healthy {
		t.Fatalf("status = %+v, want unhealthy", st)
	}
	if !strings.Contains(st.LastError, FailUnreachable.String()) {
		t.Errorf("last_error = %q, want it to name the failure kind", st.LastError)
	}
}

func TestRecoveredAfterUnreachable(t *testing.T) {
	h := newHarness(t)
	h.pointAt(deadURL(t))
	for i := 0; i < failThreshold; i++ {
		h.observe()
	}
	h.pointAt(snapshotServer(t, func() string { return snapshotBody(1, "2026-11-08T09:00:00.000+08:00", 3) }).URL)

	if obs, _ := h.observe(); obs.Kind != CollectOK {
		t.Fatalf("kind = %s, want ok", obs.Kind)
	}
	if n := countEvents(h.readEvents(), EventRecovered); n != 1 {
		t.Fatalf("recovered events = %d, want 1", n)
	}
	st := h.readStatus()
	if st.Status != StatusRunning || !st.Healthy || st.LastError != "" {
		t.Fatalf("status = %+v, want running/healthy without last_error", st)
	}
}

func TestRestartDetected(t *testing.T) {
	pid := 100
	startedAt := "2026-11-08T09:00:00.000+08:00"
	srv := snapshotServer(t, func() string { return snapshotBody(pid, startedAt, 10) })

	h := newHarness(t)
	h.pointAt(srv.URL)
	if _, events := h.observe(); len(events) != 0 {
		t.Fatalf("first observation must not emit events, got %v", events)
	}

	pid, startedAt = 200, "2026-11-08T11:00:00.000+08:00"
	if _, events := h.observe(); countEvents(events, EventRestarted) != 1 {
		t.Fatalf("events = %v, want one restarted event", events)
	}
	ev := h.readEvents()[0]
	if ev.Type != EventRestarted || ev.Severity != "warning" {
		t.Fatalf("event = %+v", ev)
	}
	if !strings.Contains(ev.Message, "100") || !strings.Contains(ev.Message, "200") {
		t.Errorf("message = %q, want both pids", ev.Message)
	}
	if ev.Summary == nil || ev.Summary.PID != 200 {
		t.Errorf("summary = %+v, want the new pid", ev.Summary)
	}

	// 同一进程不再重复报重启
	if _, events := h.observe(); countEvents(events, EventRestarted) != 0 {
		t.Fatalf("restarted repeated for the same pid: %v", events)
	}
}

func TestPidFileDisambiguation(t *testing.T) {
	t.Run("never started", func(t *testing.T) {
		h := newHarness(t)
		h.pointAt(deadURL(t))
		h.observe()
		if got := h.readStatus().LastError; !strings.Contains(got, "not started") {
			t.Fatalf("last_error = %q, want it to say the server never started", got)
		}
	})

	t.Run("crashed", func(t *testing.T) {
		h := newHarness(t)
		h.pointAt(deadURL(t))
		const deadPID = 999999
		if processRunning(deadPID) {
			t.Skipf("pid %d is in use on this machine", deadPID)
		}
		writePidFile(t, h.runner.cfg.PidFile, deadPID)
		h.observe()
		if got := h.readStatus().LastError; !strings.Contains(got, "crashed") {
			t.Fatalf("last_error = %q, want it to say the server crashed", got)
		}
	})

	t.Run("starting", func(t *testing.T) {
		h := newHarness(t)
		h.pointAt(deadURL(t))
		writePidFile(t, h.runner.cfg.PidFile, os.Getpid())
		h.observe()
		if got := h.readStatus().LastError; !strings.Contains(got, "starting") {
			t.Fatalf("last_error = %q, want it to say the server is starting", got)
		}
	})
}

func writePidFile(t *testing.T, path string, pid int) {
	t.Helper()
	raw, err := json.Marshal(map[string]int{"pid": pid})
	if err != nil {
		t.Fatalf("marshal pid: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write pid file: %v", err)
	}
}
