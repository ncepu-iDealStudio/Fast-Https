package static

import (
	"os"
	"path/filepath"
	"runtime"
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

// R-10：symlink_policy=reject 拦下的 404 也要留下拒绝原因。
func TestSymlinkRejectLogCarriesUnifiedFields(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(logging.CloseLogFiles)
	if err := logging.Open(dir); err != nil {
		t.Fatal(err)
	}

	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skipf("创建符号链接失败：%v", err)
	}

	cfg := &listener.ListenCfg{StaticRoot: root, SymlinkPolicy: config.SYMLINK_REJECT}
	ev, _ := newStaticEvent(t, "GET", "/link/secret.txt")
	if got := getResBytes(&dynlog.DynLogger{}, cfg, filepath.Join(root, "link", "secret.txt"), "close", ev); got != -1 {
		t.Fatalf("symlink reject getResBytes = %d, want -1", got)
	}

	raw, err := os.ReadFile(filepath.Join(dir, logging.SafeLogName))
	if err != nil {
		t.Fatal(err)
	}
	out := string(raw)
	for _, want := range []string{
		`"reject_reason":"symlink_rejected"`,
		`"rule":"symlink_policy"`,
		`"remote_ip":"pipe"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("拒绝日志缺少 %s：%s", want, out)
		}
	}
}

// Windows 保留设备名不是路径穿越，日志用 invalid_path + 精确规则名。
func TestWindowsReservedNameRejectLog(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("仅 Windows 有保留设备名语义")
	}
	dir := t.TempDir()
	t.Cleanup(logging.CloseLogFiles)
	if err := logging.Open(dir); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	cfg := &listener.ListenCfg{StaticRoot: root}
	ev, _ := newStaticEvent(t, "GET", "/CON")
	if got := getResBytes(&dynlog.DynLogger{}, cfg, filepath.Join(root, "CON"), "close", ev); got != -1 {
		t.Fatalf("保留设备名 getResBytes = %d, want -1", got)
	}

	raw, err := os.ReadFile(filepath.Join(dir, logging.SafeLogName))
	if err != nil {
		t.Fatal(err)
	}
	out := string(raw)
	for _, want := range []string{
		`"reject_reason":"invalid_path"`,
		`"rule":"windows_reserved_name"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("拒绝日志缺少 %s：%s", want, out)
		}
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
