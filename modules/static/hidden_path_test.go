package static

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fast-https/core"
	"fast-https/runtime/dynlog"
	"fast-https/runtime/listener"
	"fast-https/runtime/request"
	"fast-https/runtime/response"
)

func TestHiddenPathSegment(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{path: "/", want: false},
		{path: "/index.html", want: false},
		{path: "/assets/app.3f9a2c.js", want: false},
		{path: "/docs/.gitignore", want: true},
		{path: "/.git/config", want: true},
		{path: "/.env", want: true},
		{path: "/a/.htaccess", want: true},
		{path: "/.git?ref=main", want: true},
		{path: "//.git/config", want: true},
		// ACME http-01 的公开目录必须放行，否则站点根下自托管的挑战文件取不到
		{path: "/.well-known/acme-challenge/token", want: false},
		{path: "/.well-known/", want: false},
		// 只放行一层：.well-known 里面再藏隐藏文件仍然拒绝
		{path: "/.well-known/.git/config", want: true},
		// ".." 由 staticPathInRoot 负责，不在这里重复判定
		{path: "/a/../b", want: false},
	}

	for _, c := range cases {
		if got := hiddenPathSegment(c.path); got != c.want {
			t.Errorf("hiddenPathSegment(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

// .git/config 里的内容不能通过静态服务读出来
func TestGetResBytesRejectsHiddenFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "config"), []byte("CANARY"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &listener.ListenCfg{Path: "/", StaticRoot: root}
	ev := &core.Event{RR: core.RRcircle{Req: request.RequestInit(false)}}
	ev.RR.Req.Path = "/.git/config"
	ev.RR.Res = response.ResponseInit()

	if got := getResBytes(&dynlog.DynLogger{}, cfg, root+"/.git/config", "close", ev); got != -1 {
		t.Fatalf("hidden file returned %d, want -1 (not found)", got)
	}
	if body := string(ev.RR.Res.GetBody()); body != "" {
		t.Fatalf("hidden file leaked into response body: %q", body)
	}
}

// 站点根本身位于带点目录（如 /home/user/.local/site）时不能误伤整站
func TestGetResBytesServesFilesWhenRootHasDottedParent(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, ".local", "site")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}

	server, client := net.Pipe()
	t.Cleanup(func() { _ = client.Close() })
	ev := core.NewEvent(&listener.Listener{}, server)
	ev.RR.Req = request.RequestInit(false)
	ev.RR.Req.Method = "GET"
	ev.RR.Req.Path = "/a.txt"
	ev.RR.ReqBuf = make([]byte, 4096)
	ev.RR.Res = response.ResponseInit()
	cfg := &listener.ListenCfg{Path: "/", StaticRoot: root}

	got := readPipe(t, client, server, func() {
		if res := getResBytes(&dynlog.DynLogger{}, cfg, root+"/a.txt", "close", ev); res != 1 {
			t.Errorf("normal file under a dotted parent returned %d, want 1", res)
		}
	})
	if !strings.Contains(got, "ok") {
		t.Fatalf("expected file body, got %q", got)
	}
}
