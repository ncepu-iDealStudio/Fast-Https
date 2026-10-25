package rewrite

import (
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"fast-https/core"
	"fast-https/runtime/listener"
	"fast-https/runtime/request"
	"fast-https/runtime/response"
)

// ReWriteEvent 必须回 301 + Location，且连接随后关闭
func TestReWriteEventIssues301(t *testing.T) {
	server, client := net.Pipe()
	t.Cleanup(func() { _ = client.Close() })
	ev := core.NewEvent(&listener.Listener{}, server)
	ev.EventWrite = func(ev *core.Event, _ []byte) error {
		ev.Conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
		_, err := ev.Conn.Write(ev.ResponseBytes())
		return err
	}
	ev.RR.Req = request.RequestInit(false)
	ev.RR.Res = response.ResponseInit()

	done := make(chan string, 1)
	go func() {
		buf, _ := io.ReadAll(client)
		done <- string(buf)
	}()

	ReWriteEvent(&listener.ListenCfg{ReWrite: "https://example.com/"}, ev)

	select {
	case got := <-done:
		if !strings.Contains(got, "301") || !strings.Contains(got, "Location: https://example.com/") {
			t.Fatalf("redirect response: %q", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("301 response never arrived")
	}
}

func TestReWriteFilterAlwaysPasses(t *testing.T) {
	if !ReWriteFilter(&listener.ListenCfg{}, nil) {
		t.Fatal("rewrite filter must always pass")
	}
}
