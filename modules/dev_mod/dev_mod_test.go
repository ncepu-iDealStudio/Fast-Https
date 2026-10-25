package devmod

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

func TestDevEventServesDevBody(t *testing.T) {
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

	DevEvent(&listener.ListenCfg{}, ev)

	select {
	case got := <-done:
		if !strings.Contains(got, "200") || !strings.Contains(got, "this is dev mod") {
			t.Fatalf("dev response: %q", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("dev response never arrived")
	}
}

func TestDevFilterAlwaysPasses(t *testing.T) {
	if !DevFilter(&listener.ListenCfg{}, nil) {
		t.Fatal("dev filter must always pass")
	}
}
