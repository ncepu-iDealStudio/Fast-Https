package static

import (
	"bytes"
	"compress/gzip"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fast-https/config"
	"fast-https/modules/core"
	"fast-https/modules/core/dynlog"
	"fast-https/modules/core/listener"
	"fast-https/modules/core/request"
	"fast-https/modules/core/response"
)

func readPipe(t *testing.T, client net.Conn, server net.Conn, write func()) string {
	t.Helper()
	done := make(chan string, 1)
	go func() {
		_ = client.SetDeadline(time.Now().Add(2 * time.Second))
		buf, _ := io.ReadAll(client)
		done <- string(buf)
	}()
	write()
	_ = server.Close()
	return <-done
}

func TestStaticHeadOmitsBody(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("hello-body"), 0o644); err != nil {
		t.Fatal(err)
	}
	server, client := net.Pipe()
	t.Cleanup(func() { _ = client.Close() })
	ev := core.NewEvent(&listener.Listener{}, server)
	ev.RR.Req = request.RequestInit(false)
	ev.RR.Req.Method = "HEAD"
	ev.RR.Req.Path = "/index.html"
	ev.RR.ReqBuf = make([]byte, 4096)
	ev.RR.Res = response.ResponseInit()
	cfg := &listener.ListenCfg{Path: "/", StaticRoot: dir}

	got := readPipe(t, client, server, func() {
		if getResBytes(&dynlog.DynLogger{}, cfg, dir+"/index.html", "close", ev) != 1 {
			t.Error("expected file")
		}
	})
	if strings.Contains(got, "hello-body") {
		t.Fatalf("HEAD wrote the file body: %q", got)
	}
	if !strings.Contains(got, "Content-Length: 10") {
		t.Fatalf("HEAD length: %q", got)
	}
	if strings.Contains(got, "Content-Encoding") {
		t.Fatalf("HEAD encoded without gzip request: %q", got)
	}
}

func TestStaticGzipOnlyWhenRequested(t *testing.T) {
	dir := t.TempDir()
	raw := []byte("hello-body-gzip")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &listener.ListenCfg{Path: "/", StaticRoot: dir, Zip: config.ZIP_GZIP}

	plain := serveStatic(t, cfg, "GET", "")
	if strings.Contains(plain, "Content-Encoding") || !strings.Contains(plain, "hello-body-gzip") {
		t.Fatalf("plain response: %q", plain)
	}

	encoded := serveStatic(t, cfg, "GET", "gzip")
	if !strings.Contains(encoded, "Content-Encoding: gzip") {
		t.Fatalf("missing gzip header: %q", encoded)
	}
	parts := strings.SplitN(encoded, "\r\n\r\n", 2)
	if len(parts) != 2 {
		t.Fatalf("response %q", encoded)
	}
	reader, err := gzip.NewReader(bytes.NewReader([]byte(parts[1])))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, raw) {
		t.Fatalf("decoded %q", got)
	}
}

func TestNotFoundUsesConfiguredPage(t *testing.T) {
	dir := t.TempDir()
	page := filepath.Join(dir, "404.html")
	if err := os.WriteFile(page, []byte("<h1>missing</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := NotFoundResponse(&listener.ListenCfg{ErrorNotFound: page})
	if !bytes.Contains(res.Body, []byte("missing")) {
		t.Fatalf("body %q", res.Body)
	}
	fallback := NotFoundResponse(&listener.ListenCfg{ErrorNotFound: filepath.Join(dir, "nope.html")})
	if bytes.Contains(fallback.Body, []byte("missing")) {
		t.Fatal("missing file replaced the default page")
	}
}

func serveStatic(t *testing.T, cfg *listener.ListenCfg, method, accept string) string {
	t.Helper()
	server, client := net.Pipe()
	t.Cleanup(func() { _ = client.Close() })
	ev := core.NewEvent(&listener.Listener{}, server)
	ev.RR.Req = request.RequestInit(false)
	ev.RR.Req.Method = method
	ev.RR.Req.Path = "/a.txt"
	ev.RR.Req.Headers["Accept-Encoding"] = accept
	ev.RR.ReqBuf = make([]byte, 4096)
	ev.RR.Res = response.ResponseInit()
	return readPipe(t, client, server, func() {
		if getResBytes(&dynlog.DynLogger{}, cfg, cfg.StaticRoot+"/a.txt", "close", ev) != 1 {
			t.Error("expected file")
		}
	})
}
