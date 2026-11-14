package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestSchemaMigrationStopsAndRetries(t *testing.T) {
	dir := t.TempDir()
	migrationHook = 2
	t.Cleanup(func() { migrationHook = 0 })
	if _, err := OpenStore(dir); err == nil {
		t.Fatal("migration 2 should fail")
	}
	db, err := sql.Open("sqlite", filepath.Join(dir, "agent.db"))
	if err != nil {
		t.Fatal(err)
	}
	var version int
	if err := db.QueryRow(`SELECT version FROM agent_schema WHERE id = 1`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 1 {
		t.Fatalf("version = %d, want 1", version)
	}
	var tasks int
	if err := db.QueryRow(`SELECT COUNT(*) FROM agent_task`).Scan(&tasks); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	migrationHook = 0
	st, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	version, err = st.SchemaVersion()
	if err != nil || version != 3 {
		t.Fatalf("version = %d err=%v", version, err)
	}
	if err := st.Beat(time.Now(), 1, "2026-11-14T00:00:00Z", "", ""); err != nil {
		t.Fatal(err)
	}
}

func TestSchemaKeepsExistingRows(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.db")
	if err := os.WriteFile(path, []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	st := mustStore(t, dir)
	if _, err := st.InsertEvent(lineEvent{Type: "5xx_ratio", Message: "keep", Time: "2026-11-14T00:00:00.000Z"}, "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", time.Now()); err != nil {
		t.Fatal(err)
	}
	_ = st.Close()
	st = mustStore(t, dir)
	defer st.Close()
	var n int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM agent_task`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("tasks = %d", n)
	}
	version, err := st.SchemaVersion()
	if err != nil || version != 3 {
		t.Fatalf("version = %d err=%v", version, err)
	}
}
