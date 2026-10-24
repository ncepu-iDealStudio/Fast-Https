package logging_test

import (
	"encoding/json"
	"fast-https/infra/logging"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAllChannelsEmitJSON(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(logging.CloseLogFiles)
	if err := logging.Open(dir); err != nil {
		t.Fatal(err)
	}

	logging.PrintInfo("system-line")
	logging.PrintErr("error-line")
	logging.Safe().Warn().Str(logging.FieldReason, "rate_limit").Msg("safe-line")

	cases := []struct {
		file    string
		kind    string
		message string
		level   string
	}{
		{logging.SystemLogName, logging.KindSystem, "system-line", "info"},
		{logging.ErrorLogName, logging.KindError, "error-line", "error"},
		{logging.SafeLogName, logging.KindSafe, "safe-line", "warn"},
	}
	for _, c := range cases {
		record := firstRecord(t, filepath.Join(dir, c.file))
		if record["kind"] != c.kind {
			t.Errorf("%s kind = %v, want %q", c.file, record["kind"], c.kind)
		}
		if record["message"] != c.message {
			t.Errorf("%s message = %v, want %q", c.file, record["message"], c.message)
		}
		if record["level"] != c.level {
			t.Errorf("%s level = %v, want %q", c.file, record["level"], c.level)
		}
		if _, ok := record["time"].(string); !ok {
			t.Errorf("%s should carry a string time field", c.file)
		}
	}
}

func TestAccessEventFields(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(logging.CloseLogFiles)
	if err := logging.Open(dir); err != nil {
		t.Fatal(err)
	}

	logging.AccessEvent().
		Str(logging.FieldRemoteAddr, "1.2.3.4:5678").
		Str(logging.FieldMethod, "GET").
		Str(logging.FieldPath, "/index.html").
		Int(logging.FieldStatus, 503).
		Int(logging.FieldSize, 1024).
		Send()

	record := firstRecord(t, filepath.Join(dir, logging.AccessLogName))
	if record["kind"] != logging.KindAccess {
		t.Errorf("kind = %v", record["kind"])
	}
	for key, want := range map[string]string{
		logging.FieldRemoteAddr: "1.2.3.4:5678",
		logging.FieldMethod:     "GET",
		logging.FieldPath:       "/index.html",
	} {
		if record[key] != want {
			t.Errorf("%s = %v, want %q", key, record[key], want)
		}
	}
	// 状态码与长度必须是 JSON number，查询侧才能直接做区间比较。
	if n, ok := record[logging.FieldStatus].(float64); !ok || n != 503 {
		t.Errorf("status should be a number, got %T %v", record[logging.FieldStatus], record[logging.FieldStatus])
	}
	if n, ok := record[logging.FieldSize].(float64); !ok || n != 1024 {
		t.Errorf("size should be a number, got %T %v", record[logging.FieldSize], record[logging.FieldSize])
	}
}

func firstRecord(t *testing.T, path string) map[string]any {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.TrimSpace(string(body))
	if text == "" {
		t.Fatalf("%s is empty", path)
	}
	line := strings.Split(text, "\n")[0]

	var record map[string]any
	if err := json.Unmarshal([]byte(line), &record); err != nil {
		t.Fatalf("%s is not JSON: %v (%q)", path, err, line)
	}
	return record
}
