package init

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"fast-https/config"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/mail"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

var userAndHostname string
var caCert *x509.Certificate
var caKey crypto.PrivateKey

const (
	ROOT_CRT_DIR  = "httpdoc/root"
	ROOT_CRT_NAME = "root"

	CERT_DIR = "config/cert"
)

func Cert_init() {
	userAndHostname = "pzc@desktop"

	file := filepath.Join(ROOT_CRT_DIR, ROOT_CRT_NAME) + ".crt"
	fmt.Println(file)
	_, err := os.Stat(file)
	if os.IsNotExist(err) {
		new_root()
		load_ca() // init caCert   init caKey
		for _, serverconfig := range config.G_config.Servers {

			certfile := filepath.Join(CERT_DIR, serverconfig.ServerName) + ".crt"
			_, err = os.Stat(certfile)
			if os.IsNotExist(err) {
				new_cert([]string{serverconfig.ServerName})
			}
		}
	} else {
		load_ca() // init caCert   init caKey
		for _, serverconfig := range config.G_config.Servers {

			certfile := filepath.Join(CERT_DIR, serverconfig.ServerName) + ".crt"
			_, err = os.Stat(certfile)
			if os.IsNotExist(err) {
				fmt.Println("-----------")
				new_cert([]string{serverconfig.ServerName})
			}
		}
	}
	// test()
}

func Handle_err(err error, msg string) {
	if err != nil {
		fmt.Println(msg)
	}
}

func randomSerialNumber() *big.Int {
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	Handle_err(err, "failed to generate serial number")
	return serialNumber
}

func new_root() {
	priv, err := rsa.GenerateKey(rand.Reader, 3072)

	Handle_err(err, "failed to generate the CA key")
	pub := priv.Public()

	spkiASN1, err := x509.MarshalPKIXPublicKey(pub)
	Handle_err(err, "failed to encode public key")

	var spki struct {
		Algorithm        pkix.AlgorithmIdentifier
		SubjectPublicKey asn1.BitString
	}
	_, err = asn1.Unmarshal(spkiASN1, &spki)
	Handle_err(err, "failed to decode public key")

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
	Handle_err(err, "failed to generate CA certificate")

	privDER, err := x509.MarshalPKCS8PrivateKey(priv)
	Handle_err(err, "failed to encode CA key")

	err = os.WriteFile(filepath.Join(ROOT_CRT_DIR, ROOT_CRT_NAME)+".key", pem.EncodeToMemory(
		&pem.Block{Type: "PRIVATE KEY", Bytes: privDER}), 0400)
	Handle_err(err, "failed to save CA key")

	err = os.WriteFile(filepath.Join(ROOT_CRT_DIR, ROOT_CRT_NAME)+".crt", pem.EncodeToMemory(
		&pem.Block{Type: "CERTIFICATE", Bytes: cert}), 0644)
	Handle_err(err, "failed to save CA certificate")

	log.Printf("Created a new local CA \n")
}

func load_ca() {
	fmt.Println(filepath.Join(ROOT_CRT_DIR, ROOT_CRT_NAME) + ".crt")

	certPEMBlock, err := os.ReadFile(filepath.Join(ROOT_CRT_DIR, ROOT_CRT_NAME) + ".crt")
	Handle_err(err, "failed to read the CA certificate")

	certDERBlock, _ := pem.Decode(certPEMBlock)
	if certDERBlock == nil || certDERBlock.Type != "CERTIFICATE" {
		log.Fatalln("ERROR: failed to read the CA certificate: unexpected content")
	}
	caCert, err = x509.ParseCertificate(certDERBlock.Bytes)
	Handle_err(err, "failed to parse the CA certificate")

	keyPEMBlock, err := os.ReadFile(filepath.Join(ROOT_CRT_DIR, ROOT_CRT_NAME) + ".key")

	Handle_err(err, "failed to read the CA key")
	keyDERBlock, _ := pem.Decode(keyPEMBlock)
	if keyDERBlock == nil || keyDERBlock.Type != "PRIVATE KEY" {
		log.Fatalln("ERROR: failed to read the CA key: unexpected content")
	}
	caKey, err = x509.ParsePKCS8PrivateKey(keyDERBlock.Bytes)
	Handle_err(err, "failed to parse the CA key")
}

func new_cert(hosts []string) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	Handle_err(err, "failed to generate certificate key")
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
	Handle_err(err, "failed to generate certificate")

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert})
	privDER, err := x509.MarshalPKCS8PrivateKey(priv)
	Handle_err(err, "failed to encode certificate key")
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDER})

	err = os.WriteFile(filepath.Join(CERT_DIR, hosts[0])+".pem", certPEM, 0644) // hosts is not nil
	Handle_err(err, "failed to save certificate")
	err = os.WriteFile(filepath.Join(CERT_DIR, hosts[0])+"-key.pem", privPEM, 0600)
	Handle_err(err, "failed to save certificate key")

	log.Printf("It will expire on %s \n\n", expiration.Format("2 January 2006"))
}

func test() {
	certs := []tls.Certificate{}
	crt, err := tls.LoadX509KeyPair("./cert.pem", "key.pem")
	if err != nil {
		log.Fatal("Error load " + "./cert.pem" + " cert")
	}
	certs = append(certs, crt)
	tlsConfig := &tls.Config{}
	tlsConfig.Certificates = certs
	tlsConfig.Time = time.Now
	tlsConfig.Rand = rand.Reader

	listener, err := tls.Listen("tcp", "0.0.0.0:443", tlsConfig)
	if err != nil {
		log.Fatal("Error starting the server:", err)
	}
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println("Error accepting connection:", err)
			continue
		}
		fmt.Println(conn.RemoteAddr())
		req := []byte{}
		reve_len, err := conn.Read(req)
		if err != nil {
			fmt.Println(err)
		}
		fmt.Println(reve_len, req)

		conn.Write([]byte("HTTP/1.1 200 OK\r\n\r\nhello"))
		conn.Close()
	}

}
