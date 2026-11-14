# Batch 3 执行结果：运行可靠性

日期：2026-11-14。上一批是 `163ca8f`。

## 1. 改了什么

1. SQLite 用 `agent_schema.version` 按 1、2、3 顺序迁移。某一步失败时不抬高版本号，已提交的前一步保留，下次启动从失败的版本再试。版本 3 增加心跳表和日志开头标记列。
2. 主循环每次跑完写心跳：pid、启动时间、`heartbeat_at`、最近错误。`agent status` 不再把库文件存在当成在线。30 秒内有心跳是 `online`，更旧是 `offline`，没有库或没有心跳是 `unknown`。
3. 读 `events.jsonl` 时记下文件开头的标记。文件被换成另一份、而且新文件比旧偏移更长时，从开头重读，不再从旧偏移中间切开。
4. 主循环监听中断信号，`ticker.Stop`，采集用的 HTTP 和 CLI 沿用已有的 context 超时。已经写入的事件偏移在 `runOnce` 返回前保存。

## 2. 新增测试

- `TestSchemaMigrationStopsAndRetries`
- `TestSchemaKeepsExistingRows`
- `TestReplacedLogIsReadFromStart`
- `management/agentview` 的 `TestHeartbeatStatus`
- `TestAgentStatusMissing` 改为期望 `unknown`

## 3. 实际测试

```text
go test ./agent/ ./cmd/ ./management/agentview/ -count=1
ok  fast-https/agent
ok  fast-https/cmd
ok  fast-https/management/agentview
```

## 4. 尚未解决

- 文档仍有过时描述，留在 Batch 4。
- 心跳是本进程写进自己的库，没有独立注册中心。
- 日志身份用文件开头的哈希，不是 inode。开头相同的替换文件不会被当成新文件。
- `agent approve` 仍是另一个进程。它留下的 `executing` 记录靠下次批准时再看名单，Agent 主循环不执行摘除。

## 5. 完整 Git diff

相对 `163ca8f` 的源码 diff，不含本报告。

