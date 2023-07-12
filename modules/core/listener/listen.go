package listener

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
	Lfd   int
}

func Listen() []ListenInfo {
	lisi := make([]ListenInfo, 4)
	lisi[0].Lfd = UnixListen("tcp", "127.0.0.1:8080")
	lisi[0].laddr = "127.0.0.1"
	lisi[0].lport = 8080
	lisi[0].Ltype = HTTP

	return lisi
}
