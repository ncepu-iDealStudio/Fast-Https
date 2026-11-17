package static

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fast-https/config"
	"fast-https/infra/logging"
	"fast-https/runtime/dynlog"
	"fast-https/runtime/listener"
)

// R-10：路径穿越与隐藏路径的拒绝日志带统一字段，且不记录原始路径。
func TestPathRejectLogsCarryUnifiedFields(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(logging.CloseLogFiles)
	if err := logging.Open(dir); err != nil {
		t.Fatal(err)
	}

	cfg := &listener.ListenCfg{StaticRoot: t.TempDir()}
	for _, path := range []string{"/../etc/passwd", "/.git/config"} {
		ev, _ := newStaticEvent(t, "GET", path)
		if got := getResBytes(&dynlog.DynLogger{}, cfg, path, "close", ev); got != -1 {
			t.Fatalf("%s: getResBytes = %d, want -1", path, got)
		}
	}

	raw, err := os.ReadFile(filepath.Join(dir, logging.SafeLogName))
	if err != nil {
		t.Fatal(err)
	}
	out := string(raw)
	for _, want := range []string{
		`"reject_reason":"path_traversal"`,
		`"rule":"static_root"`,
		`"reject_reason":"hidden_path"`,
		`"rule":"hidden_path_segment"`,
		`"remote_ip":"pipe"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("拒绝日志缺少 %s：%s", want, out)
		}
	}
	if strings.Contains(out, "etc/passwd") {
		t.Fatalf("拒绝日志不应记录原始路径：%s", out)
	}
}

// 压缩跳过原因决定 reject_reason=compression_skipped 的 rule 字段。
func TestCompressibleReason(t *testing.T) {
	cfg := &listener.ListenCfg{Zip: config.ZIP_GZIP, ZipMinLength: 100}
	cases := []struct {
		name        string
		cfg         *listener.ListenCfg
		contentType string
		size        int64
		want        string
	}{
		{"未启用压缩", &listener.ListenCfg{Zip: config.ZIP_NONE}, "text/html", 1000, "zip"},
		{"低于阈值", cfg, "text/html", 10, "zip_min_length"},
		{"已压缩类型", &listener.ListenCfg{Zip: config.ZIP_GZIP, ZipSkipTypes: []string{"image/*"}}, "image/png", 1000, "zip_skip_types"},
		{"可以压缩", cfg, "text/html", 1000, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := compressibleReason(tc.cfg, tc.contentType, tc.size); got != tc.want {
				t.Fatalf("compressibleReason = %q, want %q", got, tc.want)
			}
		})
	}
}
