package cmd_test

import (
	"fast-https/cmd/internal/configcmd"
	"fast-https/config"
	"path/filepath"
	"testing"
)

// TestResolvePathsAreAbsolute 锁住外部控制面依赖的契约：四个字段一律是绝对路径，
// 且末段与配置层常量一致。归档布局下常量是相对的，这里必须已经被补成绝对。
func TestResolvePathsAreAbsolute(t *testing.T) {
	paths := configcmd.ResolvePaths()

	cases := []struct {
		name string
		got  string
		want string
	}{
		{"config", paths.Config, config.CONFIG_FILE_PATH},
		{"cert", paths.Cert, config.CERT_DIR},
		{"log", paths.Log, config.DEFAULT_LOG_ROOT},
		{"pid", paths.Pid, config.PID_FILE},
		{"state", paths.State, config.STATE_DIR},
	}
	for _, c := range cases {
		if c.got == "" {
			t.Errorf("%s 路径为空", c.name)
			continue
		}
		if !filepath.IsAbs(c.got) {
			t.Errorf("%s 应为绝对路径，得到 %q", c.name, c.got)
		}
		if filepath.Base(c.got) != filepath.Base(c.want) {
			t.Errorf("%s 末段应为 %q，得到 %q", c.name, filepath.Base(c.want), filepath.Base(c.got))
		}
	}
}
