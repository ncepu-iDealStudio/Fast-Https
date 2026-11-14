package agentview

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestHeartbeatStatus(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "agent.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
CREATE TABLE agent_task (task_id TEXT PRIMARY KEY, updated_at TEXT);
CREATE TABLE agent_heartbeat (
  id INTEGER PRIMARY KEY,
  heartbeat_at TEXT
);`); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 14, 3, 0, 0, 0, time.UTC)
	if _, err := db.Exec(`INSERT INTO agent_heartbeat VALUES (1, ?)`, now.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	st, err := readStatusAt(dir, now)
	if err != nil || st.Status != "online" {
		t.Fatalf("status = %+v err=%v", st, err)
	}
	st, err = readStatusAt(dir, now.Add(time.Minute))
	if err != nil || st.Status != "offline" {
		t.Fatalf("stale status = %+v err=%v", st, err)
	}
}
