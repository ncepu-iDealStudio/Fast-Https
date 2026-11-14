package main

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// errRetryTask 让 InsertEvent 再试一次。测试用它模拟 SQLite 暂时失败。
var errRetryTask = errors.New("retry task insert")

var insertTaskRow = func(tx *sql.Tx, taskID, eventID, at string) error {
	_, err := tx.Exec(`
INSERT INTO agent_task (task_id, event_id, status, attempts, created_at, updated_at)
VALUES (?, ?, 'PENDING', 0, ?, ?)`, taskID, eventID, at, at)
	return err
}

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
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if _, err := db.Exec(`PRAGMA busy_timeout = 5000`); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
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
	if err != nil {
		return err
	}
	for _, col := range []struct{ table, name, decl string }{
		{"agent_event", "snapshot_started_at", "TEXT"},
		{"agent_plan", "verify_text", "TEXT"},
		{"agent_plan", "on_failure", "TEXT"},
		{"agent_approval", "decided_at", "TEXT"},
		{"agent_approval", "actor", "TEXT"},
	} {
		if err := ensureColumn(s.db, col.table, col.name, col.decl); err != nil {
			return err
		}
	}
	return s.migrateWaitingHuman()
}

func ensureColumn(db *sql.DB, table, name, decl string) error {
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var col, typ string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &col, &typ, &notNull, &dflt, &pk); err != nil {
			return err
		}
		if col == name {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + name + ` ` + decl)
	return err
}

func (s *Store) migrateWaitingHuman() error {
	_, err := s.db.Exec(`
UPDATE agent_task SET status = 'WAITING_HUMAN'
WHERE status = 'WAITING_APPROVAL'
  AND task_id IN (
    SELECT task_id FROM agent_plan
    WHERE IFNULL(steps_json, '') IN ('', '[]', 'null')
  )`)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
UPDATE agent_approval SET status = 'expired'
WHERE status = 'pending'
  AND plan_id IN (
    SELECT plan_id FROM agent_plan
    WHERE IFNULL(steps_json, '') IN ('', '[]', 'null')
  )`)
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

// InsertEvent 在同一个事务里写入事件和任务。事件已在且任务也在时返回 false。
// 只有事件、没有任务的历史行会补建任务。任务插入失败会回滚事件。
func (s *Store) InsertEvent(ev lineEvent, id string, now time.Time) (bool, error) {
	var added bool
	var err error
	for attempt := 0; attempt < 8; attempt++ {
		added, err = s.insertEventOnce(ev, id, now)
		if err == nil || (!errors.Is(err, errRetryTask) && !sqliteBusy(err)) {
			return added, err
		}
	}
	return added, err
}

func (s *Store) insertEventOnce(ev lineEvent, id string, now time.Time) (bool, error) {
	at := now.UTC().Format(time.RFC3339)
	started := ""
	if ev.Snapshot != nil {
		started = ev.Snapshot.StartedAt
	}
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.Exec(`
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
		var count int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM agent_task WHERE event_id = ?`, id).Scan(&count); err != nil {
			return false, err
		}
		if count > 0 {
			return false, tx.Commit()
		}
	}
	if err := insertTaskRow(tx, "t_"+id, id, at); err != nil {
		if sqliteConstraint(err) {
			return false, nil
		}
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func sqliteBusy(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "locked") || strings.Contains(msg, "busy")
}

func sqliteConstraint(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "constraint") || strings.Contains(msg, "unique")
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
