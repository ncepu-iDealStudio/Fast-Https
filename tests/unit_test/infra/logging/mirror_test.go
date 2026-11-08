package logging_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fast-https/infra/logging"
)

func TestConsoleWarnMirroredToErrorLog(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(logging.CloseLogFiles)
	if err := logging.Open(dir); err != nil {
		t.Fatal(err)
	}

	logging.ConsoleLog.Error().Msg("reload kept previous config")
	logging.ConsoleLog.Warn().Msg("console-warn")

	records := allRecords(t, filepath.Join(dir, logging.ErrorLogName))
	if len(records) < 2 {
		t.Fatalf("error.log 行数 = %d, want >= 2", len(records))
	}
	foundErr, foundWarn := false, false
	for _, rec := range records {
		if rec["kind"] != logging.KindError {
			t.Errorf("kind = %v, want %q", rec["kind"], logging.KindError)
		}
		if rec["source"] != logging.SourceConsole {
			t.Errorf("source = %v, want %q", rec["source"], logging.SourceConsole)
		}
		switch rec["message"] {
		case "reload kept previous config":
			foundErr = true
			if rec["level"] != "error" {
				t.Errorf("error 行 level = %v", rec["level"])
			}
		case "console-warn":
			foundWarn = true
			if rec["level"] != "warn" {
				t.Errorf("warn 行 level = %v", rec["level"])
			}
		}
	}
	if !foundErr || !foundWarn {
		t.Fatalf("缺少镜像行：err=%v warn=%v records=%v", foundErr, foundWarn, records)
	}
}

func TestConsoleInfoNotMirrored(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(logging.CloseLogFiles)
	if err := logging.Open(dir); err != nil {
		t.Fatal(err)
	}

	logging.ConsoleLog.Info().Msg("console-info")
	logging.ConsoleLog.Debug().Msg("console-debug")
	logging.PrintErr("print-err")

	body := readLog(t, filepath.Join(dir, logging.ErrorLogName))
	if strings.Contains(body, "console-info") || strings.Contains(body, "console-debug") {
		t.Fatalf("Info/Debug 不应写入 error.log：%s", body)
	}
	if !strings.Contains(body, "print-err") {
		t.Fatalf("PrintErr 应仍写入 error.log：%s", body)
	}
}

func TestMirrorBeforeOpenIsNoop(t *testing.T) {
	logging.CloseLogFiles()
	logging.ConsoleLog.Error().Msg("before-open")

	dir := t.TempDir()
	t.Cleanup(logging.CloseLogFiles)
	if err := logging.Open(dir); err != nil {
		t.Fatal(err)
	}
	body := readLog(t, filepath.Join(dir, logging.ErrorLogName))
	if strings.Contains(body, "before-open") {
		t.Fatalf("Open 之前的控制台错误不应缓冲：%s", body)
	}
}

func allRecords(t *testing.T, path string) []map[string]any {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.TrimSpace(string(body))
	if text == "" {
		t.Fatalf("%s is empty", path)
	}
	var records []map[string]any
	for _, line := range strings.Split(text, "\n") {
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("%s is not JSON: %v (%q)", path, err, line)
		}
		records = append(records, rec)
	}
	return records
}
