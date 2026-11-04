package config_test

import (
	"fast-https/config"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create dir failed: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
}

func TestValidateConfigFile_ValidMainAndInclude(t *testing.T) {
	tmp := t.TempDir()
	mainPath := filepath.Join(tmp, "fast-https.json")
	includePath := filepath.Join(tmp, "conf.d", "site.json")

	writeFile(t, mainPath, `{
  "http": {
    "server": [
      {
        "listen": 8080,
        "server_name": "localhost",
        "location": [
          {
            "url": "/",
            "type": "local",
            "root": "./httpdoc/root",
            "index": ["index.html"]
          }
        ]
      }
    ],
    "include": ["./conf.d"]
  }
}`)

	writeFile(t, includePath, `{
  "listen": "8081",
  "server_name": "api.local",
  "location": [
    {
      "url": "/api",
      "type": "proxy",
      "proxy_pass": "http://127.0.0.1:9000"
    }
  ]
}`)

	// include 的相对路径和运行期一样按 FAST_HTTPS_BASE_DIR（进程工作目录）解析，
	// 不是按主配置文件所在目录，所以要把工作目录切到临时目录。
	t.Chdir(tmp)

	if err := config.ValidateConfigFile(mainPath); err != nil {
		t.Fatalf("expected valid config, got error: %v", err)
	}
}

func TestValidateConfigFile_InvalidListenPort(t *testing.T) {
	tmp := t.TempDir()
	mainPath := filepath.Join(tmp, "fast-https.json")

	writeFile(t, mainPath, `{
  "http": {
    "server": [
      {
        "listen": "bad-port",
        "server_name": "localhost",
        "location": [
          {"url": "/", "type": "local", "root": "./httpdoc/root"}
        ]
      }
    ]
  }
}`)

	err := config.ValidateConfigFile(mainPath)
	if err == nil || !strings.Contains(err.Error(), "invalid listen port") {
		t.Fatalf("expected invalid listen port error, got: %v", err)
	}
}

func TestValidateConfigFile_SSLMissingCertFile(t *testing.T) {
	tmp := t.TempDir()
	mainPath := filepath.Join(tmp, "fast-https.json")

	writeFile(t, mainPath, `{
  "http": {
    "server": [
      {
        "listen": "443 ssl",
        "server_name": "localhost",
        "ssl_certificate": "config/cert/missing.pem",
        "ssl_certificate_key": "config/cert/missing-key.pem",
        "location": [
          {"url": "/", "type": "local", "root": "./httpdoc/root"}
        ]
      }
    ]
  }
}`)

	err := config.ValidateConfigFile(mainPath)
	if err == nil || !strings.Contains(err.Error(), "ssl_certificate not found") {
		t.Fatalf("expected missing ssl certificate error, got: %v", err)
	}
}

func TestValidateConfigFile_ProxyWithoutProxyPass(t *testing.T) {
	tmp := t.TempDir()
	mainPath := filepath.Join(tmp, "fast-https.json")

	writeFile(t, mainPath, `{
  "http": {
    "server": [
      {
        "listen": 8080,
        "server_name": "localhost",
        "location": [
          {"url": "/api", "type": "proxy"}
        ]
      }
    ]
  }
}`)

	err := config.ValidateConfigFile(mainPath)
	if err == nil || !strings.Contains(err.Error(), "proxy_pass is required") {
		t.Fatalf("expected missing proxy_pass error, got: %v", err)
	}
}

func TestValidateConfigFile_InvalidIncludeServer(t *testing.T) {
	tmp := t.TempDir()
	mainPath := filepath.Join(tmp, "fast-https.json")
	includePath := filepath.Join(tmp, "conf.d", "broken.json")

	writeFile(t, mainPath, `{
  "http": {
    "server": [
      {
        "listen": 8080,
        "server_name": "localhost",
        "location": [
          {"url": "/", "type": "local", "root": "./httpdoc/root"}
        ]
      }
    ],
    "include": ["./conf.d"]
  }
}`)

	writeFile(t, includePath, `{
  "listen": "8082",
  "server_name": "broken.local"
}`)

	// 同上：include 相对路径按进程工作目录解析。
	t.Chdir(tmp)

	err := config.ValidateConfigFile(mainPath)
	if err == nil || !strings.Contains(err.Error(), "location must contain at least one rule") {
		t.Fatalf("expected include location validation error, got: %v", err)
	}
}
