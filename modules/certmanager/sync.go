package certmanager

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fast-https/config"
	"path/filepath"
	"strings"
)

// normalizeName 统一域名大小写与结尾的点，避免 acme.example 与
// ACME.example. 被当成两个名字分别申请。
func normalizeName(name string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(name), "."))
}

// certPaths 返回某个域名在数据面实际读取的 PEM 路径。
// 取配置里对应 server 块的 ssl_certificate/ssl_certificate_key，
// 因为监听器 reload 读的是这两个路径，写到别处等于没换证书。
// 多域名 server 块：每个域名分别申请一张证书，按域名返回兜底路径
// config/cert/<domain>.pem，loadCerts 会加载所有域名的证书。
func certPaths(domain string) (certPath, keyPath string) {
	for _, server := range config.Current().Servers {
		if !strings.Contains(server.Listen, "ssl") {
			continue
		}
		// 多域名：检查该 server 块的任意域名是否匹配。
		matched := false
		for _, raw := range server.ServerNames() {
			if normalizeName(raw) == domain {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		// 多域名 server 块：每个域名用兜底路径（按域名命名），
		// 不用 SSLCertificate 指向的路径（那是第一个域名的）。
		if len(server.ServerNames()) > 1 {
			return filepath.Join(config.CERT_DIR, domain) + ".pem",
				filepath.Join(config.CERT_DIR, domain) + "-key.pem"
		}
		if server.SSLCertificate == "" || server.SSLCertificateKey == "" {
			continue
		}
		return server.SSLCertificate, server.SSLCertificateKey
	}
	// 兜底与 CertInit 的命名保持一致：config/cert/<server_name>.pem。
	return filepath.Join(config.CERT_DIR, domain) + ".pem",
		filepath.Join(config.CERT_DIR, domain) + "-key.pem"
}

// pemPair 把证书链与私钥编码成 PEM。
func pemPair(certDERs [][]byte, privateKey any) (certPEM, keyPEM []byte, err error) {
	if len(certDERs) == 0 {
		return nil, nil, errors.New("acme: empty certificate chain")
	}
	for _, der := range certDERs {
		certPEM = append(certPEM, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})...)
	}

	switch key := privateKey.(type) {
	case *ecdsa.PrivateKey:
		der, err := x509.MarshalECPrivateKey(key)
		if err != nil {
			return nil, nil, err
		}
		keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})
	default:
		der, err := x509.MarshalPKCS8PrivateKey(privateKey)
		if err != nil {
			return nil, nil, err
		}
		keyPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	}
	return certPEM, keyPEM, nil
}

