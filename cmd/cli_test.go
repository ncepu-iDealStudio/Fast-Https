package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"fast-https/config"
	"fast-https/infra/logging"

	"github.com/spf13/cobra"
)

// runCLI 走和 main 相同的入口（runWith），返回 stdout、stderr 与退出码。
func runCLI(t *testing.T, args ...string) (stdout, stderr string, exit int) {
	t.Helper()
	return runCLIWith(t, nil, args...)
}

// runCLIWith 允许在执行前往命令树里加测试专用命令。
func runCLIWith(t *testing.T, mutate func(root *cobra.Command), args ...string) (string, string, int) {
	t.Helper()
	root := RootCmdForLang(LangZH)
	if mutate != nil {
		mutate(root)
	}
	t.Cleanup(func() {
		currentLang = LangZH
		logging.SetConsoleOutput(os.Stdout)
	})

	var out, errOut bytes.Buffer
	code := runWith(root, args, &out, &errOut)
	return out.String(), errOut.String(), code
}

// decodeOneDocument 要求 stdout 恰好是一个 JSON 文档，前后没有多余字节。
func decodeOneDocument(t *testing.T, raw string) map[string]any {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(raw))
	var doc map[string]any
	if err := dec.Decode(&doc); err != nil {
		t.Fatalf("stdout 不是 JSON：%v\n%s", err, raw)
	}
	if err := dec.Decode(&map[string]any{}); err != io.EOF {
		t.Fatalf("stdout 在第一个 JSON 文档之后还有内容：\n%s", raw)
	}
	return doc
}

func TestEnvelopeSuccess(t *testing.T) {
	out, _, code := runCLI(t, "service", "version", "--json", "--request-id", "req-1")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	doc := decodeOneDocument(t, out)

	if doc["ok"] != true || doc["command"] != "service.version" || doc["request_id"] != "req-1" {
		t.Fatalf("封套字段不对：%v", doc)
	}
	if _, ok := doc["data"].(map[string]any); !ok {
		t.Fatalf("data 应为对象：%v", doc["data"])
	}
	if w, ok := doc["warnings"].([]any); !ok || w == nil {
		t.Fatalf("warnings 必须是数组而不是 null：%v", doc["warnings"])
	}
	if _, has := doc["error"]; has {
		t.Fatal("成功封套不应带 error")
	}
}

func TestEnvelopeError(t *testing.T) {
	out, _, code := runCLI(t, "service", "version", "--bogus", "--json")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	doc := decodeOneDocument(t, out)

	if doc["ok"] != false || doc["command"] != "service.version" {
		t.Fatalf("封套字段不对：%v", doc)
	}
	body, ok := doc["error"].(map[string]any)
	if !ok || body["code"] != CodeUsage || body["message"] == "" {
		t.Fatalf("error 不对：%v", doc["error"])
	}
	if d, ok := body["details"].([]any); !ok || d == nil {
		t.Fatalf("details 必须是数组：%v", body["details"])
	}
	if _, has := doc["data"]; has {
		t.Fatal("失败封套不应带 data")
	}
}

func TestExitCodeTable(t *testing.T) {
	cases := map[string]int{
		CodeInternal:         1,
		CodeUsage:            2,
		CodePermissionDenied: 3,
		CodeServiceDown:      4,
		CodeConfigInvalid:    5,
		CodeOperationFailed:  6,
		CodeRecoveryRequired: 6,
		CodeOperationUnknown: 7,
		CodeTimeout:          8,
		CodeNotFound:         9,
		CodeConflict:         10,
		CodeRequestIDClash:   10,
		CodeUnmetDependency:  11,
		CodeNotSupported:     11,
	}
	for code, want := range cases {
		if got := ExitCode(newCLIError(code, "x")); got != want {
			t.Errorf("%s 的退出码 = %d, want %d", code, got, want)
		}
	}
	if got := ExitCode(nil); got != 0 {
		t.Errorf("nil 的退出码 = %d, want 0", got)
	}
	if got := ExitCode(io.ErrUnexpectedEOF); got != 1 {
		t.Errorf("普通错误的退出码 = %d, want 1", got)
	}
	if got := ExitCode(newCLIError("NO_SUCH_CODE", "x")); got != 1 {
		t.Errorf("未知错误码的退出码 = %d, want 1", got)
	}
}

