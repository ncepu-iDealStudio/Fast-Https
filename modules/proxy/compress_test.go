package proxy

import (
	"bytes"
	"compress/gzip"
	"io"
	"strconv"
	"strings"
	"testing"

	"fast-https/config"
	"fast-https/core"
	"fast-https/runtime/listener"
	"fast-https/runtime/request"
	"fast-https/runtime/response"
)

// newCompressEvent 构造一条只用于压缩策略测试的响应事件。
func newCompressEvent(method, accept, contentType string, body []byte) *core.Event {
	ev := core.NewEvent(&listener.Listener{}, &stubConn{})
	ev.RR.Req = request.RequestInit(false)
	ev.RR.Req.Method = method
	ev.RR.Req.Headers["Accept-Encoding"] = accept
	ev.RR.Res = response.ResponseInit()
	ev.RR.Res.SetFirstLine(200, "OK")
	if contentType != "" {
		ev.RR.Res.SetHeader("Content-Type", contentType)
	}
	ev.RR.Res.SetBody(body)
	return ev
}

// TestProxyCompressResponse 覆盖代理响应的压缩条件与头部改写。
func TestProxyCompressResponse(t *testing.T) {
	raw := []byte(strings.Repeat("proxy-compress-body ", 80))

	t.Run("gzip 压缩并改写长度", func(t *testing.T) {
		ev := newCompressEvent("GET", "gzip", "text/plain", raw)
		cfg := &listener.ListenCfg{Zip: config.ZIP_GZIP}
		compressResponse(cfg, ev)

		if got := ev.RR.Res.Headers["Content-Encoding"]; got != "gzip" {
			t.Fatalf("Content-Encoding = %q", got)
		}
		if got := ev.RR.Res.Headers["Content-Length"]; got != strconv.Itoa(len(ev.RR.Res.Body)) {
			t.Fatalf("Content-Length = %q, body = %d", got, len(ev.RR.Res.Body))
		}
		if !strings.Contains(ev.RR.Res.Headers["Vary"], "Accept-Encoding") {
			t.Fatalf("Vary = %q", ev.RR.Res.Headers["Vary"])
		}
		zr, err := gzip.NewReader(bytes.NewReader(ev.RR.Res.Body))
		if err != nil {
			t.Fatal(err)
		}
		defer zr.Close()
		got, err := io.ReadAll(zr)
		if err != nil || !bytes.Equal(got, raw) {
			t.Fatalf("解压后内容不一致: %v", err)
		}
	})

	t.Run("按 q 值选择", func(t *testing.T) {
		ev := newCompressEvent("GET", "br;q=0.1, gzip;q=1", "text/plain", raw)
		compressResponse(&listener.ListenCfg{Zip: config.ZIP_GZIP_BR}, ev)
		if got := ev.RR.Res.Headers["Content-Encoding"]; got != "gzip" {
			t.Fatalf("应选 q 更高的 gzip，实得 %q", got)
		}
	})

	t.Run("上游已压缩不二次压缩", func(t *testing.T) {
		ev := newCompressEvent("GET", "gzip", "text/plain", raw)
		ev.RR.Res.SetHeader("Content-Encoding", "br")
		compressResponse(&listener.ListenCfg{Zip: config.ZIP_GZIP}, ev)
		if got := ev.RR.Res.Headers["Content-Encoding"]; got != "br" {
			t.Fatalf("Content-Encoding 被改写为 %q", got)
		}
		if !bytes.Equal(ev.RR.Res.Body, raw) {
			t.Fatal("正文不应变化")
		}
	})

	t.Run("已压缩类型跳过", func(t *testing.T) {
		ev := newCompressEvent("GET", "gzip", "image/png", raw)
		compressResponse(&listener.ListenCfg{Zip: config.ZIP_GZIP}, ev)
		if got := ev.RR.Res.Headers["Content-Encoding"]; got != "" {
			t.Fatalf("png 不应压缩，得到 %q", got)
		}
	})

	t.Run("部分通配跳过", func(t *testing.T) {
		ev := newCompressEvent("GET", "gzip", "application/json", raw)
		compressResponse(&listener.ListenCfg{Zip: config.ZIP_GZIP, ZipSkipTypes: []string{"application/j*"}}, ev)
		if got := ev.RR.Res.Headers["Content-Encoding"]; got != "" {
			t.Fatalf("部分通配应跳过，得到 %q", got)
		}
	})

	t.Run("小于阈值跳过", func(t *testing.T) {
		ev := newCompressEvent("GET", "gzip", "text/plain", []byte("tiny"))
		compressResponse(&listener.ListenCfg{Zip: config.ZIP_GZIP, ZipMinLength: 1024}, ev)
		if got := ev.RR.Res.Headers["Content-Encoding"]; got != "" {
			t.Fatalf("小响应不应压缩，得到 %q", got)
		}
	})

	t.Run("HEAD 不压", func(t *testing.T) {
		ev := newCompressEvent("HEAD", "gzip", "text/plain", raw)
		compressResponse(&listener.ListenCfg{Zip: config.ZIP_GZIP}, ev)
		if got := ev.RR.Res.Headers["Content-Encoding"]; got != "" {
			t.Fatalf("HEAD 不应压缩，得到 %q", got)
		}
	})

	t.Run("206 不压", func(t *testing.T) {
		ev := newCompressEvent("GET", "gzip", "text/plain", raw)
		ev.RR.Res.SetFirstLine(206, "Partial Content")
		compressResponse(&listener.ListenCfg{Zip: config.ZIP_GZIP}, ev)
		if got := ev.RR.Res.Headers["Content-Encoding"]; got != "" {
			t.Fatalf("206 不应压缩，得到 %q", got)
		}
	})

	t.Run("websocket 不压", func(t *testing.T) {
		ev := newCompressEvent("GET", "gzip", "text/plain", raw)
		ev.Upgrade = "websocket"
		compressResponse(&listener.ListenCfg{Zip: config.ZIP_GZIP}, ev)
		if got := ev.RR.Res.Headers["Content-Encoding"]; got != "" {
			t.Fatalf("websocket 不应压缩，得到 %q", got)
		}
	})

	t.Run("chunked 改成长度分帧", func(t *testing.T) {
		ev := newCompressEvent("GET", "gzip", "text/plain", raw)
		ev.RR.Res.SetHeader("Transfer-Encoding", "chunked")
		compressResponse(&listener.ListenCfg{Zip: config.ZIP_GZIP}, ev)
		if ev.RR.Res.Headers["Transfer-Encoding"] != "" {
			t.Fatal("压缩后应移除 Transfer-Encoding")
		}
		if ev.RR.Res.Headers["Content-Length"] == "" {
			t.Fatal("压缩后应补 Content-Length")
		}
	})
}
