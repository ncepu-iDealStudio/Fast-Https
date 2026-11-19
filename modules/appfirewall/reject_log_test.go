package appfirewall

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fast-https/infra/logging"
)

// R-10：WAF 拦截日志带 reject_reason / rule / remote_ip。
func TestWAFBlockLogCarriesUnifiedFields(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(logging.CloseLogFiles)
	if err := logging.Open(dir); err != nil {
		t.Fatal(err)
	}
	if err := BuildWAF(cfgWith([]string{"sql"}, nil)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ShutdownWAF)

	ev := testEvent("POST", "/login", "user=admin&pswd=' OR '1'='1")
	if Allowed(nil, ev) {
		t.Fatal("SQL 注入应被 WAF 拦截")
	}

	raw, err := os.ReadFile(filepath.Join(dir, logging.SafeLogName))
	if err != nil {
		t.Fatal(err)
	}
	out := string(raw)
	for _, want := range []string{
		`"reject_reason":"waf"`,
		`"rule":"waf"`,
		`"remote_ip":"203.0.113.7"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("WAF 拒绝日志缺少 %s：%s", want, out)
		}
	}
}
