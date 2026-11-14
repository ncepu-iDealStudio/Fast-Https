package main

import (
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestInsertEventAndTaskTogether(t *testing.T) {
	dir := t.TempDir()
	st := mustStore(t, dir)
	defer st.Close()
	ev := lineEvent{Type: "5xx_ratio", Severity: "warning", Message: "up"}
	added, err := st.InsertEvent(ev, "abc", time.Now())
	if err != nil || !added {
		t.Fatalf("added=%v err=%v", added, err)
	}
	added, err = st.InsertEvent(ev, "abc", time.Now())
	if err != nil || added {
		t.Fatalf("duplicate added=%v err=%v", added, err)
	}
	var tasks int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM agent_task WHERE event_id = 'abc'`).Scan(&tasks); err != nil {
		t.Fatal(err)
	}
	if tasks != 1 {
		t.Fatalf("tasks = %d", tasks)
	}
}

func TestTaskInsertFailureRollsBackEvent(t *testing.T) {
	prev := insertTaskRow
	insertTaskRow = func(tx *sql.Tx, taskID, eventID, at string) error {
		return errors.New("task insert failed")
	}
	t.Cleanup(func() { insertTaskRow = prev })
	dir := t.TempDir()
	st := mustStore(t, dir)
	defer st.Close()
	_, err := st.InsertEvent(lineEvent{Type: "5xx_ratio", Message: "up"}, "def", time.Now())
	if err == nil {
		t.Fatal("expected task insert error")
	}
	var events int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM agent_event WHERE event_id = 'def'`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 0 {
		t.Fatalf("event committed without a task: %d", events)
	}
}

func TestOrphanEventIsRepaired(t *testing.T) {
	dir := t.TempDir()
	st := mustStore(t, dir)
	defer st.Close()
	if _, err := st.db.Exec(`INSERT INTO agent_event (event_id, monitor_type, summary, received_at, source) VALUES ('old', '5xx_ratio', 'up', '2026-11-14T00:00:00Z', 'test')`); err != nil {
		t.Fatal(err)
	}
	added, err := st.InsertEvent(lineEvent{Type: "5xx_ratio", Message: "up"}, "old", time.Now())
	if err != nil || !added {
		t.Fatalf("added=%v err=%v", added, err)
	}
	var tasks int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM agent_task WHERE event_id = 'old'`).Scan(&tasks); err != nil {
		t.Fatal(err)
	}
	if tasks != 1 {
		t.Fatalf("tasks = %d", tasks)
	}
}

func TestInsertEventRetriesTransientFailure(t *testing.T) {
	prev := insertTaskRow
	var calls int
	insertTaskRow = func(tx *sql.Tx, taskID, eventID, at string) error {
		calls++
		if calls == 1 {
			return errRetryTask
		}
		return prev(tx, taskID, eventID, at)
	}
	t.Cleanup(func() { insertTaskRow = prev })
	dir := t.TempDir()
	st := mustStore(t, dir)
	defer st.Close()
	added, err := st.InsertEvent(lineEvent{Type: "5xx_ratio", Message: "up"}, "retry", time.Now())
	if err != nil || !added {
		t.Fatalf("added=%v err=%v", added, err)
	}
	if calls != 2 {
		t.Fatalf("attempts = %d", calls)
	}
}

func TestConcurrentInsertCreatesOneTask(t *testing.T) {
	dir := t.TempDir()
	st := mustStore(t, dir)
	defer st.Close()
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := st.InsertEvent(lineEvent{Type: "5xx_ratio", Message: "up"}, "same", time.Now())
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var tasks int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM agent_task WHERE event_id = 'same'`).Scan(&tasks); err != nil {
		t.Fatal(err)
	}
	if tasks != 1 {
		t.Fatalf("tasks = %d", tasks)
	}
}

func TestWaitingApprovalWithoutPlanBecomesHuman(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", dir+"/agent.db")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
CREATE TABLE agent_task (task_id TEXT PRIMARY KEY, event_id TEXT, status TEXT, attempts INTEGER, context_json TEXT, created_at TEXT, updated_at TEXT);
CREATE TABLE agent_plan (plan_id TEXT PRIMARY KEY, task_id TEXT, event_id TEXT, risk TEXT, plan_hash TEXT, steps_json TEXT, needs_approval INTEGER);
CREATE TABLE agent_approval (plan_id TEXT PRIMARY KEY, status TEXT, reason TEXT, created_at TEXT);
INSERT INTO agent_task VALUES ('t1', 'e1', 'WAITING_APPROVAL', 0, '{}', 't', 't');
INSERT INTO agent_plan VALUES ('p1', 't1', 'e1', 'low', 'h', '[]', 1);
INSERT INTO agent_approval VALUES ('p1', 'pending', 'look at the certificate', 't');
`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	st := mustStore(t, dir)
	defer st.Close()
	status, err := st.taskStatus("t1")
	if err != nil || status != "WAITING_HUMAN" {
		t.Fatalf("status = %s err=%v", status, err)
	}
	var approval string
	if err := st.db.QueryRow(`SELECT status FROM agent_approval WHERE plan_id = 'p1'`).Scan(&approval); err != nil {
		t.Fatal(err)
	}
	if approval != "expired" {
		t.Fatalf("approval = %s", approval)
	}
	status, err = st.taskStatus("t1")
	if err != nil || status != "WAITING_HUMAN" {
		t.Fatalf("second open status = %s err=%v", status, err)
	}
}
