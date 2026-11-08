//go:build !rpm

package config

import "testing"

// 归档 / Docker 布局：解压即用，所有路径都相对工作目录。
// 这些字面量是 旧版 单前缀布局的原值，任何一个改动都会让 tar.gz 用户
// 找不到配置文件或日志，因此必须逐字锁住。
func TestArchiveLayoutPaths(t *testing.T) {
	cases := []struct{ name, got, want string }{
		{"CONFIG_FILE_PATH", CONFIG_FILE_PATH, "./config/fast-https.json"},
		{"MIME_FILE_PATH", MIME_FILE_PATH, "./config/mime.json"},
		{"CERT_DIR", CERT_DIR, "./config/cert"},
		{"ROOT_CRT_DIR", ROOT_CRT_DIR, "./httpdoc/root"},
		{"PID_FILE", PID_FILE, "./fast-https.pid"},
		{"DEFAULT_LOG_ROOT", DEFAULT_LOG_ROOT, "./logs"},
		{"STATE_DIR", STATE_DIR, "./state"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}
}
