package static

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/gzip"

	"fast-https/config"
	"fast-https/runtime/listener"
)

func writeStaticFile(t *testing.T, dir, name string, content []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), content, 0o644); err != nil {
		t.Fatal(err)
	}
}

// ZIP_BR：客户端接受 br 时用 brotli 压缩
func TestStaticBrotliWhenConfiguredAndAccepted(t *testing.T) {
	dir := t.TempDir()
	raw := []byte(strings.Repeat("hello-brotli-body ", 50))
	writeStaticFile(t, dir, "a.txt", raw)
	cfg := &listener.ListenCfg{Path: "/", StaticRoot: dir, Zip: config.ZIP_BR}

	resp := serveStatic(t, cfg, "GET", "br, gzip")
	if !strings.Contains(resp, "Content-Encoding: br") {
		t.Fatalf("missing Content-Encoding: br in %q", firstLine(resp))
	}
	decodeBody(t, resp, "br", raw)
}

// ZIP_BR：客户端只认 gzip 时不压缩（br 配置不回退到 gzip）
func TestStaticBrotliNotServedWithoutBrAccept(t *testing.T) {
	dir := t.TempDir()
	writeStaticFile(t, dir, "a.txt", []byte("plain-body"))
	cfg := &listener.ListenCfg{Path: "/", StaticRoot: dir, Zip: config.ZIP_BR}

	resp := serveStatic(t, cfg, "GET", "gzip")
	if strings.Contains(resp, "Content-Encoding") {
		t.Fatalf("unexpected Content-Encoding in %q", firstLine(resp))
	}
	if !strings.Contains(resp, "plain-body") {
		t.Fatal("body should be sent uncompressed")
	}
}

// ZIP_GZIP_BR：优先 br，客户端不认 br 时回退 gzip
func TestStaticNegotiationPrefersBrotli(t *testing.T) {
	dir := t.TempDir()
	raw := []byte(strings.Repeat("negotiated-body ", 50))
	writeStaticFile(t, dir, "a.txt", raw)
	cfg := &listener.ListenCfg{Path: "/", StaticRoot: dir, Zip: config.ZIP_GZIP_BR}

	both := serveStatic(t, cfg, "GET", "gzip, br")
	if !strings.Contains(both, "Content-Encoding: br") {
		t.Fatalf("want br for accept %q, got %q", "gzip, br", firstLine(both))
	}
	decodeBody(t, both, "br", raw)

	gzipOnly := serveStatic(t, cfg, "GET", "gzip")
	if !strings.Contains(gzipOnly, "Content-Encoding: gzip") {
		t.Fatalf("want gzip fallback, got %q", firstLine(gzipOnly))
	}
	decodeBody(t, gzipOnly, "gzip", raw)
}

// ZIP_GZIP：只配 gzip 时，客户端只发 br 不会被压缩
func TestStaticGzipConfigIgnoresBrOnlyClient(t *testing.T) {
	dir := t.TempDir()
	writeStaticFile(t, dir, "a.txt", []byte("gzip-only-body"))
	cfg := &listener.ListenCfg{Path: "/", StaticRoot: dir, Zip: config.ZIP_GZIP}

	resp := serveStatic(t, cfg, "GET", "br")
	if strings.Contains(resp, "Content-Encoding") {
		t.Fatalf("unexpected Content-Encoding in %q", firstLine(resp))
	}
}

// 配置了压缩的 location 响应必须带 Vary: Accept-Encoding
func TestStaticSetsVaryWhenZipConfigured(t *testing.T) {
	dir := t.TempDir()
	writeStaticFile(t, dir, "a.txt", []byte("vary-body"))
	cfg := &listener.ListenCfg{Path: "/", StaticRoot: dir, Zip: config.ZIP_BR}

	resp := serveStatic(t, cfg, "GET", "gzip")
	if !strings.Contains(resp, "Vary: Accept-Encoding") {
		t.Fatalf("missing Vary header in %q", firstLine(resp))
	}
}

func firstLine(resp string) string {
	if i := strings.Index(resp, "\r\n\r\n"); i >= 0 {
		return resp[:i]
	}
	return resp
}

// 按内容编码解出响应体并与原始内容比对
func decodeBody(t *testing.T, resp, encoding string, want []byte) {
	t.Helper()
	parts := strings.SplitN(resp, "\r\n\r\n", 2)
	if len(parts) != 2 {
		t.Fatalf("response %q has no body separator", firstLine(resp))
	}
	var (
		got []byte
		err error
	)
	switch encoding {
	case "br":
		got, err = io.ReadAll(brotli.NewReader(strings.NewReader(parts[1])))
	case "gzip":
		zr, zerr := gzip.NewReader(strings.NewReader(parts[1]))
		if zerr != nil {
			t.Fatalf("gzip reader: %v", zerr)
		}
		defer zr.Close()
		got, err = io.ReadAll(zr)
	default:
		t.Fatalf("unknown encoding %q", encoding)
	}
	if err != nil {
		t.Fatalf("decode %s body: %v", encoding, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("decoded body %q, want %q", got, want)
	}
}
