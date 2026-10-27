package proxy

// isClosed 报告连接是否已关闭，池归还前必须检查。
func (p *Proxy) isClosed() bool { return p.closed }

func (p *Proxy) read(data []byte) (int, error) {
	return p.ProxyConn.Read(data)
}

func (p *Proxy) write(data []byte) (int, error) {
	return p.ProxyConn.Write(data)
}

func (p *Proxy) close() error {
	p.closed = true
	return p.ProxyConn.Close()
}

func (p *Proxy) readSSL(data []byte) (int, error) {
	return p.ProxyTlsConn.Read(data)
}

func (p *Proxy) writeSSL(data []byte) (int, error) {
	return p.ProxyTlsConn.Write(data)
}

func (p *Proxy) closeSSL() error {
	p.closed = true
	return p.ProxyTlsConn.Close()
}
