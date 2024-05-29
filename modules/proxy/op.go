package proxy

func (p *Proxy) read(data []byte) (int, error) {
	return p.ProxyConn.Read(data)
}

func (p *Proxy) write(data []byte) (int, error) {
	return p.ProxyConn.Write(data)
}

func (p *Proxy) close() error {
	return p.ProxyConn.Close()
}

func (p *Proxy) readSSL(data []byte) (int, error) {
	return p.ProxyTlsConn.Read(data)
}

func (p *Proxy) writeSSL(data []byte) (int, error) {
	return p.ProxyTlsConn.Write(data)
}

func (p *Proxy) closeSSL() error {
	return p.ProxyTlsConn.Close()
}
