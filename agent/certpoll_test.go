package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCertPollThreshold(t *testing.T) {
	mon := t.TempDir()
	state := t.TempDir()
	now := time.Date(2026, 11, 10, 2, 0, 0, 0, time.UTC)
	body := []byte(`{"ok":true,"data":{"certificates":[
		{"domains":["soon.example"],"file":"E:/secret/soon.pem","not_after":"2026-11-20T00:00:00Z","days_left":9},
		{"domains":["later.example"],"file":"E:/secret/later.pem","not_after":"2027-01-01T00:00:00Z","days_left":40}
	]}}`)
	n, err := PollCerts(mon, state, "fast-https", 30, func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if name != "fast-https" || len(args) != 3 || args[0] != "cert" || args[1] != "list" || args[2] != "--json" {
			t.Fatalf("argv = %s %v", name, args)
		}
		return body, nil
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("added = %d, want 1", n)
	}
	if _, err := CollectPending(state, CollectOptions{Now: now, RuntimeURL: "http://127.0.0.1:1", Run: okRunner}); err != nil {
		t.Fatal(err)
	}
	if _, err := DiagnosePending(state, now, nil); err != nil {
		t.Fatal(err)
	}
	st := mustStore(t, state)
	defer st.Close()
	id := certEventID("soon.example", "2026-11-20T00:00:00Z")
	status, err := st.taskStatus("t_" + id)
	if err != nil {
		t.Fatal(err)
	}
	if status != "WAITING_HUMAN" {
		t.Fatalf("status = %s", status)
	}
	var msg string
	if err := st.db.QueryRow(`SELECT summary FROM agent_event WHERE event_id = ?`, id).Scan(&msg); err != nil {
		t.Fatal(err)
	}
	if msg != "certificate expiring: soon.example days_left=9" {
		t.Fatalf("message = %q", msg)
	}

	again, err := PollCerts(mon, state, "fast-https", 30, func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return body, nil
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if again != 0 {
		t.Fatalf("second poll added %d", again)
	}
}

func TestCertPollSkipsWhenMonitorAlreadyWrote(t *testing.T) {
	mon := t.TempDir()
	state := t.TempDir()
	writeEvents(t, mon, `{"type":"certificate_expiring","message":"certificate expiring: soon.example days_left=9"}`+"\n")
	n, err := PollCerts(mon, state, "fast-https", 30, func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return []byte(`{"ok":true,"data":{"certificates":[{"domains":["soon.example"],"not_after":"2026-11-20T00:00:00Z","days_left":9}]}}`), nil
	}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("added = %d, want 0", n)
	}
}

func TestCertPollFailureDoesNotInvent(t *testing.T) {
	state := t.TempDir()
	n, err := PollCerts(t.TempDir(), state, "fast-https", 30, func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return nil, errors.New("exit 1")
	}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("added = %d", n)
	}
	st := mustStore(t, state)
	defer st.Close()
	count, err := st.TaskCount()
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("tasks = %d", count)
	}
	audits, err := st.AuditCount("cert_list_failed")
	if err != nil || audits != 1 {
		t.Fatalf("audits = %d err=%v", audits, err)
	}
}

func okRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	return []byte(`{"ok":true}`), nil
}
