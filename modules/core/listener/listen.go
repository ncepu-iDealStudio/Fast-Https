package listener

import (
	"log"
	"net"
)

const (
	HTTP        = 1
	HTTPS       = 2
	HTTP_PROXY  = 3
	HTTPS_PROXY = 4
	TCP_PROXY   = 5
)

// one listen port arg
type ListenInfo struct {
	Ltype int
	laddr string
	lport int
	Lfd   net.Listener
}

func Listen() []ListenInfo {
	lisi := make([]ListenInfo, 4)

	lisi[0].Lfd = listen("tcp", "127.0.0.1:8080")
	lisi[0].laddr = "127.0.0.1"
	lisi[0].lport = 8080
	lisi[0].Ltype = HTTP

	lisi[1].Lfd = listen("tcp", "127.0.0.1:443")
	lisi[1].laddr = "127.0.0.1"
	lisi[1].lport = 443
	lisi[1].Ltype = HTTPS

	lisi[2].Lfd = listen("tcp", "127.0.0.1:9000")
	lisi[2].laddr = "127.0.0.1"
	lisi[2].lport = 9000
	lisi[2].Ltype = HTTP_PROXY

	lisi[3].Lfd = listen("tcp", "127.0.0.1:9090")
	lisi[3].laddr = "127.0.0.1"
	lisi[3].lport = 9090
	lisi[3].Ltype = TCP_PROXY

	return lisi
}

func listen(ltype string, laddr string) net.Listener {
	listener, err := net.Listen(ltype, laddr)
	if err != nil {
		log.Fatal("Error starting the server:", err)
	}
	return listener
}