```diff
diff --git a/agent/certpoll.go b/agent/certpoll.go
index aac56c2..1a6acde 100644
--- a/agent/certpoll.go
+++ b/agent/certpoll.go
@@ -29,7 +29,20 @@ func PollCerts(monitorDir, stateDir, binary string, warnDays int, run cmdRunner,
 	if now.IsZero() {
 		now = time.Now()
 	}
-	body, err := runAllowed(context.Background(), run, binary, []string{"cert", "list", "--json"})
+	return pollCerts(context.Background(), monitorDir, stateDir, binary, warnDays, run, now)
+}
+
+func pollCerts(ctx context.Context, monitorDir, stateDir, binary string, warnDays int, run cmdRunner, now time.Time) (int, error) {
+	if warnDays < 1 {
+		warnDays = defaultCertWarnDays
+	}
+	if now.IsZero() {
+		now = time.Now()
+	}
+	if ctx == nil {
+		ctx = context.Background()
+	}
+	body, err := runAllowed(ctx, run, binary, []string{"cert", "list", "--json"})
 	st, sterr := OpenStore(stateDir)
 	if sterr != nil {
 		return 0, sterr
diff --git a/agent/collect.go b/agent/collect.go
index 61f1646..1c05995 100644
--- a/agent/collect.go
+++ b/agent/collect.go
@@ -13,6 +13,7 @@ type CollectOptions struct {
 	RuntimeURL string
 	Run        cmdRunner
 	Now        time.Time
+	Ctx        context.Context
 }
 
 type contextDoc struct {
@@ -38,7 +39,11 @@ func CollectPending(stateDir string, opt CollectOptions) (int, error) {
 	}
 	n := 0
 	for _, id := range ids {
-		raw, err := json.Marshal(buildContext(context.Background(), opt))
+		ctx := opt.Ctx
+		if ctx == nil {
+			ctx = context.Background()
+		}
+		raw, err := json.Marshal(buildContext(ctx, opt))
 		if err != nil {
 			return n, err
 		}
diff --git a/agent/ingest.go b/agent/ingest.go
index a0e4b65..7ebc55f 100644
--- a/agent/ingest.go
+++ b/agent/ingest.go
@@ -23,11 +23,11 @@ func IngestOnce(monitorDir, stateDir string, now time.Time) (IngestResult, error
 }
 
 func ingestStore(st *Store, monitorDir string, now time.Time) (IngestResult, error) {
-	offset, err := st.Offset()
+	offset, prefix, err := st.Cursor()
 	if err != nil {
 		return IngestResult{}, err
 	}
-	next, lines, err := readNewLines(monitorDir, offset)
+	next, prefix, lines, err := readNewLines(monitorDir, offset, prefix)
 	if err != nil {
 		return IngestResult{}, err
 	}
@@ -50,7 +50,7 @@ func ingestStore(st *Store, monitorDir string, now time.Time) (IngestResult, err
 			res.Added++
 		}
 	}
-	if err := st.SetOffset(next); err != nil {
+	if err := st.SetCursor(next, prefix); err != nil {
 		return res, err
 	}
 	return res, nil
diff --git a/agent/main.go b/agent/main.go
index 1148ce0..840fee1 100644
--- a/agent/main.go
+++ b/agent/main.go
@@ -1,9 +1,12 @@
 package main
 
 import (
+	"context"
 	"flag"
 	"fmt"
 	"os"
+	"os/signal"
+	"syscall"
 	"time"
 
 	"fast-https/agent/model"
@@ -27,8 +30,10 @@ func main() {
 	if *stateDir != "" {
 		cfg.StateDir = *stateDir
 	}
+	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
+	defer stop()
 	if *once {
-		if err := runOnce(cfg); err != nil {
+		if err := runOnce(ctx, cfg); err != nil {
 			fmt.Fprintf(os.Stderr, "agent: %v\n", err)
 			os.Exit(1)
 		}
@@ -37,32 +42,65 @@ func main() {
 	ticker := time.NewTicker(2 * time.Second)
 	defer ticker.Stop()
 	for {
-		if err := runOnce(cfg); err != nil {
+		err := runOnce(ctx, cfg)
+		if err != nil {
 			fmt.Fprintf(os.Stderr, "agent: %v\n", err)
 		}
-		<-ticker.C
+		select {
+		case <-ctx.Done():
+			return
+		case <-ticker.C:
+		}
 	}
 }
 
-func runOnce(cfg Config) error {
+func runOnce(ctx context.Context, cfg Config) error {
+	if ctx == nil {
+		ctx = context.Background()
+	}
 	now := time.Now()
-	if _, err := IngestOnce(cfg.MonitorDir, cfg.StateDir, now); err != nil {
+	added, err := IngestOnce(cfg.MonitorDir, cfg.StateDir, now)
+	beat(cfg, now, added.Added > 0, err)
+	if err != nil {
 		return err
 	}
-	if _, err := PollCerts(cfg.MonitorDir, cfg.StateDir, cfg.FastHTTPSBin, cfg.CertWarnDays, nil, now); err != nil {
+	if _, err := pollCerts(ctx, cfg.MonitorDir, cfg.StateDir, cfg.FastHTTPSBin, cfg.CertWarnDays, nil, now); err != nil {
+		beat(cfg, now, false, err)
 		return err
 	}
 	if _, err := CollectPending(cfg.StateDir, CollectOptions{
 		Binary:     cfg.FastHTTPSBin,
 		RuntimeURL: cfg.RuntimeURL,
 		Now:        now,
+		Ctx:        ctx,
 	}); err != nil {
+		beat(cfg, now, false, err)
 		return err
 	}
-	_, err := DiagnosePending(cfg.StateDir, now, cfg.models())
+	_, err = DiagnosePending(cfg.StateDir, now, cfg.models())
+	beat(cfg, now, false, err)
 	return err
 }
 
+func beat(cfg Config, now time.Time, sawEvent bool, runErr error) {
+	st, err := OpenStore(cfg.StateDir)
+	if err != nil {
+		return
+	}
+	defer st.Close()
+	lastEvent := ""
+	if sawEvent {
+		lastEvent = now.UTC().Format(time.RFC3339)
+	}
+	msg := ""
+	if runErr != nil {
+		msg = runErr.Error()
+	}
+	_ = st.Beat(now, os.Getpid(), processStarted.UTC().Format(time.RFC3339), lastEvent, msg)
+}
+
+var processStarted = time.Now()
+
 func (c Config) models() model.Provider {
 	return model.FromEnv(c.ModelURL, c.Model, c.ModelExternalURL, c.ModelExternal, c.ModelKey)
 }
diff --git a/agent/schema_test.go b/agent/schema_test.go
new file mode 100644
index 0000000..6917156
--- /dev/null
+++ b/agent/schema_test.go
@@ -0,0 +1,75 @@
+package main
+
+import (
+	"database/sql"
+	"os"
+	"path/filepath"
+	"testing"
+	"time"
+
+	_ "modernc.org/sqlite"
+)
+
+func TestSchemaMigrationStopsAndRetries(t *testing.T) {
+	dir := t.TempDir()
+	migrationHook = 2
+	t.Cleanup(func() { migrationHook = 0 })
+	if _, err := OpenStore(dir); err == nil {
+		t.Fatal("migration 2 should fail")
+	}
+	db, err := sql.Open("sqlite", filepath.Join(dir, "agent.db"))
+	if err != nil {
+		t.Fatal(err)
+	}
+	var version int
+	if err := db.QueryRow(`SELECT version FROM agent_schema WHERE id = 1`).Scan(&version); err != nil {
+		t.Fatal(err)
+	}
+	if version != 1 {
+		t.Fatalf("version = %d, want 1", version)
+	}
+	var tasks int
+	if err := db.QueryRow(`SELECT COUNT(*) FROM agent_task`).Scan(&tasks); err != nil {
+		t.Fatal(err)
+	}
+	_ = db.Close()
+	migrationHook = 0
+	st, err := OpenStore(dir)
+	if err != nil {
+		t.Fatal(err)
+	}
+	defer st.Close()
+	version, err = st.SchemaVersion()
+	if err != nil || version != 3 {
+		t.Fatalf("version = %d err=%v", version, err)
+	}
+	if err := st.Beat(time.Now(), 1, "2026-11-14T00:00:00Z", "", ""); err != nil {
+		t.Fatal(err)
+	}
+}
+
+func TestSchemaKeepsExistingRows(t *testing.T) {
+	dir := t.TempDir()
+	path := filepath.Join(dir, "agent.db")
+	if err := os.WriteFile(path, []byte{}, 0o644); err != nil {
+		t.Fatal(err)
+	}
+	st := mustStore(t, dir)
+	if _, err := st.InsertEvent(lineEvent{Type: "5xx_ratio", Message: "keep", Time: "2026-11-14T00:00:00.000Z"}, "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", time.Now()); err != nil {
+		t.Fatal(err)
+	}
+	_ = st.Close()
+	st = mustStore(t, dir)
+	defer st.Close()
+	var n int
+	if err := st.db.QueryRow(`SELECT COUNT(*) FROM agent_task`).Scan(&n); err != nil {
+		t.Fatal(err)
+	}
+	if n != 1 {
+		t.Fatalf("tasks = %d", n)
+	}
+	version, err := st.SchemaVersion()
+	if err != nil || version != 3 {
+		t.Fatalf("version = %d err=%v", version, err)
+	}
+}
diff --git a/agent/store.go b/agent/store.go
index 3788975..0b51ac9 100644
--- a/agent/store.go
+++ b/agent/store.go
@@ -51,6 +51,37 @@ func OpenStore(dir string) (*Store, error) {
 func (s *Store) Close() error { return s.db.Close() }
 
 func (s *Store) migrate() error {
+	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS agent_schema (
+  id INTEGER PRIMARY KEY CHECK (id = 1),
+  version INTEGER NOT NULL
+)`); err != nil {
+		return err
+	}
+	var version int
+	err := s.db.QueryRow(`SELECT version FROM agent_schema WHERE id = 1`).Scan(&version)
+	if errors.Is(err, sql.ErrNoRows) {
+		version = 0
+	} else if err != nil {
+		return err
+	}
+	steps := []func() error{s.migrateV1, s.migrateV2, s.migrateV3}
+	for i, step := range steps {
+		next := i + 1
+		if version >= next {
+			continue
+		}
+		if err := step(); err != nil {
+			return fmt.Errorf("schema %d: %w", next, err)
+		}
+		if _, err := s.db.Exec(`INSERT OR REPLACE INTO agent_schema (id, version) VALUES (1, ?)`, next); err != nil {
+			return err
+		}
+		version = next
+	}
+	return nil
+}
+
+func (s *Store) migrateV1() error {
 	_, err := s.db.Exec(`
 CREATE TABLE IF NOT EXISTS agent_event (
   event_id TEXT PRIMARY KEY,
@@ -107,8 +138,12 @@ CREATE TABLE IF NOT EXISTS agent_offset (
 );
 INSERT OR IGNORE INTO agent_offset (id, monitor_offset) VALUES (1, 0);
 `)
-	if err != nil {
-		return err
+	return err
+}
+
+func (s *Store) migrateV2() error {
+	if migrationHook == 2 {
+		return errors.New("migration 2 failed")
 	}
 	for _, col := range []struct{ table, name, decl string }{
 		{"agent_event", "snapshot_started_at", "TEXT"},
@@ -125,6 +160,25 @@ INSERT OR IGNORE INTO agent_offset (id, monitor_offset) VALUES (1, 0);
 	return s.migrateWaitingHuman()
 }
 
+func (s *Store) migrateV3() error {
+	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS agent_heartbeat (
+  id INTEGER PRIMARY KEY CHECK (id = 1),
+  agent_id TEXT,
+  pid INTEGER,
+  started_at TEXT,
+  heartbeat_at TEXT,
+  last_event_at TEXT,
+  last_success_at TEXT,
+  last_error TEXT
+)`); err != nil {
+		return err
+	}
+	return ensureColumn(s.db, "agent_offset", "prefix", "TEXT")
+}
+
+// migrationHook 让测试在指定版本失败。0 表示不拦截。
+var migrationHook int
+
 func ensureColumn(db *sql.DB, table, name, decl string) error {
 	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
 	if err != nil {
@@ -182,6 +236,40 @@ func (s *Store) SetOffset(n int64) error {
 	return err
 }
 
+func (s *Store) Cursor() (int64, string, error) {
+	var n int64
+	var prefix sql.NullString
+	err := s.db.QueryRow(`SELECT monitor_offset, prefix FROM agent_offset WHERE id = 1`).Scan(&n, &prefix)
+	return n, prefix.String, err
+}
+
+func (s *Store) SetCursor(n int64, prefix string) error {
+	_, err := s.db.Exec(`UPDATE agent_offset SET monitor_offset = ?, prefix = ? WHERE id = 1`, n, prefix)
+	return err
+}
+
+func (s *Store) Beat(now time.Time, pid int, started, lastEvent, lastErr string) error {
+	at := now.UTC().Format(time.RFC3339)
+	_, err := s.db.Exec(`
+INSERT INTO agent_heartbeat (id, agent_id, pid, started_at, heartbeat_at, last_event_at, last_success_at, last_error)
+VALUES (1, 'local', ?, ?, ?, NULLIF(?, ''), CASE WHEN ? = '' THEN NULL ELSE ? END, ?)
+ON CONFLICT(id) DO UPDATE SET
+  pid = excluded.pid,
+  started_at = excluded.started_at,
+  heartbeat_at = excluded.heartbeat_at,
+  last_event_at = COALESCE(excluded.last_event_at, agent_heartbeat.last_event_at),
+  last_success_at = CASE WHEN excluded.last_error = '' THEN excluded.heartbeat_at ELSE agent_heartbeat.last_success_at END,
+  last_error = excluded.last_error`,
+		pid, started, at, lastEvent, lastErr, at, lastErr)
+	return err
+}
+
+func (s *Store) SchemaVersion() (int, error) {
+	var v int
+	err := s.db.QueryRow(`SELECT version FROM agent_schema WHERE id = 1`).Scan(&v)
+	return v, err
+}
+
 func (s *Store) TaskCount() (int, error) {
 	var n int
 	err := s.db.QueryRow(`SELECT COUNT(*) FROM agent_task`).Scan(&n)
diff --git a/agent/tail.go b/agent/tail.go
index 7ccccff..21e5023 100644
--- a/agent/tail.go
+++ b/agent/tail.go
@@ -2,38 +2,46 @@ package main
 
 import (
 	"bufio"
+	"crypto/sha256"
+	"encoding/hex"
 	"fmt"
 	"io"
 	"os"
 	"path/filepath"
+	"strconv"
+	"strings"
 )
 
 const eventsFile = "events.jsonl"
 
-// readNewLines 从 offset 读取完整行。文件变短（轮转）时从 0 重新读。
-// 返回新偏移和每一行的文本（不含换行）。
-func readNewLines(monitorDir string, offset int64) (int64, []string, error) {
+// readNewLines 从 offset 读取完整行。
+// prefix 是上次看到的文件开头。文件被换掉时，即使新文件更长，也从 0 读。
+// 文件变短时同样从 0 读。返回新偏移、新的开头标记和每一行文本。
+func readNewLines(monitorDir string, offset int64, prefix string) (int64, string, []string, error) {
 	path := filepath.Join(monitorDir, eventsFile)
 	info, err := os.Lstat(path)
 	if err != nil {
 		if os.IsNotExist(err) {
-			return offset, nil, nil
+			return offset, prefix, nil, nil
 		}
-		return offset, nil, err
+		return offset, prefix, nil, err
 	}
 	if info.Mode()&os.ModeSymlink != 0 {
-		return offset, nil, fmt.Errorf("events file is a symlink and will not be followed")
-	}
-	if info.Size() < offset {
-		offset = 0
+		return offset, prefix, nil, fmt.Errorf("events file is a symlink and will not be followed")
 	}
 	f, err := os.Open(path)
 	if err != nil {
-		return offset, nil, err
+		return offset, prefix, nil, err
 	}
 	defer f.Close()
+	if replaced, id, err := fileReplaced(f, prefix); err != nil {
+		return offset, prefix, nil, err
+	} else if replaced || info.Size() < offset {
+		offset = 0
+		prefix = id
+	}
 	if _, err := f.Seek(offset, io.SeekStart); err != nil {
-		return offset, nil, err
+		return offset, prefix, nil, err
 	}
 	r := bufio.NewReader(f)
 	var lines []string
@@ -50,10 +58,68 @@ func readNewLines(monitorDir string, offset int64) (int64, []string, error) {
 			}
 		}
 		if err == io.EOF {
-			return offset, lines, nil
+			if next, err := prefixOf(f); err == nil && next != "" {
+				prefix = next
+			}
+			return offset, prefix, lines, nil
 		}
 		if err != nil {
-			return offset, lines, err
+			return offset, prefix, lines, err
 		}
 	}
 }
+
+func fileReplaced(f *os.File, prefix string) (bool, string, error) {
+	if prefix == "" {
+		return false, "", nil
+	}
+	n, sum, err := splitPrefix(prefix)
+	if err != nil {
+		return true, "", nil
+	}
+	got, err := hashFirst(f, n)
+	if err != nil {
+		return true, "", nil
+	}
+	return got != sum, got, nil
+}
+
+func prefixOf(f *os.File) (string, error) {
+	info, err := f.Stat()
+	if err != nil {
+		return "", err
+	}
+	n := int(info.Size())
+	if n > 64 {
+		n = 64
+	}
+	if n == 0 {
+		return "", nil
+	}
+	sum, err := hashFirst(f, n)
+	if err != nil {
+		return "", err
+	}
+	return strconv.Itoa(n) + ":" + sum, nil
+}
+
+func splitPrefix(prefix string) (int, string, error) {
+	nText, sum, ok := strings.Cut(prefix, ":")
+	if !ok {
+		return 0, "", fmt.Errorf("bad prefix")
+	}
+	n, err := strconv.Atoi(nText)
+	if err != nil || n < 1 {
+		return 0, "", fmt.Errorf("bad prefix")
+	}
+	return n, sum, nil
+}
+
+func hashFirst(f *os.File, n int) (string, error) {
+	buf := make([]byte, n)
+	if _, err := f.ReadAt(buf, 0); err != nil {
+		return "", err
+	}
+	sum := sha256.Sum256(buf)
+	return hex.EncodeToString(sum[:8]), nil
+}
diff --git a/agent/tail_test.go b/agent/tail_test.go
new file mode 100644
index 0000000..3978090
--- /dev/null
+++ b/agent/tail_test.go
@@ -0,0 +1,37 @@
+package main
+
+import (
+	"os"
+	"path/filepath"
+	"strings"
+	"testing"
+)
+
+func TestReplacedLogIsReadFromStart(t *testing.T) {
+	dir := t.TempDir()
+	path := filepath.Join(dir, eventsFile)
+	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
+		t.Fatal(err)
+	}
+	offset, prefix, lines, err := readNewLines(dir, 0, "")
+	if err != nil {
+		t.Fatal(err)
+	}
+	if len(lines) != 1 || lines[0] != "old" || offset != 4 || prefix == "" {
+		t.Fatalf("first read offset=%d prefix=%s lines=%v", offset, prefix, lines)
+	}
+	replacement := strings.Repeat("x", 40) + "\nFIRST\nSECOND\n"
+	if int64(len(replacement)) <= offset {
+		t.Fatal("replacement must be larger than the old offset")
+	}
+	if err := os.WriteFile(path, []byte(replacement), 0o644); err != nil {
+		t.Fatal(err)
+	}
+	_, _, lines, err = readNewLines(dir, offset, prefix)
+	if err != nil {
+		t.Fatal(err)
+	}
+	if len(lines) < 2 || !strings.Contains(lines[0], "x") || lines[len(lines)-1] != "SECOND" {
+		t.Fatalf("rotated read = %v", lines)
+	}
+}
diff --git a/cmd/agent.go b/cmd/agent.go
index fc0de4d..bd17329 100644
--- a/cmd/agent.go
+++ b/cmd/agent.go
@@ -14,8 +14,8 @@ func newAgentCmd() *cobra.Command {
 		Use:   "agent",
 		Short: t("查询 AI Agent 的任务与待审批", "query AI agent tasks and pending approvals"),
 		Long: t(
-			"只读 Agent 的状态库，不启动 Agent，也不创建任务。\n库文件不存在时 status 为 not_running。",
-			"Read the agent state database. This does not start the agent or create tasks.\nA missing database reports status not_running."),
+			"只读 Agent 的状态库，不启动 Agent，也不创建任务。\n没有心跳或库文件不存在时 status 为 unknown；心跳超过 30 秒为 offline。",
+			"Read the agent state database. This does not start the agent or create tasks.\nA missing database or heartbeat is unknown. A heartbeat older than 30 seconds is offline."),
 		SilenceUsage: true,
 		Args: func(cmd *cobra.Command, args []string) error {
 			if len(args) == 0 {
diff --git a/cmd/cli_agent_test.go b/cmd/cli_agent_test.go
index 00e6094..f20b969 100644
--- a/cmd/cli_agent_test.go
+++ b/cmd/cli_agent_test.go
@@ -25,7 +25,7 @@ func TestAgentStatusMissing(t *testing.T) {
 	}
 	doc := decodeOneDocument(t, out)
 	data := doc["data"].(map[string]any)
-	if data["status"] != "not_running" {
+	if data["status"] != "unknown" {
 		t.Fatalf("status = %v", data)
 	}
 	if _, err := os.Stat(filepath.Join(dir, "agent.db")); !os.IsNotExist(err) {
diff --git a/management/agentview/agentview.go b/management/agentview/agentview.go
index d37ab00..4362449 100644
--- a/management/agentview/agentview.go
+++ b/management/agentview/agentview.go
@@ -7,6 +7,7 @@ import (
 	"os"
 	"path/filepath"
 	"strings"
+	"time"
 
 	"fast-https/management/logquery"
 
@@ -59,8 +60,14 @@ func ResolveDir(explicit string) string {
 	return filepath.Join("state", "agent")
 }
 
+const heartbeatFresh = 30 * time.Second
+
 func ReadStatus(dir string) (Status, error) {
-	out := Status{Status: "not_running", Source: "agent_db"}
+	return readStatusAt(dir, time.Now())
+}
+
+func readStatusAt(dir string, now time.Time) (Status, error) {
+	out := Status{Status: "unknown", Source: "agent_db"}
 	db, err := openReadOnly(dir)
 	if err != nil {
 		if os.IsNotExist(err) {
@@ -69,9 +76,22 @@ func ReadStatus(dir string) (Status, error) {
 		return Status{}, err
 	}
 	defer db.Close()
-	out.Status = "present"
 	_ = db.QueryRow(`SELECT COUNT(*) FROM agent_task`).Scan(&out.Tasks)
 	_ = db.QueryRow(`SELECT IFNULL(MAX(updated_at), '') FROM agent_task`).Scan(&out.LastUpdated)
+	var beat sql.NullString
+	err = db.QueryRow(`SELECT heartbeat_at FROM agent_heartbeat WHERE id = 1`).Scan(&beat)
+	if err != nil || !beat.Valid || strings.TrimSpace(beat.String) == "" {
+		return out, nil
+	}
+	at, err := time.Parse(time.RFC3339, beat.String)
+	if err != nil {
+		return out, nil
+	}
+	if now.Sub(at) <= heartbeatFresh {
+		out.Status = "online"
+		return out, nil
+	}
+	out.Status = "offline"
 	return out, nil
 }
 
diff --git a/management/agentview/status_test.go b/management/agentview/status_test.go
new file mode 100644
index 0000000..f6a3c6b
--- /dev/null
+++ b/management/agentview/status_test.go
@@ -0,0 +1,39 @@
+package agentview
+
+import (
+	"database/sql"
+	"path/filepath"
+	"testing"
+	"time"
+
+	_ "modernc.org/sqlite"
+)
+
+func TestHeartbeatStatus(t *testing.T) {
+	dir := t.TempDir()
+	db, err := sql.Open("sqlite", filepath.Join(dir, "agent.db"))
+	if err != nil {
+		t.Fatal(err)
+	}
+	if _, err := db.Exec(`
+CREATE TABLE agent_task (task_id TEXT PRIMARY KEY, updated_at TEXT);
+CREATE TABLE agent_heartbeat (
+  id INTEGER PRIMARY KEY,
+  heartbeat_at TEXT
+);`); err != nil {
+		t.Fatal(err)
+	}
+	now := time.Date(2026, 11, 14, 3, 0, 0, 0, time.UTC)
+	if _, err := db.Exec(`INSERT INTO agent_heartbeat VALUES (1, ?)`, now.Format(time.RFC3339)); err != nil {
+		t.Fatal(err)
+	}
+	_ = db.Close()
+	st, err := readStatusAt(dir, now)
+	if err != nil || st.Status != "online" {
+		t.Fatalf("status = %+v err=%v", st, err)
+	}
+	st, err = readStatusAt(dir, now.Add(time.Minute))
+	if err != nil || st.Status != "offline" {
+		t.Fatalf("stale status = %+v err=%v", st, err)
+	}
+}
```
