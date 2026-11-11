package cli

import (
	"errors"
	"strings"
	"testing"
)

func TestFriendlyMessage(t *testing.T) {
	cases := []struct {
		name, msg, cmd, want string
	}{
		{"runtime unreachable", "SERVICE_UNREACHABLE: runtime snapshot unreachable: Get \"http://127.0.0.1:9088/runtime\": dial tcp connectex: No connection could be made", "runtime status", "运行快照暂不可用"},
		{"no server configured", "no server configured: add http.server or a server file under http.include", "site disable", "停用唯一站点"},
		{"already enabled", "CONFLICT: site conflict: site default is already enabled", "site enable", "已经是启用状态"},
		{"already disabled", "CONFLICT: site conflict: site default is already disabled", "site disable", "已经是停用状态"},
		{"not found", "NOT_FOUND: site not found: foo", "site show", "找不到该站点"},
		{"not supported inline", "NOT_SUPPORTED: site foo is inline in the main config (http.server)", "site update", "内联在主配置"},
		{"log missing", "LOG_SOURCE_UNAVAILABLE: log file not found: access.log", "logs access", "日志文件还不存在"},
		{"permission", "PERMISSION_DENIED: Access is denied", "config apply", "权限不足"},
		{"timeout", "TIMEOUT: context deadline exceeded", "runtime status", "操作超时"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := FriendlyError(errors.New(c.msg), c.cmd)
			if !strings.Contains(got, c.want) {
				t.Errorf("FriendlyError(%q) = %q, want containing %q", c.msg, got, c.want)
			}
			// 人话提示必须中英双语（\n 分隔），且不能把整段 Go 错误原样甩出
			if !strings.Contains(got, "\n") {
				t.Errorf("提示应含中英双语：%q", got)
			}
			if strings.Contains(got, "dial tcp connectex") {
				t.Errorf("不应保留 Go 技术细节：%q", got)
			}
		})
	}
}

func TestFriendlyMessageUnmatchedClips(t *testing.T) {
	long := strings.Repeat("x", 400)
	got := FriendlyMessage(long, "some cmd")
	if len([]rune(got)) > 265 {
		t.Errorf("未知错误应截断到 260 字符，得到 %d", len([]rune(got)))
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("截断应以省略号结尾：%q", got)
	}
}

func TestFriendlyErrorNil(t *testing.T) {
	if got := FriendlyError(nil, "x"); got != "" {
		t.Errorf("nil error 应返回空串，得到 %q", got)
	}
}
