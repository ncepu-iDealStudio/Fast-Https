package init

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"errors"
	"fast-https/config"
	"fast-https/infra/logging"
	"fmt"
	"math/big"
	"net"
	"net/mail"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var userAndHostname string
var caCert *x509.Certificate
var caKey crypto.PrivateKey

// CertInit 生成缺失的本地 CA 与自签服务器证书，返回 error 而不是只往
// 错误日志写一行：装成发行版包后目录归 root 所有，普通用户直接跑 start
// 时写盘会失败，若在这里吞掉错误，用户只能看到后面那句莫名其妙的
// "ssl_certificate not found"。
func CertInit() error {
	userAndHostname = "fast-https@ncepu.edu.cn"
	if err := os.MkdirAll(config.CERT_DIR, 0o755); err != nil {
		return fmt.Errorf("create certificate directory %s: %w", config.CERT_DIR, err)
	}
	// CA 文件写在 ROOT_CRT_DIR 下，首次启动那个目录可能还不存在，
	// 不建目录会写失败，随后 loadCa 读到空内容直接空指针崩溃。
	if err := os.MkdirAll(config.ROOT_CRT_DIR, 0o755); err != nil {
		return fmt.Errorf("create CA directory %s: %w", config.ROOT_CRT_DIR, err)
	}

	file := filepath.Join(config.ROOT_CRT_DIR, config.ROOT_CRT_NAME) + ".crt"
	if _, err := os.Stat(file); os.IsNotExist(err) {
		if err := newRoot(); err != nil {
			return err
		}
	}

	if err := loadCa(); err != nil {
		return err
	}

	for _, serverconfig := range config.Current().Servers {
		if serverconfig.ServerName == "" || !strings.Contains(serverconfig.Listen, "ssl") {
			continue
		}

		certfile := filepath.Join(config.CERT_DIR, serverconfig.ServerName) + ".pem"
		if _, err := os.Stat(certfile); !os.IsNotExist(err) {
			continue
		}
		if err := newCert([]string{serverconfig.ServerName}); err != nil {
			return err
		}
		logging.PrintInfo(certfile, " created")
	}

	return nil
}

func randomSerialNumber() *big.Int {
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		logging.PrintErr(err, "failed to generate serial number")
	}

	return serialNumber
}

func newRoot() error {
	priv, err := rsa.GenerateKey(rand.Reader, 3072)
	if err != nil {
		return fmt.Errorf("generate CA key: %w", err)
	}
	pub := priv.Public()

	spkiASN1, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return fmt.Errorf("encode CA public key: %w", err)
	}
	var spki struct {
		Algorithm        pkix.AlgorithmIdentifier
		SubjectPublicKey asn1.BitString
	}
	if _, err := asn1.Unmarshal(spkiASN1, &spki); err != nil {
		return fmt.Errorf("decode CA public key: %w", err)
	}

	skid := sha1.Sum(spki.SubjectPublicKey.Bytes)

	tpl := &x509.Certificate{
		SerialNumber: randomSerialNumber(),
		Subject: pkix.Name{
			Organization:       []string{"fast-https"},
			OrganizationalUnit: []string{userAndHostname},

			CommonName: "fast-https " + userAndHostname,
		},
		SubjectKeyId: skid[:],

		NotAfter:  time.Now().AddDate(10, 0, 0),
		NotBefore: time.Now(),

		KeyUsage: x509.KeyUsageCertSign,

		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLenZero:        true,
	}

	cert, err := x509.CreateCertificate(rand.Reader, tpl, tpl, pub, priv)
	if err != nil {
		return fmt.Errorf("generate CA certificate: %w", err)
	}

	privDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return fmt.Errorf("encode CA key: %w", err)
	}

	keyPath := filepath.Join(config.ROOT_CRT_DIR, config.ROOT_CRT_NAME) + ".key"
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(
		&pem.Block{Type: "PRIVATE KEY", Bytes: privDER}), 0o400); err != nil {
		return fmt.Errorf("save CA key %s: %w", keyPath, err)
	}

	crtPath := filepath.Join(config.ROOT_CRT_DIR, config.ROOT_CRT_NAME) + ".crt"
	if err := os.WriteFile(crtPath, pem.EncodeToMemory(
		&pem.Block{Type: "CERTIFICATE", Bytes: cert}), 0o644); err != nil {
		return fmt.Errorf("save CA certificate %s: %w", crtPath, err)
	}

	logging.PrintInfo("Created a new local CA")
	return nil
}

