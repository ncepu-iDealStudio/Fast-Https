package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Store 是 Agent 自己的 SQLite 文件，不使用 Server 的状态目录。
type Store struct {
	db *sql.DB
}

func OpenStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", filepath.Join(dir, "agent.db"))
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	_, _ = db.Exec(`ALTER TABLE agent_event ADD COLUMN snapshot_started_at TEXT`)
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS agent_event (
  event_id TEXT PRIMARY KEY,
  monitor_type TEXT,
  severity TEXT,
  summary TEXT,
  received_at TEXT,
  source TEXT,
  snapshot_started_at TEXT
);
CREATE TABLE IF NOT EXISTS agent_task (
  task_id TEXT PRIMARY KEY,
  event_id TEXT NOT NULL,
  status TEXT NOT NULL,
  attempts INTEGER NOT NULL DEFAULT 0,
  context_json TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS agent_diagnosis (
  task_id TEXT PRIMARY KEY,
  classification TEXT,
  subject TEXT,
  cause TEXT,
  impact TEXT,
  evidence_json TEXT,
  needs_human INTEGER
);
CREATE TABLE IF NOT EXISTS agent_plan (
  plan_id TEXT PRIMARY KEY,
  task_id TEXT,
  event_id TEXT,
  risk TEXT,
  plan_hash TEXT,
  steps_json TEXT,
  needs_approval INTEGER
);
CREATE TABLE IF NOT EXISTS agent_approval (
  plan_id TEXT PRIMARY KEY,
  status TEXT,
  reason TEXT,
  created_at TEXT
);
CREATE TABLE IF NOT EXISTS agent_audit (
  id INTEGER PRIMARY KEY,
  at TEXT,
  task_id TEXT,
  kind TEXT,
  payload_json TEXT
);
CREATE TABLE IF NOT EXISTS agent_offset (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  monitor_offset INTEGER NOT NULL
);
INSERT OR IGNORE INTO agent_offset (id, monitor_offset) VALUES (1, 0);
`)
	return err
}

func (s *Store) Offset() (int64, error) {
	var n int64
	err := s.db.QueryRow(`SELECT monitor_offset FROM agent_offset WHERE id = 1`).Scan(&n)
	return n, err
}

func (s *Store) SetOffset(n int64) error {
	_, err := s.db.Exec(`UPDATE agent_offset SET monitor_offset = ? WHERE id = 1`, n)
	return err
}

func (s *Store) TaskCount() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM agent_task`).Scan(&n)
	return n, err
}

func (s *Store) AuditCount(kind string) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM agent_audit WHERE kind = ?`, kind).Scan(&n)
	return n, err
}

// InsertEvent 在事件首次出现时写入事件和一条 PENDING 任务。已存在则返回 false。
func (s *Store) InsertEvent(ev lineEvent, id string, now time.Time) (bool, error) {
	at := now.UTC().Format(time.RFC3339)
	started := ""
	if ev.Snapshot != nil {
		started = ev.Snapshot.StartedAt
	}
	res, err := s.db.Exec(`
INSERT OR IGNORE INTO agent_event (event_id, monitor_type, severity, summary, received_at, source, snapshot_started_at)
VALUES (?, ?, ?, ?, ?, 'fast_https_monitor', ?)`,
		id, ev.Type, ev.Severity, ev.Message, at, started)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if n == 0 {
		return false, nil
	}
	_, err = s.db.Exec(`
INSERT INTO agent_task (task_id, event_id, status, attempts, created_at, updated_at)
VALUES (?, ?, 'PENDING', 0, ?, ?)`,
		"t_"+id, id, at, at)
	return err == nil, err
}

func (s *Store) Audit(kind, payload string, now time.Time) error {
	_, err := s.db.Exec(`
INSERT INTO agent_audit (at, task_id, kind, payload_json) VALUES (?, '', ?, ?)`,
		now.UTC().Format(time.RFC3339), kind, payload)
	if err != nil {
		return fmt.Errorf("audit: %w", err)
	}
	return nil
}

func (s *Store) tasksWithoutContext() ([]string, error) {
	rows, err := s.db.Query(`
SELECT task_id FROM agent_task
WHERE status IN ('PENDING', 'COLLECTING') AND (context_json IS NULL OR context_json = '')
ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) saveContext(taskID, raw string, now time.Time) error {
	_, err := s.db.Exec(`
UPDATE agent_task SET context_json = ?, status = 'PENDING', updated_at = ? WHERE task_id = ?`,
		raw, now.UTC().Format(time.RFC3339), taskID)
	return err
}

func (s *Store) ContextJSON(taskID string) (string, error) {
	var raw sql.NullString
	err := s.db.QueryRow(`SELECT context_json FROM agent_task WHERE task_id = ?`, taskID).Scan(&raw)
	if err != nil {
		return "", err
	}
	return raw.String, nil
}
