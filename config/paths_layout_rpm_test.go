//go:build rpm

package config

import "testing"

// 包安装布局：四个基址分属 /etc、/usr/share、/var/log、/run，且都拼出
// 绝对路径，不依赖进程工作目录。
func TestPackageLayoutPaths(t *testing.T) {
	cases := []struct{ name, got, want string }{
		{"FAST_HTTPS_ETC_DIR", FAST_HTTPS_ETC_DIR, "/etc/fast-https"},
		{"FAST_HTTPS_DATA_DIR", FAST_HTTPS_DATA_DIR, "/usr/share/fast-https"},
		{"FAST_HTTPS_LOG_DIR", FAST_HTTPS_LOG_DIR, "/var/log/fast-https"},
		{"FAST_HTTPS_RUN_DIR", FAST_HTTPS_RUN_DIR, "/run/fast-https"},
		{"FAST_HTTPS_STATE_DIR", FAST_HTTPS_STATE_DIR, "/var/lib/fast-https"},
		{"CONFIG_FILE_PATH", CONFIG_FILE_PATH, "/etc/fast-https/fast-https.json"},
		{"MIME_FILE_PATH", MIME_FILE_PATH, "/etc/fast-https/mime.json"},
		{"CERT_DIR", CERT_DIR, "/etc/fast-https/cert"},
		{"ROOT_CRT_DIR", ROOT_CRT_DIR, "/usr/share/fast-https/httpdoc/root"},
		{"PID_FILE", PID_FILE, "/run/fast-https/fast-https.pid"},
		{"DEFAULT_LOG_ROOT", DEFAULT_LOG_ROOT, "/var/log/fast-https"},
		{"STATE_DIR", STATE_DIR, "/var/lib/fast-https"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}
}
