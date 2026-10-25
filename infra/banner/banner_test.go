package banner

import (
	"bytes"
	"strings"
	"testing"

	"github.com/fatih/color"
)

// 交换 color.Output 捕获输出，用完恢复
func capture(t *testing.T, fn func()) string {
	t.Helper()
	old := color.Output
	oldNoColor := color.NoColor
	t.Cleanup(func() {
		color.Output = old
		color.NoColor = oldNoColor
	})
	buf := &bytes.Buffer{}
	color.Output = buf
	fn()
	return buf.String()
}

// 非 TTY（NoColor）路径：纯文本，无 ANSI 转义，含版本号
func TestPrintLogoNoColor(t *testing.T) {
	color.NoColor = true
	out := capture(t, func() { PrintLogo("V1.3.2") })

	if strings.Contains(out, "\x1b[") {
		t.Error("NoColor mode must not emit ANSI escape sequences")
	}
	if !strings.Contains(out, "V1.3.2") {
		t.Errorf("logo should contain the injected version, got %q", out)
	}
	if !strings.Contains(out, "███████╗") {
		t.Error("logo art missing")
	}
	if !strings.Contains(out, "gitee.com/ncepu-bj/fast-https") {
		t.Error("logo should contain the project links")
	}
}

// 强制彩色路径：出现 256 色 SGR 序列（38;5;N 渐变）
func TestPrintLogoColored(t *testing.T) {
	color.NoColor = false
	out := capture(t, func() { PrintLogo("V1.3.2") })

	if !strings.Contains(out, "\x1b[38;5;51m") {
		t.Error("gradient start color (256-color 51) missing")
	}
	if !strings.Contains(out, "\x1b[38;5;21m") {
		t.Error("gradient end color (256-color 21) missing")
	}
	if !strings.Contains(out, "V1.3.2") {
		t.Error("version missing in colored output")
	}
}

// 阶段分隔线可用
func TestStageSeparators(t *testing.T) {
	out := capture(t, func() {
		PrintInitialStart()
		PrintPortsListenerStart()
		PrintInitialEnd()
	})
	for _, want := range []string{"System Initialization Start", "Port Listening Start", "System Initialization End"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q", want)
		}
	}
}
