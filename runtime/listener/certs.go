package listener

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"slices"
	"sync"

	"fast-https/config"
	"fast-https/infra/logging"
)

// certSet is the certificate list served by an already-open TLS listener.
// Reload replaces the list in place so the listening socket stays up.
type certSet struct {
	mu    sync.RWMutex
	certs []tls.Certificate
	// defaultCert 是 tls.default_certificate 指定的兜底证书，未配置为 nil。
	defaultCert *tls.Certificate
}

func newCertSet() *certSet {
	return &certSet{}
}

func (c *certSet) get(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	if c == nil {
		return nil, errors.New("no certificate")
	}
	// TLS-ALPN-01 挑战：客户端在 ALPN 里点名 acme-tls/1，此时必须返回
	// 控制面生成的临时证书，而不是站点证书。未命中就继续正常的选证书逻辑。
	if hello != nil && slices.Contains(hello.SupportedProtos, ACMEALPNProto) {
		if cert, ok := tlsALPNChallengeCert(hello); ok {
			return cert, nil
		}
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
	if name != "" {
		// 同一域名可能部署多张证书（典型是 RSA + ECDSA）。全部收集后按
		// 客户端能力挑一张，而不是永远用列表里第一张。
		var matches []*tls.Certificate
		for i := range c.certs {
			if certMatches(c.certs[i], name) {
				matches = append(matches, &c.certs[i])
			}
		}
		if len(matches) > 0 {
			chosen := *chooseCert(hello, matches)
			return &chosen, nil
		}
	}
	// 未知 SNI（含空 SNI）优先用显式兜底证书，保持握手指向运维指定的证书。
	if c.defaultCert != nil {
		chosen := *c.defaultCert
		return &chosen, nil
	}
	logUnknownSNI(name)
	chosen := c.certs[0]
	return &chosen, nil
}

// logUnknownSNI 记录一次未知 SNI 回落。字段与统一拒绝日志对齐
// （reject_reason=unknown_sni），便于按字段检索。
func logUnknownSNI(sni string) {
	logging.ConsoleLog.Warn().
		Str("reject_reason", "unknown_sni").
		Str("sni", sni).
		Msg("unknown TLS SNI: falling back to the first certificate " +
			"(set tls.default_certificate to choose an explicit fallback)")
}

// chooseCert 在同一域名命中的多张证书里按客户端能力挑一张。
//
// 典型场景是同一域名同时部署 RSA 与 ECDSA 两张证书文件：客户端只支持
// 其中一种签名算法时返回兼容的那张。没有 ClientHello 或客户端没带
// signature_algorithms 扩展时退回第一张，保持旧行为；全部不兼容时同样
// 退回第一张，让标准库按它自己的错误结束握手，而不是静默换一张。
func chooseCert(hello *tls.ClientHelloInfo, matches []*tls.Certificate) *tls.Certificate {
	if hello != nil && len(hello.SignatureSchemes) > 0 {
		for _, cert := range matches {
			if hello.SupportsCertificate(cert) == nil {
				return cert
			}
		}
	}
	return matches[0]
}

func (c *certSet) replace(cfgs []ListenCfg) error {
	loaded, err := loadCerts(cfgs)
	if err != nil {
		return err
	}
	def := loadDefaultCertificate()
	c.mu.Lock()
	c.certs = loaded
	c.defaultCert = def
	c.mu.Unlock()
	return nil
}

// loadDefaultCertificate 读取 tls.default_certificate。未配置返回 nil；
// 读取失败只告警——握手仍要能回落到第一张证书，不能因为兜底证书坏了
// 就让所有 TLS 端口起不来。配置校验阶段已经检查过文件存在。
func loadDefaultCertificate() *tls.Certificate {
	cfg := config.Current().TLS
	if !cfg.HasDefaultCertificate() {
		return nil
	}
	cert, err := tls.LoadX509KeyPair(cfg.DefaultCertificate.Certificate, cfg.DefaultCertificate.Key)
	if err != nil {
		logging.ConsoleLog.Warn().Err(err).
			Str("certificate", cfg.DefaultCertificate.Certificate).
			Msg("load tls.default_certificate failed, falling back to the first certificate")
		return nil
	}
	return &cert
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
		// 多域名：加载其余域名的证书，SNI 按域名匹配。
		for _, extra := range item.ExtraCerts {
			ecrt, err := tls.LoadX509KeyPair(extra.SslKey, extra.SslValue)
			if err != nil {
				return nil, err
			}
			loaded = append(loaded, ecrt)
		}
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
