package certmanager

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fast-https/config"
	"fast-https/infra/files"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/caddyserver/certmagic"
	"github.com/mholt/acmez/v3"
	"github.com/mholt/acmez/v3/acme"
)

// caIssuer 是 certmagic.Issuer 的一个实现：签发流程自己走 acmez，
// 因为 http-01 / tls-alpn-01 的应答必须落到本项目的数据面，
// 而 certmagic 内置的 solver 会自己监听 80/443。
// 续期调度、私钥生成、CSR 与证书落盘仍然交给 certmagic。
type caIssuer struct {
	ca         config.ACMECA
	email      string
	storageDir string
	solvers    map[string]acmez.Solver
	logger     *slog.Logger

	mu      sync.Mutex
	client  *acmez.Client
	account acme.Account
}

func newCAIssuer(ca config.ACMECA, email, storageDir string, solvers map[string]acmez.Solver, logger *slog.Logger) *caIssuer {
	return &caIssuer{
		ca:         ca,
		email:      email,
		storageDir: storageDir,
		solvers:    solvers,
		logger:     logger,
	}
}

// IssuerKey 标识这组（CA、邮箱）配置。certmagic 用它区分不同 issuer
// 落盘的证书资源，同一个 key 才会被当成同一份证书。
func (i *caIssuer) IssuerKey() string {
	return "fast-https-acme/" + i.ca.Name + "/" + i.ca.Directory
}

// Issue 实现 certmagic.Issuer：用 acmez 走完订单，返回 PEM 证书链。
func (i *caIssuer) Issue(ctx context.Context, csr *x509.CertificateRequest) (*certmagic.IssuedCertificate, error) {
	client, account, err := i.acmeClient(ctx)
	if err != nil {
		return nil, err
	}
	params, err := acmez.OrderParametersFromCSR(account, csr)
	if err != nil {
		return nil, fmt.Errorf("acme: build order for %s: %w", i.ca.Name, err)
	}
	certs, err := client.ObtainCertificate(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("acme: issue via %s: %w", i.ca.Name, err)
	}
	if len(certs) == 0 || len(certs[0].ChainPEM) == 0 {
		return nil, fmt.Errorf("acme: %s returned no certificate chain", i.ca.Name)
	}
	chain := certs[0].ChainPEM
	return &certmagic.IssuedCertificate{
		Certificate: chain,
		Metadata: map[string]string{
			"ca":              i.ca.Directory,
			"certificate_url": certs[0].URL,
		},
	}, nil
}

// acmeClient 首次调用时注册（或找回）账户，之后复用。
func (i *caIssuer) acmeClient(ctx context.Context) (*acmez.Client, acme.Account, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.client != nil {
		return i.client, i.account, nil
	}

	key, err := i.accountKey()
	if err != nil {
		return nil, acme.Account{}, err
	}
	acmeClient := &acme.Client{
		Directory:    i.ca.Directory,
		UserAgent:    acmeUserAgent,
		Logger:       i.logger,
		HTTPClient:   &http.Client{Timeout: 60 * time.Second},
		PollTimeout:  5 * time.Minute,
		PollInterval: 2 * time.Second,
	}
	account := acme.Account{
		Contact:              []string{"mailto:" + i.email},
		TermsOfServiceAgreed: true,
		PrivateKey:           key,
	}
	if i.ca.EABKid != "" && i.ca.EABHmacKey != "" {
		if err := account.SetExternalAccountBinding(ctx, acmeClient,
			acme.EAB{KeyID: i.ca.EABKid, MACKey: i.ca.EABHmacKey}); err != nil {
			return nil, acme.Account{}, fmt.Errorf("acme: bind external account for %s: %w", i.ca.Name, err)
		}
	}

	client := &acmez.Client{Client: acmeClient, ChallengeSolvers: i.solvers}
	registered, err := client.GetAccount(ctx, account)
	if err != nil {
		registered, err = client.NewAccount(ctx, account)
		if err != nil {
			return nil, acme.Account{}, fmt.Errorf("acme: register account at %s: %w", i.ca.Directory, err)
		}
	}
	i.client = client
	i.account = registered
	return client, registered, nil
}

// accountKey 读取或生成账户私钥。账户私钥丢了等于换账户，
// 所以和证书一起放在配置的存储目录里，随备份一起走。
func (i *caIssuer) accountKey() (crypto.Signer, error) {
	dir := filepath.Join(i.storageDir, "accounts", accountDirName(i.ca.Directory))
	path := filepath.Join(dir, "account.key")

	if data, err := os.ReadFile(path); err == nil {
		key, err := parseAccountKey(data)
		if err != nil {
			return nil, fmt.Errorf("acme: parse %s: %w", path, err)
		}
		return key, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("acme: read %s: %w", path, err)
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})
	if err := files.WriteAtomic(path, pemBytes, 0o600); err != nil {
		return nil, fmt.Errorf("acme: write account key %s: %w", path, err)
	}
	return key, nil
}

func parseAccountKey(data []byte) (crypto.Signer, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("no PEM block")
	}
	if key, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	signer, ok := parsed.(crypto.Signer)
	if !ok {
		return nil, errors.New("unsupported private key type")
	}
	return signer, nil
}

// accountDirName 把目录 URL 转成稳定的目录名，便于人工排查缓存。
func accountDirName(directory string) string {
	if u, err := url.Parse(directory); err == nil && u.Host != "" {
		return u.Host
	}
	return "ca"
}
