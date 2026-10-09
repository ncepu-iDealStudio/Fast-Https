package server

import (
	"errors"
	"fast-https/modules/core/listener"
	"net"
	"testing"
	"time"
)

func TestReloadMissingCertKeepsListener(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	previous := reloadConfig
	reloadConfig = func() error {
		return errors.New("ssl_certificate not found: missing.pem")
	}
	t.Cleanup(func() { reloadConfig = previous })

	s := &Server{Listens: []listener.Listener{{Lfd: ln, Port: "18091"}}}
	s.Reload()

	conn, err := net.DialTimeout("tcp", ln.Addr().String(), time.Second)
	if err != nil {
		t.Fatalf("listener closed after rejected certificate reload: %v", err)
	}
	_ = conn.Close()
}
