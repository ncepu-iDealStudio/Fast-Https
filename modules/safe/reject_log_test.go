package safe

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fast-https/core"
	"fast-https/infra/logging"
	"fast-https/runtime/listener"
)

// R-10：黑名单/限流类拒绝日志除了既有 reason，还要带 reject_reason、
// rule 与 remote_ip，统一按字段检索。
func TestLogRejectCarriesUnifiedFields(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(logging.CloseLogFiles)
	if err := logging.Open(dir); err != nil {
		t.Fatal(err)
	}

	server, client := net.Pipe()
	t.Cleanup(func() { _ = client.Close(); _ = server.Close() })
	ev := core.NewEvent(&listener.Listener{Port: "18080"}, server)

	logReject(ev, ReasonBlacklist, "security.blacklist")

	raw, err := os.ReadFile(filepath.Join(dir, logging.SafeLogName))
	if err != nil {
		t.Fatal(err)
	}
	out := string(raw)
	for _, want := range []string{
		`"reason":"blacklist"`,
		`"reject_reason":"blacklist"`,
		`"rule":"security.blacklist"`,
		`"remote_ip":"pipe"`,
		`"remote_addr":"pipe"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("拒绝日志缺少 %s：%s", want, out)
		}
	}
}
