package static

import (
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"fast-https/config"
	"fast-https/core"
	"fast-https/runtime/dynlog"
	"fast-https/runtime/listener"
	"fast-https/runtime/request"
	"fast-https/runtime/response"
)

func serveStaticRaw(t *testing.T, cfg *listener.ListenCfg, headers map[string]string) string {
	t.Helper()
	server, client := net.Pipe()
	t.Cleanup(func() { _ = client.Close() })
	ev := core.NewEvent(&listener.Listener{}, server)
	ev.RR.Req = request.RequestInit(false)
	ev.RR.Req.Method = "GET"
	ev.RR.Req.Path = "/a.txt"
	for k, v := range headers {
		ev.RR.Req.Headers[k] = v
	}
	ev.RR.ReqBuf = make([]byte, 4096)
	ev.RR.Res = response.ResponseInit()
	return readPipe(t, client, server, func() {
		if getResBytes(&dynlog.DynLogger{}, cfg, cfg.StaticRoot+"/a.txt", "close", ev) != 1 {
			t.Error("expected file")
		}
	})
}

// 200 响应带 Last-Modified；If-Modified-Since 不早于修改时间 → 304 无正文
func TestStaticConditionalRequest(t *testing.T) {
	dir := t.TempDir()
	writeStaticFile(t, dir, "a.txt", []byte("conditional-body"))
	cfg := &listener.ListenCfg{Path: "/", StaticRoot: dir, Zip: config.ZIP_NONE}

	first := serveStaticRaw(t, cfg, nil)
	lm := headerValue(first, "Last-Modified")
	if lm == "" {
		t.Fatalf("missing Last-Modified in %q", first)
	}
	// Date 头必须是 RFC 7231 格式（曾是 time.Now().String() 的违规输出）
	if d := headerValue(first, "Date"); d == "" {
		t.Fatal("missing Date header")
	} else if _, err := http.ParseTime(d); err != nil {
		t.Fatalf("Date %q is not RFC 7231: %v", d, err)
	}
	if !strings.Contains(first, "conditional-body") {
		t.Fatalf("first response missing body: %q", first)
	}

	ims := time.Now().UTC().Add(time.Hour).Format(http.TimeFormat)
	notMod := serveStaticRaw(t, cfg, map[string]string{"If-Modified-Since": ims})
	if !strings.Contains(notMod, "304 Not Modified") {
		t.Fatalf("want 304, got %q", firstLineOf(notMod))
	}
	if strings.Contains(notMod, "conditional-body") {
		t.Fatal("304 must not carry a body")
	}

	// 早于修改时间的条件 → 200 全量
	old := time.Now().UTC().Add(-time.Hour).Format(http.TimeFormat)
	modified := serveStaticRaw(t, cfg, map[string]string{"If-Modified-Since": old})
	if !strings.Contains(modified, "conditional-body") {
		t.Fatal("older If-Modified-Since should get a full 200")
	}
	_ = lm
}

func TestStaticRangeRequests(t *testing.T) {
	dir := t.TempDir()
	writeStaticFile(t, dir, "a.txt", []byte("0123456789"))
	cfg := &listener.ListenCfg{Path: "/", StaticRoot: dir, Zip: config.ZIP_NONE}

	// bytes=0-4
	r := serveStaticRaw(t, cfg, map[string]string{"Range": "bytes=0-4"})
	if !strings.Contains(r, "206 Partial Content") || !strings.Contains(r, "Content-Range: bytes 0-4/10") {
		t.Fatalf("range 0-4 response: %q", firstLineOf(r))
	}
	if !strings.Contains(r, "01234") || strings.Contains(r, "56789") {
		t.Fatalf("range 0-4 body wrong: %q", r)
	}

	// bytes=5-
	r = serveStaticRaw(t, cfg, map[string]string{"Range": "bytes=5-"})
	if !strings.Contains(r, "Content-Range: bytes 5-9/10") || !strings.Contains(r, "56789") {
		t.Fatalf("open range response: %q", r)
	}

	// -3 后缀
	r = serveStaticRaw(t, cfg, map[string]string{"Range": "bytes=-3"})
	if !strings.Contains(r, "Content-Range: bytes 7-9/10") || !strings.Contains(r, "789") {
		t.Fatalf("suffix range response: %q", r)
	}

	// 越界 → 416
	r = serveStaticRaw(t, cfg, map[string]string{"Range": "bytes=100-200"})
	if !strings.Contains(r, "416 Range Not Satisfiable") || !strings.Contains(r, "Content-Range: bytes */10") {
		t.Fatalf("out-of-range response: %q", firstLineOf(r))
	}

	// 多段范围 → 忽略 Range，200 全量
	r = serveStaticRaw(t, cfg, map[string]string{"Range": "bytes=0-1,3-4"})
	if strings.Contains(r, "Content-Range") || !strings.Contains(r, "0123456789") {
		t.Fatalf("multi-range should fall back to 200: %q", firstLineOf(r))
	}
}

// 带压缩配置时 Range 请求优先于压缩（不做 br/gzip）
func TestStaticRangeSkipsCompression(t *testing.T) {
	dir := t.TempDir()
	writeStaticFile(t, dir, "a.txt", []byte("0123456789"))
	cfg := &listener.ListenCfg{Path: "/", StaticRoot: dir, Zip: config.ZIP_BR}

	r := serveStaticRaw(t, cfg, map[string]string{
		"Range":           "bytes=0-4",
		"Accept-Encoding": "br, gzip",
	})
	if strings.Contains(r, "Content-Encoding") {
		t.Fatalf("range response must not be compressed: %q", firstLineOf(r))
	}
	if !strings.Contains(r, "01234") {
		t.Fatalf("range body missing: %q", r)
	}
}

func headerValue(resp, name string) string {
	for _, line := range strings.Split(firstLineOf(resp), "\r\n") {
		if strings.HasPrefix(strings.ToLower(line), strings.ToLower(name)+": ") {
			return strings.TrimSpace(line[len(name)+2:])
		}
	}
	return ""
}

func firstLineOf(resp string) string {
	if i := strings.Index(resp, "\r\n\r\n"); i >= 0 {
		return resp[:i]
	}
	return resp
}
