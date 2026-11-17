package compress

import (
	"testing"

	"fast-https/config"
)

// TestWantedQValues 锁定 q 值协商：q 高者优先，q 相同按服务端偏好 br，
// 未显式列出的编码回落到 `*` 的 q。
func TestWantedQValues(t *testing.T) {
	cases := []struct {
		name   string
		zip    uint16
		accept string
		want   string
	}{
		{"q 高者优先 gzip", config.ZIP_GZIP_BR, "gzip;q=1, br;q=0.1", "gzip"},
		{"q 高者优先 br", config.ZIP_GZIP_BR, "br;q=0.9, gzip;q=0.2", "br"},
		{"q 相同按偏好", config.ZIP_GZIP_BR, "br;q=0.5, gzip;q=0.5", "br"},
		{"省略 q 视为 1", config.ZIP_GZIP_BR, "gzip, br;q=0.5", "gzip"},
		{"通配 q 参与比较", config.ZIP_GZIP_BR, "*;q=0.2, gzip;q=0.5", "gzip"},
		{"通配让 br 胜出", config.ZIP_GZIP_BR, "*;q=0.8, gzip;q=0.5", "br"},
		{"显式 q=0 覆盖通配", config.ZIP_GZIP_BR, "br;q=0, *;q=1", "gzip"},
		{"全拒绝", config.ZIP_GZIP_BR, "br;q=0, gzip;q=0", ""},
		{"未配置", config.ZIP_NONE, "gzip, br", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Wanted(c.zip, c.accept); got != c.want {
				t.Fatalf("Wanted(%v, %q) = %q, want %q", c.zip, c.accept, got, c.want)
			}
		})
	}
}

// TestMatchMediaTypeWildcards 覆盖精确、整类、部分与全局通配。
func TestMatchMediaTypeWildcards(t *testing.T) {
	cases := []struct {
		pattern string
		mime    string
		want    bool
	}{
		{"image/png", "image/png", true},
		{"image/png", "image/jpeg", false},
		{"image/*", "image/png", true},
		{"image/*", "video/mp4", false},
		{"*/*", "application/json", true},
		{"*", "anything/here", true},
		{"image/p*", "image/png", true},
		{"image/p*", "image/jpeg", false},
		{"Image/PNG", "image/png", true},
		{"image/[", "image/png", false},
		{"png", "image/png", false},
	}
	for _, c := range cases {
		t.Run(c.pattern+"/"+c.mime, func(t *testing.T) {
			if got := matchMediaType(c.pattern, c.mime); got != c.want {
				t.Fatalf("matchMediaType(%q, %q) = %v, want %v", c.pattern, c.mime, got, c.want)
			}
		})
	}
}

// TestSkipBuiltinAndCustom 确认空清单用内置表，非空清单整体替换。
func TestSkipBuiltinAndCustom(t *testing.T) {
	if !Skip("image/png; charset=binary", nil) {
		t.Fatal("内置清单应跳过 png")
	}
	if Skip("text/plain", nil) {
		t.Fatal("内置清单不应跳过 text/plain")
	}
	if !Skip("application/json", []string{"application/j*"}) {
		t.Fatal("自定义部分通配应命中")
	}
	if Skip("image/png", []string{"text/*"}) {
		t.Fatal("自定义清单应整体替换内置表")
	}
}
