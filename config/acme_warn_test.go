package config

import (
	"bytes"
	"fast-https/infra/logging"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

// TestWarnHTTP01ConsidersIncludedSites 守一个很容易误报的判断：
// 站点通常只写在 conf.d 里（由 http.include 展开），主配置的 http.server 是空的。
// 只看主配置就会对"80 明明在听"的站点配置告警，
// 把真正的配置问题（没有 443、没有公网域名）盖在一条假警告下面。
func TestWarnHTTP01ConsidersIncludedSites(t *testing.T) {
	cases := []struct {
		name     string
		listen   string
		wantWarn bool
	}{
		{"include 里的站点监听 80 时不告警", `[80, "443 ssl h2"]`, false},
		{"include 里的站点没有 80 时告警", `["443 ssl h2"]`, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			sites := filepath.Join(dir, "conf.d")
			if err := os.MkdirAll(sites, 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			cert := filepath.Join(dir, "site.pem")
			key := filepath.Join(dir, "site-key.pem")
			for _, path := range []string{cert, key} {
				if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
					t.Fatalf("write %s: %v", path, err)
				}
			}

			site := fmt.Sprintf(`{
  "listen": %s,
  "server_name": "example.com",
  "ssl_certificate": %q,
  "ssl_certificate_key": %q,
  "location": [{"url": "/", "type": "local", "root": %q}]
}`, c.listen, cert, key, dir)
			if err := os.WriteFile(filepath.Join(sites, "site.json"), []byte(site), 0o644); err != nil {
				t.Fatalf("write site: %v", err)
			}

			main := fmt.Sprintf(`{
  "acme": {"enabled": true, "email": "ops@example.com", "challenge": ["http-01"]},
  "http": {"include": [%q]}
}`, sites)
			mainPath := filepath.Join(dir, "fast-https.json")
			if err := os.WriteFile(mainPath, []byte(main), 0o644); err != nil {
				t.Fatalf("write main: %v", err)
			}

			var buf bytes.Buffer
			old := logging.ConsoleLog
			logging.ConsoleLog = zerolog.New(&buf)
			t.Cleanup(func() { logging.ConsoleLog = old })

			if err := ValidateConfigFile(mainPath); err != nil {
				t.Fatalf("validate: %v", err)
			}

			warned := strings.Contains(buf.String(), "no http.server listens on port 80")
			if warned != c.wantWarn {
				t.Fatalf("warned = %v, want %v；输出：%s", warned, c.wantWarn, buf.String())
			}
		})
	}
}
