package auth

import (
	"encoding/base64"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"fast-https/config"
	"fast-https/core"
	"fast-https/runtime/listener"
	"fast-https/runtime/request"
	"fast-https/runtime/response"
)

func basic(user, pswd string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pswd))
}

// 畸形 Authorization 头必须安全拒绝，不允许任何 panic/越界路径
func TestParseBasicAuthMalformed(t *testing.T) {
	cases := []string{
		"",
		"Basic",
		basic("u", "p") + " extra",
		"Bearer " + base64.StdEncoding.EncodeToString([]byte("u:p")),
		"Basic !!!not-base64!!!",
		"Basic " + base64.StdEncoding.EncodeToString([]byte("no-colon-here")),
	}
	for _, c := range cases {
		if _, _, ok := parseBasicAuth(c); ok {
			t.Errorf("parseBasicAuth(%q) accepted, want rejected", c)
		}
	}
}

func TestParseBasicAuthValid(t *testing.T) {
	user, pswd, ok := parseBasicAuth(basic("alice", "s3cret"))
	if !ok || user != "alice" || pswd != "s3cret" {
		t.Fatalf("got %q/%q ok=%v", user, pswd, ok)
	}
	// scheme 大小写不敏感，用户名含冒号时密码取余下全部
	user, pswd, ok = parseBasicAuth(basic("a:b", "p"))
	if !ok || user != "a" || pswd != "b:p" {
		t.Fatalf("colon-in-user: got %q/%q", user, pswd)
	}
}

func newAuthEvent(t *testing.T, authType, user, pswd, header string) (*core.Event, net.Conn) {
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
	ev.RR.Req.Headers["Authorization"] = header
	ev.RR.Res = response.ResponseInit()
	ev.Conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	return ev, client
}

func TestAuthHandler(t *testing.T) {
	cfg := &listener.ListenCfg{Auth: config.PathAuth{AuthType: "Basic", User: "alice", Pswd: "s3cret"}}

	t.Run("no auth configured passes through", func(t *testing.T) {
		ev, _ := newAuthEvent(t, "", "", "", "")
		if !AuthHandler(&listener.ListenCfg{}, ev) {
			t.Fatal("empty auth config must pass")
		}
	})

	t.Run("wrong and missing credentials rejected with 401", func(t *testing.T) {
		for _, hdr := range []string{"", basic("alice", "wrong"), " garbage "} {
			ev, client := newAuthEvent(t, "Basic", "alice", "s3cret", hdr)

			// net.Pipe 无缓冲：必须先有读者，AuthHandler 的同步写才能完成
			done := make(chan string, 1)
			go func() {
				buf, _ := io.ReadAll(client)
				done <- string(buf)
			}()

			if AuthHandler(cfg, ev) {
				t.Fatalf("header %q must be rejected", hdr)
			}
			select {
			case got := <-done:
				if !strings.Contains(got, "401") || !strings.Contains(got, "WWW-Authenticate") {
					t.Fatalf("401 response malformed: %q", got)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("401 response never arrived")
			}
		}
	})

	t.Run("correct credentials pass", func(t *testing.T) {
		ev, _ := newAuthEvent(t, "Basic", "alice", "s3cret", basic("alice", "s3cret"))
		if !AuthHandler(cfg, ev) {
			t.Fatal("valid credentials must pass")
		}
	})
}

func TestAuthExists(t *testing.T) {
	if AuthExists(&listener.ListenCfg{}) {
		t.Fatal("empty AuthType means no auth")
	}
	if !AuthExists(&listener.ListenCfg{Auth: config.PathAuth{AuthType: "Basic"}}) {
		t.Fatal("AuthType set means auth active")
	}
}
