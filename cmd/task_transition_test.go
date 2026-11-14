package cmd

import (
	"database/sql"
	"sync"
	"testing"
)

func TestRequireTransitionChecksResult(t *testing.T) {
	db := taskDB(t)
	if _, err := db.Exec(`INSERT INTO agent_task (task_id, status) VALUES ('t1', 'EXECUTING')`); err != nil {
		t.Fatal(err)
	}
	if err := requireTransition(db, "t1", "EXECUTING", "VERIFYING"); err != nil {
		t.Fatal(err)
	}
	if err := requireTransition(db, "t1", "VERIFYING", "COMPLETED"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE agent_task SET status = 'WAITING_HUMAN' WHERE task_id = 't1'`); err != nil {
		t.Fatal(err)
	}
	if err := requireTransition(db, "t1", "VERIFYING", "COMPLETED"); err == nil {
		t.Fatal("WAITING_HUMAN was marked completed")
	}
	if err := requireTransition(db, "missing", "EXECUTING", "VERIFYING"); err == nil {
		t.Fatal("missing task was accepted")
	}
}

func TestRequireTransitionLosesTheRace(t *testing.T) {
	db := taskDB(t)
	if _, err := db.Exec(`INSERT INTO agent_task (task_id, status) VALUES ('t1', 'EXECUTING')`); err != nil {
		t.Fatal(err)
	}
	path := taskDBPath(db)
	var wg sync.WaitGroup
	wins := 0
	var mu sync.Mutex
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := sql.Open("sqlite", path)
			if err != nil {
				t.Error(err)
				return
			}
			defer conn.Close()
			_, _ = conn.Exec(`PRAGMA busy_timeout = 5000`)
			if err := requireTransition(conn, "t1", "EXECUTING", "VERIFYING"); err == nil {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if wins != 1 {
		t.Fatalf("transitions = %d, want 1", wins)
	}
}

func taskDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", t.TempDir()+"/task.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TABLE agent_task (task_id TEXT PRIMARY KEY, status TEXT, updated_at TEXT)`); err != nil {
		t.Fatal(err)
	}
	_, _ = db.Exec(`PRAGMA busy_timeout = 5000`)
	return db
}

func taskDBPath(db *sql.DB) string {
	var name string
	_ = db.QueryRow(`PRAGMA database_list`).Scan(new(int), new(string), &name)
	return name
}
