package certmanager

import (
	"context"
	"crypto/tls"
	"errors"
	"fast-https/infra/logging"
	"fast-https/runtime/listener"
	"slices"
	"sync"

	"github.com/mholt/acmez/v3"
	"github.com/mholt/acmez/v3/acme"
)

// tlsalpn01Solver 生成 RFC 8737 要求的临时证书，注册到数据面的
// 挑战证书表。数据面在握手时按 SNI 查表，因此控制面不需要自己监听 443。
type tlsalpn01Solver struct {
	mu    sync.RWMutex
	certs map[string]*tls.Certificate
}

func newTLSALPN01Solver() *tlsalpn01Solver {
	return &tlsalpn01Solver{certs: make(map[string]*tls.Certificate)}
}

// Present 实现 acmez.Solver。
func (s *tlsalpn01Solver) Present(_ context.Context, chal acme.Challenge) error {
	cert, err := acmez.TLSALPN01ChallengeCert(chal)
	if err != nil {
		return err
	}
	name := normalizeName(chal.Identifier.Value)
	if name == "" {
		return errors.New("acme: tls-alpn-01 challenge without identifier")
	}
	s.mu.Lock()
	s.certs[name] = cert
	s.mu.Unlock()
	logging.ConsoleLog.Debug().Msgf("acme: tls-alpn-01 certificate ready for %s", name)
	return nil
}

// CleanUp 实现 acmez.Solver。
func (s *tlsalpn01Solver) CleanUp(_ context.Context, chal acme.Challenge) error {
	s.mu.Lock()
	delete(s.certs, normalizeName(chal.Identifier.Value))
	s.mu.Unlock()
	return nil
}

// certificate 供数据面握手时查询。只有客户端点名 acme-tls/1 才应答，
// 普通握手必须继续使用站点证书。
func (s *tlsalpn01Solver) certificate(hello *tls.ClientHelloInfo) (*tls.Certificate, bool) {
	if hello == nil || !slices.Contains(hello.SupportedProtos, listener.ACMEALPNProto) {
		return nil, false
	}
	name := normalizeName(hello.ServerName)
	if name == "" {
		return nil, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	cert, ok := s.certs[name]
	return cert, ok
}
