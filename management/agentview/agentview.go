// Package agentview 只读 Agent 的 SQLite 状态。不创建任务，也不启动 Agent 进程。
package agentview

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"fast-https/management/logquery"

	_ "modernc.org/sqlite"
)

const EnvDir = "FASTHTTPS_AGENT_DIR"

var ErrNotFound = errors.New("agent task not found")

// Status 是 agent status 的数据。库文件不存在时为 not_running。
type Status struct {
	Status      string `json:"status"`
	Source      string `json:"source"`
	Tasks       int    `json:"tasks,omitempty"`
	LastUpdated string `json:"last_updated,omitempty"`
}

type TaskSummary struct {
	TaskID    string `json:"task_id"`
	EventID   string `json:"event_id"`
	Type      string `json:"type"`
	Status    string `json:"status"`
	UpdatedAt string `json:"updated_at"`
}

type TaskDetail struct {
	TaskSummary
	Message        string `json:"message"`
	Classification string `json:"classification,omitempty"`
	Cause          string `json:"cause,omitempty"`
	Steps          string `json:"steps,omitempty"`
	Decision       string `json:"decision,omitempty"`
}

type Approval struct {
	PlanID    string `json:"plan_id"`
	Status    string `json:"status"`
	Reason    string `json:"reason"`
	CreatedAt string `json:"created_at"`
}

func ResolveDir(explicit string) string {
	if v := strings.TrimSpace(explicit); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv(EnvDir)); v != "" {
		return v
	}
	return filepath.Join("state", "agent")
}

func ReadStatus(dir string) (Status, error) {
	out := Status{Status: "not_running", Source: "agent_db"}
	db, err := openReadOnly(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return Status{}, err
	}
	defer db.Close()
	out.Status = "present"
	_ = db.QueryRow(`SELECT COUNT(*) FROM agent_task`).Scan(&out.Tasks)
	_ = db.QueryRow(`SELECT IFNULL(MAX(updated_at), '') FROM agent_task`).Scan(&out.LastUpdated)
	return out, nil
}

func ListTasks(dir, status string) ([]TaskSummary, error) {
	db, err := openReadOnly(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []TaskSummary{}, nil
		}
		return nil, err
	}
	defer db.Close()
	q := `
SELECT t.task_id, t.event_id, IFNULL(e.monitor_type, ''), t.status, t.updated_at
FROM agent_task t
LEFT JOIN agent_event e ON e.event_id = t.event_id`
	var args []any
	if status != "" {
		q += ` WHERE t.status = ?`
		args = append(args, status)
	}
	q += ` ORDER BY t.updated_at`
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TaskSummary
	for rows.Next() {
		var t TaskSummary
		if err := rows.Scan(&t.TaskID, &t.EventID, &t.Type, &t.Status, &t.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	if out == nil {
		out = []TaskSummary{}
	}
	return out, rows.Err()
}

func GetTask(dir, id string) (TaskDetail, error) {
	db, err := openReadOnly(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return TaskDetail{}, ErrNotFound
		}
		return TaskDetail{}, err
	}
	defer db.Close()
	var t TaskDetail
	err = db.QueryRow(`
SELECT t.task_id, t.event_id, IFNULL(e.monitor_type, ''), t.status, t.updated_at, IFNULL(e.summary, ''),
       IFNULL(d.classification, ''), IFNULL(d.cause, ''), IFNULL(p.steps_json, ''),
       CASE WHEN p.needs_approval = 1 THEN 'REQUIRE_APPROVAL' WHEN p.plan_id IS NULL THEN '' ELSE 'NO_WRITE' END
FROM agent_task t
LEFT JOIN agent_event e ON e.event_id = t.event_id
LEFT JOIN agent_diagnosis d ON d.task_id = t.task_id
LEFT JOIN agent_plan p ON p.task_id = t.task_id
WHERE t.task_id = ?`, id).Scan(
		&t.TaskID, &t.EventID, &t.Type, &t.Status, &t.UpdatedAt, &t.Message,
		&t.Classification, &t.Cause, &t.Steps, &t.Decision)
	if errors.Is(err, sql.ErrNoRows) {
		return TaskDetail{}, ErrNotFound
	}
	if err != nil {
		return TaskDetail{}, err
	}
	t.Message = scrub(t.Message)
	t.Cause = scrub(t.Cause)
	return t, nil
}

func ListApprovals(dir string) ([]Approval, error) {
	db, err := openReadOnly(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Approval{}, nil
		}
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(`SELECT plan_id, status, IFNULL(reason, ''), created_at FROM agent_approval WHERE status = 'pending' ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Approval
	for rows.Next() {
		var a Approval
		if err := rows.Scan(&a.PlanID, &a.Status, &a.Reason, &a.CreatedAt); err != nil {
			return nil, err
		}
		a.Reason = scrub(a.Reason)
		out = append(out, a)
	}
	if out == nil {
		out = []Approval{}
	}
	return out, rows.Err()
}

func openReadOnly(dir string) (*sql.DB, error) {
	path := filepath.Join(dir, "agent.db")
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	dsn := "file:" + filepath.ToSlash(path) + "?mode=ro"
	return sql.Open("sqlite", dsn)
}

func scrub(s string) string {
	s = logquery.Redact(s)
	s = strings.ReplaceAll(s, "ssl_certificate_key", "")
	return s
}
