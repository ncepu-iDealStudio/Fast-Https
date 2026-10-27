package listener

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"sync"
)

// certSet is the certificate list served by an already-open TLS listener.
// Reload replaces the list in place so the listening socket stays up.
type certSet struct {
	mu    sync.RWMutex
	certs []tls.Certificate
}

func newCertSet() *certSet {
	return &certSet{}
}

func (c *certSet) get(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	if c == nil {
		return nil, errors.New("no certificate")
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.certs) == 0 {
		return nil, errors.New("no certificate")
	}
	name := ""
	if hello != nil {
		name = hello.ServerName
	}
	for i := range c.certs {
		if name != "" && certMatches(c.certs[i], name) {
			chosen := c.certs[i]
			return &chosen, nil
		}
	}
	chosen := c.certs[0]
	return &chosen, nil
}

func (c *certSet) replace(cfgs []ListenCfg) error {
	loaded, err := loadCerts(cfgs)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.certs = loaded
	c.mu.Unlock()
	return nil
}

func loadCerts(cfgs []ListenCfg) ([]tls.Certificate, error) {
	var loaded []tls.Certificate
	seen := map[string]struct{}{}
	for _, item := range cfgs {
		if _, ok := seen[item.ServerName]; ok {
			continue
		}
		seen[item.ServerName] = struct{}{}
		crt, err := tls.LoadX509KeyPair(item.SSL.SslKey, item.SSL.SslValue)
		if err != nil {
			return nil, err
		}
		loaded = append(loaded, crt)
	}
	if len(loaded) == 0 {
		return nil, errors.New("ssl listen has no certificate")
	}
	return loaded, nil
}

func certMatches(cert tls.Certificate, serverName string) bool {
	if len(cert.Certificate) == 0 {
		return false
	}
	leaf := cert.Leaf
	if leaf == nil {
		parsed, err := x509.ParseCertificate(cert.Certificate[0])
		if err != nil {
			return false
		}
		leaf = parsed
	}
	return leaf.VerifyHostname(serverName) == nil
}
