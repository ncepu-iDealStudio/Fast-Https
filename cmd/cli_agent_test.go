package cmd

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	if data["status"] != "unknown" {
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

func TestAgentApproveDrain(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "fast-https.json")
	body := `{
	  "http": {"server": [{
	    "listen": "80",
	    "server_name": "a.example",
	    "location": [{"url": "/", "type": "proxy", "proxy_pass": ["http://10.0.0.1:8080", "http://10.0.0.2:8080"]}]
	  }]}
	}`
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(cfgPath)
	drainFile := filepath.Join(dir, "drained.json")
	t.Setenv("FASTHTTPS_UPSTREAM_DRAIN_FILE", drainFile)
	t.Setenv("FASTHTTPS_STATE_DIR", dir)
	t.Setenv(agentview.EnvDir, dir)
	serveApproveRuntime(t, `{
	  "schema_version":"1",
	  "upstream":{"upstreams":[
	    {"address":"10.0.0.1:8080","health":"unhealthy","healthy":false},
	    {"address":"10.0.0.2:8080","health":"healthy","healthy":true}
	  ]}
	}`)
	steps := []string{"upstream drain 10.0.0.1:8080"}
	writeApprovablePlan(t, filepath.Join(dir, "agent.db"), "e-approve", steps)

	out, _, code := runCLI(t, "agent", "approve", "p_e-approve", "--config", cfgPath, "--json")
	if code != 2 {
		t.Fatalf("missing --yes exit = %d\n%s", code, out)
	}
	if _, err := os.Stat(drainFile); !os.IsNotExist(err) {
		t.Fatal("approve without --yes wrote the drain list")
	}

	out, _, code = runCLI(t, "agent", "approve", "p_e-approve", "--config", cfgPath, "--yes", "--json")
	if code != 0 {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	doc := decodeOneDocument(t, out)
	if doc["command"] != "agent.approve" {
		t.Fatalf("command = %v", doc["command"])
	}
	raw, err := os.ReadFile(drainFile)
	if err != nil || !strings.Contains(string(raw), "10.0.0.1:8080") {
		t.Fatalf("drain file = %s err=%v", raw, err)
	}
	after, _ := os.ReadFile(cfgPath)
	if string(after) != string(before) {
		t.Fatal("proxy_pass file changed")
	}

	db, err := sql.Open("sqlite", filepath.Join(dir, "agent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`UPDATE agent_plan SET steps_json = ?`, `["upstream drain 10.9.9.9:1"]`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE agent_approval SET status = 'pending'`); err != nil {
		t.Fatal(err)
	}
	out, _, code = runCLI(t, "agent", "approve", "p_e-approve", "--config", cfgPath, "--yes", "--json")
	if code != 10 {
		t.Fatalf("hash mismatch exit = %d\n%s", code, out)
	}
}

func writeApprovablePlan(t *testing.T, path, eventID string, steps []string) {
	t.Helper()
	rawSteps, err := json.Marshal(steps)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	hash := planHash(eventID, "low", approveVerify, approveOnFailure, steps)
	if _, err := db.Exec(`
CREATE TABLE agent_event (event_id TEXT PRIMARY KEY, monitor_type TEXT, severity TEXT, summary TEXT, received_at TEXT, source TEXT, snapshot_started_at TEXT);
CREATE TABLE agent_task (task_id TEXT PRIMARY KEY, event_id TEXT, status TEXT, attempts INTEGER, context_json TEXT, created_at TEXT, updated_at TEXT);
CREATE TABLE agent_plan (plan_id TEXT PRIMARY KEY, task_id TEXT, event_id TEXT, risk TEXT, plan_hash TEXT, steps_json TEXT, needs_approval INTEGER, verify_text TEXT, on_failure TEXT);
CREATE TABLE agent_approval (plan_id TEXT PRIMARY KEY, status TEXT, reason TEXT, created_at TEXT, decided_at TEXT, actor TEXT);
`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO agent_task VALUES ('t1', ?, 'WAITING_APPROVAL', 0, '{}', '2026-11-10T00:00:00Z', '2026-11-10T00:00:01Z')`, eventID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO agent_plan VALUES (?, 't1', ?, 'low', ?, ?, 1, ?, ?)`, "p_"+eventID, eventID, hash, string(rawSteps), approveVerify, approveOnFailure); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO agent_approval (plan_id, status, reason, created_at) VALUES (?, 'pending', 'drain one peer', '2026-11-10T00:00:02Z')`, "p_"+eventID); err != nil {
		t.Fatal(err)
	}
}

func serveApproveRuntime(t *testing.T, body string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("FASTHTTPS_RUNTIME_URL", srv.URL)
}

func TestApproveClaimIsSingleWinner(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "agent.db")
	writeApprovablePlan(t, dbPath, "e-race", []string{"upstream drain 10.0.0.1:8080"})
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`PRAGMA busy_timeout = 5000`); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wins := 0
	var mu sync.Mutex
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := sql.Open("sqlite", dbPath)
			if err != nil {
				t.Error(err)
				return
			}
			defer conn.Close()
			_, _ = conn.Exec(`PRAGMA busy_timeout = 5000`)
			ok, err := claimApproval(conn, "p_e-race")
			if err != nil {
				t.Error(err)
				return
			}
			if ok {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if wins != 1 {
		t.Fatalf("claims = %d, want 1", wins)
	}
}

func TestApproveRejectsStaleHealth(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "fast-https.json")
	body := `{
	  "http": {"server": [{
	    "listen": "80",
	    "server_name": "a.example",
	    "location": [{"url": "/", "type": "proxy", "proxy_pass": ["http://10.0.0.1:8080", "http://10.0.0.2:8080"]}]
	  }]}
	}`
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	drainFile := filepath.Join(dir, "drained.json")
	t.Setenv("FASTHTTPS_UPSTREAM_DRAIN_FILE", drainFile)
	t.Setenv("FASTHTTPS_STATE_DIR", dir)
	t.Setenv(agentview.EnvDir, dir)
	serveApproveRuntime(t, `{
	  "schema_version":"1",
	  "upstream":{"upstreams":[
	    {"address":"10.0.0.1:8080","health":"unknown"},
	    {"address":"10.0.0.2:8080","health":"healthy","healthy":true}
	  ]}
	}`)
	writeApprovablePlan(t, filepath.Join(dir, "agent.db"), "e-stale", []string{"upstream drain 10.0.0.1:8080"})
	_, _, code := runCLI(t, "agent", "approve", "p_e-stale", "--config", cfgPath, "--yes", "--json")
	if code != 6 {
		t.Fatalf("exit = %d, want 6", code)
	}
	if _, err := os.Stat(drainFile); !os.IsNotExist(err) {
		t.Fatal("stale health wrote the drain list")
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
