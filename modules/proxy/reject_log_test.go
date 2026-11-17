package proxy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fast-https/config"
	"fast-https/infra/logging"
	"fast-https/runtime/listener"
)

// R-10：代理响应按策略或协商跳过压缩时，同样写 compression_skipped，
// rule 与静态文件路径保持一致。
func TestCompressionSkipLogs(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(logging.CloseLogFiles)
	if err := logging.Open(dir); err != nil {
		t.Fatal(err)
	}

	body := []byte(strings.Repeat("proxy-compress-body ", 80))
	cases := []struct {
		name   string
		cfg    *listener.ListenCfg
		accept string
	}{
		{"低于阈值", &listener.ListenCfg{Zip: config.ZIP_GZIP, ZipMinLength: 4096}, "gzip"},
		{"已压缩类型", &listener.ListenCfg{Zip: config.ZIP_GZIP, ZipSkipTypes: []string{"text/plain"}}, "gzip"},
		{"客户端没要", &listener.ListenCfg{Zip: config.ZIP_GZIP}, ""},
	}
	for _, tc := range cases {
		ev := newCompressEvent("GET", tc.accept, "text/plain", body)
		compressResponse(tc.cfg, ev)
		if got := ev.RR.Res.Headers["Content-Encoding"]; got != "" {
			t.Fatalf("%s: 不应压缩，Content-Encoding = %q", tc.name, got)
		}
	}

	raw, err := os.ReadFile(filepath.Join(dir, logging.SafeLogName))
	if err != nil {
		t.Fatal(err)
	}
	out := string(raw)
	for _, want := range []string{
		`"reject_reason":"compression_skipped"`,
		`"rule":"zip_min_length"`,
		`"rule":"zip_skip_types"`,
		`"rule":"accept_encoding"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("代理压缩跳过日志缺少 %s：%s", want, out)
		}
	}
}
