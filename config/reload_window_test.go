// 配置已改为 atomic 快照（架构收敛计划阶段 4b），本测试在 -race 下运行，
// 验证 reload 期间读取端永远看到完整的新配置或完整的旧配置。

package config_test

import (
	"fast-https/config"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestReloadNeverExposesClearedConfig(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(filepath.Dir(wd)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	previousConfig := *config.Current()
	previousMIME := config.MIME()
	t.Cleanup(func() {
		config.Store(&previousConfig, previousMIME)
	})

	path := filepath.Join(t.TempDir(), "fast-https.json")
	content := `{
  "http": {
    "server": [
      {
        "listen": 8080,
        "server_name": "window.local",
        "location": [
          {"url": "/", "type": "local", "root": "./httpdoc/root", "index": ["index.html"]}
        ]
      }
    ]
  }
}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := config.LoadFile(path); err != nil {
		t.Fatal(err)
	}

	var bad atomic.Int64
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
			}
			if config.Current().Limit.MaxBodySize == 0 || len(config.Current().Servers) == 0 {
				bad.Add(1)
			}
			if config.MIME()["css"] == "" {
				bad.Add(1)
			}
		}
	}()

	for i := 0; i < 50; i++ {
		if err := config.ReloadFile(path); err != nil {
			close(stop)
			<-done
			t.Fatalf("reload %d: %v", i, err)
		}
	}
	close(stop)
	<-done

	if n := bad.Load(); n != 0 {
		t.Fatalf("reader saw cleared config or MIME table %d times", n)
	}
}
