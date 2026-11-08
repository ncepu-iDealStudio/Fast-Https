package cmd

import (
	"os"
	"sort"
	"strings"
	"testing"

	"fast-https/config"
)

// 桌面端与 systemd 依赖下面这些旧命令的输出与退出码，CLI 改造期间必须保持不变。
// 这里锁住的是对外契约，不是实现细节。

// config path --json 原有五个键保持不变：桌面端按键解析，多一个 state 不影响。
func TestLegacyConfigPathJSONKeys(t *testing.T) {
	out, _, code := runCLI(t, "config", "path", "--json")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}

	doc := decodeOneDocument(t, out)
	legacy := []string{"cert", "config", "log", "pid", "root_crt"}
	for _, k := range legacy {
		if s, _ := doc[k].(string); s == "" {
			t.Errorf("%s 不应为空", k)
		}
	}
	if s, _ := doc["state"].(string); s == "" {
		t.Fatal("state 不应为空")
	}
	if len(doc) != 6 {
		var keys []string
		for k := range doc {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		t.Fatalf("config path --json 的键 = %v, want 原五个加上 state", keys)
	}
}

// config path 的文本形态也是契约：每行 "名字 路径"。
func TestLegacyConfigPathText(t *testing.T) {
	out, _, code := runCLI(t, "config", "path")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	for _, name := range []string{"config", "cert", "log", "pid", "root_crt", "state"} {
		if !strings.Contains(out, name) {
			t.Errorf("文本输出缺少 %q：\n%s", name, out)
		}
	}
}

// 桌面端取 version 输出的第一行当版本号显示。
func TestLegacyVersionOutput(t *testing.T) {
	out, _, code := runCLI(t, "version")
	want := "fast-https " + config.CURRENT_VERSION + "\n"
	if code != 0 || out != want {
		t.Fatalf("exit=%d out=%q, want %q", code, out, want)
	}
}

func skipIfPidFilePresent(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(config.PID_FILE); err == nil {
		t.Skipf("%s 已存在，跳过依赖「没有 pid 文件」的用例", config.PID_FILE)
	}
}

// status 在没有 pid 文件时返回错误（退出码 1），文案含 "not running"。
func TestLegacyStatusWhenNotRunning(t *testing.T) {
	skipIfPidFilePresent(t)

	_, errOut, code := runCLI(t, "status")
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.HasPrefix(errOut, "Error: ") || !strings.Contains(errOut, "not running") {
		t.Fatalf("stderr = %q", errOut)
	}
}

// stop 在没有 pid 文件时按「没在跑」处理并成功：桌面端 stopServer 依赖这一点。
func TestLegacyStopWhenNotRunning(t *testing.T) {
	skipIfPidFilePresent(t)

	_, _, code := runCLI(t, "stop")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
}
