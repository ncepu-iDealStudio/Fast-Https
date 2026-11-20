package configcmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fast-https/cmd/internal/cli"
)

// tlsFixture 造一份能通过完整校验的主配置，并返回证书/私钥路径。
func tlsFixture(t *testing.T) (main, cert, key string) {
	t.Helper()
	dir := t.TempDir()
	root := t.TempDir()
	cert = filepath.Join(dir, "def.pem")
	key = filepath.Join(dir, "def.key")
	if err := os.WriteFile(cert, []byte("PEM"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(key, []byte("KEY"), 0o644); err != nil {
		t.Fatal(err)
	}
	main = filepath.Join(dir, "fast-https.json")
	body := `{
  // 主配置
  "http": {"server": [{"listen": [18088], "server_name": "tls.example", "location": [{"url": "/", "type": "local", "root": ` + quote(root) + `}]}]}
}
`
	if err := os.WriteFile(main, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return main, cert, key
}

func quote(s string) string {
	return `"` + strings.ReplaceAll(filepath.ToSlash(s), `\`, `\\`) + `"`
}

func runTLS(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := newConfigTLSCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestConfigTLSSetAndClear(t *testing.T) {
	main, cert, key := tlsFixture(t)
	if _, err := runTLS(t, "--config", main, "--cert", cert, "--key", key, "--yes"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(main)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"default_certificate"`) || !strings.Contains(string(raw), "def.pem") {
		t.Fatalf("没写进配置:\n%s", raw)
	}
	if !strings.Contains(string(raw), "// 主配置") {
		t.Fatalf("注释丢了:\n%s", raw)
	}

	if _, err := runTLS(t, "--config", main, "--cert=", "--key=", "--yes"); err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile(main)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "default_certificate") {
		t.Fatalf("清除失败:\n%s", raw)
	}
}

func TestConfigTLSRequiresPair(t *testing.T) {
	main, cert, _ := tlsFixture(t)
	_, err := runTLS(t, "--config", main, "--cert", cert)
	var ce *cli.CLIError
	if !errors.As(err, &ce) || ce.Code != cli.CodeUsage {
		t.Fatalf("只传 --cert 应报用法错误，got %v", err)
	}
}

func TestConfigTLSNeedsFlags(t *testing.T) {
	main, _, _ := tlsFixture(t)
	_, err := runTLS(t, "--config", main)
	var ce *cli.CLIError
	if !errors.As(err, &ce) || ce.Code != cli.CodeUsage {
		t.Fatalf("不带参数应报用法错误，got %v", err)
	}
}

func TestConfigTLSRejectsMissingFile(t *testing.T) {
	main, cert, _ := tlsFixture(t)
	_, err := runTLS(t, "--config", main, "--cert", cert, "--key", filepath.Join(filepath.Dir(cert), "absent.key"), "--yes")
	var ce *cli.CLIError
	if !errors.As(err, &ce) || ce.Code != cli.CodeConfigInvalid {
		t.Fatalf("私钥不存在应报配置错误，got %v", err)
	}
	raw, readErr := os.ReadFile(main)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if strings.Contains(string(raw), "default_certificate") {
		t.Fatal("校验失败时不应写盘")
	}
}

func TestConfigTLSDryRun(t *testing.T) {
	main, cert, key := tlsFixture(t)
	out, err := runTLS(t, "--config", main, "--cert", cert, "--key", key, "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "default_certificate") {
		t.Fatalf("dry-run 应打印改写结果，got %q", out)
	}
	raw, err := os.ReadFile(main)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "default_certificate") {
		t.Fatal("dry-run 不应写盘")
	}
}

func TestConfigTLSShow(t *testing.T) {
	main, cert, key := tlsFixture(t)
	if _, err := runTLS(t, "--config", main, "--cert", cert, "--key", key, "--yes"); err != nil {
		t.Fatal(err)
	}
	out, err := runTLS(t, "show", "--config", main)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, cert) || !strings.Contains(out, key) {
		t.Fatalf("show 输出 = %q", out)
	}
}
