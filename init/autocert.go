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
	"fast-https/config"
	"fast-https/utils/message"
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

func CertInit() {
	userAndHostname = "fast-https@ncepu.edu.cn"

	file := filepath.Join(config.ROOT_CRT_DIR, config.ROOT_CRT_NAME) + ".crt"
	// message.PrintInfo(file)
	_, err := os.Stat(file)

	if os.IsNotExist(err) {
		newRoot()
	}

	loadCa() // init caCert   init caKey
	for _, serverconfig := range config.GConfig.Servers {

		certfile := filepath.Join(config.CERT_DIR, serverconfig.ServerName) + ".pem"

		if serverconfig.ServerName != "" && strings.Contains(serverconfig.Listen, "ssl") {
			_, err = os.Stat(certfile)
			if os.IsNotExist(err) {
				newCert([]string{serverconfig.ServerName})
				message.PrintInfo(certfile, " created")
			}
		}
	}
}

func randomSerialNumber() *big.Int {
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		message.PrintErr(err, "failed to generate serial number")
	}

	return serialNumber
}

func newRoot() {
	priv, err := rsa.GenerateKey(rand.Reader, 3072)
	if err != nil {
		message.PrintErr(err, "failed to generate the CA key")
	}
	pub := priv.Public()

	spkiASN1, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		message.PrintErr(err, "failed to encode public key")
	}
	var spki struct {
		Algorithm        pkix.AlgorithmIdentifier
		SubjectPublicKey asn1.BitString
	}
	_, err = asn1.Unmarshal(spkiASN1, &spki)
	if err != nil {
		message.PrintErr(err, "failed to decode public key")
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
		message.PrintErr(err, "failed to generate CA certificate")
	}

	privDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		message.PrintErr(err, "failed to encode CA key")
	}

	err = os.WriteFile(filepath.Join(config.ROOT_CRT_DIR, config.ROOT_CRT_NAME)+".key", pem.EncodeToMemory(
		&pem.Block{Type: "PRIVATE KEY", Bytes: privDER}), 0400)
	if err != nil {
		message.PrintErr(err, "failed to save CA key")
	}

	err = os.WriteFile(filepath.Join(config.ROOT_CRT_DIR, config.ROOT_CRT_NAME)+".crt", pem.EncodeToMemory(
		&pem.Block{Type: "CERTIFICATE", Bytes: cert}), 0644)
	if err != nil {
		message.PrintErr(err, "failed to save CA certificate")
	}

	message.PrintInfo("Created a new local CA")
}

func loadCa() {
	// message.PrintInfo(filepath.Join(config.ROOT_CRT_DIR, config.ROOT_CRT_NAME) + ".crt")

	certPEMBlock, err := os.ReadFile(filepath.Join(config.ROOT_CRT_DIR, config.ROOT_CRT_NAME) + ".crt")
	if err != nil {
		message.PrintErr(err, "failed to read the CA certificate")
	}

	certDERBlock, _ := pem.Decode(certPEMBlock)
	if certDERBlock == nil || certDERBlock.Type != "CERTIFICATE" {
		message.PrintErr("ERROR: failed to read the CA certificate: unexpected content")
	}
	caCert, err = x509.ParseCertificate(certDERBlock.Bytes)
	if err != nil {
		message.PrintErr(err, "failed to parse the CA certificate")
	}

	keyPEMBlock, err := os.ReadFile(filepath.Join(config.ROOT_CRT_DIR, config.ROOT_CRT_NAME) + ".key")
	if err != nil {
		message.PrintErr(err, "failed to read the CA key")
	}
	keyDERBlock, _ := pem.Decode(keyPEMBlock)
	if keyDERBlock == nil || keyDERBlock.Type != "PRIVATE KEY" {
		message.PrintErr("ERROR: failed to read the CA key: unexpected content")
	}
	caKey, err = x509.ParsePKCS8PrivateKey(keyDERBlock.Bytes)
	if err != nil {
		message.PrintErr(err, "failed to parse the CA key")
	}
}

func newCert(hosts []string) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		message.PrintErr(err, "failed to generate certificate key")
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
		message.PrintErr(err, "failed to generate certificate")
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert})

	privDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		message.PrintErr(err, "failed to encode certificate key")
	}

	privPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDER})

	err = os.WriteFile(filepath.Join(config.CERT_DIR, hosts[0])+".pem", certPEM, 0644) // hosts is not nil

	if err != nil {
		message.PrintErr(err, "failed to save certificate")
	}

	err = os.WriteFile(filepath.Join(config.CERT_DIR, hosts[0])+"-key.pem", privPEM, 0600)
	if err != nil {
		message.PrintErr(err, "failed to save certificate key")
		// message.PrintInfo("It will expire on %s \n\n", expiration.Format("2 January 2006"))
	}
}
