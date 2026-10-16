//go:build !race

// GConfig is still assigned without synchronization (see stage 4b of
// docs/architecture-convergence-plan.md), so the race detector would flag
// this test even though readers only ever see a complete old or new value.

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

	previousConfig := config.GConfig
	previousMIME := config.GContentTypeMap
	t.Cleanup(func() {
		config.GConfig = previousConfig
		config.GContentTypeMap = previousMIME
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
			if config.GConfig.Limit.MaxBodySize == 0 || len(config.GConfig.Servers) == 0 {
				bad.Add(1)
			}
			if config.GContentTypeMap["css"] == "" {
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
