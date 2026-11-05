package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTestConfig 落一份配置到临时文件，返回路径。
func writeTestConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fast-https.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

const validConfig = `{
  "http": {
    "server": [
      {
        "listen": 8080,
        "server_name": "localhost",
        "location": [
          {"url": "/", "type": "local", "root": "./httpdoc/root"}
        ]
      }
    ]
  }
}`

const invalidListenConfig = `{
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
}`

func TestValidateConfigAcceptsValidConfig(t *testing.T) {
	if err := validateConfig(writeTestConfig(t, validConfig)); err != nil {
		t.Fatal(err)
	}
}

func TestValidateConfigRejectsInvalidConfig(t *testing.T) {
	err := validateConfig(writeTestConfig(t, invalidListenConfig))
	if err == nil || !strings.Contains(err.Error(), "listen") {
		t.Fatalf("err = %v", err)
	}
}

// 配置写坏时 reload 必须原地停住：不发信号、不碰 pid 文件，把原因直接带回去。
// 以前 reload 只发信号，错误只出现在 daemon.log，终端上看起来是成功的。
func TestReloadStopsOnInvalidConfig(t *testing.T) {
	err := reload(writeTestConfig(t, invalidListenConfig), filepath.Join(t.TempDir(), "no.pid"))
	if err == nil {
		t.Fatal("配置无效时 reload 应该失败")
	}
	if !strings.Contains(err.Error(), "listen") {
		t.Fatalf("错误信息应带具体字段，实际：%v", err)
	}
	if !strings.Contains(err.Error(), "已保留运行中的配置") {
		t.Fatalf("错误信息应说明未生效，实际：%v", err)
	}
}

// 配置有效但没有在跑时，reload 不算失败：说明「没在跑」并正常退出。
func TestReloadWithoutRunningServerIsNotAnError(t *testing.T) {
	pidPath := filepath.Join(t.TempDir(), "no.pid")
	if err := reload(writeTestConfig(t, validConfig), pidPath); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Fatal("没在跑时不应该写出 pid 文件")
	}
}
