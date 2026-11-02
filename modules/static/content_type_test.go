package static

import (
	"testing"

	"fast-https/config"
)

// http.default_type 必须作用到未知扩展名的响应上，而不是固定返回内置常量。
func TestGetContentTypeUsesConfiguredDefault(t *testing.T) {
	previous := *config.Current()
	previousMIME := config.MIME()
	t.Cleanup(func() { config.Store(&previous, previousMIME) })

	config.Store(
		&config.Fast_Https{DefaultType: "text/plain; charset=utf-8"},
		map[string]string{"css": "text/css"},
	)

	if got := getContentType("/static/app.unknownext"); got != "text/plain; charset=utf-8" {
		t.Fatalf("unknown extension = %q, want configured default_type", got)
	}
	if got := getContentType("/static/no-extension"); got != "text/plain; charset=utf-8" {
		t.Fatalf("no extension = %q, want configured default_type", got)
	}
	if got := getContentType("/static/trailing."); got != "text/plain; charset=utf-8" {
		t.Fatalf("trailing dot = %q, want configured default_type", got)
	}
	if got := getContentType("/static/app.css"); got != "text/css" {
		t.Fatalf("mime table hit = %q, want text/css", got)
	}
	if got := getContentType("/static/app.css?v=2"); got != "text/css" {
		t.Fatalf("query string = %q, want text/css", got)
	}
}

// 扩展名大小写不该决定 Content-Type：表里键是小写，IMG.PNG 也要命中。
func TestGetContentTypeIgnoresExtensionCase(t *testing.T) {
	previous := *config.Current()
	previousMIME := config.MIME()
	t.Cleanup(func() { config.Store(&previous, previousMIME) })

	config.Store(
		&config.Fast_Https{},
		map[string]string{"css": "text/css", "png": "image/png"},
	)

	cases := []struct {
		path string
		want string
	}{
		{path: "/static/app.CSS", want: "text/css"},
		{path: "/static/app.Css", want: "text/css"},
		{path: "/static/app.CSS?v=2", want: "text/css"},
		{path: "/static/logo.PNG", want: "image/png"},
	}
	for _, c := range cases {
		if got := getContentType(c.path); got != c.want {
			t.Errorf("getContentType(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}

// 未加载配置（快照是零值）时回落内置常量，不能返回空串。
func TestGetContentTypeFallsBackToBuiltin(t *testing.T) {
	previous := *config.Current()
	previousMIME := config.MIME()
	t.Cleanup(func() { config.Store(&previous, previousMIME) })

	config.Store(&config.Fast_Https{}, map[string]string{})

	if got := getContentType("/static/app.unknownext"); got != config.HTTP_DEFAULT_CONTENT_TYPE {
		t.Fatalf("empty config = %q, want %q", got, config.HTTP_DEFAULT_CONTENT_TYPE)
	}
}
