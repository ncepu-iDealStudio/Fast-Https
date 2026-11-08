package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEventFileRotation(t *testing.T) {
	dir := t.TempDir()
	s := &Sink{Dir: dir, EventMaxBytes: 300, EventKeep: 2}

	for i := 0; i < 40; i++ {
		ev := Event{
			Time:     time.Unix(int64(i), 0).UTC().Format(TimeFormat),
			Type:     "test",
			Severity: "info",
			Message:  strings.Repeat("x", 80),
		}
		if err := s.AppendEvent(ev); err != nil {
			t.Fatalf("append event %d: %v", i, err)
		}
	}

	main := filepath.Join(dir, EventsFile)
	info, err := os.Stat(main)
	if err != nil {
		t.Fatalf("stat %s: %v", EventsFile, err)
	}
	if info.Size() > 300 {
		t.Errorf("%s is %d bytes, want at most 300", EventsFile, info.Size())
	}
	for _, name := range []string{EventsFile + ".1", EventsFile + ".2"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("rotated file %s missing: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, EventsFile+".3")); err == nil {
		t.Errorf("%s.3 exists, want it removed (event_keep=2)", EventsFile)
	}
}

func TestSnapshotFileRotatesToo(t *testing.T) {
	dir := t.TempDir()
	s := &Sink{Dir: dir, EventMaxBytes: 200, EventKeep: 1}
	for i := 0; i < 5; i++ {
		raw, err := json.Marshal(map[string]any{"schema_version": "1", "n": i, "pad": strings.Repeat("y", 100)})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if err := s.AppendSnapshot(raw); err != nil {
			t.Fatalf("append snapshot %d: %v", i, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, SnapshotsFile+".1")); err != nil {
		t.Errorf("snapshots rotation did not happen: %v", err)
	}
}

func TestWriteStatusRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := &Sink{Dir: dir}
	want := MonitorStatus{
		Status:        StatusUnhealthy,
		Version:       "test",
		PID:           77,
		Healthy:       false,
		LastCollectAt: "2026-11-08T10:00:00.000Z",
		LastEventAt:   "2026-11-08T10:00:05.000Z",
		LastError:     "unreachable: dial tcp: refused",
	}
	if err := s.WriteStatus(want); err != nil {
		t.Fatalf("write status: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, StatusFile+".tmp")); err == nil {
		t.Error("temporary status file left behind")
	}
	raw, err := os.ReadFile(filepath.Join(dir, StatusFile))
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	var got MonitorStatus
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("status is not JSON: %v", err)
	}
	if got.SchemaVersion != StatusSchemaVersion {
		t.Errorf("schema_version = %q, want %q", got.SchemaVersion, StatusSchemaVersion)
	}
	if got.Status != want.Status || got.Healthy != want.Healthy || got.LastError != want.LastError {
		t.Errorf("status = %+v, want %+v", got, want)
	}
}

// 事件只写白名单里的五项，内部拓扑与凭据不该出现在文件里。
func TestEventFieldsWhitelist(t *testing.T) {
	dir := t.TempDir()
	s := &Sink{Dir: dir}
	ev := Event{
		Time:     "2026-11-08T10:00:00.000Z",
		Type:     EventUnreachable,
		Severity: "error",
		Message:  "3 consecutive collection failures",
		Summary:  &EventSummary{State: "running", PID: 9, StartedAt: "2026-11-08T09:00:00.000Z"},
	}
	if err := s.AppendEvent(ev); err != nil {
		t.Fatalf("append event: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, EventsFile))
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	for _, banned := range []string{"blacklist", "ssl_certificate", "authorization", "cookie", "upstream"} {
		if strings.Contains(strings.ToLower(string(raw)), banned) {
			t.Errorf("event file contains forbidden word %q: %s", banned, raw)
		}
	}
}
