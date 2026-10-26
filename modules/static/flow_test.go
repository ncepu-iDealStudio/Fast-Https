package static

import (
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fast-https/core"
	"fast-https/runtime/dynlog"
	"fast-https/runtime/listener"
	"fast-https/runtime/request"
	"fast-https/runtime/response"
)

// newStaticEvent 构造一个可写出响应的静态请求事件
func newStaticEvent(t *testing.T, method, path string) (*core.Event, net.Conn) {
	t.Helper()
	server, client := net.Pipe()
	t.Cleanup(func() { _ = client.Close() })
	ev := core.NewEvent(&listener.Listener{}, server)
	ev.EventWrite = func(ev *core.Event, _ []byte) error {
		ev.Conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
		_, err := ev.Conn.Write(ev.ResponseBytes())
		return err
	}
	ev.RR.Req = request.RequestInit(false)
	ev.RR.Req.Method = method
	ev.RR.Req.Path = path
	ev.RR.ReqBuf = make([]byte, 4096)
	ev.RR.Res = response.ResponseInit()
	return ev, client
}

// startReader 先起读取协程再返回收集函数：net.Pipe 无缓冲，
// handler 的同步写必须有并发读者才能完成
func startReader(t *testing.T, client net.Conn) func() string {
	t.Helper()
	done := make(chan string, 1)
	go func() {
		client.SetReadDeadline(time.Now().Add(2 * time.Second))
		buf, _ := io.ReadAll(client)
		done <- string(buf)
	}()
	return func() string {
		t.Helper()
		select {
		case got := <-done:
			return got
		case <-time.After(3 * time.Second):
			t.Fatal("response never arrived")
			return ""
		}
	}
}

// HandelSlash：目录路径缺尾部斜杠时 301 补齐（只在 location 非 / 且无剩余路径时）
func TestHandelSlashRedirectsMissingTrailingSlash(t *testing.T) {
	ev, client := newStaticEvent(t, "GET", "/docs")
	ev.RR.PathLocation = []int{0, 5} // 匹配到 "/docs"，无剩余路径
	ev.RR.OriginPath = ""
	collect := startReader(t, client)

	if HandelSlash(&listener.ListenCfg{Path: "/docs"}, ev) {
		t.Fatal("request without trailing slash must be intercepted")
	}
	got := collect()
	if !strings.Contains(got, "301") || !strings.Contains(got, "Location: /docs/") {
		t.Fatalf("301 response: %q", got)
	}

	// 已有剩余路径（OriginPath 非空）或 location 就是 / 时直接放行
	ev2, _ := newStaticEvent(t, "GET", "/docs/a")
	ev2.RR.PathLocation = []int{0, 5}
	ev2.RR.OriginPath = "/a"
	if !HandelSlash(&listener.ListenCfg{Path: "/docs"}, ev2) {
		t.Fatal("request with remaining path must pass through")
	}
	ev3, _ := newStaticEvent(t, "GET", "/")
	if !HandelSlash(&listener.ListenCfg{Path: "/"}, ev3) {
		t.Fatal("root location must pass through")
	}
}

// StaticEvent：命中与 404 两条主路径 × keep-alive/关闭两种写法
func TestStaticEventServesAndMisses(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("static-index"), 0o644)
	cfg := &listener.ListenCfg{Path: "/", StaticRoot: dir, StaticIndex: []string{"index.html"}}

	t.Run("keepalive hit", func(t *testing.T) {
		ev, client := newStaticEvent(t, "GET", "/index.html")
		ev.RR.Req.Headers["Connection"] = "keep-alive"
		collect := startReader(t, client)
		StaticEvent(cfg, ev)
		got := collect()
		if !strings.Contains(got, "static-index") {
			t.Fatalf("body: %q", got)
		}
		if !ev.Reuse {
			t.Fatal("keepalive request should mark ev reusable")
		}
	})

	t.Run("close hit", func(t *testing.T) {
		ev, client := newStaticEvent(t, "GET", "/index.html")
		ev.RR.Req.Headers["Connection"] = "close"
		collect := startReader(t, client)
		StaticEvent(cfg, ev)
		got := collect()
		if !strings.Contains(got, "static-index") {
			t.Fatalf("body: %q", got)
		}
	})

	t.Run("missing file yields 404", func(t *testing.T) {
		ev, client := newStaticEvent(t, "GET", "/no-such-file")
		ev.RR.Req.Headers["Connection"] = "close"
		collect := startReader(t, client)
		StaticEvent(cfg, ev)
		got := collect()
		if !strings.Contains(got, "404") {
			t.Fatalf("expected 404, got %q", got)
		}
	})
}

// direct.go 的 event301：Location 与 Connection 头
func TestEvent301(t *testing.T) {
	ev, client := newStaticEvent(t, "GET", "/x")
	collect := startReader(t, client)
	event301(ev, "/x/")
	got := collect()
	if !strings.Contains(got, "301") ||
		!strings.Contains(got, "Location: /x/") ||
		!strings.Contains(got, "Connection: close") {
		t.Fatalf("301: %q", got)
	}
}

// 目录缺结尾斜杠：301 到带斜杠的地址（nginx try_files $uri/ 的行为），
// 否则 /about 这种链接会直接 404、页面里的相对链接也会失效。
func TestStaticEventRedirectsDirectoryWithoutTrailingSlash(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "about"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "about", "index.html"), []byte("about-page"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &listener.ListenCfg{Path: "/", StaticRoot: dir, StaticIndex: []string{"index.html"}}

	t.Run("目录缺斜杠 301 且保留 query", func(t *testing.T) {
		ev, client := newStaticEvent(t, "GET", "/about?from=home")
		ev.RR.Req.Headers["Connection"] = "close"
		collect := startReader(t, client)
		StaticEvent(cfg, ev)
		got := collect()
		if !strings.Contains(got, "301") || !strings.Contains(got, "Location: /about/?from=home") {
			t.Fatalf("301 response: %q", got)
		}
	})

	t.Run("带斜杠返回索引页", func(t *testing.T) {
		ev, client := newStaticEvent(t, "GET", "/about/")
		ev.RR.Req.Headers["Connection"] = "close"
		collect := startReader(t, client)
		StaticEvent(cfg, ev)
		if got := collect(); !strings.Contains(got, "about-page") {
			t.Fatalf("directory index: %q", got)
		}
	})

	t.Run("不存在的目录不重定向", func(t *testing.T) {
		ev, client := newStaticEvent(t, "GET", "/nope")
		ev.RR.Req.Headers["Connection"] = "close"
		collect := startReader(t, client)
		StaticEvent(cfg, ev)
		if got := collect(); !strings.Contains(got, "404") {
			t.Fatalf("missing path should be 404: %q", got)
		}
	})
}

var _ = dynlog.DynLogger{} // StaticEvent 依赖 dynlog 记录，保持引用一致
