package certmanager

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"io/fs"
	"os"
	"strings"
	"time"
)

// ErrNoCertificate 表示文件里没有可解析的证书。
var ErrNoCertificate = errors.New("no usable certificate in file")

// Info 是证书文件里第一张证书（叶子证书）的摘要。不含任何私钥信息。
type Info struct {
	Subject    string
	Issuer     string
	Names      []string
	NotBefore  time.Time
	NotAfter   time.Time
	SelfSigned bool
	// LocalCA 表示由本项目的本地 CA 签发（颁发者组织为 fast-https）。
	LocalCA bool
	// Chain 是文件里证书的张数。
	Chain int

	leaf *x509.Certificate
}

// Inspect 读取证书文件并解析第一张证书。文件不存在时返回的错误满足 errors.Is(err, fs.ErrNotExist)；
// 文件里没有证书块或解析失败时返回 ErrNoCertificate 或 x509 的解析错误。
func Inspect(path string) (*Info, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var leaf *x509.Certificate
	chain := 0
	for rest := raw; len(rest) > 0; {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, err
		}
		if leaf == nil {
			leaf = cert
		}
		chain++
	}
	if leaf == nil {
		return nil, ErrNoCertificate
	}

	info := &Info{
		Subject:    subjectOf(leaf),
		Issuer:     issuerOf(leaf),
		NotBefore:  leaf.NotBefore,
		NotAfter:   leaf.NotAfter,
		SelfSigned: bytes.Equal(leaf.RawIssuer, leaf.RawSubject),
		Chain:      chain,
		leaf:       leaf,
	}
	info.Names = append(info.Names, leaf.DNSNames...)
	for _, ip := range leaf.IPAddresses {
		info.Names = append(info.Names, ip.String())
	}
	for _, org := range leaf.Issuer.Organization {
		if strings.EqualFold(org, "fast-https") {
			info.LocalCA = true
		}
	}
	return info, nil
}

func subjectOf(c *x509.Certificate) string {
	if c.Subject.CommonName != "" {
		return c.Subject.CommonName
	}
	if len(c.DNSNames) > 0 {
		return c.DNSNames[0]
	}
	return c.Subject.String()
}

func issuerOf(c *x509.Certificate) string {
	if c.Issuer.CommonName != "" {
		return c.Issuer.CommonName
	}
	return c.Issuer.String()
}

// DaysLeft 返回距离到期的整天数（向零取整），已过期时为负数或零。
func (i *Info) DaysLeft(now time.Time) int {
	return int(i.NotAfter.Sub(now) / (24 * time.Hour))
}

// VerifyHostname 检查证书是否覆盖指定域名（含通配符规则）。
func (i *Info) VerifyHostname(host string) error {
	return i.leaf.VerifyHostname(host)
}

// KeyPairState 是证书与私钥配对检查的结果。只用固定的枚举值，
// 不把底层错误文本带出去，避免任何私钥相关内容进入输出。
type KeyPairState string

const (
	KeyPairOK             KeyPairState = "ok"
	KeyPairInvalid        KeyPairState = "invalid"
	KeyPairKeyMissing     KeyPairState = "key_missing"
	KeyPairKeyUnreadable  KeyPairState = "key_unreadable"
	KeyPairCertUnreadable KeyPairState = "cert_unreadable"
)

// CheckKeyPair 判断证书文件与私钥文件是否配对。私钥只在内存里参与一次配对计算，
// 不会被返回或写到任何地方。
func CheckKeyPair(certPath, keyPath string) KeyPairState {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return KeyPairCertUnreadable
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return KeyPairKeyMissing
		}
		return KeyPairKeyUnreadable
	}
	if _, err := tls.X509KeyPair(certPEM, keyPEM); err != nil {
		return KeyPairInvalid
	}
	return KeyPairOK
}
