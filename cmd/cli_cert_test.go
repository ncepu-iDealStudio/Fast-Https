package cmd

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type certCmdFixture struct {
	dir, main, key string
}

// writeCertCmdFixture 生成一个有效证书的站点和一个证书文件不存在的站点，证书与私钥现场生成。
func writeCertCmdFixture(t *testing.T) certCmdFixture {
	t.Helper()
	dir := t.TempDir()
	confd := filepath.Join(dir, "conf.d")
	if err := os.MkdirAll(confd, 0o755); err != nil {
		t.Fatal(err)
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(now.UnixNano()),
		Subject:      pkix.Name{CommonName: "valid.example"},
		DNSNames:     []string{"valid.example"},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(90 * 24 * time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certPath := filepath.Join(dir, "valid.pem")
	keyPath := filepath.Join(dir, "valid-key.pem")
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatal(err)
	}

	site := func(name, serverName, cert, key string) {
		body := fmt.Sprintf(`{"listen": ["443 ssl"], "server_name": %q, "ssl_certificate": %q, "ssl_certificate_key": %q,
  "location": [{"url": "/", "type": "local", "root": "."}]}`, serverName, cert, key)
		if err := os.WriteFile(filepath.Join(confd, name+".json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	site("valid", "valid.example", certPath, keyPath)
	site("gone", "gone.example", filepath.Join(dir, "gone.pem"), filepath.Join(dir, "gone-key.pem"))

	main := filepath.Join(dir, "fast-https.json")
	if err := os.WriteFile(main, []byte(fmt.Sprintf(`{"http": {"include": [%q]}}`, confd)), 0o644); err != nil {
		t.Fatal(err)
	}
	return certCmdFixture{dir: dir, main: main, key: keyPath}
}

func TestCertListJSON(t *testing.T) {
	f := writeCertCmdFixture(t)

	out, _, code := runCLI(t, "cert", "list", "--config", f.main, "--json")
	if code != 0 {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	data := decodeOneDocument(t, out)["data"].(map[string]any)
	status := map[string]string{}
	for _, c := range data["certificates"].([]any) {
		entry := c.(map[string]any)
		status[entry["domains"].([]any)[0].(string)] = entry["status"].(string)
	}
	if status["valid.example"] != "valid" || status["gone.example"] != "missing" {
		t.Fatalf("status = %v", status)
	}
}

func TestCertListText(t *testing.T) {
	f := writeCertCmdFixture(t)

	out, _, code := runCLI(t, "cert", "list", "--config", f.main)
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 3 || !strings.HasPrefix(lines[0], "DOMAINS") {
		t.Fatalf("文本输出不对：\n%s", out)
	}
}

func TestCertCheckJSON(t *testing.T) {
	f := writeCertCmdFixture(t)

	out, _, code := runCLI(t, "cert", "check", "--domain", "valid.example", "--config", f.main, "--json")
	if code != 0 {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	data := decodeOneDocument(t, out)["data"].(map[string]any)
	if data["passed"] != true {
		t.Fatalf("data = %v", data)
	}
	keyPair := data["checks"].(map[string]any)["key_pair"].(map[string]any)
	if keyPair["checked"] != true || keyPair["ok"] != true {
		t.Fatalf("key_pair = %v", keyPair)
	}
}

// 证书有问题也是一次成功的查询：退出码 0，问题写在数据里。
func TestCertCheckProblemIsStillSuccessfulQuery(t *testing.T) {
	f := writeCertCmdFixture(t)

	out, _, code := runCLI(t, "cert", "check", "--domain", "gone.example", "--config", f.main, "--json")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	data := decodeOneDocument(t, out)["data"].(map[string]any)
	if data["passed"] != false {
		t.Fatalf("缺失的证书不应通过：%v", data)
	}
	if exists := data["checks"].(map[string]any)["exists"].(map[string]any); exists["ok"] != false {
		t.Fatalf("exists = %v", exists)
	}
}

func TestCertCheckTextMarksUncheckedItems(t *testing.T) {
	f := writeCertCmdFixture(t)

	out, _, code := runCLI(t, "cert", "check", "--domain", "gone.example", "--config", f.main)
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out, "exists:") || !strings.Contains(out, "FAILED") || !strings.Contains(out, "not checked") {
		t.Fatalf("文本输出应区分失败与未检查：\n%s", out)
	}
}

func TestCertCheckUsageAndNotFound(t *testing.T) {
	f := writeCertCmdFixture(t)

	if _, _, code := runCLI(t, "cert", "check", "--config", f.main, "--json"); code != 2 {
		t.Fatalf("缺少 --domain exit = %d, want 2", code)
	}
	if _, _, code := runCLI(t, "cert", "check", "--domain", "nobody.example", "--config", f.main, "--json"); code != 9 {
		t.Fatalf("未知域名 exit = %d, want 9", code)
	}
	if _, _, code := runCLI(t, "cert", "list", "--config", filepath.Join(f.dir, "nope.json"), "--json"); code != 9 {
		t.Fatalf("配置不存在 exit = %d, want 9", code)
	}
}

func TestCertCommandsNeverPrintPrivateKey(t *testing.T) {
	f := writeCertCmdFixture(t)
	raw, err := os.ReadFile(f.key)
	if err != nil {
		t.Fatal(err)
	}
	body := strings.TrimSpace(strings.Split(strings.TrimSpace(string(raw)), "\n")[1])

	for _, args := range [][]string{
		{"cert", "list", "--config", f.main, "--json"},
		{"cert", "list", "--config", f.main},
		{"cert", "check", "--domain", "valid.example", "--config", f.main, "--json"},
		{"cert", "check", "--domain", "valid.example", "--config", f.main},
	} {
		out, errOut, code := runCLI(t, args...)
		if code != 0 {
			t.Fatalf("%v exit = %d", args, code)
		}
		for _, s := range []string{out, errOut} {
			if strings.Contains(s, "PRIVATE KEY") || strings.Contains(s, body) || strings.Contains(s, "valid-key.pem") {
				t.Errorf("%v 的输出泄露了私钥内容或路径", args)
			}
		}
	}
}

func TestCertCommandsAreReadOnly(t *testing.T) {
	f := writeCertCmdFixture(t)
	before := hashTree(t, f.dir)

	for _, args := range [][]string{
		{"cert", "list", "--config", f.main, "--json"},
		{"cert", "check", "--domain", "valid.example", "--config", f.main, "--json"},
	} {
		if _, _, code := runCLI(t, args...); code != 0 {
			t.Fatalf("%v exit = %d", args, code)
		}
	}
	after := hashTree(t, f.dir)
	if len(before) != len(after) {
		t.Fatalf("文件数变了：%d -> %d", len(before), len(after))
	}
	for path, sum := range before {
		if after[path] != sum {
			t.Fatalf("只读命令改了 %s", path)
		}
	}
}
