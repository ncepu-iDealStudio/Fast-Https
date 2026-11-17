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

// proxyFixture 造一份能通过完整校验的主配置。
func proxyFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	root := t.TempDir()
	main := filepath.Join(dir, "fast-https.json")
	body := `{
  // 主配置
  "http": {"server": [{"listen": [18089], "server_name": "proxy.example", "location": [{"url": "/", "type": "local", "root": ` + quote(root) + `}]}]}
}
`
	if err := os.WriteFile(main, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return main
}

func runProxy(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := newConfigProxyCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestConfigProxySetShowAndClear(t *testing.T) {
	main := proxyFixture(t)
	if _, err := runProxy(t, "--config", main, "--trusted-proxies", "10.0.0.0/8, 192.168.1.10", "--max-hops", "10", "--preserve-host=false"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(main)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"proxy"`, "10.0.0.0/8", "192.168.1.10", `"max_hops": 10`, `"preserve_host": false`, "// 主配置"} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("配置缺少 %s:\n%s", want, raw)
		}
	}

	out, err := runProxy(t, "show", "--config", main)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"10.0.0.0/8, 192.168.1.10", "max_hops:         10", "preserve_host:    false"} {
		if !strings.Contains(out, want) {
			t.Fatalf("show 缺少 %q:\n%s", want, out)
		}
	}

	// 只改一项时其它值保留。
	if _, err := runProxy(t, "--config", main, "--max-hops", "3"); err != nil {
		t.Fatal(err)
	}
	out, err = runProxy(t, "show", "--config", main)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "max_hops:         3") || !strings.Contains(out, "10.0.0.0/8") {
		t.Fatalf("补丁覆盖了其它字段:\n%s", out)
	}

	if _, err := runProxy(t, "--config", main, "--trusted-proxies=", "--max-hops", "0", "--preserve-host"); err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile(main)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"proxy"`) {
		t.Fatalf("全部回默认值后应删除 proxy 段:\n%s", raw)
	}
}

func TestConfigProxyRejectsInvalidCIDR(t *testing.T) {
	main := proxyFixture(t)
	_, err := runProxy(t, "--config", main, "--trusted-proxies", "10.0.0.0/33")
	var ce *cli.CLIError
	if !errors.As(err, &ce) || ce.Code != cli.CodeConfigInvalid {
		t.Fatalf("非法 CIDR 应报配置错误，got %v", err)
	}
	raw, readErr := os.ReadFile(main)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if strings.Contains(string(raw), `"proxy"`) {
		t.Fatal("校验失败时不应写盘")
	}
}

func TestConfigProxyNeedsFlags(t *testing.T) {
	main := proxyFixture(t)
	_, err := runProxy(t, "--config", main)
	var ce *cli.CLIError
	if !errors.As(err, &ce) || ce.Code != cli.CodeUsage {
		t.Fatalf("不带参数应报用法错误，got %v", err)
	}
}

func TestConfigProxyDryRun(t *testing.T) {
	main := proxyFixture(t)
	out, err := runProxy(t, "--config", main, "--preserve-host=false", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"preserve_host": false`) {
		t.Fatalf("dry-run 应打印改写结果，got %q", out)
	}
	raw, err := os.ReadFile(main)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"proxy"`) {
		t.Fatal("dry-run 不应写盘")
	}
}
