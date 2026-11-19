package events

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fast-https/infra/logging"
)

// R-10：header 431 / body 413 的拒绝日志必须带统一的 reject_reason、
// rule 与 remote_ip 字段，且不记录原始请求路径。
func TestRejectLogsCarryUnifiedFields(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(logging.CloseLogFiles)
	if err := logging.Open(dir); err != nil {
		t.Fatal(err)
	}

	rejectHeaderTooLarge(newHandlerEvent(t, stubListener(), "GET", "/", "example.com"))
	rejectTooLargeBody(newHandlerEvent(t, stubListener(), "GET", "/", "example.com"))

	raw, err := os.ReadFile(filepath.Join(dir, logging.SafeLogName))
	if err != nil {
		t.Fatal(err)
	}
	out := string(raw)
	for _, want := range []string{
		`"reject_reason":"header_too_large"`,
		`"reject_reason":"body_too_large"`,
		`"rule":"max_header_size"`,
		`"rule":"max_body_size"`,
		`"remote_ip":"pipe"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("拒绝日志缺少 %s：%s", want, out)
		}
	}
	if strings.Contains(out, `"path"`) {
		t.Fatalf("拒绝日志不应记录原始路径：%s", out)
	}
}
