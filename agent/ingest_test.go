package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTailIdempotent(t *testing.T) {
	mon := t.TempDir()
	state := t.TempDir()
	line := `{"time":"2026-11-10T01:00:00.000Z","type":"unreachable","severity":"error","message":"down","event_id":"0123456789abcdef0123456789abcdef","snapshot":{"pid":9,"started_at":"2026-11-10T00:00:00.000Z"}}`
	writeEvents(t, mon, line+"\n"+line+"\n")
	now := time.Date(2026, 11, 10, 2, 0, 0, 0, time.UTC)
	res, err := IngestOnce(mon, state, now)
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 1 {
		t.Fatalf("added = %d, want 1", res.Added)
	}
	st, err := OpenStore(state)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	n, err := st.TaskCount()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("tasks = %d, want 1", n)
	}
}

func TestResumeOffset(t *testing.T) {
	mon := t.TempDir()
	state := t.TempDir()
	now := time.Date(2026, 11, 10, 2, 0, 0, 0, time.UTC)
	first := `{"time":"2026-11-10T01:00:00.000Z","type":"unreachable","severity":"error","message":"one","event_id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`
	writeEvents(t, mon, first+"\n")
	if _, err := IngestOnce(mon, state, now); err != nil {
		t.Fatal(err)
	}
	second := `{"time":"2026-11-10T01:05:00.000Z","type":"recovered","severity":"info","message":"two","event_id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}`
	appendEvents(t, mon, second+"\n")
	res, err := IngestOnce(mon, state, now)
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 1 {
		t.Fatalf("added = %d, want 1", res.Added)
	}
	st, err := OpenStore(state)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	n, err := st.TaskCount()
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("tasks = %d, want 2", n)
	}
}

func TestSkipBadLine(t *testing.T) {
	mon := t.TempDir()
	state := t.TempDir()
	good := `{"time":"2026-11-10T01:00:00.000Z","type":"5xx_ratio","severity":"warning","message":"ratio","event_id":"cccccccccccccccccccccccccccccccc"}`
	writeEvents(t, mon, "not-json\n"+good+"\n")
	res, err := IngestOnce(mon, state, time.Date(2026, 11, 10, 2, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 1 || res.Skipped != 1 {
		t.Fatalf("result = %+v, want added 1 skipped 1", res)
	}
	st, err := OpenStore(state)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	n, err := st.AuditCount("bad_line")
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("bad_line audits = %d", n)
	}
}

func TestLegacyLineUsesMonitorID(t *testing.T) {
	mon := t.TempDir()
	state := t.TempDir()
	line := `{"time":"2026-11-10T01:00:00.000Z","type":"unreachable","severity":"error","message":"down","snapshot":{"pid":9,"started_at":"2026-11-10T00:00:00.000Z"}}`
	writeEvents(t, mon, line+"\n")
	if _, err := IngestOnce(mon, state, time.Now()); err != nil {
		t.Fatal(err)
	}
	st, err := OpenStore(state)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var id string
	if err := st.db.QueryRow(`SELECT event_id FROM agent_event`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	want := eventID(lineEvent{
		Type: "unreachable", Severity: "error", Message: "down",
		Snapshot: &lineSnapshot{PID: 9, StartedAt: "2026-11-10T00:00:00.000Z"},
	})
	if id != want {
		t.Fatalf("event_id = %s, want %s", id, want)
	}
}

func writeEvents(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, eventsFile), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func appendEvents(t *testing.T, dir, body string) {
	t.Helper()
	f, err := os.OpenFile(filepath.Join(dir, eventsFile), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(body); err != nil {
		t.Fatal(err)
	}
}
