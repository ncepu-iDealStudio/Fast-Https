package init

import (
	"fast-https/config"
	"os"
	"path/filepath"
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
	for i, server := range config.GConfig.Servers {
		t.Logf("server[%d] listen=%q name=%q cert=%q key=%q", i, server.Listen, server.ServerName, server.SSLCertificate, server.SSLCertificateKey)
	}
	CertInit()
	certPath := filepath.Join("config", "cert", "localhost.pem")
	if _, err := os.Stat(certPath); err != nil {
		t.Fatalf("cert not created: %v", err)
	}
	if err := config.CheckConfig(); err != nil {
		t.Fatal(err)
	}
}
