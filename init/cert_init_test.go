package init

import (
	"fast-https/config"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCertInitCreatesLocalhostCert(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := wd
	if _, statErr := os.Stat(filepath.Join(wd, "go.mod")); statErr != nil {
		root = filepath.Dir(wd)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	if err := config.Init(); err != nil {
		t.Fatal(err)
	}
	for i, server := range config.Current().Servers {
		t.Logf("server[%d] listen=%q name=%q cert=%q key=%q", i, server.Listen, server.ServerName, server.SSLCertificate, server.SSLCertificateKey)
	}
	if err := CertInit(); err != nil {
		t.Fatalf("cert init: %v", err)
	}
	certPath := filepath.Join("config", "cert", "localhost.pem")
	if _, err := os.Stat(certPath); err != nil {
		t.Fatalf("cert not created: %v", err)
	}
	if err := config.CheckConfig(); err != nil {
		t.Fatal(err)
	}

	// CA 私钥不能落在站点根下：静态模块只挡 ../ 逃逸，
	// 放在 httpdoc/root 下能被 HTTP 直接下载。
	keyDir, err := filepath.Abs(config.ROOT_CRT_KEY_DIR)
	if err != nil {
		t.Fatal(err)
	}
	for _, server := range config.Current().Servers {
		for _, p := range server.Path {
			if p.Root == "" {
				continue
			}
			if rel, err := filepath.Rel(p.Root, keyDir); err == nil && !strings.HasPrefix(rel, "..") {
				t.Fatalf("CA key dir %s is inside site root %s", keyDir, p.Root)
			}
		}
	}
}
