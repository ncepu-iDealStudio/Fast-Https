package response

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// Date 头必须是 RFC 7231 的 IMF-fixdate，可被标准解析器读取
func TestCachedDateFormat(t *testing.T) {
	d := CachedDate()
	if _, err := http.ParseTime(d); err != nil {
		t.Fatalf("CachedDate() = %q is not RFC 7231 parseable: %v", d, err)
	}
	if !strings.HasSuffix(d, " GMT") {
		t.Errorf("CachedDate() = %q, want GMT suffix", d)
	}
}

// 同一秒内的调用必须复用同一个字符串（跨秒边界由条件判断容错）
func TestCachedDateReusedWithinSecond(t *testing.T) {
	a := CachedDate()
	sec := time.Now().Unix()
	b := CachedDate()
	if time.Now().Unix() == sec && a != b {
		t.Errorf("same second returned different dates: %q vs %q", a, b)
	}
}
