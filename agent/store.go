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
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS agent_schema (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  version INTEGER NOT NULL
)`); err != nil {
		return err
	}
	var version int
	err := s.db.QueryRow(`SELECT version FROM agent_schema WHERE id = 1`).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		version = 0
	} else if err != nil {
		return err
	}
	steps := []func() error{s.migrateV1, s.migrateV2, s.migrateV3}
	for i, step := range steps {
		next := i + 1
		if version >= next {
			continue
		}
		if err := step(); err != nil {
			return fmt.Errorf("schema %d: %w", next, err)
		}
		if _, err := s.db.Exec(`INSERT OR REPLACE INTO agent_schema (id, version) VALUES (1, ?)`, next); err != nil {
			return err
		}
		version = next
	}
	return nil
}

func (s *Store) migrateV1() error {
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

func (s *Store) migrateV2() error {
	if migrationHook == 2 {
		return errors.New("migration 2 failed")
	}
	for _, col := range []struct{ table, name, decl string }{
		{"agent_event", "snapshot_started_at", "TEXT"},
		{"agent_event", "event_time", "TEXT"},
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

func (s *Store) migrateV3() error {
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS agent_heartbeat (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  agent_id TEXT,
  pid INTEGER,
  started_at TEXT,
  heartbeat_at TEXT,
  last_event_at TEXT,
  last_success_at TEXT,
  last_error TEXT
)`); err != nil {
		return err
	}
	return ensureColumn(s.db, "agent_offset", "prefix", "TEXT")
}

// migrationHook 让测试在指定版本失败。0 表示不拦截。
var migrationHook int

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

func (s *Store) Cursor() (int64, string, error) {
	var n int64
	var prefix sql.NullString
	err := s.db.QueryRow(`SELECT monitor_offset, prefix FROM agent_offset WHERE id = 1`).Scan(&n, &prefix)
	return n, prefix.String, err
}

func (s *Store) SetCursor(n int64, prefix string) error {
	_, err := s.db.Exec(`UPDATE agent_offset SET monitor_offset = ?, prefix = ? WHERE id = 1`, n, prefix)
	return err
}

func (s *Store) Beat(now time.Time, pid int, started, lastEvent, lastErr string) error {
	at := now.UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
INSERT INTO agent_heartbeat (id, agent_id, pid, started_at, heartbeat_at, last_event_at, last_success_at, last_error)
VALUES (1, 'local', ?, ?, ?, NULLIF(?, ''), CASE WHEN ? = '' THEN NULL ELSE ? END, ?)
ON CONFLICT(id) DO UPDATE SET
  pid = excluded.pid,
  started_at = excluded.started_at,
  heartbeat_at = excluded.heartbeat_at,
  last_event_at = COALESCE(excluded.last_event_at, agent_heartbeat.last_event_at),
  last_success_at = CASE WHEN excluded.last_error = '' THEN excluded.heartbeat_at ELSE agent_heartbeat.last_success_at END,
  last_error = excluded.last_error`,
		pid, started, at, lastEvent, lastErr, at, lastErr)
	return err
}

func (s *Store) SchemaVersion() (int, error) {
	var v int
	err := s.db.QueryRow(`SELECT version FROM agent_schema WHERE id = 1`).Scan(&v)
	return v, err
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
INSERT OR IGNORE INTO agent_event (event_id, monitor_type, severity, summary, received_at, source, snapshot_started_at, event_time)
VALUES (?, ?, ?, ?, ?, 'fast_https_monitor', ?, ?)`,
		id, ev.Type, ev.Severity, ev.Message, at, started, ev.Time)
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

// taskTransitions 是允许的状态变化。恢复一条故障只从这些状态进入 COMPLETED。
var taskTransitions = map[string]map[string]bool{
	"PENDING":          {"COLLECTING": true, "WAITING_APPROVAL": true, "WAITING_HUMAN": true, "COMPLETED": true, "ESCALATED": true, "IGNORED": true, "FAILED": true},
	"COLLECTING":       {"PENDING": true},
	"WAITING_APPROVAL": {"EXECUTING": true, "WAITING_HUMAN": true},
	"EXECUTING":        {"VERIFYING": true, "FAILED": true, "WAITING_HUMAN": true},
	"VERIFYING":        {"COMPLETED": true, "FAILED": true, "WAITING_HUMAN": true},
	"ESCALATED":        {"COMPLETED": true},
	"WAITING_HUMAN":    {"COMPLETED": true},
}

// TransitionTask 只在当前状态属于允许的来源时更新。from 为空时读取当前状态。
func (s *Store) TransitionTask(taskID, from, to string, now time.Time) (bool, error) {
	at := now.UTC().Format(time.RFC3339)
	if from == "" {
		var current string
		if err := s.db.QueryRow(`SELECT status FROM agent_task WHERE task_id = ?`, taskID).Scan(&current); err != nil {
			return false, err
		}
		from = current
	}
	if !taskTransitions[from][to] {
		return false, nil
	}
	res, err := s.db.Exec(`UPDATE agent_task SET status = ?, updated_at = ? WHERE task_id = ? AND status = ?`, to, at, taskID, from)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}
