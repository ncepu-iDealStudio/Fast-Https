package cmd

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fast-https/management/agentview"
)

func TestApprovalRestoredWhenOperationCannotStart(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeProxyConfig(t, dir)
	blocker := filepath.Join(dir, "not-a-directory")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FASTHTTPS_STATE_DIR", blocker)
	t.Setenv("FASTHTTPS_UPSTREAM_DRAIN_FILE", filepath.Join(dir, "drained.json"))
	t.Setenv(agentview.EnvDir, dir)
	serveApproveRuntime(t, twoUpstreamRuntime("unhealthy", "healthy"))
	writeApprovablePlan(t, filepath.Join(dir, "agent.db"), "e-op", []string{"upstream drain 10.0.0.1:8080"})

	_, _, code := runCLI(t, "agent", "approve", "p_e-op", "--config", cfgPath, "--yes", "--json")
	if code == 0 {
		t.Fatal("approve succeeded without an operation record")
	}
	approval, task := approvalAndTask(t, filepath.Join(dir, "agent.db"))
	if approval != "pending" || task != "WAITING_APPROVAL" {
		t.Fatalf("approval=%s task=%s", approval, task)
	}
}

func TestApprovalRestoredWhenDrainDoesNotStart(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeProxyConfig(t, dir)
	t.Setenv("FASTHTTPS_STATE_DIR", dir)
	t.Setenv("FASTHTTPS_UPSTREAM_DRAIN_FILE", filepath.Join(dir, "drained.json"))
	t.Setenv(agentview.EnvDir, dir)
	serveApproveRuntime(t, twoUpstreamRuntime("unhealthy", "healthy"))
	writeApprovablePlan(t, filepath.Join(dir, "agent.db"), "e-prep", []string{"upstream drain 10.0.0.1:8080"})
	prev := prepareApprovedDrain
	prepareApprovedDrain = func() error { return errors.New("stopped before the list write") }
	t.Cleanup(func() { prepareApprovedDrain = prev })

	_, _, code := runCLI(t, "agent", "approve", "p_e-prep", "--config", cfgPath, "--yes", "--json")
	if code == 0 {
		t.Fatal("approve succeeded")
	}
	approval, task := approvalAndTask(t, filepath.Join(dir, "agent.db"))
	if approval != "pending" || task != "WAITING_APPROVAL" {
		t.Fatalf("approval=%s task=%s", approval, task)
	}
}

func TestUncertainDrainDoesNotReturnToPending(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeProxyConfig(t, dir)
	blocked := filepath.Join(dir, "blocked")
	if err := os.WriteFile(blocked, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FASTHTTPS_STATE_DIR", dir)
	t.Setenv("FASTHTTPS_UPSTREAM_DRAIN_FILE", filepath.Join(blocked, "drained.json"))
	t.Setenv(agentview.EnvDir, dir)
	serveApproveRuntime(t, twoUpstreamRuntime("unhealthy", "healthy"))
	writeApprovablePlan(t, filepath.Join(dir, "agent.db"), "e-uncertain", []string{"upstream drain 10.0.0.1:8080"})

	_, _, code := runCLI(t, "agent", "approve", "p_e-uncertain", "--config", cfgPath, "--yes", "--json")
	if code == 0 {
		t.Fatal("approve succeeded")
	}
	approval, task := approvalAndTask(t, filepath.Join(dir, "agent.db"))
	if approval != "executing" || task != "WAITING_HUMAN" {
		t.Fatalf("approval=%s task=%s", approval, task)
	}
	_, _, again := runCLI(t, "agent", "approve", "p_e-uncertain", "--config", cfgPath, "--yes", "--json")
	if again == 0 {
		t.Fatal("a second approve ran the action")
	}
	approval, task = approvalAndTask(t, filepath.Join(dir, "agent.db"))
	if approval == "pending" || approval == "approved" {
		t.Fatalf("second approve changed approval to %s", approval)
	}
}

func writeProxyConfig(t *testing.T, dir string) string {
	t.Helper()
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
	return cfgPath
}

func twoUpstreamRuntime(first, second string) string {
	return `{
	  "schema_version":"1",
	  "upstream":{"upstreams":[
	    {"address":"10.0.0.1:8080","health":"` + first + `"},
	    {"address":"10.0.0.2:8080","health":"` + second + `","healthy":true}
	  ]}
	}`
}

func approvalAndTask(t *testing.T, dbPath string) (string, string) {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var approval, task string
	if err := db.QueryRow(`SELECT status FROM agent_approval`).Scan(&approval); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT status FROM agent_task`).Scan(&task); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(approval) == "" || strings.TrimSpace(task) == "" {
		t.Fatalf("empty status approval=%q task=%q", approval, task)
	}
	return approval, task
}
