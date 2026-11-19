package filters

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fast-https/config"
	"fast-https/core"
	"fast-https/infra/logging"
	"fast-https/runtime/listener"
	"fast-https/runtime/request"
)

// R-10：host_check 的 warn 与 enforce 都要留下统一字段；
// warn 只告警，enforce 会真正拒绝（421）。
func TestHostCheckCarriesUnifiedFields(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode string
		want bool
	}{
		{"warn 放行但记录", config.HostCheckWarn, false},
		{"enforce 拒绝并记录", config.HostCheckEnforce, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Cleanup(logging.CloseLogFiles)
			if err := logging.Open(dir); err != nil {
				t.Fatal(err)
			}

			prev := *config.Current()
			next := prev
			next.Security = &config.SecurityConfig{HostCheck: tc.mode}
			config.Store(&next, config.MIME())
			t.Cleanup(func() { config.Store(&prev, config.MIME()) })

			server, client := net.Pipe()
			t.Cleanup(func() { _ = client.Close(); _ = server.Close() })
			ev := core.NewEvent(&listener.Listener{Port: "18080", HostMap: map[string][]listener.ListenCfg{}}, server)
			ev.RR.Req = request.RequestInit(false)
			ev.RR.Req.Headers["Host"] = "ghost.example"

			if got := HostCheckRejects(ev); got != tc.want {
				t.Fatalf("HostCheckRejects = %v, want %v", got, tc.want)
			}

			raw, err := os.ReadFile(filepath.Join(dir, logging.SafeLogName))
			if err != nil {
				t.Fatal(err)
			}
			out := string(raw)
			for _, want := range []string{
				`"reason":"host_check"`,
				`"reject_reason":"host_check"`,
				`"rule":"security.host_check"`,
				`"remote_ip":"pipe"`,
			} {
				if !strings.Contains(out, want) {
					t.Fatalf("host_check 日志缺少 %s：%s", want, out)
				}
			}
		})
	}
}
