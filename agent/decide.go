package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"fast-https/agent/model"
)

type openTask struct {
	taskID    string
	eventID   string
	kind      string
	message   string
	startedAt string
	eventTime string
	context   string
}

// DiagnosePending 处理已有上下文的 PENDING 任务。narrator 只改写说明文字；为 nil 时保留规则原文。
// 分类、步骤和策略不交给模型。
func DiagnosePending(stateDir string, now time.Time, narrator model.Provider) (int, error) {
	st, err := OpenStore(stateDir)
	if err != nil {
		return 0, err
	}
	defer st.Close()
	tasks, err := st.pendingDiagnose()
	if err != nil {
		return 0, err
	}
	n := 0
	for _, task := range tasks {
		if task.kind == "recovered" {
			if err := st.closeRecovered(task, now); err != nil {
				return n, err
			}
			n++
			continue
		}
		d := diagnose(task.kind, task.message, task.startedAt, task.context)
		if text, ok := narrate(context.Background(), narrator, d); ok {
			d.Cause = text
		}
		p := makePlan(task.taskID, task.eventID, d.Classification, drainStep(d.Classification, task.context))
		decision := policyOf(d.Classification, p.Steps)
		status := statusFor(d.Classification, decision, p.Steps)
		if err := st.saveDiagnosis(task, d, p, decision, status, now); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func (s *Store) pendingDiagnose() ([]openTask, error) {
	rows, err := s.db.Query(`
SELECT t.task_id, t.event_id, e.monitor_type, e.summary, IFNULL(e.snapshot_started_at, ''), IFNULL(e.event_time, ''), IFNULL(t.context_json, '')
FROM agent_task t
JOIN agent_event e ON e.event_id = t.event_id
WHERE t.status = 'PENDING' AND t.context_json IS NOT NULL AND t.context_json != ''
ORDER BY t.created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []openTask
	for rows.Next() {
		var t openTask
		if err := rows.Scan(&t.taskID, &t.eventID, &t.kind, &t.message, &t.startedAt, &t.eventTime, &t.context); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) closeRecovered(task openTask, now time.Time) error {
	ids, err := s.matchRecovered(task)
	if err != nil {
		return err
	}
	switch len(ids) {
	case 0:
		_, err = s.TransitionTask(task.taskID, "PENDING", "IGNORED", now)
		return err
	case 1:
		ok, err := s.TransitionTask(ids[0], "", "COMPLETED", now)
		if err != nil || !ok {
			return err
		}
		_, err = s.TransitionTask(task.taskID, "PENDING", "COMPLETED", now)
		return err
	default:
		_, err = s.TransitionTask(task.taskID, "PENDING", "WAITING_HUMAN", now)
		return err
	}
}

// matchRecovered 只配对同一进程启动周期内、发生在恢复之前的 unreachable。
// 多条都对得上时返回全部，调用方不得关闭。5xx、端口和证书任务不在范围内。
func (s *Store) matchRecovered(task openTask) ([]string, error) {
	rows, err := s.db.Query(`
SELECT t.task_id FROM agent_task t
JOIN agent_event e ON e.event_id = t.event_id
WHERE e.monitor_type = 'unreachable'
  AND e.snapshot_started_at = ?
  AND e.snapshot_started_at != ''
  AND t.status NOT IN ('COMPLETED', 'IGNORED', 'FAILED', 'CLOSED')
  AND (IFNULL(e.event_time, '') = '' OR e.event_time <= ?)
ORDER BY t.task_id`, task.startedAt, task.eventTime)
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

func (s *Store) saveDiagnosis(task openTask, d Diagnosis, p Plan, decision, status string, now time.Time) error {
	at := now.UTC().Format(time.RFC3339)
	evidence, _ := json.Marshal(d.Evidence)
	steps, _ := json.Marshal(p.Steps)
	needs := 0
	if d.NeedsHuman {
		needs = 1
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`
INSERT INTO agent_diagnosis (task_id, classification, subject, cause, impact, evidence_json, needs_human)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		task.taskID, d.Classification, d.Subject, d.Cause, d.Impact, string(evidence), needs); err != nil {
		return err
	}
	approval := 0
	if decision == policyApproval && executableDrain(p.Steps) {
		approval = 1
	}
	if _, err := tx.Exec(`
INSERT INTO agent_plan (plan_id, task_id, event_id, risk, plan_hash, steps_json, needs_approval, verify_text, on_failure)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, task.taskID, task.eventID, p.Risk, p.Hash, string(steps), approval, p.Verify, p.OnFailure); err != nil {
		return err
	}
	if decision == policyApproval && executableDrain(p.Steps) {
		if _, err := tx.Exec(`
INSERT INTO agent_approval (plan_id, status, reason, created_at) VALUES (?, 'pending', ?, ?)`,
			p.ID, d.Cause, at); err != nil {
			return err
		}
	}
	res, err := tx.Exec(`UPDATE agent_task SET status = ?, updated_at = ? WHERE task_id = ? AND status = 'PENDING'`, status, at, task.taskID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fmt.Errorf("task %s was not PENDING", task.taskID)
	}
	if _, err := tx.Exec(`
INSERT INTO agent_audit (at, task_id, kind, payload_json) VALUES (?, ?, 'diagnosis', ?)`,
		at, task.taskID, d.Cause); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) taskStatus(taskID string) (string, error) {
	var status string
	err := s.db.QueryRow(`SELECT status FROM agent_task WHERE task_id = ?`, taskID).Scan(&status)
	return status, err
}

func (s *Store) diagnosisClass(taskID string) (string, error) {
	var class string
	err := s.db.QueryRow(`SELECT classification FROM agent_diagnosis WHERE task_id = ?`, taskID).Scan(&class)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return class, err
}