// 表里每个错误码都必须有退出码，防止新增常量忘了登记。
func TestEveryErrorCodeHasExitCode(t *testing.T) {
	for _, code := range []string{
		CodeInternal, CodeUsage, CodePermissionDenied, CodeServiceDown, CodeConfigInvalid,
		CodeOperationFailed, CodeRecoveryRequired, CodeOperationUnknown, CodeTimeout,
		CodeNotFound, CodeConflict, CodeRequestIDClash, CodeUnmetDependency, CodeNotSupported,
	} {
		if _, ok := exitCodes[code]; !ok {
			t.Errorf("错误码 %s 没有退出码", code)
		}
	}
}

// 配置加载等路径会写控制台日志；--json 下它们必须去 stderr，stdout 只剩封套。
func TestJSONModeStdoutIsOneDocument(t *testing.T) {
	noisy := func(root *cobra.Command) {
		root.AddCommand(&cobra.Command{
			Use: "zz-noisy",
			RunE: func(cmd *cobra.Command, args []string) error {
				logging.ConsoleLog.Info().Msg("noise-from-config-load")
				return emitOK(cmd, "zz.noisy", map[string]any{}, nil)
			},
		})
	}

	out, errOut, code := runCLIWith(t, noisy, "zz-noisy", "--json")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	decodeOneDocument(t, out)
	if strings.Contains(out, "noise-from-config-load") {
		t.Fatalf("日志混进了 stdout：\n%s", out)
	}
	if !strings.Contains(errOut, "noise-from-config-load") {
		t.Fatalf("日志应出现在 stderr：\n%s", errOut)
	}
}

func TestUnknownFlagIsUsageError(t *testing.T) {
	_, errOut, code := runCLI(t, "service", "version", "--bogus")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.HasPrefix(errOut, "Error: ") {
		t.Fatalf("文本模式应与 cobra 一致，以 \"Error: \" 开头：%q", errOut)
	}
}

func TestExtraPositionalArgIsUsageError(t *testing.T) {
	if _, _, code := runCLI(t, "version", "extra"); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if _, _, code := runCLI(t, "no-such-command"); code != 2 {
		t.Fatalf("未知命令 exit = %d, want 2", code)
	}
}

func TestNegativeTimeoutIsUsageError(t *testing.T) {
	out, _, code := runCLI(t, "service", "version", "--timeout", "-5s", "--json")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	doc := decodeOneDocument(t, out)
	if doc["error"].(map[string]any)["code"] != CodeUsage {
		t.Fatalf("应为 USAGE_ERROR：%v", doc)
	}
}

func TestCLIErrorKeepsItsCode(t *testing.T) {
	failing := func(root *cobra.Command) {
		root.AddCommand(&cobra.Command{
			Use: "zz-fail",
			RunE: func(cmd *cobra.Command, args []string) error {
				e := newCLIError(CodeConfigInvalid, "config is broken")
				e.Command = "zz.fail"
				e.Details = []ErrorDetail{{Path: "http.server[0]", Message: "bad listen"}}
				return e
			},
		})
	}

	out, _, code := runCLIWith(t, failing, "zz-fail", "--json")
	if code != 5 {
		t.Fatalf("exit = %d, want 5", code)
	}
	doc := decodeOneDocument(t, out)
	body := doc["error"].(map[string]any)
	if doc["command"] != "zz.fail" || body["code"] != CodeConfigInvalid {
		t.Fatalf("错误封套不对：%v", doc)
	}
	if d := body["details"].([]any); len(d) != 1 {
		t.Fatalf("details = %v", d)
	}
}

func TestServiceVersionJSON(t *testing.T) {
	out, _, code := runCLI(t, "service", "version", "--json")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	data := decodeOneDocument(t, out)["data"].(map[string]any)
	if data["cli_version"] != config.CURRENT_VERSION {
		t.Fatalf("cli_version = %v, want %s", data["cli_version"], config.CURRENT_VERSION)
	}
	if v, has := data["server_version"]; !has || v != nil {
		t.Fatalf("server_version 应存在且为 null：%v", data)
	}
}

func TestServiceVersionText(t *testing.T) {
	out, _, code := runCLI(t, "service", "version")
	if code != 0 || out != "fast-https "+config.CURRENT_VERSION+"\n" {
		t.Fatalf("exit=%d out=%q", code, out)
	}
}