// loadCa 读取本地 CA 证书与私钥。
// 读不出来就返回错误：调用方必须停下，不能带着 nil 的 caCert/caKey
// 继续签发证书（旧实现在这里会解引用空的 PEM 块直接 panic）。
func loadCa() error {
	crtPath := filepath.Join(config.ROOT_CRT_DIR, config.ROOT_CRT_NAME) + ".crt"
	certPEMBlock, err := os.ReadFile(crtPath)
	if err != nil {
		return fmt.Errorf("read CA certificate %s: %w", crtPath, err)
	}

	certDERBlock, _ := pem.Decode(certPEMBlock)
	if certDERBlock == nil || certDERBlock.Type != "CERTIFICATE" {
		return fmt.Errorf("read CA certificate %s: unexpected content", crtPath)
	}
	caCert, err = x509.ParseCertificate(certDERBlock.Bytes)
	if err != nil {
		return fmt.Errorf("parse CA certificate %s: %w", crtPath, err)
	}

	keyPath := filepath.Join(config.ROOT_CRT_DIR, config.ROOT_CRT_NAME) + ".key"
	keyPEMBlock, err := os.ReadFile(keyPath)
	if err != nil {
		return fmt.Errorf("read CA key %s: %w", keyPath, err)
	}
	keyDERBlock, _ := pem.Decode(keyPEMBlock)
	if keyDERBlock == nil || keyDERBlock.Type != "PRIVATE KEY" {
		return fmt.Errorf("read CA key %s: unexpected content", keyPath)
	}
	caKey, err = x509.ParsePKCS8PrivateKey(keyDERBlock.Bytes)
	if err != nil {
		return fmt.Errorf("parse CA key %s: %w", keyPath, err)
	}
	return nil
}

func newCert(hosts []string) error {
	if len(hosts) == 0 {
		return errors.New("generate certificate: no host given")
	}

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("generate certificate key: %w", err)
	}
	pub := priv.Public()

	expiration := time.Now().AddDate(2, 3, 0)

	tpl := &x509.Certificate{
		SerialNumber: randomSerialNumber(),
		Subject: pkix.Name{
			Organization:       []string{"fast-https"},
			OrganizationalUnit: []string{userAndHostname},
		},

		NotBefore: time.Now(), NotAfter: expiration,

		KeyUsage: x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
	}

	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			tpl.IPAddresses = append(tpl.IPAddresses, ip)
		} else if email, err := mail.ParseAddress(h); err == nil && email.Address == h {
			tpl.EmailAddresses = append(tpl.EmailAddresses, h)
		} else if uriName, err := url.Parse(h); err == nil && uriName.Scheme != "" && uriName.Host != "" {
			tpl.URIs = append(tpl.URIs, uriName)
		} else {
			tpl.DNSNames = append(tpl.DNSNames, h)
		}
	}

	if len(tpl.IPAddresses) > 0 || len(tpl.DNSNames) > 0 || len(tpl.URIs) > 0 {
		tpl.ExtKeyUsage = append(tpl.ExtKeyUsage, x509.ExtKeyUsageServerAuth)
	}
	if len(tpl.EmailAddresses) > 0 {
		tpl.ExtKeyUsage = append(tpl.ExtKeyUsage, x509.ExtKeyUsageEmailProtection)
	}

	cert, err := x509.CreateCertificate(rand.Reader, tpl, caCert, pub, caKey)
	if err != nil {
		return fmt.Errorf("generate certificate for %s: %w", hosts[0], err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert})

	privDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return fmt.Errorf("encode certificate key for %s: %w", hosts[0], err)
	}

	privPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDER})

	certPath := filepath.Join(config.CERT_DIR, hosts[0]) + ".pem"
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		return fmt.Errorf("save certificate %s: %w", certPath, err)
	}

	keyPath := filepath.Join(config.CERT_DIR, hosts[0]) + "-key.pem"
	if err := os.WriteFile(keyPath, privPEM, 0o600); err != nil {
		return fmt.Errorf("save certificate key %s: %w", keyPath, err)
	}

	return nil
}
