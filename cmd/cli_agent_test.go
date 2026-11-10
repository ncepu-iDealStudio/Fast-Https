package cmd

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fast-https/management/agentview"

	_ "modernc.org/sqlite"
)

func TestAgentStatusMissing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(agentview.EnvDir, dir)
	out, _, code := runCLI(t, "agent", "status", "--json")
	if code != 0 {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	doc := decodeOneDocument(t, out)
	data := doc["data"].(map[string]any)
	if data["status"] != "not_running" {
		t.Fatalf("status = %v", data)
	}
	if _, err := os.Stat(filepath.Join(dir, "agent.db")); !os.IsNotExist(err) {
		t.Fatalf("status created a database: %v", err)
	}
	_, _, code = runCLI(t, "agent", "nope", "--json")
	if code != 2 {
		t.Fatalf("unknown subcommand exit = %d, want 2", code)
	}
}

func TestAgentApprovalsReadOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.db")
	writeAgentFixture(t, path)
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(agentview.EnvDir, dir)
	out, _, code := runCLI(t, "agent", "approvals", "--json")
	if code != 0 {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	doc := decodeOneDocument(t, out)
	if strings.Contains(out, "ssl_certificate_key") || strings.Contains(out, "BEGIN PRIVATE KEY") {
		t.Fatalf("approval leaked secret material: %s", out)
	}
	items := doc["data"].(map[string]any)["approvals"].([]any)
	if len(items) != 1 {
		t.Fatalf("approvals = %v", items)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if after.Size() != before.Size() {
		t.Fatalf("read-only query changed db size %d -> %d", before.Size(), after.Size())
	}
}

func TestAgentTaskJSON(t *testing.T) {
	dir := t.TempDir()
	writeAgentFixture(t, filepath.Join(dir, "agent.db"))
	t.Setenv(agentview.EnvDir, dir)
	out, _, code := runCLI(t, "agent", "task", "get", "t1", "--json")
	if code != 0 {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	doc := decodeOneDocument(t, out)
	if doc["command"] != "agent.task.get" {
		t.Fatalf("command = %v", doc["command"])
	}
	data := doc["data"].(map[string]any)
	if data["task_id"] != "t1" || data["classification"] != "http_5xx_spike" {
		t.Fatalf("task = %v", data)
	}
}

func writeAgentFixture(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`
CREATE TABLE agent_event (event_id TEXT PRIMARY KEY, monitor_type TEXT, severity TEXT, summary TEXT, received_at TEXT, source TEXT, snapshot_started_at TEXT);
CREATE TABLE agent_task (task_id TEXT PRIMARY KEY, event_id TEXT, status TEXT, attempts INTEGER, context_json TEXT, created_at TEXT, updated_at TEXT);
CREATE TABLE agent_diagnosis (task_id TEXT PRIMARY KEY, classification TEXT, subject TEXT, cause TEXT, impact TEXT, evidence_json TEXT, needs_human INTEGER);
CREATE TABLE agent_plan (plan_id TEXT PRIMARY KEY, task_id TEXT, event_id TEXT, risk TEXT, plan_hash TEXT, steps_json TEXT, needs_approval INTEGER);
CREATE TABLE agent_approval (plan_id TEXT PRIMARY KEY, status TEXT, reason TEXT, created_at TEXT);
INSERT INTO agent_event VALUES ('e1','5xx_ratio','warning','ratio','2026-11-10T00:00:00Z','fast_https_monitor','');
INSERT INTO agent_task VALUES ('t1','e1','WAITING_APPROVAL',0,'{}','2026-11-10T00:00:00Z','2026-11-10T00:00:01Z');
INSERT INTO agent_diagnosis VALUES ('t1','http_5xx_spike','server','ratio','5xx','[]',1);
INSERT INTO agent_plan VALUES ('p1','t1','e1','low','abc','[]',1);
INSERT INTO agent_approval VALUES ('p1','pending','see ssl_certificate_key and -----BEGIN PRIVATE KEY-----secret-----END PRIVATE KEY-----','2026-11-10T00:00:02Z');
`)
	if err != nil {
		t.Fatal(err)
	}
}
